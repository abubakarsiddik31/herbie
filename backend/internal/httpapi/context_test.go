package httpapi

import (
	"context"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
	"github.com/abubakarsiddik31/golem/tokens"
)

func contextTestServer(t *testing.T, factory chat.CounterFactory) *Server {
	t.Helper()
	reg := chat.NewModelRegistryWithCounterFactory(chat.ProviderKeys{Gemini: "test"}, factory)
	agent, err := chat.New(reg, golem.UsageLimit{}, chat.DefaultToolEnv())
	if err != nil {
		t.Fatal(err)
	}
	return &Server{deps: ServerDeps{Agent: agent}}
}

func TestContextBreakdownExact(t *testing.T) {
	srv := contextTestServer(t, func(string, string) (tokens.Counter, error) {
		return testmodel.CountFunc(func(_ context.Context, in tokens.CountInput) (int, error) {
			if len(in.Messages) != 3 {
				t.Fatalf("counter saw %d messages, want system+history+prompt", len(in.Messages))
			}
			return 1234, nil
		}), nil
	})
	history := []model.Message{{Role: model.RoleUser, Content: "hello"}}
	bd := srv.contextBreakdown(context.Background(), "u-1",
		chat.RunSpec{Model: "gemini-3.5-flash", SystemPrompt: "system"},
		history, "next", nil, nil)
	if !bd.Exact || bd.TotalTokens != 1234 {
		t.Fatalf("breakdown = %+v", bd)
	}
	if bd.SystemTokens == 0 || bd.PromptTokens == 0 || bd.HistoryTokens == 0 {
		t.Fatalf("components missing: %+v", bd)
	}
}

func TestContextBreakdownTrimsHistoryLikeTheRun(t *testing.T) {
	saw := 0
	srv := contextTestServer(t, func(string, string) (tokens.Counter, error) {
		return testmodel.CountFunc(func(_ context.Context, in tokens.CountInput) (int, error) {
			saw = len(in.Messages)
			return 1, nil
		}), nil
	})
	history := make([]model.Message, 50)
	for i := range history {
		history[i] = model.Message{Role: model.RoleUser, Content: "hello"}
	}
	bd := srv.contextBreakdown(context.Background(), "u-1",
		chat.RunSpec{Model: "gemini-3.5-flash", SystemPrompt: "system"},
		history, "next", nil, nil)
	if !bd.Exact {
		t.Fatalf("expected exact count: %+v", bd)
	}
	if saw != 42 { // 40 trimmed + system + prompt
		t.Fatalf("counter saw %d messages, want 42", saw)
	}
}

func TestContextBreakdownCountsAttachmentsOnce(t *testing.T) {
	srv := contextTestServer(t, func(string, string) (tokens.Counter, error) {
		return nil, chat.ErrNoTokenCounter
	})
	history := []model.Message{{
		Role: model.RoleUser, Content: "hello",
		Parts: []model.Part{model.ImageData("image/png", make([]byte, 1<<20))},
	}}
	bd := srv.contextBreakdown(context.Background(), "u-1",
		chat.RunSpec{Model: "gemini-3.5-flash", SystemPrompt: "system"},
		history, "next", nil, nil)
	if bd.AttachmentTokens != 1000 {
		t.Fatalf("attachment tokens = %d", bd.AttachmentTokens)
	}
	if bd.TotalTokens > 5000 {
		t.Fatalf("attachment bytes were double counted: %+v", bd)
	}
}

func TestContextBreakdownFallsBackToEstimate(t *testing.T) {
	srv := contextTestServer(t, func(string, string) (tokens.Counter, error) {
		return nil, chat.ErrNoTokenCounter
	})
	history := []model.Message{{Role: model.RoleUser, Content: "hello"}}
	bd := srv.contextBreakdown(context.Background(), "u-1",
		chat.RunSpec{Model: "gemini-3.5-flash", SystemPrompt: "system"},
		history, "next", nil, nil)
	if bd.Exact {
		t.Fatalf("unexpected exact count: %+v", bd)
	}
	want := chat.EstimateTextTokens("system") + chat.EstimateHistoryTextTokens(history) + chat.EstimateTextTokens("next")
	if bd.TotalTokens != want {
		t.Fatalf("total = %d, want %d (%+v)", bd.TotalTokens, want, bd)
	}
}
