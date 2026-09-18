package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Conversation struct {
	ID     string
	UserID string
	Title  string
	Model  string // "" = server default
	// Temperature nil = provider default.
	Temperature  *float64
	SystemPrompt string // "" = built-in prompt
	RagEnabled   bool   // per-conversation document search (default true)
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ConversationPatch carries optional conversation changes. Pointer fields
// left nil are left unchanged (SetSettings) or fall back to defaults
// (Create). ClearTemperature resets the conversation to the provider-default
// temperature (NULL), which Temperature being nil cannot express.
type ConversationPatch struct {
	Model            *string
	Temperature      *float64
	SystemPrompt     *string
	RagEnabled       *bool
	ClearTemperature bool
}

type Conversations struct{ pool *pgxpool.Pool }

func NewConversations(pool *pgxpool.Pool) *Conversations { return &Conversations{pool: pool} }

const conversationColumns = `id, user_id::text, title, model, temperature, system_prompt, rag_enabled, created_at, updated_at`

func scanConversation(row pgx.Row) (Conversation, error) {
	var conv Conversation
	err := row.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.Model, &conv.Temperature,
		&conv.SystemPrompt, &conv.RagEnabled, &conv.CreatedAt, &conv.UpdatedAt)
	return conv, err
}

func (c *Conversations) Create(ctx context.Context, userID, title string, patch ConversationPatch) (Conversation, error) {
	row := c.pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title, model, temperature, system_prompt, rag_enabled) VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+conversationColumns,
		userID, title, derefString(patch.Model), patch.Temperature, derefString(patch.SystemPrompt), derefBool(patch.RagEnabled, true))
	conv, err := scanConversation(row)
	if err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

func (c *Conversations) List(ctx context.Context, userID, q string) ([]Conversation, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT `+conversationColumns+` FROM conversations
		 WHERE user_id = $1 AND ($2 = '' OR title ILIKE '%' || $2 || '%' ESCAPE '\')
		 ORDER BY updated_at DESC`, userID, escapeLike(q))
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		conv, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, conv)
	}
	return out, rows.Err()
}

func (c *Conversations) ByID(ctx context.Context, id, userID string) (Conversation, error) {
	conv, err := scanConversation(c.pool.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM conversations WHERE id = $1 AND user_id = $2`, id, userID))
	if err != nil {
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

// SetSettings applies the non-nil fields of patch to one conversation.
// ClearTemperature wins over Temperature.
func (c *Conversations) SetSettings(ctx context.Context, id, userID string, patch ConversationPatch) error {
	var sets []string
	args := []any{id, userID}
	if patch.Model != nil {
		args = append(args, *patch.Model)
		sets = append(sets, fmt.Sprintf("model = $%d", len(args)))
	}
	if patch.ClearTemperature {
		sets = append(sets, "temperature = NULL")
	} else if patch.Temperature != nil {
		args = append(args, *patch.Temperature)
		sets = append(sets, fmt.Sprintf("temperature = $%d", len(args)))
	}
	if patch.SystemPrompt != nil {
		args = append(args, *patch.SystemPrompt)
		sets = append(sets, fmt.Sprintf("system_prompt = $%d", len(args)))
	}
	if patch.RagEnabled != nil {
		args = append(args, *patch.RagEnabled)
		sets = append(sets, fmt.Sprintf("rag_enabled = $%d", len(args)))
	}
	if len(sets) == 0 {
		return nil
	}
	_, err := c.pool.Exec(ctx,
		`UPDATE conversations SET `+strings.Join(sets, ", ")+` WHERE id = $1 AND user_id = $2`, args...)
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

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefBool(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// escapeLike quotes LIKE metacharacters so search terms match literally.
func escapeLike(q string) string {
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, `%`, `\%`)
	return strings.ReplaceAll(q, `_`, `\_`)
}
