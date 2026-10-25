package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// fakeBotRoutines mirrors storage.BotRoutines semantics in memory: Create
// forces Enabled and defaults the timezone to UTC (and cannot express a
// birth last_run_at, exactly like the SQL INSERT), SetLastRun records every
// stamp so tests can assert the scheduler consumed an occurrence.
type fakeBotRoutines struct {
	mu     sync.Mutex
	nextID int
	rows   []*storage.BotRoutine
	stamps []time.Time
}

func newFakeBotRoutines() *fakeBotRoutines { return &fakeBotRoutines{} }

var _ BotRoutineStore = (*fakeBotRoutines)(nil)

func (f *fakeBotRoutines) Create(_ context.Context, r storage.BotRoutine) (storage.BotRoutine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	r.ID = fmt.Sprintf("rt-%d", f.nextID)
	if r.Timezone == "" {
		r.Timezone = "UTC"
	}
	r.Enabled = true
	r.LastRunAt = nil
	now := time.Now().UTC()
	r.CreatedAt, r.UpdatedAt = now, now
	f.rows = append(f.rows, &r)
	return r, nil
}

func (f *fakeBotRoutines) Get(_ context.Context, userID, id string) (storage.BotRoutine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id && r.UserID == userID {
			return *r, nil
		}
	}
	return storage.BotRoutine{}, storage.ErrNotFound
}

func (f *fakeBotRoutines) ListByBot(_ context.Context, userID, botID string) ([]storage.BotRoutine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.BotRoutine
	for i := len(f.rows) - 1; i >= 0; i-- {
		if f.rows[i].UserID == userID && f.rows[i].BotID == botID {
			out = append(out, *f.rows[i])
		}
	}
	return out, nil
}

