package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

const (
	maxBotTaskChars  = 8000
	botRunsDefaultN  = 20
	botRunsMaxN      = 100
	botRunErrCancel  = "cancelled before start"
	botRunStatusWait = "waiting_approval"
)

// botRunDTO is a bot run as the API sees it: activity is the decoded
// journal (never null — an empty run marshals []), timestamps are RFC3339
// or null while the run has not reached the point yet, and costUsd is the
// micro-USD total converted to dollars.
type botRunDTO struct {
	ID             string           `json:"id"`
	BotID          string           `json:"botId"`
	ConversationID string           `json:"conversationId"`
	Status         string           `json:"status"`
	Task           string           `json:"task"`
	TriggerSource  string           `json:"triggerSource"`
	Error          string           `json:"error"`
	Activity       []map[string]any `json:"activity"`
	InputTokens    int              `json:"inputTokens"`
	OutputTokens   int              `json:"outputTokens"`
	Requests       int              `json:"requests"`
	CostUSD        float64          `json:"costUsd"`
	CreatedAt      *string          `json:"createdAt"`
	StartedAt      *string          `json:"startedAt"`
	FinishedAt     *string          `json:"finishedAt"`
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(timeRFC3339)
	return &s
}

func toBotRunDTO(run storage.BotRun) botRunDTO {
	activity := []map[string]any{}
	if len(run.Activity) > 0 {
		var decoded []map[string]any
		if err := json.Unmarshal(run.Activity, &decoded); err == nil && decoded != nil {
			activity = decoded
		}
	}
	return botRunDTO{
		ID: run.ID, BotID: run.BotID, ConversationID: run.ConversationID,
		Status: run.Status, Task: run.Task, TriggerSource: run.TriggerSource,
		Error: run.Error, Activity: activity,
		InputTokens: run.InputTokens, OutputTokens: run.OutputTokens,
		Requests: run.Requests, CostUSD: float64(run.CostMicroUsd) / 1e6,
		CreatedAt: formatTimePtr(run.CreatedAt), StartedAt: formatTimePtr(run.StartedAt),
		FinishedAt: formatTimePtr(run.FinishedAt),
	}
}

// botRunPendingDTO is one approval-gated call a parked run is waiting on.
type botRunPendingDTO struct {
	CallID   string          `json:"callId"`
	ToolName string          `json:"toolName"`
	Args     json.RawMessage `json:"args"`
	Reason   string          `json:"reason"`
}

