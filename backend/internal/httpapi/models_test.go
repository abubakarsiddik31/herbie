package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	if got.Default != "gemini-2.5-flash" || len(got.Models) != 2 || got.Models[0].ID != "gemini-2.5-flash" {
		t.Fatalf("gemini-only keys should list two gemini models: %+v", got)
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
