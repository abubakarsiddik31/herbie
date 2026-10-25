package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BotMemory struct {
	ID        string
	BotID     string
	UserID    string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type BotMemories struct{ pool *pgxpool.Pool }

func NewBotMemories(pool *pgxpool.Pool) *BotMemories { return &BotMemories{pool: pool} }

func (m *BotMemories) Add(ctx context.Context, mem BotMemory) (BotMemory, error) {
	row := m.pool.QueryRow(ctx, `
		INSERT INTO bot_memories (bot_id, user_id, content) VALUES ($1,$2,$3)
		RETURNING id, created_at, updated_at`, mem.BotID, mem.UserID, mem.Content)
	err := row.Scan(&mem.ID, &mem.CreatedAt, &mem.UpdatedAt)
	return mem, err
}

func (m *BotMemories) ListRecent(ctx context.Context, userID, botID string, limit int) ([]BotMemory, error) {
	rows, err := m.pool.Query(ctx, `
		SELECT id, bot_id, user_id, content, created_at, updated_at
		FROM bot_memories WHERE bot_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT $3`, botID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BotMemory
	for rows.Next() {
		var mem BotMemory
		if err := rows.Scan(&mem.ID, &mem.BotID, &mem.UserID, &mem.Content, &mem.CreatedAt, &mem.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, mem)
	}
	return out, rows.Err()
}

func (m *BotMemories) Delete(ctx context.Context, userID, botID, id string) error {
	tag, err := m.pool.Exec(ctx, `DELETE FROM bot_memories WHERE id=$1 AND bot_id=$2 AND user_id=$3`, id, botID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
