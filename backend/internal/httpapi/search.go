package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/embedding"
	"github.com/abubakarsiddik31/golem/model"
)

// MessageSearchStore bridges full-text message search to storage.
type MessageSearchStore interface {
	Search(ctx context.Context, userID, rawQuery string, limit, offset int) ([]storage.SearchHit, bool, error)
}

var _ MessageSearchStore = (*storage.MessageSearch)(nil)

// MessageEmbedder reranks FTS candidates by cosine similarity. It matches
// *rag.Embedder; nil means semantic search is unavailable.
type MessageEmbedder interface {
	EmbedQuery(ctx context.Context, text string) (embedding.Result, error)
	EmbedDocuments(ctx context.Context, texts []string) (embedding.Result, error)
}

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
)

type searchHitDTO struct {
	MessageID      string `json:"messageId"`
	ConversationID string `json:"conversationId"`
	Title          string `json:"title"`
	Role           string `json:"role"`
	// Snippet brackets matched terms with the private-use runes
	// storage.SnippetHitStart/Stop; the client must render them as text
	// nodes, never as HTML.
	Snippet   string   `json:"snippet"`
	CreatedAt string   `json:"createdAt"`
	Score     *float64 `json:"score,omitempty"`
}

// handleSearchMessages serves the owner's message search. Full-text results
// are optionally reranked by embedding similarity when semantic=1 and an
// embedder is configured. Archived conversations are excluded; pagination is
// offset-based for FTS and offset-free for semantic reranking.
func (s *Server) handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "q is required")
		return
	}
	limit := defaultSearchLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "limit must be a positive integer")
			return
		}
		if n > maxSearchLimit {
			n = maxSearchLimit
		}
		limit = n
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "offset must be a non-negative integer")
			return
		}
		offset = n
	}
	semantic := r.URL.Query().Get("semantic") == "1" && s.deps.SemanticEmbedder != nil

	var hits []storage.SearchHit
	var hasMore bool
	var err error
	if semantic {
		// Over-fetch a bounded candidate pool for reranking; never more than
		// twice the requested page (capped) to keep embedding cost sane.
		candidateLimit := limit * 2
		if candidateLimit > maxSearchLimit {
			candidateLimit = maxSearchLimit
		}
		hits, _, err = s.deps.MessageSearch.Search(r.Context(), userID, q, candidateLimit, 0)
		if err == nil {
			reranked, rerr := s.rerankSearch(r.Context(), userID, q, hits)
			if rerr != nil {
				if s.deps.Log != nil {
					s.deps.Log.Warn("semantic search failed; using full-text order", "err", rerr)
				}
				semantic = false
				hits, hasMore, err = s.deps.MessageSearch.Search(r.Context(), userID, q, limit, offset)
			} else {
				hits = reranked
				hasMore = false
			}
		}
	} else {
		hits, hasMore, err = s.deps.MessageSearch.Search(r.Context(), userID, q, limit, offset)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not search messages")
		return
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]searchHitDTO, len(hits))
	for i, h := range hits {
		out[i] = searchHitDTO{
			MessageID: h.MessageID, ConversationID: h.ConversationID, Title: h.Title,
			Role: h.Role, Snippet: h.Snippet, CreatedAt: h.CreatedAt.UTC().Format(timeRFC3339),
		}
		if semantic {
			score := h.Score
			out[i].Score = &score
		}
	}
	var next *int
	if hasMore {
		n := offset + len(out)
		next = &n
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": out, "nextOffset": next, "semantic": semantic})
}

func (s *Server) rerankSearch(ctx context.Context, userID, query string, hits []storage.SearchHit) ([]storage.SearchHit, error) {
	if len(hits) == 0 {
		return hits, nil
	}
	qres, err := s.deps.SemanticEmbedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(qres.Vectors) == 0 || len(qres.Vectors[0]) == 0 {
		return nil, errors.New("embed query returned an empty vector")
	}
	s.meterEmbedding(ctx, userID, qres.Usage)
	texts := make([]string, len(hits))
	for i, h := range hits {
		text := h.Content
		if strings.TrimSpace(text) == "" {
			text = strings.ReplaceAll(strings.ReplaceAll(h.Snippet, storage.SnippetHitStart, ""), storage.SnippetHitStop, "")
		}
		texts[i] = text
	}
	dres, err := s.deps.SemanticEmbedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(dres.Vectors) != len(hits) {
		return nil, fmt.Errorf("embed documents returned %d vectors for %d texts", len(dres.Vectors), len(hits))
	}
	queryVec := qres.Vectors[0]
	for i := range hits {
		hits[i].Score = cosine(queryVec, dres.Vectors[i])
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	s.meterEmbedding(ctx, userID, dres.Usage)
	return hits, nil
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// meterEmbedding prices query/candidate embeddings from the RAG rate table.
func (s *Server) meterEmbedding(ctx context.Context, userID string, usage model.Usage) {
	if s.deps.Usage == nil || usage.InputTokens == 0 {
		return
	}
	name := s.deps.Cfg.RAG.EmbeddingModel
	if name == "" {
		name = "gemini-embedding-001"
	}
	_ = s.deps.Usage.Add(ctx, storage.UsageEvent{
		UserID: userID, Kind: "embedding", Model: name,
		InputTokens: usage.InputTokens, Estimated: true,
		CostMicros: s.deps.Rates.RatesFor(name).ChatCostMicros(usage.InputTokens, 0),
	})
}
