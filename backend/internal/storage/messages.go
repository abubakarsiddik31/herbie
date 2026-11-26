package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Message struct {
	ID             string
	ConversationID string
	UserID         string
	// ParentID chains every stored row into the conversation's message tree;
	// empty means the row is a root (the first prompt of a conversation).
	ParentID string
	// ReplyToID links a user row to the message it quotes; empty = no quote.
	// The FK clears it when the quoted row is deleted, while QuoteText keeps
	// the server-side snapshot the model saw.
	ReplyToID string
	QuoteText string
	// ClientMessageID is the client's idempotency key for a user prompt.
	ClientMessageID string
	Role            string
	Content         string
	Data            []byte // golem model.Message JSON (durable additive-only)
	Sources         []byte // cited chunks as the sources SSE rows ([] when the run never searched)
	InputTokens     int
	OutputTokens    int
	Requests        int
	CostMicros      int64
	Truncated       bool
	Model           string // answering model, stamped on the last assistant row
	CreatedAt       time.Time
}

type Messages struct{ pool *pgxpool.Pool }

func NewMessages(pool *pgxpool.Pool) *Messages { return &Messages{pool: pool} }

// Add stores one message row and returns its generated ID. ParentID is
// written as NULL when empty (a conversation's first prompt).
func (m *Messages) Add(ctx context.Context, msg Message) (string, error) {
	if len(msg.Sources) == 0 {
		msg.Sources = []byte("[]")
	}
	msg.Content = SanitizeText(msg.Content)
	msg.Model = SanitizeText(msg.Model)
	msg.Data = SanitizeJSONBytes(msg.Data)
	msg.Sources = SanitizeJSONBytes(msg.Sources)
	msg.QuoteText = SanitizeText(msg.QuoteText)
	var parent, replyTo, quote, clientID any
	if msg.ParentID != "" {
		parent = msg.ParentID
	}
	if msg.ReplyToID != "" {
		replyTo = msg.ReplyToID
	}
	if msg.QuoteText != "" {
		quote = msg.QuoteText
	}
	if msg.ClientMessageID != "" {
		clientID = msg.ClientMessageID
	}
	var id string
	err := m.pool.QueryRow(ctx,
		`INSERT INTO messages
		 (conversation_id, user_id, role, content, data, sources, input_tokens, output_tokens, requests, cost_micro_usd, truncated, model, parent_id, reply_to_id, quote_text, client_message_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 RETURNING id`,
		msg.ConversationID, msg.UserID, msg.Role, msg.Content, msg.Data, msg.Sources,
		msg.InputTokens, msg.OutputTokens, msg.Requests, msg.CostMicros, msg.Truncated, msg.Model, parent, replyTo, quote, clientID).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_messages_client_id" {
			return "", ErrDuplicate
		}
		return "", fmt.Errorf("add message: %w", err)
	}
	return id, nil
}

// messageTreeColumns is the recursive walk's explicit column list. The final
// SELECT still casts the ids for scanning; the CTE keeps parent/reply ids as
// uuid so the join and UNION type-check.
const messageTreeColumns = `id, conversation_id, user_id, parent_id, reply_to_id, quote_text, client_message_id, role, content, data, sources, input_tokens, output_tokens, requests, cost_micro_usd, truncated, model, created_at`

// prefixed qualifies every comma-separated column in cols with prefix.
func prefixed(prefix, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

const messageColumns = `id, conversation_id::text, user_id::text, COALESCE(parent_id::text, '') AS parent_id,
		       COALESCE(reply_to_id::text, '') AS reply_to_id, COALESCE(quote_text, '') AS quote_text,
		       COALESCE(client_message_id, '') AS client_message_id,
		       role, content, data, sources,
		       input_tokens, output_tokens, requests, cost_micro_usd, truncated, model, created_at`

func scanMessage(row pgx.Row) (Message, error) {
	var msg Message
	err := row.Scan(&msg.ID, &msg.ConversationID, &msg.UserID, &msg.ParentID, &msg.ReplyToID, &msg.QuoteText,
		&msg.ClientMessageID, &msg.Role, &msg.Content, &msg.Data, &msg.Sources, &msg.InputTokens, &msg.OutputTokens, &msg.Requests, &msg.CostMicros,
		&msg.Truncated, &msg.Model, &msg.CreatedAt)
	return msg, err
}

func (m *Messages) ForConversation(ctx context.Context, convID, userID string) ([]Message, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT `+messageColumns+`
		 FROM messages WHERE conversation_id = $1 AND user_id = $2
		 ORDER BY created_at, id`, convID, userID)
	if err != nil {
		return nil, fmt.Errorf("messages for conversation: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

// ByID loads one message row owned by userID.
func (m *Messages) ByID(ctx context.Context, msgID, userID string) (Message, error) {
	msg, err := scanMessage(m.pool.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE id = $1 AND user_id = $2`, msgID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}
		return Message{}, fmt.Errorf("message by id: %w", err)
	}
	return msg, nil
}

