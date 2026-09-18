package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
