package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type searchCall struct {
	userID        string
	query         string
	limit, offset int
}

type fakeSearch struct {
	calls   []searchCall
	hits    []storage.SearchHit
	hasMore bool
	err     error
}

func (f *fakeSearch) Search(_ context.Context, userID, q string, limit, offset int) ([]storage.SearchHit, bool, error) {
	f.calls = append(f.calls, searchCall{userID: userID, query: q, limit: limit, offset: offset})
	return f.hits, f.hasMore, f.err
}

func newSearchServer(t *testing.T, search MessageSearchStore) (http.Handler, string) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(ServerDeps{
		Cfg:           config.Config{},
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:          authtest.NewService(testSecret),
		Tokens:        tm,
		MessageSearch: search,
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token
}

func searchRequest(t *testing.T, h http.Handler, token, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/search/messages"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSearchMessages(t *testing.T) {
	fake := &fakeSearch{
		hits: []storage.SearchHit{
			{MessageID: "m-1", ConversationID: "c-1", Title: "Deploy", Role: "assistant",
				Snippet: "use " + storage.SnippetHitStart + "deploy" + storage.SnippetHitStop, CreatedAt: time.Now()},
			{MessageID: "m-2", ConversationID: "c-2", Title: "Other", Role: "user",
				Snippet: "another", CreatedAt: time.Now()},
		},
		hasMore: true,
	}
	h, token := newSearchServer(t, fake)

	rec := searchRequest(t, h, token, "?q=deploy&limit=2&offset=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("search: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Hits       []searchHitDTO `json:"hits"`
		NextOffset *int           `json:"nextOffset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Hits) != 2 || body.Hits[0].MessageID != "m-1" || body.Hits[0].ConversationID != "c-1" {
		t.Fatalf("hits = %+v", body.Hits)
	}
	if body.NextOffset == nil || *body.NextOffset != 3 {
		t.Fatalf("nextOffset = %v", body.NextOffset)
	}
	if len(fake.calls) != 1 || fake.calls[0] != (searchCall{userID: "u-1", query: "deploy", limit: 2, offset: 1}) {
		t.Fatalf("store call = %+v", fake.calls)
	}

	// Last page: no next offset.
	fake.hasMore = false
	rec = searchRequest(t, h, token, "?q=deploy")
	if rec.Code != http.StatusOK || !jsonHasNull(rec.Body.Bytes(), "nextOffset") {
		t.Fatalf("last page = %d %s", rec.Code, rec.Body.String())
	}
	if fake.calls[len(fake.calls)-1].limit != defaultSearchLimit {
		t.Fatalf("default limit = %d", fake.calls[len(fake.calls)-1].limit)
	}

	// Limits clamp to the maximum.
	searchRequest(t, h, token, "?q=deploy&limit=999")
	if got := fake.calls[len(fake.calls)-1].limit; got != maxSearchLimit {
		t.Fatalf("clamped limit = %d", got)
	}
}

func jsonHasNull(raw []byte, key string) bool {
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	return string(v[key]) == "null"
}

func TestSearchMessagesValidation(t *testing.T) {
	fake := &fakeSearch{}
	h, token := newSearchServer(t, fake)

	for _, query := range []string{"", "?q=", "?q=deploy&limit=0", "?q=deploy&limit=x", "?q=deploy&offset=-1"} {
		if rec := searchRequest(t, h, token, query); rec.Code != http.StatusBadRequest {
			t.Fatalf("%q = %d %s", query, rec.Code, rec.Body.String())
		}
	}
	if len(fake.calls) != 0 {
		t.Fatalf("store called for invalid input: %+v", fake.calls)
	}

	fake.err = errors.New("boom")
	if rec := searchRequest(t, h, token, "?q=deploy"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("store error = %d", rec.Code)
	}
}
