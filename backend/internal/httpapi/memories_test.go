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

func TestMemoriesCRUD(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())

	// Initially empty list
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/memories", token))
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var list []storage.Memory
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}

	// Create memory
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSONAuthed(http.MethodPost, "/api/memories", token,
		map[string]string{"content": "Prefers dark mode"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: %d %s", rec.Code, rec.Body.String())
	}
	var created storage.Memory
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Content != "Prefers dark mode" || created.ID == "" {
		t.Fatalf("unexpected created memory: %+v", created)
	}

	// List shows the created memory
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/memories", token))
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Content != "Prefers dark mode" {
		t.Fatalf("expected 1 item, got %+v", list)
	}

	// Delete specific memory
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete, "/api/memories/"+created.ID, token))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}

	// List empty again
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/memories", token))
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}

	// Create 2 memories
	h.ServeHTTP(httptest.NewRecorder(), reqJSONAuthed(http.MethodPost, "/api/memories", token,
		map[string]string{"content": "Fact 1"}))
	h.ServeHTTP(httptest.NewRecorder(), reqJSONAuthed(http.MethodPost, "/api/memories", token,
		map[string]string{"content": "Fact 2"}))

	// Clear all memories
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete, "/api/memories", token))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("clear: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/memories", token))
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Fatalf("expected empty after clear, got %d", len(list))
	}
}

func TestMemoriesValidation(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())

	// Empty content
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSONAuthed(http.MethodPost, "/api/memories", token, map[string]string{"content": ""}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty content, got %d", rec.Code)
	}

	// Content too long
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSONAuthed(http.MethodPost, "/api/memories", token,
		map[string]string{"content": strings.Repeat("a", 1001)}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized content, got %d", rec.Code)
	}
}

func TestRunSpecInjectsMemories(t *testing.T) {
	mems := newFakeMemories()
	_, _ = mems.Create(context.Background(), "u-1", "Lives in New York")
	_, _ = mems.Create(context.Background(), "u-1", "Uses Go and TypeScript")

	profiles := newFakeProfiles()
	profiles.texts["u-1"] = "Be concise."

	s := &Server{deps: ServerDeps{
		Memories: mems,
		Profiles: profiles,
	}}

	conv := storage.Conversation{UserID: "u-1", Model: "gemini-2.5-flash", SystemPrompt: "Answer as a pirate."}
	spec := s.runSpecFor(context.Background(), conv)

	if !strings.Contains(spec.SystemPrompt, "[User memory]") {
		t.Fatalf("expected [User memory] in system prompt: %s", spec.SystemPrompt)
	}
	if !strings.Contains(spec.SystemPrompt, "Lives in New York") {
		t.Fatalf("expected memory fact in system prompt: %s", spec.SystemPrompt)
	}
	if !strings.Contains(spec.SystemPrompt, "[User preferences]\nBe concise.") {
		t.Fatalf("expected user preferences in system prompt: %s", spec.SystemPrompt)
	}
	if !strings.Contains(spec.SystemPrompt, "Answer as a pirate.") {
		t.Fatalf("expected conv prompt in system prompt: %s", spec.SystemPrompt)
	}
}
