package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func scriptedPongModel() *testmodel.Scripted {
	return testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "pong"},
		Usage:   model.Usage{InputTokens: 3, OutputTokens: 2},
	})
}

func TestListModelsFiltersByConfiguredKeys(t *testing.T) {
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), newFakeConvos(newFakeMsgs()), newFakeMsgs(), newFakeUsage())

	req := reqJSON(http.MethodGet, "/api/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Default string `json:"default"`
		Models  []struct {
			ID       string `json:"id"`
			Label    string `json:"label"`
			Provider string `json:"provider"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Default != "gemini-3.5-flash" || len(got.Models) != 6 || got.Models[0].ID != "gemini-3.5-flash" {
		t.Fatalf("gemini-only keys should list gemini models: %+v", got)
	}
}

func TestPatchConversationSettings(t *testing.T) {
	agent := newTestAgent(t, scriptedPongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())

	conv := convs.mustCreate("u-1", "")
	temp := 0.5
	req := reqJSON(http.MethodPatch, "/api/conversations/"+conv.ID, map[string]any{
		"model": "gemini-2.5-pro", "temperature": temp, "systemPrompt": "Answer like a pirate.",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	stored := convs.convs[conv.ID]
	if stored.Model != "gemini-2.5-pro" || stored.Temperature == nil || *stored.Temperature != temp || stored.SystemPrompt != "Answer like a pirate." {
		t.Fatalf("settings not stored: %+v", stored)
	}
}

func TestPatchConversationSettingsValidation(t *testing.T) {
	agent := newTestAgent(t, scriptedPongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"unknown model", map[string]any{"model": "gpt-99"}},
		{"unavailable provider", map[string]any{"model": "gpt-5"}}, // only gemini key configured
		{"temperature too high", map[string]any{"temperature": 2.5}},
		{"temperature negative", map[string]any{"temperature": -0.1}},
		{"system prompt too long", map[string]any{"systemPrompt": strings.Repeat("x", 4001)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := reqJSON(http.MethodPatch, "/api/conversations/"+conv.ID, tc.body)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
	if convs.convs[conv.ID].Model != "" {
		t.Fatalf("rejected settings must not persist: %+v", convs.convs[conv.ID])
	}
}

func TestPatchClearTemperature(t *testing.T) {
	agent := newTestAgent(t, scriptedPongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	temp := 0.7
	if err := convs.SetSettings(nil, conv.ID, "u-1", storage.ConversationPatch{Temperature: &temp}); err != nil {
		t.Fatal(err)
	}
	req := reqJSON(http.MethodPatch, "/api/conversations/"+conv.ID, map[string]any{"clearTemperature": true})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if convs.convs[conv.ID].Temperature != nil {
		t.Fatalf("temperature should be cleared: %+v", convs.convs[conv.ID])
	}
}

func TestCreateConversationWithSettings(t *testing.T) {
	agent := newTestAgent(t, scriptedPongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())

	req := reqJSON(http.MethodPost, "/api/conversations", map[string]any{
		"model": "gemini-2.5-pro", "systemPrompt": "be terse",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	for _, conv := range convs.convs {
		if conv.Model == "gemini-2.5-pro" {
			if conv.SystemPrompt != "be terse" || conv.Temperature != nil {
				t.Fatalf("create settings partially stored: %+v", conv)
			}
			return
		}
	}
	t.Fatalf("no conversation created with model: %+v", convs.convs)
}

func TestSendMessageUsesConversationModelAndSystemPrompt(t *testing.T) {
	agent := newTestAgent(t, scriptedPongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	h, token := newHandlerServer(t, agent, convs, msgs, usage)

	conv := convs.mustCreate("u-1", "")
	convs.convs[conv.ID] = mustMerge(t, convs.convs[conv.ID], "gemini-2.5-pro", "answer in rhyme")

	rec := postMessage(t, h, token, conv.ID, "hello")
	for _, want := range []string{"event: done", `"model":"gemini-2.5-pro"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("missing %q in SSE:\n%s", want, rec.Body.String())
		}
	}
	if len(usage.events) != 1 || usage.events[0].Model != "gemini-2.5-pro" {
		t.Fatalf("usage event model wrong: %+v", usage.events)
	}
	// The assistant row records the answering model.
	last := msgs.forConv(conv.ID, "u-1")[1]
	if last.Model != "gemini-2.5-pro" {
		t.Fatalf("assistant row model = %q", last.Model)
	}
}

func mustMerge(t *testing.T, conv storage.Conversation, modelID, sysPrompt string) storage.Conversation {
	t.Helper()
	conv.Model = modelID
	conv.SystemPrompt = sysPrompt
	return conv
}

