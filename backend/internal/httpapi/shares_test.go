package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type fakeShares struct {
	convs  *fakeConvos
	byConv map[string]string
	byHash map[string]string
}

func newFakeShares(convs *fakeConvos) *fakeShares {
	return &fakeShares{convs: convs, byConv: map[string]string{}, byHash: map[string]string{}}
}

var _ ShareStore = (*fakeShares)(nil)

func (f *fakeShares) owner(convID, userID string) bool {
	conv, ok := f.convs.convs[convID]
	return ok && conv.UserID == userID
}

func (f *fakeShares) Share(_ context.Context, convID, userID, hash string) (string, bool, error) {
	if !f.owner(convID, userID) {
		return "", false, storage.ErrNotFound
	}
	if existing, ok := f.byConv[convID]; ok {
		return existing, false, nil
	}
	f.byConv[convID] = hash
	f.byHash[hash] = convID
	return hash, true, nil
}

func (f *fakeShares) Unshare(_ context.Context, convID, userID string) error {
	if !f.owner(convID, userID) {
		return storage.ErrNotFound
	}
	hash, ok := f.byConv[convID]
	if !ok {
		return storage.ErrNotFound
	}
	delete(f.byConv, convID)
	delete(f.byHash, hash)
	return nil
}

func (f *fakeShares) SharedHash(_ context.Context, convID, userID string) (string, error) {
	if !f.owner(convID, userID) {
		return "", storage.ErrNotFound
	}
	return f.byConv[convID], nil
}

func (f *fakeShares) Resolve(_ context.Context, hash string) (string, string, error) {
	convID, ok := f.byHash[hash]
	if !ok {
		return "", "", storage.ErrNotFound
	}
	return convID, f.convs.convs[convID].UserID, nil
}

func newShareTestServer(t *testing.T) (http.Handler, string, *fakeConvos, *fakeMsgs) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h := NewServer(ServerDeps{
		Cfg:      config.Config{},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:     authtest.NewService(testSecret),
		Tokens:   tm,
		Profiles: newFakeProfiles(),
		Convos:   convs,
		Msgs:     msgs,
		Shares:   newFakeShares(convs),
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token, convs, msgs
}

func authedReq(method, path, token string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestShareUnsharePublicLoop(t *testing.T) {
	h, token, convs, msgs := newShareTestServer(t)
	conv := convs.mustCreate("u-1", "Shared thread")
	_ = msgs.Add(context.Background(), storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "hi"})
	_ = msgs.Add(context.Background(), storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "assistant", Content: "hello", InputTokens: 3, OutputTokens: 5, CostMicros: 100, Model: "m"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/conversations/"+conv.ID+"/shares", token))
	if rec.Code != http.StatusCreated {
		t.Fatalf("share: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.Token == "" {
		t.Fatal("share must return the raw token once")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/shared/"+created.Token, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public thread: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"title":"Shared thread"`) || !strings.Contains(body, `"content":"hello"`) {
		t.Fatalf("public thread body: %s", body)
	}
	for _, leaked := range []string{"usage", "costUsd", "systemPrompt", "temperature"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("public thread leaks %q: %s", leaked, body)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/conversations/"+conv.ID+"/shares", token))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"shared":true`) {
		t.Fatalf("reshare: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/conversations/"+conv.ID+"/shares", token))
	if !strings.Contains(rec.Body.String(), `"shared":true`) {
		t.Fatalf("state: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodDelete, "/api/conversations/"+conv.ID+"/shares", token))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unshare: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/shared/"+created.Token, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revoked link should 404, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/conversations/"+conv.ID+"/shares", token))
	if !strings.Contains(rec.Body.String(), `"shared":false`) {
		t.Fatalf("state after revoke: %s", rec.Body.String())
	}
}

func TestShareRejects(t *testing.T) {
	h, token, _, _ := newShareTestServer(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/conversations/c-nope/shares", token))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown conversation: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/shared/bogus", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bogus token: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/conversations/c-nope/shares", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("authed route without token: %d", rec.Code)
	}
}
