package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

func seedThread(msgs *fakeMsgs, convID, userID string, base time.Time, roles ...string) {
	for i, role := range roles {
		_ = msgs.Add(context.Background(), storage.Message{
			ConversationID: convID, UserID: userID, Role: role,
			Content: role + "-content", CreatedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
}

func TestDeleteMessageDropsRowAndEverythingAfter(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	other := convs.mustCreate("u-2", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	seedThread(msgs, conv.ID, "u-1", base, "user", "assistant", "user", "assistant")
	seedThread(msgs, other.ID, "u-2", base, "user", "assistant")

	mine := msgs.forConv(conv.ID, "u-1")
	if len(mine) != 4 {
		t.Fatalf("seeded %d rows, want 4", len(mine))
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete,
		"/api/conversations/"+conv.ID+"/messages/"+mine[2].ID, token))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	remaining := msgs.forConv(conv.ID, "u-1")
	if len(remaining) != 2 || remaining[0].ID != mine[0].ID || remaining[1].ID != mine[1].ID {
		t.Fatalf("deleting a question must drop it and the answer after it: %+v", remaining)
	}
	if len(msgs.forConv(other.ID, "u-2")) != 2 {
		t.Fatal("other user's thread must be untouched")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete,
		"/api/conversations/"+conv.ID+"/messages/m-999", token))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown message: %d", rec.Code)
	}

	theirs := msgs.forConv(other.ID, "u-2")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete,
		"/api/conversations/"+other.ID+"/messages/"+theirs[0].ID, token))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign message: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete,
		"/api/conversations/"+conv.ID+"/messages/"+mine[0].ID, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete: %d", rec.Code)
	}
}