// ByClientMessageID finds a user prompt by its idempotency key.
func (m *Messages) ByClientMessageID(ctx context.Context, convID, userID, clientID string) (Message, error) {
	if clientID == "" {
		return Message{}, ErrNotFound
	}
	msg, err := scanMessage(m.pool.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages
		 WHERE conversation_id = $1 AND user_id = $2 AND client_message_id = $3`,
		convID, userID, clientID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrNotFound
		}
		return Message{}, fmt.Errorf("message by client id: %w", err)
	}
	return msg, nil
}

// PathToLeaf walks parent links upward from leafID and returns the chain in
// root→leaf order. UNION makes the walk cycle-safe: a corrupted parent loop
// terminates after each row once, never recursing forever. Unknown or foreign
// leaves yield an empty path.
func (m *Messages) PathToLeaf(ctx context.Context, convID, userID, leafID string) ([]Message, error) {
	if leafID == "" {
		return nil, nil
	}
	rows, err := m.pool.Query(ctx,
		`WITH RECURSIVE path AS (
		   SELECT `+messageTreeColumns+` FROM messages
		   WHERE id = $3 AND conversation_id = $1 AND user_id = $2
		   UNION
		   SELECT `+prefixed("m.", messageTreeColumns)+` FROM messages m
		   JOIN path p ON m.id = p.parent_id
		   WHERE m.conversation_id = $1 AND m.user_id = $2
		 )
		 SELECT `+messageColumns+` FROM path ORDER BY created_at, id`, convID, userID, leafID)
	if err != nil {
		return nil, fmt.Errorf("path to leaf: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

// LatestRowID returns the newest row's ID by the conversation's canonical
// (created_at, id) order, or "" when the conversation has no messages.
func (m *Messages) LatestRowID(ctx context.Context, convID, userID string) (string, error) {
	var id string
	err := m.pool.QueryRow(ctx,
		`SELECT id FROM messages WHERE conversation_id = $1 AND user_id = $2
		 ORDER BY created_at DESC, id DESC LIMIT 1`, convID, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("latest message id: %w", err)
	}
	return id, nil
}

// DeleteBranch deletes msgID's whole subtree in one transaction. When the
// conversation's active leaf is inside that subtree the leaf falls back to
// the deleted root's parent (possibly NULL); deleting an inactive branch
// leaves the active leaf untouched. Unknown or foreign rows are ErrNotFound.
func (m *Messages) DeleteBranch(ctx context.Context, convID, msgID, userID string) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete branch: %w", err)
	}
	defer tx.Rollback(ctx)

	var parent *string
	err = tx.QueryRow(ctx,
		`SELECT parent_id::text FROM messages
		 WHERE id = $1 AND conversation_id = $2 AND user_id = $3`, msgID, convID, userID).Scan(&parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete branch lookup: %w", err)
	}

	// Move the leaf only when it belongs to the subtree being removed.
	if _, err := tx.Exec(ctx,
		`WITH RECURSIVE subtree AS (
		   SELECT id FROM messages WHERE id = $2 AND conversation_id = $1 AND user_id = $3
		   UNION
		   SELECT m.id FROM messages m JOIN subtree s ON m.parent_id = s.id
		 )
		 UPDATE conversations SET active_leaf_id = $4
		 WHERE id = $1 AND user_id = $3 AND active_leaf_id IN (SELECT id FROM subtree)`,
		convID, msgID, userID, parent); err != nil {
		return fmt.Errorf("delete branch leaf fallback: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`WITH RECURSIVE subtree AS (
		   SELECT id FROM messages WHERE id = $2 AND conversation_id = $1 AND user_id = $3
		   UNION
		   SELECT m.id FROM messages m JOIN subtree s ON m.parent_id = s.id
		 )
		 DELETE FROM messages
		 WHERE conversation_id = $1 AND user_id = $3 AND id IN (SELECT id FROM subtree)`,
		convID, msgID, userID); err != nil {
		return fmt.Errorf("delete branch: %w", err)
	}
	return tx.Commit(ctx)
}

// CopyPath inserts a display-only copy of path into newConvID under fresh
// IDs, remapping each row's parent to its copied ancestor. It returns the
// copied tail's ID ("" for an empty path). No ledger events are written: a
// fork is a view of already-metered history, not new spend.
func (m *Messages) CopyPath(ctx context.Context, srcConvID, userID, newConvID string, path []Message) (string, error) {
	if len(path) == 0 {
		return "", nil
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin copy path: %w", err)
	}
	defer tx.Rollback(ctx)

	remap := make(map[string]string, len(path))
	tail := ""
	for _, msg := range path {
		if msg.ConversationID != srcConvID || msg.UserID != userID {
			return "", ErrNotFound
		}
		var parent, replyTo any
		if msg.ParentID != "" {
			if mapped, ok := remap[msg.ParentID]; ok {
				parent = mapped
			}
		}
		// A quote target outside the copied path has no counterpart in the
		// fork; keep the snapshot but drop the dangling pointer.
		if msg.ReplyToID != "" {
			if mapped, ok := remap[msg.ReplyToID]; ok {
				replyTo = mapped
			}
		}
		sources := msg.Sources
		if len(sources) == 0 {
			sources = []byte("[]")
		}
		quoteText := SanitizeText(msg.QuoteText)
		var quote any
		if quoteText != "" {
			quote = quoteText
		}
		var id string
		err := tx.QueryRow(ctx,
			`INSERT INTO messages
			 (conversation_id, user_id, parent_id, reply_to_id, quote_text, role, content, data, sources,
			  input_tokens, output_tokens, requests, cost_micro_usd, truncated, model, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			 RETURNING id`,
			newConvID, userID, parent, replyTo, quote, msg.Role, SanitizeText(msg.Content), SanitizeJSONBytes(msg.Data),
			SanitizeJSONBytes(sources), msg.InputTokens, msg.OutputTokens, msg.Requests, msg.CostMicros,
			msg.Truncated, SanitizeText(msg.Model), msg.CreatedAt).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("copy path row: %w", err)
		}
		remap[msg.ID] = id
		tail = id
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit copy path: %w", err)
	}
	return tail, nil
}
