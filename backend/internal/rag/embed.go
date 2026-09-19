package rag

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/abubakarsiddik31/golem/embedding"
	"github.com/abubakarsiddik31/golem/model"
	gemini "github.com/abubakarsiddik31/golem/providers/gemini"
)

const defaultEmbedBatch = 96
const maxQueryCacheEntries = 1000

// Embedder wraps golem's Gemini embedder with app-side batching and an
// in-memory query embedding cache to eliminate remote roundtrips on repeat queries.
type Embedder struct {
	inner   embedding.Embedder
	batch   int
	cacheMu sync.RWMutex
	cache   map[string]embedding.Result
}

// NewEmbedder builds the Gemini-backed embedder (golem v0.7.5 port).
func NewEmbedder(apiKey, model string, dims, batch int) (*Embedder, error) {
	inner, err := gemini.NewEmbedder(gemini.EmbedderConfig{
		APIKey:     apiKey,
		Model:      model,
		Dimensions: dims,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini embedder: %w", err)
	}
	return embedderWith(inner, batch), nil
}

func embedderWith(inner embedding.Embedder, batch int) *Embedder {
	if batch <= 0 {
		batch = defaultEmbedBatch
	}
	return &Embedder{
		inner: inner,
		batch: batch,
		cache: make(map[string]embedding.Result),
	}
}

func (e *Embedder) EmbedQuery(ctx context.Context, text string) (embedding.Result, error) {
	clean := strings.TrimSpace(text)
	e.cacheMu.RLock()
	cached, ok := e.cache[clean]
	e.cacheMu.RUnlock()
	if ok {
		return cached, nil
	}

	res, err := e.inner.EmbedQuery(ctx, text)
	if err != nil {
		return embedding.Result{}, fmt.Errorf("embed query: %w", err)
	}

	e.cacheMu.Lock()
	if len(e.cache) >= maxQueryCacheEntries {
		e.cache = make(map[string]embedding.Result)
	}
	e.cache[clean] = res
	e.cacheMu.Unlock()

	return res, nil
}

// EmbedDocuments embeds in batches of at most e.batch texts, preserving
// input order; usage from all batches is summed.
func (e *Embedder) EmbedDocuments(ctx context.Context, texts []string) (embedding.Result, error) {
	if len(texts) == 0 {
		return embedding.Result{}, fmt.Errorf("embed documents: no texts")
	}
	out := embedding.Result{Vectors: make([][]float32, 0, len(texts))}
	for start := 0; start < len(texts); start += e.batch {
		end := min(start+e.batch, len(texts))
		res, err := e.inner.EmbedDocuments(ctx, texts[start:end])
		if err != nil {
			return embedding.Result{}, fmt.Errorf("embed documents [%d:%d]: %w", start, end, err)
		}
		out.Vectors = append(out.Vectors, res.Vectors...)
		out.Usage.InputTokens += res.Usage.InputTokens
	}
	return out, nil
}

var _ = model.Usage{} // embedding usage rides model.Usage verbatim
