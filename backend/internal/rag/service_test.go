package rag

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/embedding"
	"github.com/abubakarsiddik31/golem/model"
)

// --- fakes ---

type fakeVS struct {
	upserts    []upsertCall
	deletes    []string
	searchCall []searchCall
	results    []Scored
	failUpsert bool
}

type upsertCall struct {
	UserID, DocumentID, DocTitle string
	Chunks                       []Chunk
	Vectors                      [][]float32
}

type searchCall struct {
	UserID, Query string
	K             int
}

func (f *fakeVS) UpsertChunks(_ context.Context, userID, documentID, docTitle string, chunks []Chunk, vectors [][]float32) error {
	if f.failUpsert {
		return fmt.Errorf("boom")
	}
	f.upserts = append(f.upserts, upsertCall{userID, documentID, docTitle, chunks, vectors})
	return nil
}

func (f *fakeVS) DeleteDocument(_ context.Context, documentID string) error {
	f.deletes = append(f.deletes, documentID)
	return nil
}

func (f *fakeVS) HybridSearch(_ context.Context, userID, query string, k int) ([]Scored, error) {
	f.searchCall = append(f.searchCall, searchCall{userID, query, k})
	return f.results, nil
}

type fakeObjects struct {
	puts    []string
	deletes []string
}

func (f *fakeObjects) Put(_ context.Context, key, _ string, _ io.Reader, _ int64) error {
	f.puts = append(f.puts, key)
	return nil
}

func (f *fakeObjects) Delete(_ context.Context, key string) error {
	f.deletes = append(f.deletes, key)
	return nil
}

// stubEmbedder returns dims-wide vectors and counts one token per text.
type stubEmbedder struct{ dims int }

func (s stubEmbedder) EmbedDocuments(_ context.Context, texts []string) (embedding.Result, error) {
	return embedding.Result{Vectors: manyVectors(s.dims, len(texts)), Usage: model.Usage{InputTokens: len(texts)}}, nil
}

func (s stubEmbedder) EmbedQuery(_ context.Context, _ string) (embedding.Result, error) {
	return embedding.Result{Vectors: manyVectors(s.dims, 1), Usage: model.Usage{InputTokens: 7}}, nil
}

func manyVectors(dims, n int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		v := make([]float32, dims)
		out[i] = v
	}
	return out
}

func newTestService(vs VectorStore, objects ObjectUploader) *Service {
	return NewService(embedderWith(stubEmbedder{4}, 0), vs, objects)
}

// --- ingest ---

func TestIngestHappy(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	svc := newTestService(vs, objs)
	content := []byte(strings.Repeat("first block of text. ", 40) + "\n\n" + strings.Repeat("second block of text. ", 40))
	n, err := svc.Ingest(context.Background(), "u1", "d1", "notes.md", "text/markdown", content, "text/markdown")
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 2 {
		t.Fatalf("chunk count = %d, want 2", n)
	}
	if len(vs.upserts) != 1 {
		t.Fatalf("upserts: %d", len(vs.upserts))
	}
	call := vs.upserts[0]
	if call.UserID != "u1" || call.DocumentID != "d1" || call.DocTitle != "notes.md" {
		t.Fatalf("call meta: %+v", call)
	}
	if len(call.Vectors) != len(call.Chunks) {
		t.Fatalf("vectors/chunks misaligned: %d/%d", len(call.Vectors), len(call.Chunks))
	}
	wantKey := ObjectKey("u1", "d1", "notes.md")
	if len(objs.puts) != 1 || objs.puts[0] != wantKey {
		t.Fatalf("object key: %v", objs.puts)
	}
}

func TestIngestDeletesBeforeUpsert(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	svc := newTestService(vs, objs)
	content := []byte("hello world")
	if _, err := svc.Ingest(context.Background(), "u1", "d1", "a.txt", "text/plain", content, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ingest(context.Background(), "u1", "d1", "a.txt", "text/plain", content, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if len(vs.upserts) != 2 || len(vs.deletes) != 2 {
		t.Fatalf("calls: %d upserts, %d deletes", len(vs.upserts), len(vs.deletes))
	}
	if vs.deletes[0] != "d1" || vs.deletes[1] != "d1" {
		t.Fatalf("deleted ids: %v", vs.deletes)
	}
}

func TestIngestExtractFails(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	svc := newTestService(vs, objs)
	_, err := svc.Ingest(context.Background(), "u1", "d1", "x.bin", "application/zip", []byte{1}, "application/zip")
	if err == nil {
		t.Fatal("want extract error")
	}
	if len(vs.upserts) != 0 || len(vs.deletes) != 0 {
		t.Fatal("nothing must be indexed on extract failure")
	}
}

func TestIngestEmptyText(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	svc := newTestService(vs, objs)
	_, err := svc.Ingest(context.Background(), "u1", "d1", "scan.txt", "text/plain", []byte("   \n   "), "text/plain")
	if err == nil || !strings.Contains(err.Error(), "no text extracted") {
		t.Fatalf("want no-text error, got %v", err)
	}
	if len(vs.upserts) != 0 {
		t.Fatal("nothing must be indexed for empty text")
	}
}

func TestIngestUpsertFailure(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	vs.failUpsert = true
	svc := newTestService(vs, objs)
	_, err := svc.Ingest(context.Background(), "u1", "d1", "a.txt", "text/plain", []byte("text"), "text/plain")
	if err == nil {
		t.Fatal("want upsert error")
	}
	if len(objs.puts) != 1 || len(objs.deletes) != 0 {
		t.Fatal("the original must be kept when indexing fails")
	}
}

// --- search ---

func TestSearchCombinesEmbedAndIndex(t *testing.T) {
	vs, _ := &fakeVS{}, &fakeObjects{}
	vs.results = []Scored{{Chunk: Chunk{DocTitle: "a.txt", Content: "hit"}, Score: 0.9}}
	svc := newTestService(vs, &fakeObjects{})

	scored, usage, err := svc.Search(context.Background(), "u1", "capital of France", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(scored) != 1 || scored[0].Chunk.DocTitle != "a.txt" {
		t.Fatalf("scored: %+v", scored)
	}
	if usage.InputTokens != 7 {
		t.Fatalf("query usage not passed through: %+v", usage)
	}
	if vs.searchCall[0].K != 5 || vs.searchCall[0].Query != "capital of France" || vs.searchCall[0].UserID != "u1" {
		t.Fatalf("search call: %+v", vs.searchCall[0])
	}
}

func TestSearchClampsK(t *testing.T) {
	vs := &fakeVS{}
	svc := newTestService(vs, &fakeObjects{})
	if _, _, err := svc.Search(context.Background(), "u1", "q", 99); err != nil {
		t.Fatal(err)
	}
	if vs.searchCall[0].K != maxK {
		t.Fatalf("k not clamped down: %d", vs.searchCall[0].K)
	}
}

// --- key ---

func TestObjectKey(t *testing.T) {
	if got := ObjectKey("u1", "d1", "a b.txt"); got != "u1/d1/a b.txt" {
		t.Fatalf("key: %q", got)
	}
	long := strings.Repeat("x", 10001)
	if len(ObjectKey("u1", "d1", long)) > 9100 {
		t.Fatal("long filename not bounded")
	}
}
