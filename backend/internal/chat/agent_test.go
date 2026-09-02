package chat

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
	"github.com/abubakarsiddik31/golem/tool"
)

type recordingSink struct {
	deltas    []string
	ends      int
	in, out   int
	toolStart []string
	toolEnd   []string
}

func (s *recordingSink) Delta(text string) error { s.deltas = append(s.deltas, text); return nil }
func (s *recordingSink) ModelStart()             {}
func (s *recordingSink) ModelEnd(in, out int)    { s.ends++; s.in += in; s.out += out }
func (s *recordingSink) ToolStart(name string)   { s.toolStart = append(s.toolStart, name) }
func (s *recordingSink) ToolEnd(name string, ok bool) {
	s.toolEnd = append(s.toolEnd, name)
}

func respond(content string) model.Response {
	return model.Response{Message: model.Message{Role: model.RoleAssistant, Content: content},
		Usage: model.Usage{InputTokens: 10, OutputTokens: 5}}
}

func toolCallResponse(id, name string, args json.RawMessage) model.Response {
	return model.Response{Message: model.Message{
		Role: model.RoleAssistant,
		ToolCalls: []model.ToolCall{
			{ID: id, Name: name, Args: args},
		},
	}, Usage: model.Usage{InputTokens: 10, OutputTokens: 5}}
}

// stubTool returns one scripted non-HTTP tool.
func stubTool(name, result string) tool.Tool[Deps] {
	return tool.MustNew(tool.Tool[Deps]{
		Name:        name,
		Description: "stub",
		Schema:      json.RawMessage(`{"type":"object"}`),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (string, error) {
			return result, nil
		},
	})
}

// deferringTool defers for approval, then acts on the approved re-run.
func deferringTool(name string, onApproved func() string) tool.Tool[Deps] {
	return tool.MustNew(tool.Tool[Deps]{
		Name:        name,
		Description: "stub",
		Schema:      json.RawMessage(`{"type":"object"}`),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (string, error) {
			if !tool.CallApproved(ctx) {
				return "", &tool.Deferred{Kind: tool.DeferApproval, Reason: "needs sign-off"}
			}
			return onApproved(), nil
		},
	})
}

// testSpec is the RunSpec every chat-package test runs under: the fake
// registry serves the scripted model for any catalog model ID.
var testSpec = RunSpec{Model: "gemini-2.5-flash"}

func newTestAgent(m *testmodel.Scripted) *Agent {
	reg := NewModelRegistryWithFactory(ProviderKeys{Gemini: "test"}, func(string, string, *float64) (model.StreamingModel, error) {
		return m, nil
	})
	a, err := New(reg, golem.UsageLimit{}, DefaultToolEnv())
	if err != nil {
		panic(err)
	}
	return a
}

