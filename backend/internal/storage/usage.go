package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UsageEvent struct {
	UserID         string
	Kind           string
	Model          string
	ConversationID *string
	DocumentID     *string
	InputTokens    int
	OutputTokens   int
	Requests       int
	Estimated      bool
	CostMicros     int64
}

type SearchQuery struct {
	ID             string
	UserID         string
	ConversationID *string
	Query          string
	Kind           string
	Provider       string
	ResultsCount   int
	DurationMs     int64
	CreatedAt      time.Time
}

type SearchProviderCount struct {
	Provider string `json:"provider"`
	Count    int    `json:"count"`
}

type SearchDailyCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

type SearchQueryItem struct {
	ID             string    `json:"id"`
	Query          string    `json:"query"`
	Kind           string    `json:"kind"`
	Provider       string    `json:"provider"`
	ResultsCount   int       `json:"resultsCount"`
	DurationMs     int64     `json:"durationMs"`
	CreatedAt      time.Time `json:"createdAt"`
	ConversationID string    `json:"conversationId,omitempty"`
}

type SearchQueryCount struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

type SearchAnalysis struct {
	TotalQueries int                   `json:"totalQueries"`
	WebQueries   int                   `json:"webQueries"`
	DocQueries   int                   `json:"docQueries"`
	ByProvider   []SearchProviderCount `json:"byProvider"`
	Daily        []SearchDailyCount    `json:"daily"`
	Recent       []SearchQueryItem     `json:"recent"`
	TopQueries   []SearchQueryCount    `json:"topQueries"`
}

type Usage struct{ pool *pgxpool.Pool }

func NewUsage(pool *pgxpool.Pool) *Usage { return &Usage{pool: pool} }

