package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// recordingPending wraps fakePending and logs every SetStatus call so the
// approvals handler test can prove rows flip BEFORE the runner is notified.
type recordingPending struct {
	*fakePending
	mu     sync.Mutex
	events []string
}

func (p *recordingPending) append(e string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
}

func (p *recordingPending) log() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.events...)
}

func (p *recordingPending) SetStatus(ctx context.Context, userID, callID, status string) error {
	p.append("set:" + callID + ":" + status)
	return p.fakePending.SetStatus(ctx, userID, callID, status)
}

func newBotRunsTestServer(bots *fakeBots, convs ConvoStore, runs *fakeBotRuns, pending PendingStore, audits storage.AuditStore) *Server {
	return &Server{deps: ServerDeps{
		Cfg:     config.Config{},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Bots:    bots,
		Convos:  convs,
		Pending: pending,
		BotRuns: runs,
		Audits:  audits,
	}}
}

func decodeBotRunBody(t *testing.T, rec *httptest.ResponseRecorder) botRunDTO {
	t.Helper()
	var out struct {
		Run botRunDTO `json:"run"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out.Run
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return e.Error.Code
}

func TestCreateBotTaskValidation(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	srv := newBotRunsTestServer(bots, convs, newFakeBotRuns(), &fakePending{}, nil)

	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	archivedBot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Archived"})
	if err != nil {
		t.Fatal(err)
	}
	archived := true
	if _, err := bots.Update(context.Background(), "u-1", archivedBot.ID, storage.BotPatch{IsArchived: &archived}); err != nil {
		t.Fatal(err)
	}
	otherBot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	otherConv, err := convs.Create(context.Background(), "u-1", "Other chat", storage.ConversationPatch{BotID: &otherBot.ID})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		path string
		body map[string]any
		code int
		ec   string
	}{
		{"empty task", bot.ID, map[string]any{"task": ""}, http.StatusBadRequest, "invalid_task"},
		{"whitespace task", bot.ID, map[string]any{"task": "   "}, http.StatusBadRequest, "invalid_task"},
		{"long task", bot.ID, map[string]any{"task": strings.Repeat("x", 8001)}, http.StatusBadRequest, "invalid_task"},
		{"unknown bot", "missing", map[string]any{"task": "go"}, http.StatusNotFound, "not_found"},
		{"archived bot", archivedBot.ID, map[string]any{"task": "go"}, http.StatusBadRequest, "bot_archived"},
		{"wrong conversation", bot.ID, map[string]any{"task": "go", "conversationId": otherConv.ID}, http.StatusBadRequest, "wrong_conversation"},
	} {
		rec := httptest.NewRecorder()
		r := authedJSONReq(http.MethodPost, "/api/bots/"+tc.path+"/tasks", tc.body, "u-1")
		r.SetPathValue("id", tc.path)
		srv.handleCreateBotTask(rec, r)
		if rec.Code != tc.code {
			t.Fatalf("%s: want %d, got %d: %s", tc.name, tc.code, rec.Code, rec.Body.String())
		}
		if errorCode(t, rec) != tc.ec {
			t.Fatalf("%s: want code %s, got %q", tc.name, tc.ec, errorCode(t, rec))
		}
	}
}

func TestCreateBotTaskEnqueuesRun(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	runs := newFakeBotRuns()
	pending := &fakePending{}
	srv := newBotRunsTestServer(bots, convs, runs, pending, nil)

	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout", Model: "gemini-3.5-pro"})
	if err != nil {
		t.Fatal(err)
	}

	// Happy path: 202, a queued manual run, and a fresh bot conversation.
	rec := httptest.NewRecorder()
	r := authedJSONReq(http.MethodPost, "/api/bots/"+bot.ID+"/tasks", map[string]any{"task": "  check the inbox  "}, "u-1")
	r.SetPathValue("id", bot.ID)
	srv.handleCreateBotTask(rec, r)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("enqueue: want 202, got %d: %s", rec.Code, rec.Body.String())
	}
	run := decodeBotRunBody(t, rec)
	if run.Status != "queued" || run.TriggerSource != "manual" || run.Task != "check the inbox" {
		t.Fatalf("run fields wrong: %+v", run)
	}
	if run.BotID != bot.ID || run.ConversationID == "" || run.Activity == nil {
		t.Fatalf("run wiring wrong: %+v", run)
	}
	conv, err := convs.ByID(context.Background(), run.ConversationID, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if conv.BotID == nil || *conv.BotID != bot.ID {
		t.Fatalf("auto-created conversation not bot-scoped: %+v", conv)
	}

	// Same conversation again while the first run is still queued -> 409.
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bots/"+bot.ID+"/tasks",
		map[string]any{"task": "again", "conversationId": run.ConversationID}, "u-1")
	r.SetPathValue("id", bot.ID)
	srv.handleCreateBotTask(rec, r)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "run_active" {
		t.Fatalf("active guard: got %d %s", rec.Code, rec.Body.String())
	}

	// Unresolved approvals on the conversation block new tasks -> 409.
	finished := run.ConversationID
	if err := runs.SetFinished(context.Background(), run.ID, "succeeded", 0, 0, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := pending.Add(context.Background(), []storage.PendingToolCall{{
		CallID: "call-x", ConversationID: finished, UserID: "u-1",
		ToolName: "get_weather", Args: []byte(`{}`), Status: "pending",
	}}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bots/"+bot.ID+"/tasks",
		map[string]any{"task": "again", "conversationId": finished}, "u-1")
	r.SetPathValue("id", bot.ID)
	srv.handleCreateBotTask(rec, r)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "approval_pending" {
		t.Fatalf("approval guard: got %d %s", rec.Code, rec.Body.String())
	}
}

func TestListAndGetBotRun(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	runs := newFakeBotRuns()
	pending := &fakePending{}
	srv := newBotRunsTestServer(bots, convs, runs, pending, nil)

	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{BotID: &bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	first, err := runs.Create(context.Background(), storage.BotRun{
		BotID: bot.ID, ConversationID: conv.ID, UserID: "u-1", Task: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.ClaimNext(context.Background()); err != nil { // claims `first`, stamps started_at
		t.Fatal(err)
	}
	if err := runs.SetFinished(context.Background(), first.ID, "succeeded", 100, 50, 2, 1_500_000, ""); err != nil {
		t.Fatal(err)
	}
	second, err := runs.Create(context.Background(), storage.BotRun{
		BotID: bot.ID, ConversationID: conv.ID, UserID: "u-1", Task: "second",
	})
	if err != nil {
		t.Fatal(err)
	}

	// List: newest first, limit respected.
	rec := httptest.NewRecorder()
	r := authedJSONReq(http.MethodGet, "/api/bots/"+bot.ID+"/runs?limit=1", nil, "u-1")
	r.SetPathValue("id", bot.ID)
	srv.handleListBotRuns(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Runs []botRunDTO `json:"runs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Runs) != 1 || list.Runs[0].Task != "second" {
		t.Fatalf("list = %+v, want only the newest run", list.Runs)
	}

	// Get a finished run: pending must be an empty array, cost converted.
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodGet, "/api/bot-runs/"+first.ID, nil, "u-1")
	r.SetPathValue("id", first.ID)
	srv.handleGetBotRun(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Run     botRunDTO          `json:"run"`
		Pending []botRunPendingDTO `json:"pending"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Pending == nil {
		t.Fatal("pending must be an empty array, not null")
	}
	if got.Run.CostUSD != 1.5 {
		t.Fatalf("costUsd = %v, want 1.5", got.Run.CostUSD)
	}
	if got.Run.FinishedAt == nil || got.Run.StartedAt == nil {
		t.Fatalf("timestamps missing: %+v", got.Run)
	}

	// Unknown run -> 404.
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodGet, "/api/bot-runs/missing", nil, "u-1")
	r.SetPathValue("id", "missing")
	srv.handleGetBotRun(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run: want 404, got %d", rec.Code)
	}

	// A parked run surfaces its pending calls.
	if err := runs.SetWaitingApproval(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	if err := pending.Add(context.Background(), []storage.PendingToolCall{{
		CallID: "call-9", ConversationID: conv.ID, UserID: "u-1",
		ToolName: "get_weather", Args: []byte(`{"city":"x"}`), Reason: "approval required", Status: "pending",
	}}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodGet, "/api/bot-runs/"+second.ID, nil, "u-1")
	r.SetPathValue("id", second.ID)
	srv.handleGetBotRun(rec, r)
	got.Pending = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Pending) != 1 {
		t.Fatalf("pending calls = %+v, want 1", got.Pending)
	}
	if got.Pending[0].CallID != "call-9" || got.Pending[0].ToolName != "get_weather" ||
		string(got.Pending[0].Args) != `{"city":"x"}` || got.Pending[0].Reason != "approval required" {
		t.Fatalf("pending call fields wrong: %+v", got.Pending[0])
	}
}

func TestCancelBotRun(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	runs := newFakeBotRuns()
	srv := newBotRunsTestServer(bots, convs, runs, &fakePending{}, nil)

	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{BotID: &bot.ID})
	if err != nil {
		t.Fatal(err)
	}

	// Unknown run -> 404.
	rec := httptest.NewRecorder()
	r := authedJSONReq(http.MethodPost, "/api/bot-runs/missing/cancel", nil, "u-1")
	r.SetPathValue("id", "missing")
	srv.handleCancelBotRun(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run: want 404, got %d", rec.Code)
	}

	// Cancel-while-queued shortcut: settled right in the handler.
	queued, err := runs.Create(context.Background(), storage.BotRun{
		BotID: bot.ID, ConversationID: conv.ID, UserID: "u-1", Task: "queued work",
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bot-runs/"+queued.ID+"/cancel", nil, "u-1")
	r.SetPathValue("id", queued.ID)
	srv.handleCancelBotRun(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel queued: %d %s", rec.Code, rec.Body.String())
	}
	run := decodeBotRunBody(t, rec)
	if run.Status != "cancelled" || run.Error != "cancelled before start" || run.FinishedAt == nil {
		t.Fatalf("queued cancel settlement: %+v", run)
	}

	// A live (running) run goes through the runner; the response is the
	// pre-cancel snapshot.
	live, err := runs.Create(context.Background(), storage.BotRun{
		BotID: bot.ID, ConversationID: conv.ID, UserID: "u-1", Task: "live work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.ClaimNext(context.Background()); err != nil { // flips live to running
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bot-runs/"+live.ID+"/cancel", nil, "u-1")
	r.SetPathValue("id", live.ID)
	srv.handleCancelBotRun(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel running: %d %s", rec.Code, rec.Body.String())
	}
	if run := decodeBotRunBody(t, rec); run.Status != "running" {
		t.Fatalf("running cancel snapshot = %q, want running (worker settles)", run.Status)
	}

	// A terminal run -> 409.
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bot-runs/"+queued.ID+"/cancel", nil, "u-1")
	r.SetPathValue("id", queued.ID)
	srv.handleCancelBotRun(rec, r)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "already_finished" {
		t.Fatalf("finished cancel: got %d %s", rec.Code, rec.Body.String())
	}
}

func TestBotRunApprovals(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	runs := newFakeBotRuns()
	pending := &recordingPending{fakePending: &fakePending{}}
	audits := &fakeAuditStore{}
	srv := newBotRunsTestServer(bots, convs, runs, pending, audits)

	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{BotID: &bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := runs.Create(context.Background(), storage.BotRun{
		BotID: bot.ID, ConversationID: conv.ID, UserID: "u-1", Task: "work",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Not waiting -> 409 not_waiting, even with a valid body.
	rec := httptest.NewRecorder()
	r := authedJSONReq(http.MethodPost, "/api/bot-runs/"+queued.ID+"/approvals",
		map[string]any{"decisions": []map[string]any{{"callId": "call-a", "approved": true}}}, "u-1")
	r.SetPathValue("id", queued.ID)
	srv.handleBotRunApprovals(rec, r)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "not_waiting" {
		t.Fatalf("not waiting: got %d %s", rec.Code, rec.Body.String())
	}

	// Park the run with two pending calls.
	if err := runs.SetWaitingApproval(context.Background(), queued.ID); err != nil {
		t.Fatal(err)
	}
	if err := pending.Add(context.Background(), []storage.PendingToolCall{
		{CallID: "call-a", ConversationID: conv.ID, UserID: "u-1", ToolName: "get_weather",
			Args: []byte(`{"city":"x"}`), Reason: "approval required", Status: "pending"},
		{CallID: "call-b", ConversationID: conv.ID, UserID: "u-1", ToolName: "send_email",
			Args: []byte(`{"to":"x"}`), Reason: "approval required", Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}

	// Register a resume channel on a hand-built runner so the handler's
	// NotifyBotRunResume is observable; the watcher logs "notify" into the
	// same ordered event log the SetStatus wrapper writes to.
	runner := &botRunner{
		s:       srv,
		wake:    make(chan struct{}, 1),
		cancels: map[string]context.CancelFunc{},
		resumes: map[string]chan struct{}{},
	}
	srv.runner.Store(runner)
	resume := make(chan struct{})
	runner.setResume(queued.ID, resume)
	notified := make(chan struct{})
	go func() {
		<-resume
		pending.append("notify")
		close(notified)
	}()

	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bot-runs/"+queued.ID+"/approvals",
		map[string]any{"decisions": []map[string]any{
			{"callId": "call-a", "approved": true, "reason": "fine"},
			{"callId": "call-b", "approved": false, "reason": "no"},
		}}, "u-1")
	r.SetPathValue("id", queued.ID)
	srv.handleBotRunApprovals(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("decisions: %d %s", rec.Code, rec.Body.String())
	}
	<-notified // the wake landed (and was logged) — now the order is fixed

	// The response still shows waiting_approval: the runner flips it.
	if run := decodeBotRunBody(t, rec); run.Status != "waiting_approval" {
		t.Fatalf("post-decision status = %q, want waiting_approval", run.Status)
	}

	// Every SetStatus flip precedes the notify.
	events := pending.log()
	if len(events) == 0 || events[len(events)-1] != "notify" {
		t.Fatalf("decisions must flip rows before notifying: %v", events)
	}
	flips := 0
	for _, e := range events[:len(events)-1] {
		if strings.HasPrefix(e, "set:") {
			flips++
		}
	}
	if flips != 2 {
		t.Fatalf("flips = %d, want 2: %v", flips, events)
	}

	// The rows really flipped.
	rows, err := pending.ListAll(context.Background(), conv.ID, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, c := range rows {
		statuses[c.CallID] = c.Status
	}
	if statuses["call-a"] != "approved" || statuses["call-b"] != "denied" {
		t.Fatalf("row statuses = %v", statuses)
	}

	// Audit entries: one per decision, same shape handleApprovals writes.
	if len(audits.audits) != 2 {
		t.Fatalf("audit entries = %d, want 2: %+v", len(audits.audits), audits.audits)
	}
	byAction := map[string]storage.ToolAuditLog{}
	for _, a := range audits.audits {
		byAction[a.Action] = a
	}
	granted, ok := byAction["approval_granted"]
	if !ok || granted.ToolName != "get_weather" || granted.Status != "approved" ||
		granted.CallerType != "chat_agent" || granted.CallerID != conv.ID ||
		granted.InputSummary != `{"city":"x"}` || granted.UserID != "u-1" {
		t.Fatalf("approval_granted entry wrong: %+v", granted)
	}
	rejected, ok := byAction["approval_rejected"]
	if !ok || rejected.ToolName != "send_email" || rejected.Status != "rejected" {
		t.Fatalf("approval_rejected entry wrong: %+v", rejected)
	}

	// Rows are already decided: repeating a decision is rejected, and the
	// conversation now has nothing pending.
	rec = httptest.NewRecorder()
	r = authedJSONReq(http.MethodPost, "/api/bot-runs/"+queued.ID+"/approvals",
		map[string]any{"decisions": []map[string]any{{"callId": "call-a", "approved": true}}}, "u-1")
	r.SetPathValue("id", queued.ID)
	srv.handleBotRunApprovals(rec, r)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "no_pending" {
		t.Fatalf("repeat decisions: got %d %s", rec.Code, rec.Body.String())
	}
}

func TestBotRunApprovalsValidation(t *testing.T) {
	bots := newFakeBots()
	convs := newFakeConvos(newFakeMsgs())
	runs := newFakeBotRuns()
	pending := &fakePending{}
	srv := newBotRunsTestServer(bots, convs, runs, pending, nil)

	bot, err := bots.Create(context.Background(), storage.Bot{UserID: "u-1", Name: "Scout"})
	if err != nil {
		t.Fatal(err)
	}
	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{BotID: &bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Create(context.Background(), storage.BotRun{
		BotID: bot.ID, ConversationID: conv.ID, UserID: "u-1", Task: "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.SetWaitingApproval(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if err := pending.Add(context.Background(), []storage.PendingToolCall{
		{CallID: "call-a", ConversationID: conv.ID, UserID: "u-1", ToolName: "get_weather",
			Args: []byte(`{}`), Status: "pending"},
		{CallID: "call-b", ConversationID: conv.ID, UserID: "u-1", ToolName: "send_email",
			Args: []byte(`{}`), Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		decisions []map[string]any
	}{
		{"unknown callId", []map[string]any{{"callId": "call-z", "approved": true}, {"callId": "call-b", "approved": true}}},
		{"duplicate callId", []map[string]any{{"callId": "call-a", "approved": true}, {"callId": "call-a", "approved": true}, {"callId": "call-b", "approved": true}}},
		{"missing decision", []map[string]any{{"callId": "call-a", "approved": true}}},
	} {
		rec := httptest.NewRecorder()
		r := authedJSONReq(http.MethodPost, "/api/bot-runs/"+run.ID+"/approvals",
			map[string]any{"decisions": tc.decisions}, "u-1")
		r.SetPathValue("id", run.ID)
		srv.handleBotRunApprovals(rec, r)
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "validation_error" {
			t.Fatalf("%s: got %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}

	// Nothing was flipped by the rejected requests.
	rows, err := pending.ListAll(context.Background(), conv.ID, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rows {
		if c.Status != "pending" {
			t.Fatalf("rejected request flipped %q to %q", c.CallID, c.Status)
		}
	}
}
