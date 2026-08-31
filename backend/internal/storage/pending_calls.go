package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PendingToolCall is one deferred tool call awaiting a human decision.
// CallID is golem's tool-call ID from the paused run.
type PendingToolCall struct {
	CallID         string
	ConversationID string
	UserID         string
	ToolName       string
	Args           []byte
	Reason         string
	Status         string // pending | approved | denied
	CreatedAt      time.Time
}

type PendingCalls struct{ pool *pgxpool.Pool }

func NewPendingCalls(pool *pgxpool.Pool) *PendingCalls { return &PendingCalls{pool: pool} }

// Add persists the pending calls of a paused run (one insert per call).
func (p *PendingCalls) Add(ctx context.Context, calls []PendingToolCall) error {
	for _, c := range calls {
		_, err := p.pool.Exec(ctx,
			`INSERT INTO pending_tool_calls (call_id, conversation_id, user_id, tool_name, args, reason)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			c.CallID, c.ConversationID, c.UserID, c.ToolName, c.Args, c.Reason)
		if err != nil {
			return fmt.Errorf("add pending tool call: %w", err)
		}
	}
	return nil
}

// ForConversation returns the conversation's unresolved (pending) calls.
func (p *PendingCalls) ForConversation(ctx context.Context, convID, userID string) ([]PendingToolCall, error) {
	rows, err := p.pool.Query(ctx,
		`SELECT call_id, conversation_id::text, user_id::text, tool_name, args, reason, status, created_at
		 FROM pending_tool_calls
		 WHERE conversation_id = $1 AND user_id = $2 AND status = 'pending'
		 ORDER BY created_at, call_id`, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("pending calls for conversation: %w", err)
	}
	defer rows.Close()
	var out []PendingToolCall
	for rows.Next() {
		var c PendingToolCall
		if err := rows.Scan(&c.CallID, &c.ConversationID, &c.UserID, &c.ToolName, &c.Args, &c.Reason, &c.Status, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetStatus records the decision on one call. Unknown call ID (or one not
// owned by the user) maps to ErrNotFound.
func (p *PendingCalls) SetStatus(ctx context.Context, userID, callID, status string) error {
	tag, err := p.pool.Exec(ctx,
		`UPDATE pending_tool_calls SET status = $3 WHERE call_id = $1 AND user_id = $2`,
		callID, userID, status)
	if err != nil {
		return fmt.Errorf("set pending call status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
