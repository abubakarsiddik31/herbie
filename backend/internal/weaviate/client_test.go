package weaviate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
)

// recorder captures method+path+body per request and replies from a queue.
type recorder struct {
	t       *testing.T
	bodies  []string
	methods []string
	paths   []string
	reply   func(call int, w http.ResponseWriter, r *http.Request)
}

func (rec *recorder) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.bodies = append(rec.bodies, string(b))
		rec.methods = append(rec.methods, r.Method)
		rec.paths = append(rec.paths, r.URL.Path)
		rec.reply(len(rec.bodies), w, r)
	}))
}

func TestEnsureCollectionCreatesWhenMissing(t *testing.T) {
	rec := &recorder{t: t}
	rec.reply = func(call int, w http.ResponseWriter, _ *http.Request) {
		// call 1: GET schema → 404 (missing), call 2: POST schema → 200
		if call == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
	srv := rec.server()
	defer srv.Close()

	c := New(srv.URL, 768, srv.Client())
	if err := c.EnsureCollection(context.Background()); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if rec.methods[0] != "GET" || !strings.HasSuffix(rec.paths[0], "/v1/schema/DocumentChunk") {
		t.Fatalf("probe: %s %s", rec.methods[0], rec.paths[0])
	}
	if rec.methods[1] != "POST" || rec.paths[1] != "/v1/schema" {
		t.Fatalf("create: %s %s", rec.methods[1], rec.paths[1])
	}
	body := rec.bodies[1]
	for _, want := range []string{`"class":"DocumentChunk"`, `"vectorizer":"none"`, `"user_id"`, `"document_id"`, `"content"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("schema body missing %s: %s", want, body)
		}
	}
}

func TestEnsureCollectionNoopWhenExists(t *testing.T) {
	rec := &recorder{t: t}
	rec.reply = func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	srv := rec.server()
	defer srv.Close()

	c := New(srv.URL, 768, srv.Client())
	if err := c.EnsureCollection(context.Background()); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if len(rec.methods) != 1 || rec.methods[0] != "GET" {
		t.Fatalf("existing collection must not POST; calls: %v", rec.methods)
	}
}

func TestUpsertChunks(t *testing.T) {
	rec := &recorder{t: t}
	rec.reply = func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"objects":[{"result":{}},{"result":{}}]}`))
	}
	srv := rec.server()
	defer srv.Close()

	c := New(srv.URL, 4, srv.Client())
	chunks := []rag.Chunk{
		{DocumentID: "d1", DocTitle: "a.txt", Index: 0, Content: "one"},
		{DocumentID: "d1", DocTitle: "a.txt", Index: 1, Content: "two"},
	}
	vectors := [][]float32{{1, 0, 0, 0}, {0, 1, 0, 0}}
	if err := c.UpsertChunks(context.Background(), "u1", "d1", "a.txt", chunks, vectors); err != nil {
		t.Fatalf("UpsertChunks: %v", err)
	}
	var body struct {
		Objects []struct {
			Class  string         `json:"class"`
			ID     string         `json:"id"`
			Vector []float32      `json:"vector"`
			Props  map[string]any `json:"properties"`
		} `json:"objects"`
	}
	if err := json.Unmarshal([]byte(rec.bodies[0]), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Objects) != 2 {
		t.Fatalf("objects: %d", len(body.Objects))
	}
	o := body.Objects[0]
	if o.Class != "DocumentChunk" || len(o.Vector) != 4 {
		t.Fatalf("object 0: %+v", o)
	}
	if o.ID == body.Objects[1].ID {
		t.Fatal("chunk ids must be deterministic but distinct")
	}
	// deterministic: same id on a second run
	o2id := body.Objects[0].ID
	c2 := New(srv.URL, 4, srv.Client())
	_ = c2.UpsertChunks(context.Background(), "u1", "d1", "a.txt", chunks, vectors)
	var again struct {
		Objects []struct {
			ID string `json:"id"`
		} `json:"objects"`
	}
	_ = json.Unmarshal([]byte(rec.bodies[1]), &again)
	if again.Objects[0].ID != o2id {
		t.Fatalf("id not deterministic: %s vs %s", o2id, again.Objects[0].ID)
	}
	if o.Props["user_id"] != "u1" || o.Props["document_id"] != "d1" || o.Props["doc_title"] != "a.txt" {
		t.Fatalf("properties: %+v", o.Props)
	}
}

func TestUpsertChunksSurfacesObjectErrors(t *testing.T) {
	rec := &recorder{t: t}
	rec.reply = func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"objects":[{"result":{}},{"result":{"errors":{"error":[{"message":"vector width mismatch"}]}}}]}`))
	}
	srv := rec.server()
	defer srv.Close()

	c := New(srv.URL, 4, srv.Client())
	err := c.UpsertChunks(context.Background(), "u1", "d1", "a.txt",
		[]rag.Chunk{{Index: 0}, {Index: 1}}, [][]float32{{1}, {0}})
	if err == nil || !strings.Contains(err.Error(), "vector width mismatch") {
		t.Fatalf("want per-object error surfaced, got %v", err)
	}
}

func TestUpsertChunksLengthMismatch(t *testing.T) {
	c := New("http://unused", 4, nil)
	if err := c.UpsertChunks(context.Background(), "u", "d", "t",
		[]rag.Chunk{{Index: 0}}, nil); err == nil {
		t.Fatal("want mismatch error")
	}
}

func TestDeleteDocument(t *testing.T) {
	rec := &recorder{t: t}
	rec.reply = func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"results":{"matches":2}}`))
	}
	srv := rec.server()
	defer srv.Close()

	c := New(srv.URL, 768, srv.Client())
	if err := c.DeleteDocument(context.Background(), "d1"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	body := rec.bodies[0]
	for _, want := range []string{`"class":"DocumentChunk"`, `"document_id"`, `"Equal"`, `"d1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("delete body missing %s: %s", want, body)
		}
	}
}

func TestHybridSearch(t *testing.T) {
	rec := &recorder{t: t}
	rec.reply = func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"data":{"Get":{"DocumentChunk":[
			{"content":"alpha","doc_title":"T","document_id":"d1","chunk_index":0,"_additional":{"score":"0.87"}},
			{"content":"beta","doc_title":"T","document_id":"d1","chunk_index":1,"_additional":{"score":"0.10"}}
		]}}}`))
	}
	srv := rec.server()
	defer srv.Close()

	c := New(srv.URL, 768, srv.Client())
	got, err := c.HybridSearch(context.Background(), "u1", `say "hello" world`, 5)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(got) != 2 || got[0].Chunk.Content != "alpha" || got[0].Score != 0.87 || got[1].Score != 0.10 {
		t.Fatalf("rows: %+v", got)
	}
	gql := rec.bodies[0]
	for _, want := range []string{
		`hybrid:{query:"say \"hello\" world" alpha:0.5}`,
		`path:["user_id"] operator:Equal valueText:"u1"`,
		`limit:5`,
	} {
		if !strings.Contains(gql, want) {
			t.Fatalf("graphql missing %q: %s", want, gql)
		}
	}
}

func TestHybridSearchGraphQLError(t *testing.T) {
	rec := &recorder{t: t}
	rec.reply = func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"errors":[{"message":"unknown class"}]}`))
	}
	srv := rec.server()
	defer srv.Close()

	c := New(srv.URL, 768, srv.Client())
	if _, err := c.HybridSearch(context.Background(), "u1", "q", 5); err == nil {
		t.Fatal("want graphql error surfaced")
	}
}
