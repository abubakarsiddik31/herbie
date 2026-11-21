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
	ID        string
	UserID    string
	ProjectID *string
	// BotID is set for bot-scoped conversations (the always-on teammates);
	// nil for plain and project conversations. Freed (NULL) when the bot is
	// deleted, per the schema's ON DELETE SET NULL.
	BotID *string
	// ActiveLeafID selects the visible path through the message tree; nil
	// until the first message is persisted.
	ActiveLeafID *string
	Title        string
	Model        string // "" = server default
	// Temperature nil = provider default.
	Temperature *float64
	// TopP nil = provider default; MaxOutputTokens 0 = provider default;
	// Reasoning "" = provider default, else low/medium/high.
	TopP            *float64
	MaxOutputTokens int
	Reasoning       string
	SystemPrompt    string // "" = built-in prompt
	RagEnabled      bool   // per-conversation document search (default true)
	// PinnedAt/ArchivedAt are the organization timestamps; nil means the
	// conversation is neither pinned nor archived. TitleSource records who
	// named the conversation: "heuristic" (first message), "auto" (title
	// model), or "manual" (user, bot, or fork). Only heuristic titles may be
	// rewritten automatically.
	PinnedAt    *time.Time
	ArchivedAt  *time.Time
	TitleSource string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ConversationPatch carries optional conversation changes. Pointer fields
// left nil are left unchanged (SetSettings) or fall back to defaults
// (Create). ClearTemperature resets the conversation to the provider-default
// temperature (NULL), which Temperature being nil cannot express.
type ConversationPatch struct {
	Model            *string
	Temperature      *float64
	TopP             *float64
	ClearTopP        bool
	MaxOutputTokens  *int
	Reasoning        *string
	SystemPrompt     *string
	RagEnabled       *bool
	ClearTemperature bool
	ProjectID        *string
	BotID            *string
	// TitleSource overrides the schema's heuristic default at creation
	// ("manual" for bot chats and forks); empty keeps the default.
	TitleSource *string
}

type Conversations struct{ pool *pgxpool.Pool }

func NewConversations(pool *pgxpool.Pool) *Conversations { return &Conversations{pool: pool} }

const conversationColumns = `id, user_id::text, title, model, temperature, top_p, max_output_tokens, reasoning, system_prompt, rag_enabled, created_at, updated_at, project_id::text, bot_id::text, active_leaf_id::text, pinned_at, archived_at, title_source`

func scanConversation(row pgx.Row) (Conversation, error) {
	var conv Conversation
	err := row.Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.Model, &conv.Temperature,
		&conv.TopP, &conv.MaxOutputTokens, &conv.Reasoning,
		&conv.SystemPrompt, &conv.RagEnabled, &conv.CreatedAt, &conv.UpdatedAt, &conv.ProjectID, &conv.BotID, &conv.ActiveLeafID,
		&conv.PinnedAt, &conv.ArchivedAt, &conv.TitleSource)
	return conv, err
}

func (c *Conversations) Create(ctx context.Context, userID, title string, patch ConversationPatch) (Conversation, error) {
	row := c.pool.QueryRow(ctx,
		`INSERT INTO conversations (user_id, title, model, temperature, top_p, max_output_tokens, reasoning, system_prompt, rag_enabled, project_id, bot_id, active_leaf_id, title_source) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL, COALESCE(NULLIF($12, ''), 'heuristic'))
		 RETURNING `+conversationColumns,
		userID, SanitizeText(title), derefString(patch.Model), patch.Temperature, patch.TopP, derefInt(patch.MaxOutputTokens), SanitizeText(derefString(patch.Reasoning)), SanitizeText(derefString(patch.SystemPrompt)), derefBool(patch.RagEnabled, true), patch.ProjectID, patch.BotID, derefString(patch.TitleSource))
	conv, err := scanConversation(row)
	if err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return conv, nil
}

// List returns one view of the user's conversations: by default non-archived
// chats, pinned=true only pinned non-archived chats, archived=true only
// archived chats. q filters titles in every view.
func (c *Conversations) List(ctx context.Context, userID, q string, pinned, archived bool) ([]Conversation, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT `+conversationColumns+` FROM conversations
		 WHERE user_id = $1 AND ($2 = '' OR title ILIKE '%' || $2 || '%' ESCAPE '\')
		   AND (($4 AND archived_at IS NOT NULL)
		        OR (NOT $4 AND archived_at IS NULL AND (NOT $3 OR pinned_at IS NOT NULL)))
		 ORDER BY updated_at DESC`, userID, escapeLike(q), pinned, archived)
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

func (c *Conversations) ListByProject(ctx context.Context, projectID, userID string) ([]Conversation, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT `+conversationColumns+` FROM conversations
		 WHERE user_id = $1 AND project_id = $2 AND archived_at IS NULL
		 ORDER BY updated_at DESC`, userID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project conversations: %w", err)
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		conv, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		out = append(out, conv)
	}
	return out, rows.Err()
}

