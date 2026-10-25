package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// fakeBotRuns mirrors storage.BotRuns semantics in memory. ClaimNext takes
// the oldest queued run in insertion order (created_at order); RecoverRunning
// requeues running AND waiting_approval rows, per the runner's restart story.
type fakeBotRuns struct {
	mu     sync.Mutex
	nextID int
	rows   []*storage.BotRun
}

func newFakeBotRuns() *fakeBotRuns { return &fakeBotRuns{} }

var _ BotRunStore = (*fakeBotRuns)(nil)

func (f *fakeBotRuns) Create(_ context.Context, run storage.BotRun) (storage.BotRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	run.ID = fmt.Sprintf("run-%d", f.nextID)
	if run.Status == "" {
		run.Status = "queued"
	}
	if run.TriggerSource == "" {
		run.TriggerSource = "manual"
	}
	now := time.Now()
	run.CreatedAt = &now
	run.Activity = []byte("[]")
	f.rows = append(f.rows, &run)
	return run, nil
}

func (f *fakeBotRuns) Get(_ context.Context, userID, id string) (storage.BotRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id && r.UserID == userID {
			return *r, nil
		}
	}
	return storage.BotRun{}, storage.ErrNotFound
}

func (f *fakeBotRuns) ListByBot(_ context.Context, userID, botID string, limit int) ([]storage.BotRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.BotRun
	for i := len(f.rows) - 1; i >= 0 && len(out) < limit; i-- {
		if f.rows[i].UserID == userID && f.rows[i].BotID == botID {
			out = append(out, *f.rows[i])
		}
	}
	return out, nil
}

func (f *fakeBotRuns) LatestPerBot(_ context.Context, userID string) (map[string]storage.BotRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]storage.BotRun{}
	for _, r := range f.rows {
		if r.UserID != userID {
			continue
		}
		if cur, ok := out[r.BotID]; !ok || r.CreatedAt.After(*cur.CreatedAt) {
			out[r.BotID] = *r
		}
	}
	return out, nil
}

func (f *fakeBotRuns) ClaimNext(context.Context) (*storage.BotRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.Status == "queued" {
			r.Status = "running"
			now := time.Now()
			r.StartedAt = &now
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeBotRuns) SetWaitingApproval(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id {
			r.Status = "waiting_approval"
			return nil
		}
	}
	return storage.ErrNotFound
}

func (f *fakeBotRuns) SetFinished(_ context.Context, id, status string, inTok, outTok, reqs int, costMicros int64, runErr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id {
			r.Status = status
			r.InputTokens = inTok
			r.OutputTokens = outTok
			r.Requests = reqs
			r.CostMicroUsd = costMicros
			r.Error = runErr
			now := time.Now()
			r.FinishedAt = &now
			return nil
		}
	}
	return storage.ErrNotFound
}

func (f *fakeBotRuns) UpdateActivity(_ context.Context, id string, activity []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ID == id {
			r.Activity = activity
			return nil
		}
	}
	return storage.ErrNotFound
}

func (f *fakeBotRuns) RecoverRunning(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.Status == "running" || r.Status == "waiting_approval" {
			r.Status = "queued"
			r.StartedAt = nil
		}
	}
	return nil
}

func (f *fakeBotRuns) HasActiveForConversation(_ context.Context, userID, conversationID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.UserID == userID && r.ConversationID == conversationID &&
			(r.Status == "queued" || r.Status == "running" || r.Status == "waiting_approval") {
			return true, nil
		}
	}
	return false, nil
}

// fakeAgentStep scripts one agent call's result.
type fakeAgentStep struct {
	outcome chat.Outcome
	err     error
}

// fakeBotAgent scripts the runner's model calls in order and records what
// it was asked (prompt / resolutions) for assertions.
type fakeBotAgent struct {
	mu              sync.Mutex
	steps           []fakeAgentStep
	runCalls        int
	deferredCalls   int
	lastPrompt      string
	lastResolutions map[string]golem.Approval
}

func (f *fakeBotAgent) next() (chat.Outcome, error) {
	if len(f.steps) == 0 {
		return chat.Outcome{}, errors.New("fakeBotAgent: no step scripted")
	}
	st := f.steps[0]
	f.steps = f.steps[1:]
	return st.outcome, st.err
}