// handleCreateBotTask enqueues a background run: it resolves (or opens) the
// bot-scoped conversation, guards against a still-paused or busy one, and
// queues a manual run the background runner picks up. 202, not 200 — the
// work happens after the response.
func (s *Server) handleCreateBotTask(w http.ResponseWriter, r *http.Request) {
	if s.deps.Convos == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "conversations not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Task           string `json:"task"`
		ConversationID string `json:"conversationId"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	task := strings.TrimSpace(req.Task)
	if task == "" || utf8.RuneCountInString(task) > maxBotTaskChars {
		writeError(w, http.StatusBadRequest, "invalid_task", "task must be 1-8000 characters")
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
		writeError(w, http.StatusBadRequest, "bot_archived", "archived bots cannot run tasks")
		return
	}

	// Resolve the conversation: an explicit one must already be scoped to
	// this bot; otherwise a fresh bot conversation is opened exactly the way
	// handleCreateBotConversation does (model + RAG inherited, bot_id set).
	var conv storage.Conversation
	if req.ConversationID != "" {
		conv, err = s.deps.Convos.ByID(r.Context(), req.ConversationID, userID)
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
			return
		}
		if conv.BotID == nil || *conv.BotID != bot.ID {
			writeError(w, http.StatusBadRequest, "wrong_conversation", "conversation does not belong to this bot")
			return
		}
	} else {
		botModel := bot.Model
		rag := bot.RagEnabled
		conv, err = s.deps.Convos.Create(r.Context(), userID, bot.Name, storage.ConversationPatch{
			Model:      &botModel,
			RagEnabled: &rag,
			BotID:      &botID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not create conversation")
			return
		}
	}

	// One live run per bot conversation, and no new work while approvals of
	// a parked run are unresolved (blockedByPendingApproval writes the 409).
	if s.blockedByPendingApproval(w, r.Context(), conv.ID, userID) {
		return
	}
	active, err := s.deps.BotRuns.HasActiveForConversation(r.Context(), userID, conv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not check active runs")
		return
	}
	if active {
		writeError(w, http.StatusConflict, "run_active", "a run is already active for this conversation")
		return
	}

	run, err := s.deps.BotRuns.Create(r.Context(), storage.BotRun{
		BotID: bot.ID, ConversationID: conv.ID, UserID: userID,
		Status: "queued", Task: task, TriggerSource: "manual",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not enqueue run")
		return
	}
	s.WakeBotRunner()
	writeJSON(w, http.StatusAccepted, map[string]any{"run": toBotRunDTO(run)})
}

// handleListBotRuns lists a bot's runs, newest first. limit defaults to 20
// and is capped at 100; an invalid value silently falls back to the default.
func (s *Server) handleListBotRuns(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	limit := botRunsDefaultN
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = botRunsDefaultN
	}
	if limit > botRunsMaxN {
		limit = botRunsMaxN
	}
	runs, err := s.deps.BotRuns.ListByBot(r.Context(), userID, r.PathValue("id"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list runs")
		return
	}
	out := make([]botRunDTO, 0, len(runs))
	for _, run := range runs {
		out = append(out, toBotRunDTO(run))
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}

// handleGetBotRun returns one run plus the calls it is parked on: pending
// is filled only while the run waits for approvals (empty array otherwise,
// never null).
func (s *Server) handleGetBotRun(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	run, err := s.deps.BotRuns.Get(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load run")
		return
	}
	pending := []botRunPendingDTO{}
	if run.Status == botRunStatusWait && s.deps.Pending != nil {
		rows, perr := s.deps.Pending.ForConversation(r.Context(), run.ConversationID, userID)
		if perr == nil {
			for _, c := range rows {
				if c.Status != "pending" { // store contract filters; be defensive
					continue
				}
				args := json.RawMessage(c.Args)
				if len(args) == 0 {
					args = json.RawMessage("{}")
				}
				pending = append(pending, botRunPendingDTO{
					CallID: c.CallID, ToolName: c.ToolName, Args: args, Reason: c.Reason,
				})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": toBotRunDTO(run), "pending": pending})
}

// handleCancelBotRun stops a run. A queued one has no worker yet, so the
// row is settled cancelled right here; a live one (running or parked) is
// cancelled through the runner and its goroutine persists the partial
// evidence; anything terminal is a 409.
func (s *Server) handleCancelBotRun(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	run, err := s.deps.BotRuns.Get(r.Context(), userID, id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load run")
		return
	}
	switch run.Status {
	case "queued":
		// Cancel-while-queued shortcut: no worker owns the row, so the
		// handler settles it directly instead of racing a claim.
		if err := s.deps.BotRuns.SetFinished(r.Context(), id, "cancelled", 0, 0, 0, 0, botRunErrCancel); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "run not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal", "could not cancel run")
			return
		}
		run, err = s.deps.BotRuns.Get(r.Context(), userID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not load run")
			return
		}
	case "running", botRunStatusWait:
		// The worker goroutine settles the row (partial evidence included);
		// the response carries the pre-cancel snapshot.
		s.CancelBotRun(id)
	default:
		writeError(w, http.StatusConflict, "already_finished", "run already finished")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": toBotRunDTO(run)})
}

// handleBotRunApprovals decides the calls a parked run waits on and wakes
// it. It mirrors the DB half of handleApprovals exactly: every pending call
// resolved exactly once (duplicates/unknowns 400), audit entries per
// decision, and rows flipped BEFORE the resume notification — a premature
// wake merely re-parks (the runner re-checks before resuming). The run's
// status is still waiting_approval in the response; the runner flips it.
func (s *Server) handleBotRunApprovals(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	ctx := r.Context()
	run, err := s.deps.BotRuns.Get(ctx, userID, r.PathValue("id"))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load run")
		return
	}
	if run.Status != botRunStatusWait {
		writeError(w, http.StatusConflict, "not_waiting", "run is not waiting for approvals")
		return
	}
	if s.deps.Pending == nil {
		writeError(w, http.StatusInternalServerError, "internal", "approvals not configured")
		return
	}
	var req struct {
		Decisions []approvalDecision `json:"decisions"`
	}
	if err := decodeJSON(r, &req); err != nil || len(req.Decisions) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "decisions are required")
		return
	}

	convID := run.ConversationID
	pending, err := s.deps.Pending.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load pending approvals")
		return
	}
	if len(pending) == 0 {
		writeError(w, http.StatusBadRequest, "no_pending", "nothing is awaiting approval")
		return
	}

	// Every live pending call must be resolved exactly once. Audit entries
	// ride the validation pass, exactly as handleApprovals writes them.
	pendingByID := make(map[string]storage.PendingToolCall, len(pending))
	for _, c := range pending {
		pendingByID[c.CallID] = c
	}
	seen := map[string]bool{}
	for _, d := range req.Decisions {
		if _, ok := pendingByID[d.CallID]; !ok || seen[d.CallID] {
			writeError(w, http.StatusBadRequest, "validation_error", "unknown or duplicated callId "+d.CallID)
			return
		}
		seen[d.CallID] = true
		if s.deps.Audits != nil {
			action := "approval_granted"
			decisionStatus := "approved"
			if !d.Approved {
				action = "approval_rejected"
				decisionStatus = "rejected"
			}
			call := pendingByID[d.CallID]
			_ = s.deps.Audits.RecordToolAudit(ctx, storage.ToolAuditLog{
				UserID:       userID,
				CallerType:   "chat_agent",
				CallerID:     convID,
				ToolName:     call.ToolName,
				Action:       action,
				InputSummary: string(call.Args),
				Status:       decisionStatus,
				DurationMs:   0,
				CreatedAt:    time.Now(),
			})
		}
	}
	if len(seen) != len(pendingByID) {
		writeError(w, http.StatusBadRequest, "validation_error", "every pending call needs a decision")
		return
	}

	status := map[string]string{}
	for _, d := range req.Decisions {
		if d.Approved {
			status[d.CallID] = "approved"
		} else {
			status[d.CallID] = "denied"
		}
	}
	// Rows flip before the wake so a crash cannot re-ask a resolved call; a
	// failed resume is reportable, a re-ask is not.
	for callID, st := range status {
		if err := s.deps.Pending.SetStatus(ctx, userID, callID, st); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not record decision")
			return
		}
	}

	s.NotifyBotRunResume(run.ID)
	writeJSON(w, http.StatusOK, map[string]any{"run": toBotRunDTO(run)})
}
