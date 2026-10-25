package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robfig/cron/v3"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// BotRoutineStore is the bot_routines surface; *storage.BotRoutines
// satisfies it (pin below), tests run against in-memory fakes.
type BotRoutineStore interface {
	Create(ctx context.Context, routine storage.BotRoutine) (storage.BotRoutine, error)
	Get(ctx context.Context, userID, id string) (storage.BotRoutine, error)
	ListByBot(ctx context.Context, userID, botID string) ([]storage.BotRoutine, error)
	ListEnabled(ctx context.Context) ([]storage.BotRoutine, error)
	Update(ctx context.Context, userID, id string, p storage.BotRoutinePatch) (storage.BotRoutine, error)
	Delete(ctx context.Context, userID, id string) error
	SetLastRun(ctx context.Context, id string, at time.Time) error
}

var _ BotRoutineStore = (*storage.BotRoutines)(nil)

const (
	maxRoutineNameChars   = 64
	maxRoutinePromptChars = 8000
	// botRoutineTickDefault mirrors config's BOT_ROUTINE_TICK_MS default;
	// it guards zero-valued test configs the same way the runner floors its
	// own intervals.
	botRoutineTickDefault = 30 * time.Second
)

// routineParser is the strict 5-field parser: no seconds field, no @every,
// no descriptors — a routine schedule is exactly `m h dom mon dow`.
var routineParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// parseRoutineSchedule validates a routine's schedule and timezone together
// and returns the compiled schedule. An empty timezone is UTC (time.LoadLocation's
// contract), matching the store's default.
func parseRoutineSchedule(schedule, timezone string) (cron.Schedule, error) {
	sched, err := routineParser.Parse(schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid 5-field cron schedule (m h dom mon dow): %w", err)
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nil, fmt.Errorf("unknown timezone %q", timezone)
	}
	return sched, nil
}

