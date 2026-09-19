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

	limitSeen   int
	alphaSeen   float64
	docIDsSeen  []string
	expandCalls []expandCall
	rangeChunks []Chunk // canned per-doc chunk list for ExpandRange
	failUpsert  bool
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

type expandCall struct {
	DocID  string
	Lo, Hi int
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

func (f *fakeVS) HybridSearch(_ context.Context, userID, query string, _ []float32, alpha float64, limit int, docIDs []string) ([]Scored, error) {
	f.searchCall = append(f.searchCall, searchCall{userID, query, limit})
	f.limitSeen = limit
	f.alphaSeen = alpha
	f.docIDsSeen = docIDs
	return f.results, nil
}

func (f *fakeVS) ExpandRange(_ context.Context, _, docID string, lo, hi int) ([]Chunk, error) {
	f.expandCalls = append(f.expandCalls, expandCall{docID, lo, hi})
	var out []Chunk
	for _, c := range f.rangeChunks {
		if c.DocumentID == docID && c.Index >= lo && c.Index <= hi {
			out = append(out, c)
		}
	}
	return out, nil
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

// fourScored returns 4 hits in reverse relevance order (worst first).
func fourScored() []Scored {
	return []Scored{
		{Chunk: Chunk{DocumentID: "d", DocTitle: "t", Index: 0, Content: "first"}, Score: 0.1},
		{Chunk: Chunk{DocumentID: "d", DocTitle: "t", Index: 1, Content: "second"}, Score: 0.2},
		{Chunk: Chunk{DocumentID: "d", DocTitle: "t", Index: 2, Content: "third"}, Score: 0.3},
		{Chunk: Chunk{DocumentID: "d", DocTitle: "t", Index: 3, Content: "fourth"}, Score: 0.4},
	}
}

// --- ingest ---

func TestIngestHappy(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	svc := newTestService(vs, objs)
	// Two headed sections, each ~210 tokens — well under the 512-token
	// target, so each section yields exactly one chunk.
	content := []byte("# Intro\n" + strings.Repeat("first block of text. ", 40) + "\n\n## Deep\n" + strings.Repeat("second block of text. ", 40))
	n, _, err := svc.Ingest(context.Background(), "u1", "d1", "notes.md", "text/markdown", content, "text/markdown")
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 2 {
		t.Fatalf("chunk count = %d, want 2 (one per section)", n)
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
	if call.Chunks[0].Heading != "Intro" || call.Chunks[1].Heading != "Deep" {
		t.Fatalf("headings did not propagate: %q %q", call.Chunks[0].Heading, call.Chunks[1].Heading)
	}
	if call.Chunks[0].Page != 0 || call.Chunks[1].Page != 0 {
		t.Fatalf("markdown pages must be 0: %+v", call.Chunks)
	}
	for i, c := range call.Chunks {
		if c.Index != i || c.DocumentID != "d1" || c.DocTitle != "notes.md" {
			t.Fatalf("chunk %d meta: %+v", i, c)
		}
		if c.Tokens != EstTokens(c.Content) {
			t.Fatalf("chunk %d tokens not estimated: %+v", i, c)
		}
	}
	if !strings.Contains(call.Chunks[0].Content, "first block") || !strings.Contains(call.Chunks[1].Content, "second block") {
		t.Fatalf("section order not preserved: %q / %q", call.Chunks[0].Content[:32], call.Chunks[1].Content[:32])
	}
	wantKey := ObjectKey("u1", "d1", "notes.md")
	wantParsedKey := ParsedObjectKey(wantKey)
	if len(objs.puts) < 1 || objs.puts[0] != wantKey {
		t.Fatalf("object key: %v, want at least %s", objs.puts, wantKey)
	}
	if len(objs.puts) > 1 && objs.puts[1] != wantParsedKey {
		t.Fatalf("parsed object key: %v, want %s", objs.puts[1], wantParsedKey)
	}
}

func TestIngestWithProjectKey(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	svc := newTestService(vs, objs)
	key := ProjectObjectKey("u1", "p1", "d1", "report.md")
	_, _, err := svc.IngestWithKey(context.Background(), key, "u1", "d1", "report.md", "text/markdown", []byte("# Report\n\nContent"), "text/markdown")
	if err != nil {
		t.Fatal(err)
	}
	if len(objs.puts) != 2 {
		t.Fatalf("expected 2 puts (original + parsed), got %d: %v", len(objs.puts), objs.puts)
	}
	if objs.puts[0] != "u1/projects/p1/d1/report.md" {
		t.Fatalf("expected original at u1/projects/p1/d1/report.md, got %q", objs.puts[0])
	}
	if objs.puts[1] != "u1/projects/p1/d1/parsed.md" {
		t.Fatalf("expected parsed at u1/projects/p1/d1/parsed.md, got %q", objs.puts[1])
	}
}

func TestIngestDeletesBeforeUpsert(t *testing.T) {
	vs, objs := &fakeVS{}, &fakeObjects{}
	svc := newTestService(vs, objs)
	content := []byte("hello world")
	if _, _, err := svc.Ingest(context.Background(), "u1", "d1", "a.txt", "text/plain", content, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Ingest(context.Background(), "u1", "d1", "a.txt", "text/plain", content, "text/plain"); err != nil {
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
	_, _, err := svc.Ingest(context.Background(), "u1", "d1", "x.bin", "application/zip", []byte{1}, "application/zip")
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
	_, _, err := svc.Ingest(context.Background(), "u1", "d1", "scan.txt", "text/plain", []byte("   \n   "), "text/plain")
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
	_, _, err := svc.Ingest(context.Background(), "u1", "d1", "a.txt", "text/plain", []byte("text"), "text/plain")
	if err == nil {
		t.Fatal("want upsert error")
	}
	if len(objs.puts) != 1 || len(objs.deletes) != 0 {
		t.Fatal("the original must be kept when indexing fails")
	}
}

// --- search ---

func TestSearchCombinesEmbedAndIndex(t *testing.T) {
	vs := &fakeVS{}
	vs.results = []Scored{{Chunk: Chunk{DocumentID: "d", DocTitle: "a.txt", Content: "hit"}, Score: 0.9}}
	svc := newTestService(vs, &fakeObjects{})

	scored, rep, err := svc.Search(context.Background(), "u1", "capital of France", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(scored) != 1 || scored[0].Chunk.DocTitle != "a.txt" {
		t.Fatalf("scored: %+v", scored)
	}
	if rep.Embed.InputTokens != 7 {
		t.Fatalf("query usage not passed through: %+v", rep.Embed)
	}
	if rep.Reranked {
		t.Fatalf("no ranker attached, must not report rerank: %+v", rep)
	}
	if vs.searchCall[0].Query != "capital of France" || vs.searchCall[0].UserID != "u1" {
		t.Fatalf("search call: %+v", vs.searchCall[0])
	}
	if vs.alphaSeen != 0.5 {
		t.Fatalf("default alpha not passed: %v", vs.alphaSeen)
	}
	if vs.limitSeen != defaultK*4 { // default TopK=5, mult=4 → 20 candidates
		t.Fatalf("candidates = %d, want %d", vs.limitSeen, defaultK*4)
	}
	if len(vs.docIDsSeen) != 0 {
		t.Fatalf("unscoped search must pass no doc filter: %v", vs.docIDsSeen)
	}
}

func TestSearchClampsK(t *testing.T) {
	vs := &fakeVS{}
	svc := newTestService(vs, &fakeObjects{})
	// Mult 1 and a high candidate cap so the clamp on TopK is observable
	// in the candidate limit (clamped 20*1=20, unclamped 99*1=99).
	svc.WithTuning(0.5, 1, 100, 0, 0, 512, 64)
	if _, _, err := svc.Search(context.Background(), "u1", "q", SearchOptions{TopK: 99}); err != nil {
		t.Fatal(err)
	}
	if vs.limitSeen != maxK {
		t.Fatalf("k not clamped down: candidates=%d, want %d", vs.limitSeen, maxK)
	}
}

func TestSearchReranksCandidates(t *testing.T) {
	vs := &fakeVS{results: fourScored()}
	svc := newTestService(vs, &fakeObjects{})
	svc.WithTuning(0.5, 4, 40, 1, 1, 512, 64).WithRanker(NewRanker(stubModel{out: `{"ranking":[3,2,1,0]}`}, "flash"))
	rows, rep, err := svc.Search(context.Background(), "u1", "q", SearchOptions{TopK: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Chunk.Content != "fourth" {
		t.Fatalf("reranked: %+v", rows)
	}
	if !rep.Reranked || rep.RerankModel != "flash" {
		t.Fatalf("report: %+v", rep)
	}
	if vs.limitSeen != 8 { // TopK=2 * mult=4 candidates over-retrieved
		t.Fatalf("candidates: %d", vs.limitSeen)
	}
}

func TestSearchPassesDocIDs(t *testing.T) {
	vs := &fakeVS{results: fourScored()}
	svc := newTestService(vs, &fakeObjects{})
	rows, _, err := svc.Search(context.Background(), "u1", "q", SearchOptions{TopK: 2, DocIDs: []string{"d1", "d2"}})
	if err != nil || len(rows) != 2 {
		t.Fatalf("scoped: %+v %v", rows, err)
	}
	if len(vs.docIDsSeen) != 2 || vs.docIDsSeen[0] != "d1" {
		t.Fatalf("doc filter: %v", vs.docIDsSeen)
	}
}

func TestSearchExpandsWindows(t *testing.T) {
	vs := &fakeVS{
		results: []Scored{{Chunk: Chunk{DocumentID: "d1", DocTitle: "t", Index: 5, Content: "core-5"}, Score: 0.9}},
	}
	for i := 0; i < 10; i++ {
		vs.rangeChunks = append(vs.rangeChunks, Chunk{DocumentID: "d1", DocTitle: "t", Index: i, Content: fmt.Sprintf("core-%d", i)})
	}
	svc := newTestService(vs, &fakeObjects{})
	rows, _, err := svc.Search(context.Background(), "u1", "q", SearchOptions{TopK: 1})
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	if rows[0].Chunk.Content != "core-5" {
		t.Fatalf("core must stay the citation unit: %+v", rows[0])
	}
	if !strings.Contains(rows[0].Context, "core-4") || !strings.Contains(rows[0].Context, "core-6") {
		t.Fatalf("window missing neighbors: %q", rows[0].Context)
	}
	if len(vs.expandCalls) != 1 { // one range query per document, not per hit
		t.Fatalf("expand calls: %v", vs.expandCalls)
	}
}

func TestSearchNoRankerTrims(t *testing.T) {
	vs := &fakeVS{results: fourScored()}
	svc := newTestService(vs, &fakeObjects{})
	rows, rep, err := svc.Search(context.Background(), "u1", "q", SearchOptions{TopK: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Chunk.Content != "first" || rows[1].Chunk.Content != "second" {
		t.Fatalf("must trim in VS order without ranker: %+v", rows)
	}
	if rep.Reranked {
		t.Fatalf("no ranker attached, must not report rerank: %+v", rep)
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

	// Project-scoped key has folder layer: u1/projects/p1/d1/filename
	projKey := ProjectObjectKey("u1", "p1", "d1", "notes.md")
	if projKey != "u1/projects/p1/d1/notes.md" {
		t.Fatalf("unexpected project key: %q", projKey)
	}
	if parsedProj := ParsedObjectKey(projKey); parsedProj != "u1/projects/p1/d1/parsed.md" {
		t.Fatalf("unexpected parsed project key: %q", parsedProj)
	}

	// Document-scoped key has folder layer: u1/documents/d1/filename
	docKey := DocumentObjectKey("u1", "d1", "notes.md")
	if docKey != "u1/documents/d1/notes.md" {
		t.Fatalf("unexpected doc key: %q", docKey)
	}
	if parsedDoc := ParsedObjectKey(docKey); parsedDoc != "u1/documents/d1/parsed.md" {
		t.Fatalf("unexpected parsed doc key: %q", parsedDoc)
	}
}