func (u *Usage) Add(ctx context.Context, e UsageEvent) error {
	_, err := u.pool.Exec(ctx,
		`INSERT INTO usage_events
		 (user_id, kind, model, conversation_id, document_id, input_tokens, output_tokens, requests, estimated, cost_micro_usd)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		e.UserID, e.Kind, e.Model, e.ConversationID, e.DocumentID,
		e.InputTokens, e.OutputTokens, e.Requests, e.Estimated, e.CostMicros)
	if err != nil {
		return fmt.Errorf("add usage event: %w", err)
	}
	return nil
}

func (u *Usage) RecordSearch(ctx context.Context, s SearchQuery) error {
	_, err := u.pool.Exec(ctx,
		`INSERT INTO search_queries
		 (user_id, conversation_id, query, kind, provider, results_count, duration_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE(NULLIF($8, '0001-01-01 00:00:00+00'::timestamptz), now()))`,
		s.UserID, s.ConversationID, s.Query, s.Kind, s.Provider, s.ResultsCount, s.DurationMs, s.CreatedAt)
	if err != nil {
		return fmt.Errorf("record search query: %w", err)
	}
	return nil
}

type KindModelRow struct {
	Kind         string
	Model        string
	InputTokens  int
	OutputTokens int
	Requests     int
	CostMicros   int64
}

type DailyRow struct {
	Day          time.Time
	InputTokens  int
	OutputTokens int
	CostMicros   int64
}

type Summary struct {
	Totals []KindModelRow
	Daily  []DailyRow
	// Documents carries per-document embedding spend (empty without RAG).
	Documents []DocumentSpend
	// Searches carries analysis of search queries performed by the user's AI.
	Searches SearchAnalysis
}

// DocumentSpend is one document's embedding ledger rollup.
type DocumentSpend struct {
	DocumentID  string
	Filename    string
	InputTokens int
	CostMicros  int64
}

func (u *Usage) Summary(ctx context.Context, userID string, days int) (Summary, error) {
	var sum Summary
	sum.Searches.ByProvider = []SearchProviderCount{}
	sum.Searches.Daily = []SearchDailyCount{}
	sum.Searches.Recent = []SearchQueryItem{}
	sum.Searches.TopQueries = []SearchQueryCount{}

	rows, err := u.pool.Query(ctx,
		`SELECT kind, model, SUM(input_tokens)::int, SUM(output_tokens)::int,
		        COALESCE(SUM(requests),0)::int, SUM(cost_micro_usd)
		 FROM usage_events
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY kind, model ORDER BY kind, model`, userID, days)
	if err != nil {
		return sum, fmt.Errorf("usage totals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r KindModelRow
		if err := rows.Scan(&r.Kind, &r.Model, &r.InputTokens, &r.OutputTokens, &r.Requests, &r.CostMicros); err != nil {
			return sum, err
		}
		sum.Totals = append(sum.Totals, r)
	}
	if err := rows.Err(); err != nil {
		return sum, err
	}
	rows2, err := u.pool.Query(ctx,
		`SELECT date_trunc('day', created_at)::date, COALESCE(SUM(input_tokens),0)::int,
		        COALESCE(SUM(output_tokens),0)::int, COALESCE(SUM(cost_micro_usd),0)
		 FROM usage_events
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY 1 ORDER BY 1`, userID, days)
	if err != nil {
		return sum, fmt.Errorf("usage daily: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var r DailyRow
		if err := rows2.Scan(&r.Day, &r.InputTokens, &r.OutputTokens, &r.CostMicros); err != nil {
			return sum, err
		}
		sum.Daily = append(sum.Daily, r)
	}
	if err := rows2.Err(); err != nil {
		return sum, err
	}
	rows3, err := u.pool.Query(ctx,
		`SELECT u.document_id, COALESCE(MIN(d.filename), '(deleted)'),
		        COALESCE(SUM(u.input_tokens),0)::int, COALESCE(SUM(u.cost_micro_usd),0)
		 FROM usage_events u LEFT JOIN documents d ON d.id = u.document_id
		 WHERE u.user_id = $1 AND u.kind = 'embedding' AND u.document_id IS NOT NULL
		   AND u.created_at > now() - make_interval(days => $2)
		 GROUP BY u.document_id ORDER BY 4 DESC`, userID, days)
	if err != nil {
		return sum, fmt.Errorf("usage documents: %w", err)
	}
	defer rows3.Close()
	for rows3.Next() {
		var r DocumentSpend
		if err := rows3.Scan(&r.DocumentID, &r.Filename, &r.InputTokens, &r.CostMicros); err != nil {
			return sum, err
		}
		sum.Documents = append(sum.Documents, r)
	}
	if err := rows3.Err(); err != nil {
		return sum, err
	}

	// Search queries breakdown and analysis
	rowsSearchTotals, err := u.pool.Query(ctx,
		`SELECT kind, COUNT(*)::int
		 FROM search_queries
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY kind`, userID, days)
	if err == nil {
		defer rowsSearchTotals.Close()
		for rowsSearchTotals.Next() {
			var k string
			var count int
			if err := rowsSearchTotals.Scan(&k, &count); err == nil {
				sum.Searches.TotalQueries += count
				if k == "web_search" {
					sum.Searches.WebQueries += count
				} else if k == "document_search" {
					sum.Searches.DocQueries += count
				}
			}
		}
	}

	rowsProviders, err := u.pool.Query(ctx,
		`SELECT provider, COUNT(*)::int
		 FROM search_queries
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY provider ORDER BY 2 DESC`, userID, days)
	if err == nil {
		defer rowsProviders.Close()
		for rowsProviders.Next() {
			var pc SearchProviderCount
			if err := rowsProviders.Scan(&pc.Provider, &pc.Count); err == nil {
				sum.Searches.ByProvider = append(sum.Searches.ByProvider, pc)
			}
		}
	}

	rowsSearchDaily, err := u.pool.Query(ctx,
		`SELECT date_trunc('day', created_at)::date, COUNT(*)::int
		 FROM search_queries
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY 1 ORDER BY 1`, userID, days)
	if err == nil {
		defer rowsSearchDaily.Close()
		for rowsSearchDaily.Next() {
			var t time.Time
			var count int
			if err := rowsSearchDaily.Scan(&t, &count); err == nil {
				sum.Searches.Daily = append(sum.Searches.Daily, SearchDailyCount{
					Day:   t.Format("2006-01-02"),
					Count: count,
				})
			}
		}
	}

	rowsRecent, err := u.pool.Query(ctx,
		`SELECT id, query, kind, provider, results_count, duration_ms, created_at, COALESCE(conversation_id::text, '')
		 FROM search_queries
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 ORDER BY created_at DESC LIMIT 20`, userID, days)
	if err == nil {
		defer rowsRecent.Close()
		for rowsRecent.Next() {
			var qi SearchQueryItem
			if err := rowsRecent.Scan(&qi.ID, &qi.Query, &qi.Kind, &qi.Provider, &qi.ResultsCount, &qi.DurationMs, &qi.CreatedAt, &qi.ConversationID); err == nil {
				sum.Searches.Recent = append(sum.Searches.Recent, qi)
			}
		}
	}

	rowsTop, err := u.pool.Query(ctx,
		`SELECT query, COUNT(*)::int
		 FROM search_queries
		 WHERE user_id = $1 AND created_at > now() - make_interval(days => $2)
		 GROUP BY query ORDER BY 2 DESC LIMIT 10`, userID, days)
	if err == nil {
		defer rowsTop.Close()
		for rowsTop.Next() {
			var qc SearchQueryCount
			if err := rowsTop.Scan(&qc.Query, &qc.Count); err == nil {
				sum.Searches.TopQueries = append(sum.Searches.TopQueries, qc)
			}
		}
	}

	return sum, nil
}
