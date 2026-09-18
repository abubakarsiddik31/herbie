package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Message struct {
	ID             string
	ConversationID string
	UserID         string
	Role           string
	Content        string
	Data           []byte // golem model.Message JSON (durable additive-only)
	Sources        []byte // cited chunks as the sources SSE rows ([] when the run never searched)
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
	if len(msg.Sources) == 0 {
		msg.Sources = []byte("[]")
	}
	_, err := m.pool.Exec(ctx,
		`INSERT INTO messages
		 (conversation_id, user_id, role, content, data, sources, input_tokens, output_tokens, requests, cost_micro_usd, truncated, model)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		msg.ConversationID, msg.UserID, msg.Role, msg.Content, msg.Data, msg.Sources,
		msg.InputTokens, msg.OutputTokens, msg.Requests, msg.CostMicros, msg.Truncated, msg.Model)
	if err != nil {
		return fmt.Errorf("add message: %w", err)
	}
	return nil
}

const messageColumns = `id, conversation_id::text, user_id::text, role, content, data, sources,
		       input_tokens, output_tokens, requests, cost_micro_usd, truncated, model, created_at`

func scanMessage(row pgx.Row) (Message, error) {
	var msg Message
	err := row.Scan(&msg.ID, &msg.ConversationID, &msg.UserID, &msg.Role, &msg.Content,
		&msg.Data, &msg.Sources, &msg.InputTokens, &msg.OutputTokens, &msg.Requests, &msg.CostMicros,
		&msg.Truncated, &msg.Model, &msg.CreatedAt)
	return msg, err
}

func (m *Messages) ForConversation(ctx context.Context, convID, userID string) ([]Message, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, conversation_id::text, user_id::text, role, content, data, sources,
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
			&msg.Data, &msg.Sources, &msg.InputTokens, &msg.OutputTokens, &msg.Requests, &msg.CostMicros,
			&msg.Truncated, &msg.Model, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

// ByID loads one message row owned by userID.
func (m *Messages) ByID(ctx context.Context, msgID, userID string) (Message, error) {
	msg, err := scanMessage(m.pool.QueryRow(ctx,
		`SELECT id, conversation_id::text, user_id::text, role, content, data, sources,
		        input_tokens, output_tokens, requests, cost_micro_usd, truncated, model, created_at
		 FROM messages WHERE id = $1 AND user_id = $2`, msgID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}
		return Message{}, fmt.Errorf("message by id: %w", err)
	}
	return msg, nil
}

// UpdateContent rewrites one message's text and stored payload (the row's
// images, if any, are preserved by the caller passing merged data).
func (m *Messages) UpdateContent(ctx context.Context, msgID, userID, content string, data []byte) error {
	tag, err := m.pool.Exec(ctx,
		`UPDATE messages SET content = $3, data = $4 WHERE id = $1 AND user_id = $2`,
		msgID, userID, content, data)
	if err != nil {
		return fmt.Errorf("update message content: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAfter removes every row strictly after (after, afterID) in the
// conversation's (created_at, id) order — the edit-and-resend truncation.
func (m *Messages) DeleteAfter(ctx context.Context, convID, userID string, after time.Time, afterID string) (int64, error) {
	tag, err := m.pool.Exec(ctx,
		`DELETE FROM messages WHERE conversation_id = $1 AND user_id = $2 AND (created_at, id) > ($3, $4)`,
		convID, userID, after, afterID)
	if err != nil {
		return 0, fmt.Errorf("delete messages after: %w", err)
	}
	return tag.RowsAffected(), nil
}
