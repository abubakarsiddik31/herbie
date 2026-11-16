package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

func seedThread(msgs *fakeMsgs, convID, userID string, base time.Time, roles ...string) []string {
	var ids []string
	prev := ""
	for i, role := range roles {
		id, _ := msgs.Add(context.Background(), storage.Message{
			ConversationID: convID, UserID: userID, Role: role, ParentID: prev,
			Content: role + "-content", CreatedAt: base.Add(time.Duration(i) * time.Second),
		})
		ids = append(ids, id)
		prev = id
	}
	return ids
}

func TestDeleteMessageDropsSubtreeAndFallsBackToParent(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	other := convs.mustCreate("u-2", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	ids := seedThread(msgs, conv.ID, "u-1", base, "user", "assistant", "user", "assistant")
	seedThread(msgs, other.ID, "u-2", base, "user", "assistant")
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", ids[3]); err != nil {
		t.Fatal(err)
	}

	mine := msgs.forConv(conv.ID, "u-1")
	if len(mine) != 4 {
		t.Fatalf("seeded %d rows, want 4", len(mine))
	}

	// Deleting the active question drops its subtree; the leaf falls back to
	// the deleted root's parent.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete,
		"/api/conversations/"+conv.ID+"/messages/"+ids[2], token))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	remaining := msgs.forConv(conv.ID, "u-1")
	if len(remaining) != 2 || remaining[0].ID != ids[0] || remaining[1].ID != ids[1] {
		t.Fatalf("deleting a question must drop its subtree only: %+v", remaining)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != ids[1] {
		t.Fatalf("leaf did not fall back to the parent: %v", leaf)
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
		"/api/conversations/"+conv.ID+"/messages/"+ids[0], nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete: %d", rec.Code)
	}
}