func (f *fakeBotAgent) Run(_ context.Context, _ chat.Deps, _ []model.Message, prompt string, _ []model.Part, _ chat.Sink, _ []tool.Tool[chat.Deps], _ chat.RunSpec) (chat.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runCalls++
	f.lastPrompt = prompt
	return f.next()
}

func (f *fakeBotAgent) RunDeferred(_ context.Context, _ chat.Deps, _ []model.Message, res golem.DeferredResults, _ chat.Sink, _ []tool.Tool[chat.Deps], _ chat.RunSpec) (chat.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deferredCalls++
	f.lastResolutions = res.Approvals
	return f.next()
}

func (f *fakeBotAgent) snapshot() (int, int, string, map[string]golem.Approval, []fakeAgentStep) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := append([]fakeAgentStep(nil), f.steps...)
	return f.runCalls, f.deferredCalls, f.lastPrompt, f.lastResolutions, cp
}

// newBotRunnerTestServer wires the runner against fakes: no tools, no RAG,
// no web search — the agent fake never touches a network.
func newBotRunnerTestServer(agent *fakeBotAgent, runs *fakeBotRuns, convs ConvoStore, msgs MsgStore, usage UsageStore, pending PendingStore) *Server {
	s := &Server{deps: ServerDeps{
		Cfg:       config.Config{BotRunnerConcurrency: 2, BotRunnerPollInterval: 10 * time.Millisecond},
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles:  newFakeProfiles(),
		Convos:    convs,
		Msgs:      msgs,
		Usage:     usage,
		Pending:   pending,
		BotRuns:   runs,
		Rates:     cost.Table{Default: cost.Rates{ChatInputPerM: 0.3, ChatOutputPerM: 2.5}},
		ModelKeys: chat.ProviderKeys{Gemini: "test"},
	}}
	s.botAgent = agent
	return s
}

// waitForBotRun polls the fake store until the run reaches status (5s
// deadline; the repo's tests use plain for-loops, no testify).
func waitForBotRun(t *testing.T, runs *fakeBotRuns, id, status string) storage.BotRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, err := runs.Get(context.Background(), "u-1", id)
		if err == nil && run.Status == status {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	run, _ := runs.Get(context.Background(), "u-1", id)
	t.Fatalf("run %s never reached %q (last %+v)", id, status, run)
	return storage.BotRun{}
}

// messageRow reports whether the conversation has a row of the role whose
// content contains the snippet.
func messageRow(msgs *fakeMsgs, convID, role, contains string) bool {
	for _, m := range msgs.rows {
		if m.ConversationID == convID && m.Role == role && strings.Contains(m.Content, contains) {
			return true
		}
	}
	return false
}

