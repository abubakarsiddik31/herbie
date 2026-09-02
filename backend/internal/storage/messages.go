package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Message struct {
	ID             string
	ConversationID string
	UserID         string
	Role           string
	Content        string
	Data           []byte // golem model.Message JSON (durable additive-only)
	InputTokens    int
	OutputTokens   int
	Requests       int
	CostMicros     int64
	Truncated      bool
	Model          string // answering model, stamped on the last assistant row
	CreatedAt      time.Time
}

type Messages struct{ pool *pgxpool.Pool }

func NewMessages(pool *pgxpool.Pool) *Messages { return &Messages{pool: pool} }

func (m *Messages) Add(ctx context.Context, msg Message) error {
	_, err := m.pool.Exec(ctx,
		`INSERT INTO messages
		 (conversation_id, user_id, role, content, data, input_tokens, output_tokens, requests, cost_micro_usd, truncated, model)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		msg.ConversationID, msg.UserID, msg.Role, msg.Content, msg.Data,
		msg.InputTokens, msg.OutputTokens, msg.Requests, msg.CostMicros, msg.Truncated, msg.Model)
	if err != nil {
		return fmt.Errorf("add message: %w", err)
	}
	return nil
}

func (m *Messages) ForConversation(ctx context.Context, convID, userID string) ([]Message, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, conversation_id::text, user_id::text, role, content, data,
		        input_tokens, output_tokens, requests, cost_micro_usd, truncated, model, created_at
		 FROM messages WHERE conversation_id = $1 AND user_id = $2
		 ORDER BY created_at, id`, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("messages for conversation: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.UserID, &msg.Role, &msg.Content,
			&msg.Data, &msg.InputTokens, &msg.OutputTokens, &msg.Requests, &msg.CostMicros,
			&msg.Truncated, &msg.Model, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}
