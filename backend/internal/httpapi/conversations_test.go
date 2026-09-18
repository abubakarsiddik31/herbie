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
