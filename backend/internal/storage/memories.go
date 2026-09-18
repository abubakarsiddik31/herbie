package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Memory struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Memories struct{ pool *pgxpool.Pool }

func NewMemories(pool *pgxpool.Pool) *Memories { return &Memories{pool: pool} }

// List returns all remembered facts for the given user, newest first.
func (m *Memories) List(ctx context.Context, userID string) ([]Memory, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id::text, user_id::text, content, created_at, updated_at
		 FROM user_memories
		 WHERE user_id = $1
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	var res []Memory
	for rows.Next() {
		var mem Memory
		if err := rows.Scan(&mem.ID, &mem.UserID, &mem.Content, &mem.CreatedAt, &mem.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		res = append(res, mem)
	}
	return res, rows.Err()
}

// Create stores a new memory for the user.
func (m *Memories) Create(ctx context.Context, userID, content string) (Memory, error) {
	var mem Memory
	err := m.pool.QueryRow(ctx,
		`INSERT INTO user_memories (user_id, content)
		 VALUES ($1, $2)
		 RETURNING id::text, user_id::text, content, created_at, updated_at`,
		userID, content).Scan(&mem.ID, &mem.UserID, &mem.Content, &mem.CreatedAt, &mem.UpdatedAt)
	if err != nil {
		return Memory{}, fmt.Errorf("create memory: %w", err)
	}
	return mem, nil
}

// Delete removes a specific memory if owned by the user.
func (m *Memories) Delete(ctx context.Context, userID, memoryID string) error {
	res, err := m.pool.Exec(ctx,
		`DELETE FROM user_memories WHERE id = $1 AND user_id = $2`, memoryID, userID)
	if err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAll clears all memories for the user.
func (m *Memories) DeleteAll(ctx context.Context, userID string) error {
	_, err := m.pool.Exec(ctx, `DELETE FROM user_memories WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("clear memories: %w", err)
	}
	return nil
}
