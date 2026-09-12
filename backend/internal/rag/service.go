package rag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/abubakarsiddik31/golem/model"
)

// Scored pairs a chunk with its retrieval score (hybrid BM25+vector).
type Scored struct {
	Chunk Chunk
	Score float64
}

// VectorStore is the retrieval index contract. internal/weaviate.Client
// implements it; tests use fakes.
type VectorStore interface {
	UpsertChunks(ctx context.Context, userID, documentID, docTitle string, chunks []Chunk, vectors [][]float32) error
	DeleteDocument(ctx context.Context, documentID string) error
	HybridSearch(ctx context.Context, userID, query string, k int) ([]Scored, error)
}

// ObjectUploader is the slice of the object store the pipeline needs:
// keep the original upload, drop it with the document.
type ObjectUploader interface {
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
	Delete(ctx context.Context, key string) error
}

const (
	defaultK    = 5
	maxK        = 20
	maxKeyBytes = 9000 // sane cap so odd filenames cannot break the store
)

// Service is the RAG vertical: ingest (extract → chunk → embed → index)
// and search (embed query → hybrid search). It owns no storage rows; the
// HTTP layer tracks document state in Postgres around Ingest calls.
type Service struct {
	embed   *Embedder
	vs      VectorStore
	objects ObjectUploader
}

func NewService(embed *Embedder, vs VectorStore, objects ObjectUploader) *Service {
	return &Service{embed: embed, vs: vs, objects: objects}
}

// Ingest keeps the original bytes in the object store, then indexes the
// extracted text. Idempotent per document: stale vectors are deleted
// before the fresh upsert. Returns the chunk count.
func (s *Service) Ingest(ctx context.Context, userID, documentID, docTitle, mime string, content []byte, contentType string) (int, error) {
	key := ObjectKey(userID, documentID, docTitle)
	if err := s.objects.Put(ctx, key, contentType, bytes.NewReader(content), int64(len(content))); err != nil {
		return 0, fmt.Errorf("store original: %w", err)
	}
	text, err := ExtractText(mime, bytes.NewReader(content))
	if err != nil {
		return 0, fmt.Errorf("extract text: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("no text extracted from %q", docTitle)
	}
	chunks := ChunkText(documentID, docTitle, text)
	if len(chunks) == 0 {
		return 0, fmt.Errorf("no chunks produced from %q", docTitle)
	}
	res, err := s.embed.EmbedDocuments(ctx, chunkTexts(chunks))
	if err != nil {
		return 0, fmt.Errorf("embed chunks: %w", err)
	}
	if err := s.vs.DeleteDocument(ctx, documentID); err != nil {
		return 0, fmt.Errorf("clear stale vectors: %w", err)
	}
	if err := s.vs.UpsertChunks(ctx, userID, documentID, docTitle, chunks, res.Vectors); err != nil {
		return 0, fmt.Errorf("index chunks: %w", err)
	}
	return len(chunks), nil
}

// Search embeds the query and hybrid-searches the index. The returned
// Usage carries the query embedding's exact token count for the ledger.
func (s *Service) Search(ctx context.Context, userID, query string, k int) ([]Scored, model.Usage, error) {
	if k <= 0 {
		k = defaultK
	}
	if k > maxK {
		k = maxK
	}
	res, err := s.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, model.Usage{}, fmt.Errorf("embed query: %w", err)
	}
	scored, err := s.vs.HybridSearch(ctx, userID, query, k)
	if err != nil {
		return nil, model.Usage{}, fmt.Errorf("search index: %w", err)
	}
	return scored, res.Usage, nil
}

func chunkTexts(chunks []Chunk) []string {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	return texts
}

// ObjectKey builds the canonical object-store key. Only the filename's
// length is bounded here; the HTTP layer validates the name itself.
func ObjectKey(userID, documentID, filename string) string {
	if len(filename) > maxKeyBytes {
		filename = filename[:maxKeyBytes]
	}
	return userID + "/" + documentID + "/" + filename
}
