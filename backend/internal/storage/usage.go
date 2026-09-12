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
	InputTokens    int
	OutputTokens   int
	Requests       int
	Estimated      bool
	CostMicros     int64
}

type Usage struct{ pool *pgxpool.Pool }

func NewUsage(pool *pgxpool.Pool) *Usage { return &Usage{pool: pool} }

func (u *Usage) Add(ctx context.Context, e UsageEvent) error {
	_, err := u.pool.Exec(ctx,
		`INSERT INTO usage_events
		 (user_id, kind, model, conversation_id, input_tokens, output_tokens, requests, estimated, cost_micro_usd)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		e.UserID, e.Kind, e.Model, e.ConversationID,
		e.InputTokens, e.OutputTokens, e.Requests, e.Estimated, e.CostMicros)
	if err != nil {
		return fmt.Errorf("add usage event: %w", err)
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
	return sum, rows3.Err()
}
