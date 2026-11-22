package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

func TestProfileGetAndPatch(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/profile", token))
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		DefaultInstructions string `json:"defaultInstructions"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.DefaultInstructions != "" {
		t.Fatalf("fresh profile should be empty: %+v", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSONAuthed(http.MethodPatch, "/api/profile", token,
		map[string]string{"defaultInstructions": "Always be concise."}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/profile", token))
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.DefaultInstructions != "Always be concise." {
		t.Fatalf("round trip: %+v", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSONAuthed(http.MethodPatch, "/api/profile", token,
		map[string]string{"defaultInstructions": strings.Repeat("x", 4001)}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversize instructions: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSONAuthed(http.MethodPatch, "/api/profile", token, map[string]string{}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing field: %d", rec.Code)
	}
}

func reqJSONAuthed(method, path, token string, body any) *http.Request {
	req := reqJSON(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestRunSpecPrependsGlobalInstructions(t *testing.T) {
	profiles := newFakeProfiles()
	profiles.texts["u-1"] = "Always be concise."
	s := &Server{deps: ServerDeps{Profiles: profiles}}

	conv := storage.Conversation{UserID: "u-1", Model: "gemini-2.5-flash", SystemPrompt: "Speak like a pirate."}
	spec := s.runSpecFor(context.Background(), conv)
	want := "[User preferences]\nAlways be concise.\n\nSpeak like a pirate."
	if spec.SystemPrompt != want {
		t.Fatalf("system prompt:\n%q\nwant:\n%q", spec.SystemPrompt, want)
	}

	plain := s.runSpecFor(context.Background(), storage.Conversation{UserID: "u-9", Model: "m", SystemPrompt: "Hi."})
	if plain.SystemPrompt != "Hi." {
		t.Fatalf("unset instructions must leave the prompt alone: %q", plain.SystemPrompt)
	}
}

func TestRunSpecGenerationDefaultsPrecedence(t *testing.T) {
	profiles := newFakeProfiles()
	top := 0.9
	profiles.defaults["u-1"] = storage.GenerationDefaults{TopP: &top, MaxOutputTokens: 1000, Reasoning: "low"}
	projects := newFakeProjects()
	project, err := projects.Create(context.Background(), storage.Project{
		UserID: "u-1", Name: "project",
		DefaultTopP: floatPtr(0.8), DefaultMaxOutputTokens: 2000, DefaultReasoning: "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	bots := newFakeBots()
	bot, err := bots.Create(context.Background(), storage.Bot{
		UserID: "u-1", Name: "Bot", Model: "gemini-2.5-flash",
		TopP: floatPtr(0.7), MaxOutputTokens: 4000, Reasoning: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{deps: ServerDeps{Profiles: profiles, Projects: projects, Bots: bots, ModelKeys: chat.ProviderKeys{Gemini: "test"}}}

	projectConv := storage.Conversation{UserID: "u-1", ProjectID: &project.ID}
	spec := s.runSpecFor(context.Background(), projectConv)
	if spec.TopP == nil || *spec.TopP != 0.8 || spec.MaxOutputTokens != 2000 || spec.Reasoning != "medium" {
		t.Fatalf("project defaults did not win over user defaults: %+v", spec)
	}

	conversationTop := 0.5
	projectConv.TopP = &conversationTop
	projectConv.MaxOutputTokens = 300
	projectConv.Reasoning = "high"
	spec = s.runSpecFor(context.Background(), projectConv)
	if spec.TopP == nil || *spec.TopP != 0.5 || spec.MaxOutputTokens != 300 || spec.Reasoning != "high" {
		t.Fatalf("conversation values did not win: %+v", spec)
	}

	botConv := storage.Conversation{UserID: "u-1", ProjectID: &project.ID, BotID: &bot.ID}
	spec = s.runSpecFor(context.Background(), botConv)
	if spec.TopP == nil || *spec.TopP != 0.7 || spec.MaxOutputTokens != 4000 || spec.Reasoning != "high" {
		t.Fatalf("bot values did not win: %+v", spec)
	}

	userOnly := storage.Conversation{UserID: "u-1"}
	spec = s.runSpecFor(context.Background(), userOnly)
	if spec.TopP == nil || *spec.TopP != 0.9 || spec.MaxOutputTokens != 1000 || spec.Reasoning != "low" {
		t.Fatalf("user defaults did not apply: %+v", spec)
	}
}

func floatPtr(v float64) *float64 { return &v }