func TestGetConversationSurfacesUsage(t *testing.T) {
	agent := newTestAgent(t, scriptedPongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	msgs.rows = append(msgs.rows,
		storage.Message{ID: "m-1", ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "hi", Data: []byte(`{"role":"user","content":"hi"}`)},
		storage.Message{ID: "m-2", ConversationID: conv.ID, UserID: "u-1", Role: "assistant", Content: "ho",
			Data: []byte(`{"role":"assistant","content":"ho"}`), InputTokens: 1200, OutputTokens: 300, CostMicros: 8460, Model: "gemini-2.5-flash"},
	)

	req := reqJSON(http.MethodGet, "/api/conversations/"+conv.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got struct {
		Messages []struct {
			ID    string `json:"id"`
			Usage *struct {
				InputTokens  int     `json:"inputTokens"`
				OutputTokens int     `json:"outputTokens"`
				CostUsd      float64 `json:"costUsd"`
				Model        string  `json:"model"`
			} `json:"usage"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var assistantUsage *struct {
		InputTokens  int     `json:"inputTokens"`
		OutputTokens int     `json:"outputTokens"`
		CostUsd      float64 `json:"costUsd"`
		Model        string  `json:"model"`
	}
	for _, m := range got.Messages {
		if m.ID == "m-2" {
			assistantUsage = m.Usage
		}
	}
	if assistantUsage == nil || assistantUsage.InputTokens != 1200 || assistantUsage.OutputTokens != 300 ||
		assistantUsage.Model != "gemini-2.5-flash" || assistantUsage.CostUsd != 0.00846 {
		t.Fatalf("assistant usage wrong: %+v", assistantUsage)
	}
}

// endlessModel answers every run with the same streamed content — the
// truncate-and-resend endpoints run the agent more than once per test.
func endlessModel(content string) testmodel.StreamFunc {
	return testmodel.StreamFunc(func(_ context.Context, _ model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		_ = testmodel.Emit(onDelta, model.Delta{Content: content})
		return model.Response{
			Message: model.Message{Role: model.RoleAssistant, Content: content},
			Usage:   model.Usage{InputTokens: 5, OutputTokens: 3},
		}, nil
	})
}

func newEndlessAgent(t *testing.T, content string) *chat.Agent {
	t.Helper()
	reg := chat.NewModelRegistryWithFactory(chat.ProviderKeys{Gemini: "test"}, func(string, string, *float64) (model.StreamingModel, error) {
		return endlessModel(content), nil
	})
	agent, err := chat.New(reg, golem.UsageLimit{}, chat.DefaultToolEnv())
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

func TestEditMessageTruncatesAndReruns(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "again"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	if rec := postMessage(t, h, token, conv.ID, "hello"); rec.Code != http.StatusOK {
		t.Fatalf("first send: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postMessage(t, h, token, conv.ID, "second"); rec.Code != http.StatusOK {
		t.Fatalf("second send: %d %s", rec.Code, rec.Body.String())
	}
	if msgs.count() != 4 {
		t.Fatalf("expected 4 rows before edit, got %d", msgs.count())
	}

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/messages/m-1/edit", map[string]string{"content": "rewritten"})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("edit must re-run to a done event:\n%s", rec.Body.String())
	}
	rows := msgs.forConv(conv.ID, "u-1")
	if len(rows) != 2 { // edited user row + fresh assistant row
		t.Fatalf("rows after edit = %d, want 2 (truncated then re-run)", len(rows))
	}
	if rows[0].Content != "rewritten" || rows[0].ID != "m-1" {
		t.Fatalf("edited row wrong: %+v", rows[0])
	}
	if rows[1].Role != "assistant" {
		t.Fatalf("last row should be the fresh assistant answer: %+v", rows[1])
	}
}

func TestEditMessageValidation(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "x"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	_ = postMessage(t, h, token, conv.ID, "hello")

	post := func(path string, body map[string]string) *httptest.ResponseRecorder {
		req := reqJSON(http.MethodPost, path, body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := post("/api/conversations/"+conv.ID+"/messages/m-2/edit", map[string]string{"content": "nope"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("editing an assistant row must 400, got %d", rec.Code)
	}
	if rec := post("/api/conversations/"+conv.ID+"/messages/m-99/edit", map[string]string{"content": "nope"}); rec.Code != http.StatusNotFound {
		t.Fatalf("editing an unknown row must 404, got %d", rec.Code)
	}
	if msgs.forConv(conv.ID, "u-1")[1].Content == "nope" {
		t.Fatal("assistant row must be untouched")
	}
}

func TestRegenerateReplacesLastAnswer(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "fresh"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	_ = postMessage(t, h, token, conv.ID, "hello")
	_ = postMessage(t, h, token, conv.ID, "again")
	if msgs.count() != 4 {
		t.Fatalf("expected 4 rows, got %d", msgs.count())
	}

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/regenerate", map[string]any{})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("regenerate status %d: %s", rec.Code, rec.Body.String())
	}
	rows := msgs.forConv(conv.ID, "u-1")
	if len(rows) != 4 { // both turns kept; only the last answer was replaced
		t.Fatalf("rows after regenerate = %d, want 4", len(rows))
	}
	if rows[2].Role != "user" || rows[2].Content != "again" {
		t.Fatalf("history truncated past the last user message: %+v", rows)
	}
	if rows[3].Role != "assistant" {
		t.Fatalf("last row should be a fresh assistant answer: %+v", rows[3])
	}
}

func TestRegenerateEmptyConversationRejected(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "x"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/regenerate", map[string]any{})
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("regenerate on empty conversation must 400, got %d", rec.Code)
	}
}
