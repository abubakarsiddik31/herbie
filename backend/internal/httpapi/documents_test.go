package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// --- fakes ---

type fakeDocs struct {
	rows    []storage.Document
	seq     int
	status  []string
	deleted []string
}

func (f *fakeDocs) Create(_ context.Context, d storage.Document) (storage.Document, error) {
	f.seq++
	d.CreatedAt = time.Now()
	f.rows = append(f.rows, d)
	return d, nil
}

func (f *fakeDocs) Get(_ context.Context, id, userID string) (storage.Document, error) {
	for _, d := range f.rows {
		if d.ID == id && d.UserID == userID {
			return d, nil
		}
	}
	return storage.Document{}, storage.ErrNotFound
}

func (f *fakeDocs) List(_ context.Context, userID string) ([]storage.Document, error) {
	var out []storage.Document
	for _, d := range f.rows {
		if d.UserID == userID {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeDocs) SetStatus(_ context.Context, id, _ string, status, errMsg string, chunkCount int) error {
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].Status, f.rows[i].Error, f.rows[i].ChunkCount = status, errMsg, chunkCount
			f.status = append(f.status, status)
			return nil
		}
	}
	return storage.ErrNotFound
}

func (f *fakeDocs) Delete(_ context.Context, id, userID string) error {
	for i := range f.rows {
		if f.rows[i].ID == id && f.rows[i].UserID == userID {
			f.deleted = append(f.deleted, id)
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return nil
		}
	}
	return storage.ErrNotFound
}

type fakeRag struct {
	chunks int
	err    error
	calls  int
}

func (f *fakeRag) Ingest(_ context.Context, _, _, _, _ string, _ []byte, _ string) (int, error) {
	f.calls++
	return f.chunks, f.err
}

type fakeVectors struct{ deleted []string }

func (f *fakeVectors) UpsertChunks(_ context.Context, _, _, _ string, _ []rag.Chunk, _ [][]float32) error {
	return nil
}
func (f *fakeVectors) DeleteDocument(_ context.Context, documentID string) error {
	f.deleted = append(f.deleted, documentID)
	return nil
}
func (f *fakeVectors) HybridSearch(_ context.Context, _, _ string, _ int) ([]rag.Scored, error) {
	return nil, nil
}

type fakeObjects struct{ deleted []string }

func (f *fakeObjects) Put(_ context.Context, key, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (f *fakeObjects) Delete(_ context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}
func (f *fakeObjects) Get(_ context.Context, key string) (io.ReadSeekCloser, error) { return nil, nil }

// --- harness ---

func newDocsServer(t *testing.T, maxUpload int64) (http.Handler, string, *fakeDocs, *fakeRag, *fakeVectors, *fakeObjects) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{}
	cfg.RAG.MaxUploadBytes = maxUpload
	cfg.RAG.EmbeddingModel = "gemini-embedding-001"
	docs, fr, fv, fo := &fakeDocs{}, &fakeRag{}, &fakeVectors{}, &fakeObjects{}
	h := NewServer(ServerDeps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:   authtest.NewService(testSecret),
		Tokens: tm,
		Cfg:    cfg,
		Usage:  newFakeUsage(),
		Agent:  nil,
		RagSearch: func(_ context.Context, _, _ string, _ int) ([]rag.Scored, model.Usage, error) {
			return nil, model.Usage{}, nil
		},
		RAG:     fr,
		Docs:    docs,
		Vectors: fv,
		Objects: fo,
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token, docs, fr, fv, fo
}

func uploadDoc(t *testing.T, h http.Handler, token, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf strings.Builder
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, "/api/documents", strings.NewReader(buf.String()))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// --- tests ---

func TestUploadDocumentHappy(t *testing.T) {
	h, token, docs, fr, fv, fo := newDocsServer(t, 20<<20)
	fr.chunks = 2
	rec := uploadDoc(t, h, token, "notes.md", "hello\n\nworld")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got documentJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "ready" || got.ChunkCount != 2 || got.Filename != "notes.md" {
		t.Fatalf("document: %+v", got)
	}
	if fr.calls != 1 {
		t.Fatalf("ingest calls: %d", fr.calls)
	}
	// The row ends ready and the object key carries the generated id.
	if len(docs.rows) != 1 || docs.rows[0].Status != "ready" {
		t.Fatalf("rows: %+v", docs.rows)
	}
	if !strings.HasPrefix(docs.rows[0].ObjectKey, "u-1/") || !strings.HasSuffix(docs.rows[0].ObjectKey, "/notes.md") {
		t.Fatalf("object key: %q", docs.rows[0].ObjectKey)
	}
	_ = fv
	_ = fo
}

func TestUploadDocumentTypeRejected(t *testing.T) {
	h, token, docs, fr, _, _ := newDocsServer(t, 20<<20)
	rec := uploadDoc(t, h, token, "evil.exe", "MZ")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d", rec.Code)
	}
	if fr.calls != 0 || len(docs.rows) != 0 {
		t.Fatal("rejected upload must not touch storage")
	}
}

