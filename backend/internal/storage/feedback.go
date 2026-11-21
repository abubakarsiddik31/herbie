package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MessageFeedback is one user's rating of an assistant message. The handler
// enforces the assistant-row and ownership rules; the store guarantees one
// row per (user, message) via an upsert.
type MessageFeedback struct {
	ID        string
	UserID    string
	MessageID string
	Rating    string // "up" or "down"
}

type Feedback struct{ pool *pgxpool.Pool }

func NewFeedback(pool *pgxpool.Pool) *Feedback { return &Feedback{pool: pool} }

// Set upserts the user's rating for a message.
func (f *Feedback) Set(ctx context.Context, userID, messageID, rating string) error {
	_, err := f.pool.Exec(ctx,
		`INSERT INTO message_feedback (user_id, message_id, rating)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, message_id)
		 DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()`,
		userID, messageID, rating)
	if err != nil {
		return fmt.Errorf("set message feedback: %w", err)
	}
	return nil
}

// Clear removes the user's rating for a message; clearing an unrated message
// is a no-op, not an error.
func (f *Feedback) Clear(ctx context.Context, userID, messageID string) error {
	_, err := f.pool.Exec(ctx,
		`DELETE FROM message_feedback WHERE user_id = $1 AND message_id = $2`,
		userID, messageID)
	if err != nil {
		return fmt.Errorf("clear message feedback: %w", err)
	}
	return nil
}

// ForConversation maps message IDs to this user's ratings for one
// conversation. The join pins both ownership and conversation scope.
func (f *Feedback) ForConversation(ctx context.Context, convID, userID string) (map[string]string, error) {
	rows, err := f.pool.Query(ctx,
		`SELECT f.message_id::text, f.rating
		 FROM message_feedback f
		 JOIN messages m ON m.id = f.message_id
		 WHERE m.conversation_id = $1 AND m.user_id = $2 AND f.user_id = $2`,
		convID, userID)
	if err != nil {
		return nil, fmt.Errorf("feedback for conversation: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var messageID, rating string
		if err := rows.Scan(&messageID, &rating); err != nil {
			return nil, fmt.Errorf("scan message feedback: %w", err)
		}
		out[messageID] = rating
	}
	return out, rows.Err()
}