func TestRunStreamsAndCounts(t *testing.T) {
	m := testmodel.New().Respond(respond("hello "))
	a := newTestAgent(m)
	sink := &recordingSink{}
	out, err := a.Run(context.Background(), Deps{UserID: "u", ConversationID: "c"}, nil, "hi", nil, sink, nil, testSpec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Output != "hello " || out.Usage.InputTokens != 10 || out.Usage.OutputTokens != 5 {
		t.Fatalf("outcome: %+v", out)
	}
	if len(sink.deltas) == 0 || sink.ends != 1 || sink.in != 10 {
		t.Fatalf("sink not fed: %+v", sink)
	}
	if out.Requests != 1 {
		t.Fatalf("requests = %d, want 1", out.Requests)
	}
}

func TestUsageLimitSurfacesStage(t *testing.T) {
	m := testmodel.New().Respond(respond("big"))
	reg := NewModelRegistryWithFactory(ProviderKeys{Gemini: "test"}, func(string, string, *float64) (model.StreamingModel, error) {
		return m, nil
	})
	a, _ := New(reg, golem.UsageLimit{TotalTokens: 3}, DefaultToolEnv())
	sink := &recordingSink{}
	_, err := a.Run(context.Background(), Deps{}, nil, "hi", nil, sink, nil, testSpec)
	var runErr *golem.RunError
	if !errors.As(err, &runErr) || runErr.Stage != golem.StageUsage {
		t.Fatalf("expected usage-stage RunError, got %v", err)
	}
}

func TestRunExecutesToolCalls(t *testing.T) {
	m := testmodel.New().
		Respond(toolCallResponse("call-1", "get_temp", json.RawMessage(`{"city":"x"}`))).
		Respond(respond("done"))
	a := newTestAgent(m)
	sink := &recordingSink{}
	out, err := a.Run(context.Background(), Deps{}, nil, "temp?", nil, sink,
		[]tool.Tool[Deps]{stubTool("get_temp", "21C")}, testSpec)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Output != "done" {
		t.Fatalf("output = %q", out.Output)
	}
	if len(sink.toolStart) != 1 || sink.toolStart[0] != "get_temp" || len(sink.toolEnd) != 1 {
		t.Fatalf("tool events: %+v", sink)
	}
	roles := []model.Role{}
	for _, msg := range out.Messages {
		roles = append(roles, msg.Role)
	}
	n := len(roles)
	if n < 3 || roles[n-3] != model.RoleAssistant || roles[n-2] != model.RoleTool || roles[n-1] != model.RoleAssistant {
		t.Fatalf("message roles = %v", roles)
	}
	result := out.Messages[n-2]
	if result.Content != "21C" || result.ToolCallID != "call-1" {
		t.Fatalf("tool result message = %+v", result)
	}
	if out.Requests != 2 {
		t.Fatalf("requests = %d, want 2", out.Requests)
	}
}

func TestRunPausesOnApproval(t *testing.T) {
	// Only one model response: a pausing run makes no further model call.
	m := testmodel.New().
		Respond(toolCallResponse("call-1", "risky", json.RawMessage(`{}`)))
	a := newTestAgent(m)
	sink := &recordingSink{}
	out, err := a.Run(context.Background(), Deps{}, nil, "go", nil, sink,
		[]tool.Tool[Deps]{deferringTool("risky", func() string { return "acted" })}, testSpec)
	if err != nil {
		t.Fatalf("paused run must succeed: %v", err)
	}
	if out.Output != "" {
		t.Fatalf("paused output = %q", out.Output)
	}
	if len(out.Pending) != 1 {
		t.Fatalf("pending = %+v", out.Pending)
	}
	p := out.Pending[0]
	if p.CallID != "call-1" || p.ToolName != "risky" || p.Reason != "needs sign-off" {
		t.Fatalf("pending call = %+v", p)
	}
}

func TestRunDeferredResumes(t *testing.T) {
	m := testmodel.New().Respond(respond("acted safely"))
	a := newTestAgent(m)
	sink := &recordingSink{}

	// The paused run's messages end with the unanswered tool call.
	paused := []model.Message{
		{Role: model.RoleUser, Content: "go"},
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call-1", Name: "risky", Args: json.RawMessage(`{}`)}}},
	}
	out, err := a.RunDeferred(context.Background(), Deps{}, paused,
		golem.DeferredResults{Approvals: map[string]golem.Approval{"call-1": {Approved: true}}},
		sink, []tool.Tool[Deps]{deferringTool("risky", func() string { return "acted" })}, testSpec)
	if err != nil {
		t.Fatalf("RunDeferred: %v", err)
	}
	if out.Output != "acted safely" || len(out.Pending) != 0 {
		t.Fatalf("outcome: %+v", out)
	}
	if len(sink.deltas) != 1 || sink.deltas[0] != "acted safely" {
		t.Fatalf("resume must emit the answer as one delta: %+v", sink.deltas)
	}
}

func TestRunDeferredDenial(t *testing.T) {
	m := testmodel.New().Respond(respond("understood, standing down"))
	a := newTestAgent(m)
	sink := &recordingSink{}
	paused := []model.Message{
		{Role: model.RoleUser, Content: "go"},
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call-1", Name: "risky", Args: json.RawMessage(`{}`)}}},
	}
	out, err := a.RunDeferred(context.Background(), Deps{}, paused,
		golem.DeferredResults{Approvals: map[string]golem.Approval{"call-1": {Approved: false, Reason: "user denied"}}},
		sink, []tool.Tool[Deps]{deferringTool("risky", func() string { return "acted" })}, testSpec)
	if err != nil {
		t.Fatalf("RunDeferred: %v", err)
	}
	if out.Output != "understood, standing down" {
		t.Fatalf("output = %q", out.Output)
	}
	// The denial, not a tool action, is what the model saw.
	for _, msg := range out.Messages {
		if msg.Role == model.RoleTool && msg.ToolCallID == "call-1" && msg.Content == "acted" {
			t.Fatal("denied call executed its side effect")
		}
	}
}

func TestRunUsesSpecSystemPrompt(t *testing.T) {
	var got model.Request
	m := testmodel.StreamFunc(func(_ context.Context, req model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		got = req
		_ = testmodel.Emit(onDelta, model.Delta{Content: "ok"})
		return respond("ok"), nil
	})
	reg := NewModelRegistryWithFactory(ProviderKeys{Gemini: "test"}, func(string, string, *float64) (model.StreamingModel, error) {
		return m, nil
	})
	a, _ := New(reg, golem.UsageLimit{}, DefaultToolEnv())
	const pirate = "Answer like a pirate."
	sink := &recordingSink{}
	if _, err := a.Run(context.Background(), Deps{}, nil, "hi", nil, sink, nil, RunSpec{Model: "gemini-2.5-flash", SystemPrompt: pirate}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Messages) == 0 || got.Messages[0].Role != model.RoleSystem || got.Messages[0].Content != pirate {
		t.Fatalf("custom system prompt not applied: %+v", got.Messages)
	}
}