func TestUploadDocumentOversize(t *testing.T) {
	h, token, docs, fr, _, _ := newDocsServer(t, 16)
	rec := uploadDoc(t, h, token, "big.txt", strings.Repeat("x", 17))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d", rec.Code)
	}
	if fr.calls != 0 || len(docs.rows) != 0 {
		t.Fatal("oversized upload must not reach the pipeline")
	}
}

func TestUploadDocumentDisabled(t *testing.T) {
	tm, _ := auth.NewTokenMaker(testSecret)
	h := NewServer(ServerDeps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:   authtest.NewService(testSecret),
		Tokens: tm,
		Usage:  newFakeUsage(),
	})
	token, _, _ := tm.Issue("u-1", time.Now())
	rec := uploadDoc(t, h, token, "a.txt", "x")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "rag_disabled") {
		t.Fatalf("body: %s", rec.Body.String())
	}
}

func TestUploadDocumentIngestFailsKeepsRow(t *testing.T) {
	h, token, docs, fr, _, _ := newDocsServer(t, 20<<20)
	fr.err = context.DeadlineExceeded
	rec := uploadDoc(t, h, token, "a.txt", "x")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got documentJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.Error == "" {
		t.Fatalf("failed document: %+v", got)
	}
	if len(docs.rows) != 1 {
		t.Fatal("the row must be kept on failure")
	}
}

func TestListDocuments(t *testing.T) {
	h, token, docs, _, _, _ := newDocsServer(t, 20<<20)
	_, _ = docs.Create(context.Background(), storage.Document{ID: "d1", UserID: "u-1", Filename: "a.txt", CreatedAt: time.Now()})
	req, _ := http.NewRequest(http.MethodGet, "/api/documents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"documents"`) || !strings.Contains(rec.Body.String(), "a.txt") {
		t.Fatalf("body: %s", rec.Body.String())
	}
}

func TestDeleteDocument(t *testing.T) {
	h, token, docs, _, fv, fo := newDocsServer(t, 20<<20)
	d, _ := docs.Create(context.Background(), storage.Document{ID: "d1", UserID: "u-1", ObjectKey: "u-1/d1/a.txt", Filename: "a.txt"})
	req, _ := http.NewRequest(http.MethodDelete, "/api/documents/"+d.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(fv.deleted) != 1 || fv.deleted[0] != "d1" {
		t.Fatalf("vector deletes: %v", fv.deleted)
	}
	if len(fo.deleted) != 1 || fo.deleted[0] != "u-1/d1/a.txt" {
		t.Fatalf("object deletes: %v", fo.deleted)
	}
	if len(docs.deleted) != 1 || docs.deleted[0] != "d1" {
		t.Fatalf("row deletes: %v", docs.deleted)
	}
}

func TestDeleteDocumentCrossUser(t *testing.T) {
	h, token, docs, _, fv, fo := newDocsServer(t, 20<<20)
	_, _ = docs.Create(context.Background(), storage.Document{ID: "d1", UserID: "someone-else", ObjectKey: "x", Filename: "a.txt"})
	req, _ := http.NewRequest(http.MethodDelete, "/api/documents/d1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	if len(fv.deleted) != 0 || len(fo.deleted) != 0 || len(docs.deleted) != 0 {
		t.Fatal("cross-user delete must be a no-op")
	}
}
