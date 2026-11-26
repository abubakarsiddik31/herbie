package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SnippetHitStart/Stop bracket matched terms in ts_headline output. They are
// Unicode private-use runes: the client splits on them and renders text nodes
// (never HTML), so message content cannot inject markup.
const (
	SnippetHitStart = "\uE000"
	SnippetHitStop  = "\uE001"
)

const (
	// searchTextLimit mirrors the migration's truncated tsvector input.
	searchTextLimit = 100000
	maxSearchTerms  = 16
)

// SearchHit is one matching message row plus the conversation it belongs to.
type SearchHit struct {
	MessageID      string
	ConversationID string
	Title          string
	Role           string
	Snippet        string // ts_headline output bracketed by SnippetHitStart/Stop
	Content        string // truncated original text, for semantic reranking
	Score          float64
	CreatedAt      time.Time
}

type MessageSearch struct{ pool *pgxpool.Pool }

func NewMessageSearch(pool *pgxpool.Pool) *MessageSearch { return &MessageSearch{pool: pool} }

// BuildPrefixTsQuery turns raw user input into a safe `to_tsquery` string:
// punctuation and tsquery operators are discarded, each remaining term is
// quoted (single quotes doubled) and prefix-matched, and terms are ANDed.
// Empty input yields "", which callers treat as "no results". The term cap
// bounds query planning cost for pathological input.
func BuildPrefixTsQuery(raw string) string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	})
	terms := make([]string, 0, len(fields))
	for _, term := range fields {
		if len(terms) == maxSearchTerms {
			break
		}
		term = strings.ToLower(term)
		terms = append(terms, "'"+strings.ReplaceAll(term, "'", "''")+"':*")
	}
	return strings.Join(terms, " & ")
}

// Search returns up to limit visible messages (user/assistant rows with text)
// belonging to the user, newest-ranked first, excluding archived
// conversations. hasMore reports whether another page exists.
func (s *MessageSearch) Search(ctx context.Context, userID, rawQuery string, limit, offset int) ([]SearchHit, bool, error) {
	tsQuery := BuildPrefixTsQuery(rawQuery)
	if tsQuery == "" {
		return nil, false, nil
	}
	if limit <= 0 {
		limit = 20
	}
	opts := fmt.Sprintf("StartSel=%s,StopSel=%s,MaxFragments=2,MaxWords=40,MinWords=8",
		SnippetHitStart, SnippetHitStop)
	rows, err := s.pool.Query(ctx,
		`WITH q AS (SELECT to_tsquery('simple', $2) AS query)
		 SELECT m.id::text, m.conversation_id::text, c.title, m.role,
		        ts_headline('simple', left(m.content, $5), q.query, $6) AS snippet,
		        left(m.content, 2000) AS content,
		        m.created_at
		 FROM messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 CROSS JOIN q
		 WHERE m.user_id = $1
		   AND c.archived_at IS NULL
		   AND m.role IN ('user', 'assistant')
		   AND m.content <> ''
		   AND m.search_vector @@ q.query
		 ORDER BY ts_rank(m.search_vector, q.query) DESC, m.created_at DESC, m.id DESC
		 LIMIT $3 OFFSET $4`,
		userID, tsQuery, limit+1, offset, searchTextLimit, opts)
	if err != nil {
		return nil, false, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var hit SearchHit
		if err := rows.Scan(&hit.MessageID, &hit.ConversationID, &hit.Title, &hit.Role,
			&hit.Snippet, &hit.Content, &hit.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("scan search hit: %w", err)
		}
		out = append(out, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}