// ListByBot returns one bot's conversations, most recently updated first.
func (c *Conversations) ListByBot(ctx context.Context, botID, userID string) ([]Conversation, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT `+conversationColumns+` FROM conversations
		 WHERE user_id = $1 AND bot_id = $2 AND archived_at IS NULL
		 ORDER BY updated_at DESC`, userID, botID)
	if err != nil {
		return nil, fmt.Errorf("list bot conversations: %w", err)
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		conv, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		out = append(out, conv)
	}
	return out, rows.Err()
}

// LatestForBot returns the bot's most recently updated conversation, the
// one an always-on bot continues on its next run. ErrNotFound when the bot
// has no conversation yet.
func (c *Conversations) LatestForBot(ctx context.Context, botID, userID string) (Conversation, error) {
	conv, err := scanConversation(c.pool.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM conversations
		 WHERE user_id = $1 AND bot_id = $2 AND archived_at IS NULL
		 ORDER BY updated_at DESC LIMIT 1`, userID, botID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Conversation{}, ErrNotFound
		}
		return Conversation{}, fmt.Errorf("latest bot conversation: %w", err)
	}
	return conv, nil
}

// LatestIDsByUser maps each bot ID the user owns a conversation for to the
// ID of that bot's most recently updated conversation, in one query
// (DISTINCT ON keeps the first row per bot under the updated_at DESC
// ordering).
func (c *Conversations) LatestIDsByUser(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT DISTINCT ON (bot_id) bot_id::text, id::text FROM conversations
		 WHERE user_id = $1 AND bot_id IS NOT NULL AND archived_at IS NULL
		 ORDER BY bot_id, updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("latest conversation ids by bot: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var botID, convID string
		if err := rows.Scan(&botID, &convID); err != nil {
			return nil, fmt.Errorf("scan latest conversation id: %w", err)
		}
		out[botID] = convID
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

// RenameTitle applies a user rename and marks the title manual, so the
// first-message heuristic can never overwrite it later.
func (c *Conversations) RenameTitle(ctx context.Context, id, userID, title string) error {
	tag, err := c.pool.Exec(ctx,
		`UPDATE conversations SET title = $3, title_source = 'manual' WHERE id = $1 AND user_id = $2`, id, userID, SanitizeText(title))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetHeuristicTitle applies the first-message title only while the title is
// still heuristic; a manual rename or a non-heuristic source wins and the
// update becomes a no-op.
func (c *Conversations) SetHeuristicTitle(ctx context.Context, id, userID, title string) error {
	_, err := c.pool.Exec(ctx,
		`UPDATE conversations SET title = $3, title_source = 'heuristic'
		 WHERE id = $1 AND user_id = $2 AND title_source = 'heuristic'`, id, userID, SanitizeText(title))
	return err
}

// SetAutoTitle applies a model-generated title once: it only updates while
// the title is still heuristic, then marks it auto so later turns and retries
// never overwrite it. A manual rename wins by making the UPDATE a no-op.
func (c *Conversations) SetAutoTitle(ctx context.Context, id, userID, title string) error {
	_, err := c.pool.Exec(ctx,
		`UPDATE conversations SET title = $3, title_source = 'auto'
		 WHERE id = $1 AND user_id = $2 AND title_source = 'heuristic'`, id, userID, SanitizeText(title))
	return err
}

// SetPinned stamps or clears pinned_at; RowsAffected==0 means the
// conversation is unknown or belongs to another user.
func (c *Conversations) SetPinned(ctx context.Context, id, userID string, pinned bool) error {
	tag, err := c.pool.Exec(ctx,
		`UPDATE conversations SET pinned_at = CASE WHEN $3 THEN COALESCE(pinned_at, now()) ELSE NULL END
		 WHERE id = $1 AND user_id = $2`, id, userID, pinned)
	if err != nil {
		return fmt.Errorf("set pinned: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetArchived stamps or clears archived_at, mirroring SetPinned.
func (c *Conversations) SetArchived(ctx context.Context, id, userID string, archived bool) error {
	tag, err := c.pool.Exec(ctx,
		`UPDATE conversations SET archived_at = CASE WHEN $3 THEN COALESCE(archived_at, now()) ELSE NULL END
		 WHERE id = $1 AND user_id = $2`, id, userID, archived)
	if err != nil {
		return fmt.Errorf("set archived: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetActiveLeaf points the conversation at the visible message-tree leaf.
// An empty leafID clears the pointer (the conversation has no messages).
func (c *Conversations) SetActiveLeaf(ctx context.Context, id, userID, leafID string) error {
	var leaf any
	if leafID != "" {
		leaf = leafID
	}
	tag, err := c.pool.Exec(ctx,
		`UPDATE conversations SET active_leaf_id = $3 WHERE id = $1 AND user_id = $2`, id, userID, leaf)
	if err != nil {
		return fmt.Errorf("set active leaf: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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
	if patch.ClearTopP {
		sets = append(sets, "top_p = NULL")
	} else if patch.TopP != nil {
		args = append(args, *patch.TopP)
		sets = append(sets, fmt.Sprintf("top_p = $%d", len(args)))
	}
	if patch.MaxOutputTokens != nil {
		args = append(args, *patch.MaxOutputTokens)
		sets = append(sets, fmt.Sprintf("max_output_tokens = $%d", len(args)))
	}
	if patch.Reasoning != nil {
		args = append(args, SanitizeText(*patch.Reasoning))
		sets = append(sets, fmt.Sprintf("reasoning = $%d", len(args)))
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
		// No-op update still reports unknown/foreign rows instead of the
		// old silent 204.
		sets = append(sets, "updated_at = updated_at")
	}
	tag, err := c.pool.Exec(ctx,
		`UPDATE conversations SET `+strings.Join(sets, ", ")+` WHERE id = $1 AND user_id = $2`, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
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
