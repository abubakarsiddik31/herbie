package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Conversation struct {
	ID        string
	UserID    string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Conversations struct{ pool *pgxpool.Pool }

func NewConversations(pool *pgxpool.Pool) *Conversations { return &Conversations{pool: pool} }

func (c *Conversations) Create(ctx context.Context, userID, title string) (Conversation, error) {
	row := c.pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title) VALUES ($1, $2)
		 RETURNING id, user_id::text, title, created_at, updated_at`,
		userID, title)
	var conv Conversation
	err := row.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

func (c *Conversations) List(ctx context.Context, userID string) ([]Conversation, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT id, user_id::text, title, created_at, updated_at
		 FROM conversations WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		var conv Conversation
		if err := rows.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, conv)
	}
	return out, rows.Err()
}

func (c *Conversations) ByID(ctx context.Context, id, userID string) (Conversation, error) {
	row := c.pool.QueryRow(ctx,
		`SELECT id, user_id::text, title, created_at, updated_at
		 FROM conversations WHERE id = $1 AND user_id = $2`, id, userID)
	var conv Conversation
	if err := row.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Conversation{}, ErrNotFound
		}
		return Conversation{}, fmt.Errorf("conversation by id: %w", err)
	}
	return conv, nil
}

func (c *Conversations) SetTitle(ctx context.Context, id, userID, title string) error {
	_, err := c.pool.Exec(ctx,
		`UPDATE conversations SET title = $3 WHERE id = $1 AND user_id = $2`, id, userID, title)
	return err
}

func (c *Conversations) Touch(ctx context.Context, id string) error {
	_, err := c.pool.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, id)
	return err
}

func (c *Conversations) Delete(ctx context.Context, id, userID string) error {
	tag, err := c.pool.Exec(ctx,
		`DELETE FROM conversations WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (c *Conversations) CountMessages(ctx context.Context, id, userID string) (int, error) {
	var n int
	err := c.pool.QueryRow(ctx,
		`SELECT count(*) FROM messages WHERE conversation_id = $1 AND user_id = $2`, id, userID).Scan(&n)
	return n, err
}
