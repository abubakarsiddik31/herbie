package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// MessageSearchStore bridges full-text message search to storage.
type MessageSearchStore interface {
	Search(ctx context.Context, userID, rawQuery string, limit, offset int) ([]storage.SearchHit, bool, error)
}

var _ MessageSearchStore = (*storage.MessageSearch)(nil)

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
	Snippet   string `json:"snippet"`
	CreatedAt string `json:"createdAt"`
}

// handleSearchMessages serves the owner's full-text message search. Archived
// conversations are excluded by the store; pagination is offset-based and
// nextOffset is null on the last page.
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

	hits, hasMore, err := s.deps.MessageSearch.Search(r.Context(), userID, q, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not search messages")
		return
	}
	out := make([]searchHitDTO, len(hits))
	for i, h := range hits {
		out[i] = searchHitDTO{
			MessageID: h.MessageID, ConversationID: h.ConversationID, Title: h.Title,
			Role: h.Role, Snippet: h.Snippet, CreatedAt: h.CreatedAt.UTC().Format(timeRFC3339),
		}
	}
	var next *int
	if hasMore {
		n := offset + len(out)
		next = &n
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": out, "nextOffset": next})
}
