package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Bot struct {
	ID           string
	UserID       string
	Name         string
	Emoji        string
	Color        string
	Instructions string
	Model        string
	Temperature  *float64
	RagEnabled   bool
	IsArchived   bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// BotPatch carries optional bot changes. Pointer fields left nil are left
// unchanged. ClearTemperature resets the bot to the provider-default
// temperature (NULL), which Temperature being nil cannot express.
type BotPatch struct {
	Name             *string
	Emoji            *string
	Color            *string
	Instructions     *string
	Model            *string
	Temperature      *float64
	ClearTemperature bool
	RagEnabled       *bool
	IsArchived       *bool
}

type Bots struct{ pool *pgxpool.Pool }

func NewBots(pool *pgxpool.Pool) *Bots { return &Bots{pool: pool} }

func (b *Bots) Create(ctx context.Context, bot Bot) (Bot, error) {
	// Zero-valued optional fields fall back to the schema defaults, the same
	// way Conversations.Create applies its defaults in Go. A plain bool cannot
	// express "unspecified", so RAG is on at creation; callers disable it via
	// Update.
	if bot.Emoji == "" {
		bot.Emoji = "🤖"
	}
	if bot.Color == "" {
		bot.Color = "sage"
	}
	bot.RagEnabled = true
	row := b.pool.QueryRow(ctx, `
		INSERT INTO bots (user_id, name, emoji, color, instructions, model, temperature, rag_enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, emoji, color, is_archived, created_at, updated_at`,
		bot.UserID, bot.Name, bot.Emoji, bot.Color, bot.Instructions, bot.Model, bot.Temperature, bot.RagEnabled)
	err := row.Scan(&bot.ID, &bot.Emoji, &bot.Color, &bot.IsArchived, &bot.CreatedAt, &bot.UpdatedAt)
	return bot, err
}

func (b *Bots) Get(ctx context.Context, userID, id string) (Bot, error) {
	return scanBot(b.pool.QueryRow(ctx, `
		SELECT id, user_id, name, emoji, color, instructions, model, temperature, rag_enabled, is_archived, created_at, updated_at
		FROM bots WHERE id=$1 AND user_id=$2`, id, userID))
}

func (b *Bots) List(ctx context.Context, userID string) ([]Bot, error) {
	rows, err := b.pool.Query(ctx, `
		SELECT id, user_id, name, emoji, color, instructions, model, temperature, rag_enabled, is_archived, created_at, updated_at
		FROM bots WHERE user_id=$1 AND is_archived=false ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bot
	for rows.Next() {
		bot, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, bot)
	}
	return out, rows.Err()
}

func (b *Bots) Update(ctx context.Context, userID, id string, p BotPatch) (Bot, error) {
	bot, err := b.Get(ctx, userID, id)
	if err != nil {
		return Bot{}, err
	}
	if p.Name != nil {
		bot.Name = *p.Name
	}
	if p.Emoji != nil {
		bot.Emoji = *p.Emoji
	}
	if p.Color != nil {
		bot.Color = *p.Color
	}
	if p.Instructions != nil {
		bot.Instructions = *p.Instructions
	}
	if p.Model != nil {
		bot.Model = *p.Model
	}
	if p.Temperature != nil {
		bot.Temperature = p.Temperature
	}
	if p.ClearTemperature {
		bot.Temperature = nil
	}
	if p.RagEnabled != nil {
		bot.RagEnabled = *p.RagEnabled
	}
	if p.IsArchived != nil {
		bot.IsArchived = *p.IsArchived
	}
	row := b.pool.QueryRow(ctx, `
		UPDATE bots SET name=$3, emoji=$4, color=$5, instructions=$6, model=$7, temperature=$8, rag_enabled=$9, is_archived=$10, updated_at=now()
		WHERE id=$1 AND user_id=$2 RETURNING created_at, updated_at`,
		id, userID, bot.Name, bot.Emoji, bot.Color, bot.Instructions, bot.Model, bot.Temperature, bot.RagEnabled, bot.IsArchived)
	if err := row.Scan(&bot.CreatedAt, &bot.UpdatedAt); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

func (b *Bots) Delete(ctx context.Context, userID, id string) error {
	tag, err := b.pool.Exec(ctx, `DELETE FROM bots WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (b *Bots) Count(ctx context.Context, userID string) (int, error) {
	var n int
	err := b.pool.QueryRow(ctx, `SELECT count(*) FROM bots WHERE user_id=$1 AND is_archived=false`, userID).Scan(&n)
	return n, err
}

func scanBot(r interface{ Scan(...any) error }) (Bot, error) {
	var bot Bot
	err := r.Scan(&bot.ID, &bot.UserID, &bot.Name, &bot.Emoji, &bot.Color, &bot.Instructions, &bot.Model, &bot.Temperature, &bot.RagEnabled, &bot.IsArchived, &bot.CreatedAt, &bot.UpdatedAt)
	if errors.Is(err, pgxErrNoRows) {
		return Bot{}, ErrNotFound
	}
	return bot, err
}
