package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

func TestListConversationsSearchesByTitle(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	convs.mustCreate("u-1", "Moon landing plans")
	convs.mustCreate("u-1", "Grocery list")

	req := httptest.NewRequest(http.MethodGet, "/api/conversations?q=MOON", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("search: %d %s", rec.Code, rec.Body.String())
	}
	var got []storage.Conversation
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Title != "Moon landing plans" {
		t.Fatalf("q must filter titles server-side: %+v", got)
	}
}

func TestGetConversationIncludesContextMetadata(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "Test Chat")
	_ = msgs.Add(nil, storage.Message{
		ConversationID: conv.ID,
		UserID:         "u-1",
		Role:           "user",
		Content:        "Hello world",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/conversations/"+conv.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Conversation map[string]any `json:"conversation"`
		Messages     []any          `json:"messages"`
		Context      struct {
			EstimatedTokens int  `json:"estimatedTokens"`
			ThresholdTokens int  `json:"thresholdTokens"`
			KeepRecent      int  `json:"keepRecent"`
			Compacted       bool `json:"compacted"`
		} `json:"context"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Context.EstimatedTokens <= 0 {
		t.Fatalf("expected estimatedTokens > 0, got %d", got.Context.EstimatedTokens)
	}
	if got.Context.ThresholdTokens <= 0 {
		t.Fatalf("expected thresholdTokens > 0, got %d", got.Context.ThresholdTokens)
	}
	if got.Context.KeepRecent <= 0 {
		t.Fatalf("expected keepRecent > 0, got %d", got.Context.KeepRecent)
	}
}
