package rag

import (
	"context"
	"fmt"

	"github.com/abubakarsiddik31/golem/embedding"
	"github.com/abubakarsiddik31/golem/model"
	gemini "github.com/abubakarsiddik31/golem/providers/gemini"
)

const defaultEmbedBatch = 96

// Embedder wraps golem's Gemini embedder with app-side batching. Golem's
// EmbedDocuments is one provider call per invocation; the ingestion
// pipeline bounds the batch size here so a large document never rides a
// single request. Task types (RETRIEVAL_DOCUMENT / RETRIEVAL_QUERY) are
// the adapter's contract, applied by golem on each side of the split.
type Embedder struct {
	inner embedding.Embedder
	batch int
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
	return &Embedder{inner: inner, batch: batch}
}

func (e *Embedder) EmbedQuery(ctx context.Context, text string) (embedding.Result, error) {
	res, err := e.inner.EmbedQuery(ctx, text)
	if err != nil {
		return embedding.Result{}, fmt.Errorf("embed query: %w", err)
	}
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