func (f *fakeBotRoutines) ListEnabled(context.Context) ([]storage.BotRoutine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.BotRoutine
	for _, r := range f.rows {
		if r.Enabled {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (f *fakeBotRoutines) Update(_ context.Context, userID, id string, p storage.BotRoutinePatch) (storage.BotRoutine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID != id || r.UserID != userID {
			continue
		}
		if p.Name != nil {
			r.Name = *p.Name
		}
		if p.Schedule != nil {
			r.Schedule = *p.Schedule
		}
		if p.Timezone != nil {
			r.Timezone = *p.Timezone
		}
		if p.Prompt != nil {
			r.Prompt = *p.Prompt
		}
		if p.Enabled != nil {
			r.Enabled = *p.Enabled
		}
		r.UpdatedAt = time.Now().UTC()
		return *r, nil
	}
	return storage.BotRoutine{}, storage.ErrNotFound
}

func (f *fakeBotRoutines) Delete(_ context.Context, userID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.rows {
		if r.ID == id && r.UserID == userID {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

func (f *fakeBotRoutines) SetLastRun(_ context.Context, id string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id {
			t := at
			r.LastRunAt = &t
			f.stamps = append(f.stamps, at)
			return nil
		}
	}
	return storage.ErrNotFound
}

// stampedSince reports whether any SetLastRun landed at or after t (the
// scheduler's own stamps — setup stamps made before t do not count).
func (f *fakeBotRoutines) stampedSince(t time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.stamps {
		if !s.Before(t) {
			return true
		}
	}
	return false
}

// newRoutineTestServer wires the routine scheduler surface against fakes:
// the runner, its agent, and every store the scheduler's enqueue path
// touches. No network anywhere.
func newRoutineTestServer(agent *fakeBotAgent, runs *fakeBotRuns, convs ConvoStore, msgs MsgStore,
	routines *fakeBotRoutines, bots BotStore) *Server {
	s := &Server{deps: ServerDeps{
		Cfg: config.Config{
			BotRunnerConcurrency:   2,
			BotRunnerPollInterval:  10 * time.Millisecond,
			BotRoutineTickInterval: 10 * time.Millisecond,
			MaxRoutinesPerBot:      10,
		},
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles:    newFakeProfiles(),
		Convos:      convs,
		Msgs:        msgs,
		Usage:       newFakeUsage(),
		Pending:     &fakePending{},
		Bots:        bots,
		BotRuns:     runs,
		BotRoutines: routines,
		Rates:       cost.Table{Default: cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5}},
		ModelKeys:   chat.ProviderKeys{Gemini: "test"},
	}}
	s.botAgent = agent
	return s
}

// routineRuns returns the bot's runs filtered to one trigger source.
func routineRuns(t *testing.T, runs *fakeBotRuns, botID, trigger string) []storage.BotRun {
	t.Helper()
	all, err := runs.ListByBot(context.Background(), "u-1", botID, 100)
	if err != nil {
		t.Fatal(err)
	}
	var out []storage.BotRun
	for _, run := range all {
		if run.TriggerSource == trigger {
			out = append(out, run)
		}
	}
	return out
}

func TestParseRoutineSchedule(t *testing.T) {
	for _, tc := range []struct {
		name, schedule, timezone string
		wantErr                  string
	}{
		{name: "every five minutes", schedule: "*/5 * * * *", timezone: "UTC"},
		{name: "hourly, empty timezone is UTC", schedule: "0 * * * *", timezone: ""},
		{name: "too few fields", schedule: "*/5 * * *", timezone: "UTC", wantErr: "invalid 5-field cron schedule"},
		{name: "minute out of range", schedule: "61 * * * *", timezone: "UTC", wantErr: "invalid 5-field cron schedule"},
		{name: "descriptor not supported", schedule: "@hourly", timezone: "UTC", wantErr: "invalid 5-field cron schedule"},
		{name: "unknown timezone", schedule: "0 * * * *", timezone: "Mars/Phobos", wantErr: `unknown timezone "Mars/Phobos"`},
	} {
		sched, err := parseRoutineSchedule(tc.schedule, tc.timezone)
		if tc.wantErr == "" {
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			// A compiled schedule must produce future occurrences in the
			// routine's own timezone.
			now := time.Now()
			if next := sched.Next(now); !next.After(now) {
				t.Fatalf("%s: Next(%v) = %v, want a later time", tc.name, now, next)
			}
			continue
		}
		if err == nil {
			t.Fatalf("%s: want error %q, got schedule %v", tc.name, tc.wantErr, sched)
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: error %q does not contain %q", tc.name, err.Error(), tc.wantErr)
		}
	}
}

// TestRoutineSchedulerFiresDueRoutine: a routine whose last run is two
// hours old on an hourly schedule is due on the first tick — the scheduler
// must enqueue a run with trigger "routine" on the bot's conversation,
// stamp last_run (so the occurrence is consumed), and the runner must
// execute the queued run end to end. Bounded wait; nothing hangs on
// failure.
func TestRoutineSchedulerFiresDueRoutine(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	runs := newFakeBotRuns()
	routines := newFakeBotRoutines()
	bots := newFakeBots()
	agent := &fakeBotAgent{steps: []fakeAgentStep{{
		outcome: chat.Outcome{
			Output: "All clear.",
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "check the inbox"},
				{Role: model.RoleAssistant, Content: "All clear."},
			},
			Usage:    model.Usage{InputTokens: 10, OutputTokens: 5},
			Requests: 1,
		},
	}}}
	s := newRoutineTestServer(agent, runs, convs, msgs, routines, bots)

	ctx := context.Background()
	bot, err := bots.Create(ctx, storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := convs.Create(ctx, "u-1", "Scout chat", storage.ConversationPatch{BotID: &bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	routine, err := routines.Create(ctx, storage.BotRoutine{
		BotID: bot.ID, UserID: "u-1", Name: "hourly digests",
		Schedule: "0 * * * *", Timezone: "UTC", Prompt: "check the inbox",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Two hours since the last fire: the hourly occurrence is already due.
	if err := routines.SetLastRun(ctx, routine.ID, time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	s.StartBotRunner(runCtx)
	s.WakeBotRunner()

	deadline := time.Now().Add(5 * time.Second)
	fired := routineRuns(t, runs, bot.ID, "routine")
	for time.Now().Before(deadline) {
		if len(fired) >= 1 && routines.stampedSince(started) {
			break
		}
		time.Sleep(5 * time.Millisecond)
		fired = routineRuns(t, runs, bot.ID, "routine")
	}
	if len(fired) != 1 {
		t.Fatalf("routine-triggered runs = %d, want exactly 1: %+v", len(fired), fired)
	}
	if !routines.stampedSince(started) {
		t.Fatal("scheduler did not stamp last_run for the fired occurrence")
	}
	run := fired[0]
	if run.ConversationID != conv.ID {
		t.Fatalf("routine run conversation = %q, want the bot conversation %q", run.ConversationID, conv.ID)
	}
	if run.Task != "check the inbox" || run.Status == "" {
		t.Fatalf("routine run task/status wrong: %+v", run)
	}

	// The wake lands the queued run with the runner, which executes it.
	finished := waitForBotRun(t, runs, run.ID, "succeeded")
	if finished.TriggerSource != "routine" {
		t.Fatalf("finished run trigger = %q, want routine", finished.TriggerSource)
	}
	if !messageRow(msgs, conv.ID, string(model.RoleAssistant), "All clear.") {
		t.Fatalf("routine answer row missing: %+v", msgs.rows)
	}
}

// TestRunRoutineNowEndpoint: POST /run enqueues immediately with trigger
// "manual" and stamps last_run so the scheduler will not re-fire the same
// occurrence on its next tick.
func TestRunRoutineNowEndpoint(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	runs := newFakeBotRuns()
	routines := newFakeBotRoutines()
	bots := newFakeBots()
	agent := &fakeBotAgent{} // never started: no runner boots in this test
	s := newRoutineTestServer(agent, runs, convs, msgs, routines, bots)

	ctx := context.Background()
	bot, err := bots.Create(ctx, storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := convs.Create(ctx, "u-1", "Scout chat", storage.ConversationPatch{BotID: &bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	routine, err := routines.Create(ctx, storage.BotRoutine{
		BotID: bot.ID, UserID: "u-1", Name: "daily",
		Schedule: "0 9 * * *", Timezone: "UTC", Prompt: "daily digest",
	})
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	rec := httptest.NewRecorder()
	s.handleRunRoutineNow(rec, routineReq(http.MethodPost,
		"/api/bots/routines/"+routine.ID+"/run", map[string]any{}, "u-1", routine.ID))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Run botRunDTO `json:"run"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Run.TriggerSource != "manual" || out.Run.Task != "daily digest" || out.Run.Status != "queued" {
		t.Fatalf("run dto wrong: %+v", out.Run)
	}
	if out.Run.ConversationID != conv.ID {
		t.Fatalf("run conversation = %q, want %q", out.Run.ConversationID, conv.ID)
	}
	if !routines.stampedSince(started) {
		t.Fatal("run-now did not stamp last_run")
	}
	if got := routineRuns(t, runs, bot.ID, "manual"); len(got) != 1 {
		t.Fatalf("manual routine runs = %d, want 1", len(got))
	}

	// Unknown routine id is a 404, not an enqueue.
	rec = httptest.NewRecorder()
	s.handleRunRoutineNow(rec, routineReq(http.MethodPost,
		"/api/bots/routines/rt-nope/run", map[string]any{}, "u-1", "rt-nope"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown routine status %d: %s", rec.Code, rec.Body.String())
	}
}

func decodeRoutineBody(t *testing.T, rec *httptest.ResponseRecorder) botRoutineDTO {
	t.Helper()
	var out struct {
		Routine botRoutineDTO `json:"routine"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out.Routine
}

// routineReq builds an authed JSON request with the {id} path value set —
// the handlers are exercised directly, so the mux never populates it.
func routineReq(method, path string, body any, userID, id string) *http.Request {
	r := authedJSONReq(method, path, body, userID)
	r.SetPathValue("id", id)
	return r
}

// TestCreateRoutineValidation locks the create door: name/prompt rules,
// schedule+timezone validated as a pair, the per-bot cap, and the happy
// path (created enabled, nextRunAt computed server-side).
func TestCreateRoutineValidation(t *testing.T) {
	bots := newFakeBots()
	routines := newFakeBotRoutines()
	srv := &Server{deps: ServerDeps{
		Cfg:         config.Config{MaxRoutinesPerBot: 10},
		Bots:        bots,
		BotRoutines: routines,
	}}
	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	post := func(body map[string]any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.handleCreateRoutine(rec, routineReq(http.MethodPost,
			"/api/bots/"+bot.ID+"/routines", body, "u-1", bot.ID))
		return rec
	}

	for _, tc := range []struct {
		name string
		body map[string]any
		code string
	}{
		{"empty name", map[string]any{"name": "", "schedule": "*/5 * * * *", "prompt": "hi"}, "invalid_name"},
		{"long name", map[string]any{"name": strings.Repeat("x", 65), "schedule": "*/5 * * * *", "prompt": "hi"}, "invalid_name"},
		{"too few fields", map[string]any{"name": "Digest", "schedule": "*/5 * * *", "prompt": "hi"}, "invalid_schedule"},
		{"minute out of range", map[string]any{"name": "Digest", "schedule": "61 * * * *", "prompt": "hi"}, "invalid_schedule"},
		{"bad timezone", map[string]any{"name": "Digest", "schedule": "*/5 * * * *", "timezone": "Mars/Phobos", "prompt": "hi"}, "invalid_schedule"},
		{"empty prompt", map[string]any{"name": "Digest", "schedule": "*/5 * * * *", "prompt": "  "}, "invalid_prompt"},
		{"long prompt", map[string]any{"name": "Digest", "schedule": "*/5 * * * *", "prompt": strings.Repeat("x", 8001)}, "invalid_prompt"},
	} {
		rec := post(tc.body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d: %s", tc.name, rec.Code, rec.Body.String())
		}
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
			t.Fatalf("%s: decode error body: %v", tc.name, err)
		}
		if e.Error.Code != tc.code {
			t.Fatalf("%s: error code = %q, want %q (%s)", tc.name, e.Error.Code, tc.code, e.Error.Message)
		}
	}

	// Happy path: created enabled, defaults to UTC, nextRunAt is computed.
	rec := post(map[string]any{"name": "Digest", "schedule": "*/5 * * * *", "prompt": "check the inbox"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}
	dto := decodeRoutineBody(t, rec)
	if !dto.Enabled || dto.Timezone != "UTC" || dto.Schedule != "*/5 * * * *" {
		t.Fatalf("created routine wrong: %+v", dto)
	}
	if dto.NextRunAt == nil || dto.LastRunAt != nil {
		t.Fatalf("nextRunAt must be computed, lastRunAt must be null: %+v", dto)
	}

	// The per-bot cap: MAX_ROUTINES_PER_BOT (10 here) blocks the 11th —
	// the happy-path routine plus nine fillers bring the bot to ten.
	for i := 0; i < 9; i++ {
		rec := post(map[string]any{"name": fmt.Sprintf("r%d", i), "schedule": "0 * * * *", "prompt": "p"})
		if rec.Code != http.StatusCreated {
			t.Fatalf("filler routine %d status %d: %s", i, rec.Code, rec.Body.String())
		}
	}
	rec = post(map[string]any{"name": "one too many", "schedule": "0 * * * *", "prompt": "p"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit status %d: %s", rec.Code, rec.Body.String())
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error.Code != "routine_limit_reached" {
		t.Fatalf("limit error = %+v (%v), want routine_limit_reached", e, err)
	}
}

// TestUpdateRoutineValidation: PATCH re-validates the EFFECTIVE schedule
// pair (a timezone change alone must still parse against the stored
// schedule) and toggles enabled.
func TestUpdateRoutineValidation(t *testing.T) {
	bots := newFakeBots()
	routines := newFakeBotRoutines()
	srv := &Server{deps: ServerDeps{
		Cfg:         config.Config{MaxRoutinesPerBot: 10},
		Bots:        bots,
		BotRoutines: routines,
	}}
	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	routine, err := routines.Create(context.Background(), storage.BotRoutine{
		BotID: bot.ID, UserID: "u-1", Name: "Digest",
		Schedule: "0 * * * *", Timezone: "UTC", Prompt: "check the inbox",
	})
	if err != nil {
		t.Fatal(err)
	}
	patch := func(body map[string]any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		srv.handleUpdateRoutine(rec, routineReq(http.MethodPatch,
			"/api/bots/routines/"+routine.ID, body, "u-1", routine.ID))
		return rec
	}

	rec := patch(map[string]any{"schedule": "not a schedule"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad schedule status %d: %s", rec.Code, rec.Body.String())
	}
	rec = patch(map[string]any{"timezone": "Mars/Phobos"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad timezone status %d: %s", rec.Code, rec.Body.String())
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error.Code != "invalid_schedule" {
		t.Fatalf("timezone error = %+v (%v), want invalid_schedule", e, err)
	}

	off := false
	rec = patch(map[string]any{"enabled": off, "name": "Renamed"})
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle status %d: %s", rec.Code, rec.Body.String())
	}
	dto := decodeRoutineBody(t, rec)
	if dto.Enabled || dto.Name != "Renamed" || dto.Schedule != "0 * * * *" {
		t.Fatalf("updated routine wrong: %+v", dto)
	}
	// Disabled routines never reach ListEnabled, so the scheduler skips them.
	enabled, err := routines.ListEnabled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 0 {
		t.Fatalf("disabled routine still listed as enabled: %+v", enabled)
	}

	rec = httptest.NewRecorder()
	srv.handleDeleteRoutine(rec, routineReq(http.MethodDelete,
		"/api/bots/routines/"+routine.ID, nil, "u-1", routine.ID))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status %d: %s", rec.Code, rec.Body.String())
	}
	if rows, _ := routines.ListByBot(context.Background(), "u-1", bot.ID); len(rows) != 0 {
		t.Fatalf("routine not deleted: %+v", rows)
	}
}