func TestRunnerFinishesRun(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	runs := newFakeBotRuns()
	pending := &fakePending{}
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
	s := newBotRunnerTestServer(agent, runs, convs, msgs, usage, pending)

	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Create(context.Background(), storage.BotRun{
		ConversationID: conv.ID, UserID: "u-1", Task: "check the inbox",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartBotRunner(ctx)
	s.WakeBotRunner()

	finished := waitForBotRun(t, runs, run.ID, "succeeded")

	// The task became the turn's user row and the answer an assistant row.
	if !messageRow(msgs, conv.ID, string(model.RoleUser), "check the inbox") {
		t.Fatalf("task user row missing: %+v", msgs.rows)
	}
	if !messageRow(msgs, conv.ID, string(model.RoleAssistant), "All clear.") {
		t.Fatalf("assistant row missing: %+v", msgs.rows)
	}
	// A usageEventFor-shaped event: chat kind on the bot conversation.
	found := false
	for _, e := range usage.events {
		if e.Kind == "chat" && e.ConversationID != nil && *e.ConversationID == conv.ID &&
			e.InputTokens == 10 && e.OutputTokens == 5 && e.Requests == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("chat usage event missing: %+v", usage.events)
	}
	if finished.InputTokens != 10 || finished.OutputTokens != 5 || finished.Requests != 1 {
		t.Fatalf("run totals not recorded: %+v", finished)
	}
	// 10*0.3 + 5*2.5 = 15.5 → 16 micro-USD.
	if finished.CostMicroUsd != 16 {
		t.Fatalf("cost = %d, want 16", finished.CostMicroUsd)
	}
	if finished.FinishedAt == nil {
		t.Fatal("finished_at not stamped")
	}
	if _, _, prompt, _, _ := agent.snapshot(); prompt != "check the inbox" {
		t.Fatalf("agent prompt = %q", prompt)
	}
}

func TestRunnerParksAndResumesOnApproval(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	runs := newFakeBotRuns()
	pending := &fakePending{}
	agent := &fakeBotAgent{steps: []fakeAgentStep{
		{ // first call pauses on an approval-gated tool
			outcome: chat.Outcome{
				Messages: []model.Message{
					{Role: model.RoleUser, Content: "check the weather"},
					{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{
						{ID: "call-1", Name: "get_weather", Args: json.RawMessage(`{"city":"x"}`)},
					}},
				},
				Usage:    model.Usage{InputTokens: 10, OutputTokens: 5},
				Requests: 1,
				Pending: []chat.PendingApproval{{
					CallID: "call-1", ToolName: "get_weather",
					Args: json.RawMessage(`{"city":"x"}`), Reason: "approval required",
				}},
			},
		},
		{ // the resumed call completes
			outcome: chat.Outcome{
				Output: "It is 21C",
				Messages: []model.Message{
					{Role: model.RoleUser, Content: "check the weather"},
					{Role: model.RoleAssistant, Content: "It is 21C"},
				},
				Usage:    model.Usage{InputTokens: 8, OutputTokens: 4},
				Requests: 1,
			},
		},
	}}
	s := newBotRunnerTestServer(agent, runs, convs, msgs, usage, pending)

	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Create(context.Background(), storage.BotRun{
		ConversationID: conv.ID, UserID: "u-1", Task: "check the weather",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartBotRunner(ctx)
	s.WakeBotRunner()

	waitForBotRun(t, runs, run.ID, "waiting_approval")

	// SetWaitingApproval is written after the pending rows, so the parked
	// call exists by now — with its persisted (rewritten) call ID.
	if len(pending.rows) != 1 {
		t.Fatalf("parked calls = %d, want 1: %+v", len(pending.rows), pending.rows)
	}
	parkedID := pending.rows[0].CallID
	if parkedID == "call-1" || parkedID == "" {
		t.Fatalf("parked call ID not rewritten to the persisted ID: %q", parkedID)
	}
	// Decide exactly the way Task 8's approvals endpoint will: rows flip
	// BEFORE the resume notification.
	if err := pending.SetStatus(context.Background(), "u-1", parkedID, "approved"); err != nil {
		t.Fatal(err)
	}
	s.NotifyBotRunResume(run.ID)

	finished := waitForBotRun(t, runs, run.ID, "succeeded")

	runCalls, deferredCalls, _, resolutions, _ := agent.snapshot()
	if runCalls != 1 || deferredCalls != 1 {
		t.Fatalf("agent calls: run=%d deferred=%d, want 1/1", runCalls, deferredCalls)
	}
	if approved, ok := resolutions[parkedID]; !ok || !approved.Approved {
		t.Fatalf("resolutions = %+v, want %q approved", resolutions, parkedID)
	}
	if !messageRow(msgs, conv.ID, string(model.RoleAssistant), "It is 21C") {
		t.Fatalf("resumed answer row missing: %+v", msgs.rows)
	}
	// Totals across both pause cycles: (10+8) in, (5+4) out, 2 requests.
	if finished.InputTokens != 18 || finished.OutputTokens != 9 || finished.Requests != 2 {
		t.Fatalf("run totals across cycles: %+v", finished)
	}
	// 18*0.3 + 9*2.5 = 27.9 → 28 micro-USD.
	if finished.CostMicroUsd != 28 {
		t.Fatalf("cost = %d, want 28", finished.CostMicroUsd)
	}
	// Two usage events: one per pause cycle.
	if len(usage.events) != 2 {
		t.Fatalf("usage events = %d, want 2: %+v", len(usage.events), usage.events)
	}
}

// TestRunnerShutdownLeavesParkedRun locks the restart story: a parent
// cancellation while parked must NOT settle the run — the next boot's
// RecoverRunning requeues it for a fresh start.
func TestRunnerShutdownLeavesParkedRun(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	runs := newFakeBotRuns()
	pending := &fakePending{}
	agent := &fakeBotAgent{steps: []fakeAgentStep{
		{outcome: chat.Outcome{
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "check the weather"},
				{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{
					{ID: "call-1", Name: "get_weather", Args: json.RawMessage(`{}`)},
				}},
			},
			Requests: 1,
			Pending:  []chat.PendingApproval{{CallID: "call-1", ToolName: "get_weather"}},
		}},
		// A second step exists so a buggy resume-instead-of-park would not
		// hang the test on an unscripted call — it would fail assertions.
		{outcome: chat.Outcome{Output: "should not run"}},
	}}
	s := newBotRunnerTestServer(agent, runs, convs, msgs, usage, pending)

	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Create(context.Background(), storage.BotRun{
		ConversationID: conv.ID, UserID: "u-1", Task: "check the weather",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.StartBotRunner(ctx)
	s.WakeBotRunner()
	waitForBotRun(t, runs, run.ID, "waiting_approval")

	cancel() // server shutdown

	// The parked goroutine exits without settling; the row must stay
	// waiting_approval so the next boot requeues it.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := runs.Get(context.Background(), "u-1", run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "waiting_approval" {
			t.Fatalf("shutdown settled parked run to %q; it must stay waiting_approval", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// And the next boot's recovery requeues it.
	if err := runs.RecoverRunning(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := runs.Get(context.Background(), "u-1", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "queued" {
		t.Fatalf("recovered status = %q, want queued", got.Status)
	}
}

// TestRunnerCancelWhileParked: CancelBotRun on a parked run settles it as
// cancelled (the parent context is still alive, so this is a user cancel).
func TestRunnerCancelWhileParked(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	runs := newFakeBotRuns()
	pending := &fakePending{}
	agent := &fakeBotAgent{steps: []fakeAgentStep{
		{outcome: chat.Outcome{
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "do it"},
				{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{
					{ID: "call-1", Name: "get_weather", Args: json.RawMessage(`{}`)},
				}},
			},
			Requests: 1,
			Pending:  []chat.PendingApproval{{CallID: "call-1", ToolName: "get_weather"}},
		}},
	}}
	s := newBotRunnerTestServer(agent, runs, convs, msgs, usage, pending)

	conv, err := convs.Create(context.Background(), "u-1", "Scout chat", storage.ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Create(context.Background(), storage.BotRun{
		ConversationID: conv.ID, UserID: "u-1", Task: "do it",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartBotRunner(ctx)
	s.WakeBotRunner()
	waitForBotRun(t, runs, run.ID, "waiting_approval")

	s.CancelBotRun(run.ID)
	finished := waitForBotRun(t, runs, run.ID, "cancelled")
	if finished.Error != "" || finished.FinishedAt == nil {
		t.Fatalf("cancelled run settlement: %+v", finished)
	}
	// A late resume notify after the run exited is a safe no-op.
	s.NotifyBotRunResume(run.ID)
}

func TestRunSinkCapsActivity(t *testing.T) {
	sink := &runSink{}
	for i := 0; i < 60; i++ {
		sink.ToolStart(fmt.Sprintf("t%d", i))
	}
	_, activity, inTok, outTok := sink.snapshot()
	if len(activity) != 50 {
		t.Fatalf("activity capped to %d, want 50", len(activity))
	}
	if activity[0]["name"] != "t10" || activity[49]["name"] != "t59" {
		t.Fatalf("activity window wrong: %v..%v", activity[0]["name"], activity[49]["name"])
	}
	sink.ModelEnd(3, 4)
	if inTok != 0 || outTok != 0 {
		t.Fatal("snapshot returned stale token counts")
	}
	if _, activity, inTok, outTok = sink.snapshot(); inTok != 3 || outTok != 4 {
		t.Fatalf("tokens = %d/%d, want 3/4", inTok, outTok)
	}
	if len(activity) != 50 {
		t.Fatal("activity length changed unexpectedly")
	}
}
