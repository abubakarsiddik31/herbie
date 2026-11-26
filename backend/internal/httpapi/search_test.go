package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/abubakarsiddik31/golem/embedding"
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

type fakeEmbedder struct {
	query embedding.Result
	docs  embedding.Result
	err   error
}

func (f *fakeEmbedder) EmbedQuery(context.Context, string) (embedding.Result, error) {
	return f.query, f.err
}
func (f *fakeEmbedder) EmbedDocuments(context.Context, []string) (embedding.Result, error) {
	return f.docs, f.err
}

func newSemanticSearchServer(t *testing.T, search MessageSearchStore, emb MessageEmbedder) (http.Handler, string) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(ServerDeps{
		Cfg:              config.Config{},
		Log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:             authtest.NewService(testSecret),
		Tokens:           tm,
		MessageSearch:    search,
		SemanticEmbedder: emb,
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token
}

func TestSemanticSearchReranksCandidates(t *testing.T) {
	fake := &fakeSearch{hits: []storage.SearchHit{
		{MessageID: "m-1", ConversationID: "c-1", Title: "One", Role: "assistant", Content: "not relevant", CreatedAt: time.Now()},
		{MessageID: "m-2", ConversationID: "c-2", Title: "Two", Role: "user", Content: "capital of France", CreatedAt: time.Now()},
	}}
	emb := &fakeEmbedder{
		query: embedding.Result{Vectors: [][]float32{{1, 0}}},
		docs:  embedding.Result{Vectors: [][]float32{{0, 1}, {1, 0}}},
	}
	h, token := newSemanticSearchServer(t, fake, emb)
	rec := searchRequest(t, h, token, "?q=capital&semantic=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Semantic bool `json:"semantic"`
		Hits     []struct {
			MessageID string   `json:"messageId"`
			Score     *float64 `json:"score"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Semantic || len(body.Hits) != 2 || body.Hits[0].MessageID != "m-2" {
		t.Fatalf("reranked: %+v", body)
	}
	if body.Hits[0].Score == nil || *body.Hits[0].Score <= 0 {
		t.Fatalf("score missing: %+v", body.Hits[0])
	}
}

func TestSemanticSearchFallsBackOnEmbedderError(t *testing.T) {
	fake := &fakeSearch{hits: []storage.SearchHit{
		{MessageID: "m-1", ConversationID: "c-1", Title: "One", Role: "assistant", Snippet: "first", CreatedAt: time.Now()},
		{MessageID: "m-2", ConversationID: "c-2", Title: "Two", Role: "user", Snippet: "second", CreatedAt: time.Now()},
	}}
	h, token := newSemanticSearchServer(t, fake, &fakeEmbedder{err: errors.New("boom")})
	rec := searchRequest(t, h, token, "?q=capital&semantic=1")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"semantic":true`) {
		t.Fatalf("fallback: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCosine(t *testing.T) {
	if got := cosine([]float32{1, 0}, []float32{1, 0}); got != 1 {
		t.Fatalf("cosine = %v", got)
	}
	if got := cosine([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Fatalf("orthogonal = %v", got)
	}
}