// botRoutineDTO is a routine as the API sees it. nextRunAt is computed
// server-side for display (sched.Next of the last run, or of creation when
// the routine has never fired) — there is no next_run_at column; the
// scheduler recomputes due-ness from last_run_at every tick. Null when the
// stored schedule no longer parses (a legacy row, not a crash).
type botRoutineDTO struct {
	ID        string  `json:"id"`
	BotID     string  `json:"botId"`
	Name      string  `json:"name"`
	Schedule  string  `json:"schedule"`
	Timezone  string  `json:"timezone"`
	Prompt    string  `json:"prompt"`
	Enabled   bool    `json:"enabled"`
	LastRunAt *string `json:"lastRunAt"`
	NextRunAt *string `json:"nextRunAt"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

func toBotRoutineDTO(r storage.BotRoutine) botRoutineDTO {
	var next *string
	if sched, err := parseRoutineSchedule(r.Schedule, r.Timezone); err == nil {
		last := r.CreatedAt
		if r.LastRunAt != nil {
			last = *r.LastRunAt
		}
		if n := sched.Next(last); !n.IsZero() {
			s := n.UTC().Format(timeRFC3339)
			next = &s
		}
	}
	return botRoutineDTO{
		ID: r.ID, BotID: r.BotID, Name: r.Name, Schedule: r.Schedule, Timezone: r.Timezone,
		Prompt: r.Prompt, Enabled: r.Enabled, LastRunAt: formatTimePtr(r.LastRunAt),
		NextRunAt: next,
		CreatedAt: r.CreatedAt.UTC().Format(timeRFC3339), UpdatedAt: r.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

// StartRoutineScheduler boots the routine ticker as the runner's second
// goroutine: StartBotRunner calls it with the same parent context (one
// lifecycle — SIGINT/SIGTERM stops run claiming and routine firing
// together), and the loop shares the runner's wake channel so enqueues also
// nudge due-routine checks. Idempotent; nil BotRoutines is a no-op.
func (s *Server) StartRoutineScheduler(parent context.Context) {
	r := s.runner.Load()
	if r == nil || s.deps.BotRoutines == nil {
		return
	}
	tick := s.deps.Cfg.BotRoutineTickInterval
	if tick <= 0 {
		tick = botRoutineTickDefault
	}
	go r.routineLoop(parent, tick)
}

// routineLoop is the routine scheduler: every tick (or wake) it walks the
// enabled routines and fires every one whose next occurrence is due.
func (r *botRunner) routineLoop(ctx context.Context, tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-t.C:
		}
		r.fireDueRoutines(ctx)
	}
}

// fireDueRoutines fires each due enabled routine. Due-ness is recomputed
// from last_run_at (or created_at) every tick: next = sched.Next(last), due
// when next is not after now (both in the routine's timezone). A routine
// whose schedule or timezone no longer parses is skipped with a log line —
// validation happens at write time, so this only happens to rows written by
// older code or a renamed zone.
func (r *botRunner) fireDueRoutines(ctx context.Context) {
	s := r.s
	routines, err := s.deps.BotRoutines.ListEnabled(ctx)
	if err != nil {
		s.deps.Log.Error("routine list", "err", err)
		return
	}
	for _, routine := range routines {
		sched, err := parseRoutineSchedule(routine.Schedule, routine.Timezone)
		if err != nil {
			s.deps.Log.Error("routine schedule", "err", err, "routine_id", routine.ID)
			continue
		}
		loc, err := time.LoadLocation(routine.Timezone)
		if err != nil {
			s.deps.Log.Error("routine timezone", "err", err, "routine_id", routine.ID)
			continue
		}
		last := routine.CreatedAt.In(loc)
		if routine.LastRunAt != nil {
			last = routine.LastRunAt.In(loc)
		}
		if next := sched.Next(last); next.After(time.Now().In(loc)) {
			continue // not due yet
		}
		s.fireRoutine(ctx, routine)
	}
}

// routineConversation resolves the conversation a routine's runs land in:
// the bot's latest bot-scoped one, or a freshly opened one built exactly
// the way handleCreateBotConversation does (model + RAG inherited from the
// live bot, bot_id stored).
func (s *Server) routineConversation(ctx context.Context, routine storage.BotRoutine) (storage.Conversation, error) {
	conv, err := s.deps.Convos.LatestForBot(ctx, routine.BotID, routine.UserID)
	if err == nil {
		return conv, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return storage.Conversation{}, err
	}
	bot, err := s.deps.Bots.Get(ctx, routine.UserID, routine.BotID)
	if err != nil {
		return storage.Conversation{}, err
	}
	model, rag := bot.Model, bot.RagEnabled
	botID := routine.BotID
	return s.deps.Convos.Create(ctx, routine.UserID, bot.Name, storage.ConversationPatch{
		Model:      &model,
		RagEnabled: &rag,
		BotID:      &botID,
	})
}

// fireRoutine queues one due routine occurrence and stamps last_run either
// way: a skipped tick (busy conversation or unresolved approvals) consumes
// the occurrence rather than retrying it every tick. Only a failure before
// the busy-check leaves last_run alone, so the next tick retries.
func (s *Server) fireRoutine(ctx context.Context, routine storage.BotRoutine) {
	conv, err := s.routineConversation(ctx, routine)
	if err != nil {
		s.deps.Log.Error("routine conversation", "err", err, "routine_id", routine.ID)
		return
	}
	// Same guards as handleCreateBotTask, minus the HTTP: no new work while
	// a run is live on the conversation or a parked run's approvals are
	// unresolved.
	if s.deps.Pending != nil {
		pending, perr := s.deps.Pending.ForConversation(ctx, conv.ID, routine.UserID)
		if perr != nil {
			s.deps.Log.Error("routine pending check", "err", perr, "routine_id", routine.ID)
			return
		}
		if len(pending) > 0 {
			_ = s.deps.BotRoutines.SetLastRun(ctx, routine.ID, time.Now())
			return
		}
	}
	active, aerr := s.deps.BotRuns.HasActiveForConversation(ctx, routine.UserID, conv.ID)
	if aerr != nil {
		s.deps.Log.Error("routine active check", "err", aerr, "routine_id", routine.ID)
		return
	}
	if !active {
		if _, err := s.deps.BotRuns.Create(ctx, storage.BotRun{
			BotID: routine.BotID, ConversationID: conv.ID, UserID: routine.UserID,
			Status: "queued", Task: routine.Prompt, TriggerSource: "routine",
		}); err != nil {
			s.deps.Log.Error("routine enqueue", "err", err, "routine_id", routine.ID)
			return
		}
		s.WakeBotRunner()
	}
	_ = s.deps.BotRoutines.SetLastRun(ctx, routine.ID, time.Now())
}

// validateRoutineName applies the shared name rule (1..64 runes after trim).
func validateRoutineName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxRoutineNameChars {
		return "", errors.New("name must be 1-64 characters")
	}
	return name, nil
}

// validateRoutinePrompt applies the shared prompt rule (1..8000 runes after
// trim), the same budget as a manual bot task.
func validateRoutinePrompt(prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || utf8.RuneCountInString(prompt) > maxRoutinePromptChars {
		return "", errors.New("prompt must be 1-8000 characters")
	}
	return prompt, nil
}

// handleListRoutines lists a bot's routines, newest first.
func (s *Server) handleListRoutines(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	routines, err := s.deps.BotRoutines.ListByBot(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list routines")
		return
	}
	out := make([]botRoutineDTO, 0, len(routines))
	for _, routine := range routines {
		out = append(out, toBotRoutineDTO(routine))
	}
	writeJSON(w, http.StatusOK, map[string]any{"routines": out, "count": len(out)})
}

// handleCreateRoutine attaches a cron routine to a bot. The schedule and
// timezone are validated as a pair (parseRoutineSchedule), so a stored
// routine is always schedulable; creation is always enabled (the store
// forces it — disable via PATCH).
func (s *Server) handleCreateRoutine(w http.ResponseWriter, r *http.Request) {
	if s.deps.BotRoutines == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "routines not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
		Timezone string `json:"timezone"`
		Prompt   string `json:"prompt"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	name, err := validateRoutineName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_name", err.Error())
		return
	}
	timezone := strings.TrimSpace(req.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	schedule := strings.TrimSpace(req.Schedule)
	if _, err := parseRoutineSchedule(schedule, timezone); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error())
		return
	}
	prompt, err := validateRoutinePrompt(req.Prompt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_prompt", err.Error())
		return
	}
	botID := r.PathValue("id")
	bot, err := s.deps.Bots.Get(r.Context(), userID, botID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "bot not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load bot")
		return
	}
	if bot.IsArchived {
		writeError(w, http.StatusBadRequest, "bot_archived", "archived bots cannot run routines")
		return
	}
	limit := s.deps.Cfg.MaxRoutinesPerBot
	if limit <= 0 {
		limit = 10 // config default; guards zero-valued test configs
	}
	rows, err := s.deps.BotRoutines.ListByBot(r.Context(), userID, botID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list routines")
		return
	}
	if len(rows) >= limit {
		writeError(w, http.StatusBadRequest, "routine_limit_reached", "routine limit reached")
		return
	}
	routine, err := s.deps.BotRoutines.Create(r.Context(), storage.BotRoutine{
		BotID: botID, UserID: userID, Name: name,
		Schedule: schedule, Timezone: timezone, Prompt: prompt,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create routine")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"routine": toBotRoutineDTO(routine)})
}

// handleUpdateRoutine patches a routine. When schedule or timezone changes,
// the EFFECTIVE pair (new value(s) merged with the stored one) must still
// parse — a routine may never become unschedulable through an update.
func (s *Server) handleUpdateRoutine(w http.ResponseWriter, r *http.Request) {
	if s.deps.BotRoutines == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "routines not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	var req struct {
		Name     *string `json:"name"`
		Schedule *string `json:"schedule"`
		Timezone *string `json:"timezone"`
		Prompt   *string `json:"prompt"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	routine, err := s.deps.BotRoutines.Get(r.Context(), userID, id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "routine not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load routine")
		return
	}
	patch := storage.BotRoutinePatch{Enabled: req.Enabled}
	if req.Name != nil {
		name, err := validateRoutineName(*req.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_name", err.Error())
			return
		}
		patch.Name = &name
	}
	if req.Schedule != nil || req.Timezone != nil {
		schedule := strings.TrimSpace(routine.Schedule)
		if req.Schedule != nil {
			schedule = strings.TrimSpace(*req.Schedule)
		}
		timezone := strings.TrimSpace(routine.Timezone)
		if req.Timezone != nil {
			timezone = strings.TrimSpace(*req.Timezone)
			if timezone == "" {
				timezone = "UTC"
			}
		}
		if _, err := parseRoutineSchedule(schedule, timezone); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error())
			return
		}
		patch.Schedule = &schedule
		patch.Timezone = &timezone
	}
	if req.Prompt != nil {
		prompt, err := validateRoutinePrompt(*req.Prompt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_prompt", err.Error())
			return
		}
		patch.Prompt = &prompt
	}
	routine, err = s.deps.BotRoutines.Update(r.Context(), userID, id, patch)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "routine not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update routine")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"routine": toBotRoutineDTO(routine)})
}

// handleDeleteRoutine drops a routine (and with it its future firings).
func (s *Server) handleDeleteRoutine(w http.ResponseWriter, r *http.Request) {
	if s.deps.BotRoutines == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "routines not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.BotRoutines.Delete(r.Context(), userID, r.PathValue("id")); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "routine not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete routine")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRunRoutineNow fires a routine out of band: same enqueue path as the
// scheduler (conversation resolution included) with trigger "manual". The
// busy/pending guards mirror handleCreateBotTask, and last_run is stamped
// too — otherwise the scheduler would re-fire this occurrence on its next
// tick (sched.Next(last) would still be in the past).
func (s *Server) handleRunRoutineNow(w http.ResponseWriter, r *http.Request) {
	if s.deps.BotRoutines == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "routines not configured")
		return
	}
	if s.deps.Convos == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "conversations not configured")
		return
	}
	ctx := r.Context()
	userID, _ := userIDFrom(ctx)
	routine, err := s.deps.BotRoutines.Get(ctx, userID, r.PathValue("id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "routine not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load routine")
		return
	}
	bot, err := s.deps.Bots.Get(ctx, userID, routine.BotID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "bot not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load bot")
		return
	}
	if bot.IsArchived {
		writeError(w, http.StatusBadRequest, "bot_archived", "archived bots cannot run routines")
		return
	}
	conv, err := s.routineConversation(ctx, routine)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not resolve conversation")
		return
	}
	if s.blockedByPendingApproval(w, ctx, conv.ID, userID) {
		return
	}
	active, err := s.deps.BotRuns.HasActiveForConversation(ctx, userID, conv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not check active runs")
		return
	}
	if active {
		writeError(w, http.StatusConflict, "run_active", "a run is already active for this conversation")
		return
	}
	run, err := s.deps.BotRuns.Create(ctx, storage.BotRun{
		BotID: routine.BotID, ConversationID: conv.ID, UserID: userID,
		Status: "queued", Task: routine.Prompt, TriggerSource: "manual",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not enqueue run")
		return
	}
	if lerr := s.deps.BotRoutines.SetLastRun(ctx, routine.ID, time.Now()); lerr != nil {
		s.deps.Log.Error("routine set last run", "err", lerr, "routine_id", routine.ID)
	}
	s.WakeBotRunner()
	writeJSON(w, http.StatusAccepted, map[string]any{"run": toBotRunDTO(run)})
}
