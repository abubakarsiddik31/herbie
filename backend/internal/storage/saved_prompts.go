package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SavedPrompt is one reusable composer snippet owned by a user.
type SavedPrompt struct {
	ID        string
	UserID    string
	Title     string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ErrLimit reports that the per-user saved-prompt limit is reached.
var ErrLimit = errors.New("saved prompt limit reached")

type SavedPrompts struct{ pool *pgxpool.Pool }

func NewSavedPrompts(pool *pgxpool.Pool) *SavedPrompts { return &SavedPrompts{pool: pool} }

const savedPromptColumns = `id, user_id::text, title, body, created_at, updated_at`

func scanSavedPrompt(row pgx.Row) (SavedPrompt, error) {
	var p SavedPrompt
	err := row.Scan(&p.ID, &p.UserID, &p.Title, &p.Body, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// Create inserts a prompt under a per-user advisory lock so the limit cannot
// be exceeded by concurrent requests (count-then-insert is a TOCTOU race).
func (s *SavedPrompts) Create(ctx context.Context, userID, title, body string, limit int) (SavedPrompt, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SavedPrompt{}, fmt.Errorf("begin saved prompt: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, userID); err != nil {
		return SavedPrompt{}, fmt.Errorf("lock saved prompts: %w", err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM saved_prompts WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return SavedPrompt{}, fmt.Errorf("count saved prompts: %w", err)
	}
	if limit > 0 && n >= limit {
		return SavedPrompt{}, ErrLimit
	}
	p, err := scanSavedPrompt(tx.QueryRow(ctx,
		`INSERT INTO saved_prompts (user_id, title, body) VALUES ($1, $2, $3)
		 RETURNING `+savedPromptColumns,
		userID, SanitizeText(title), SanitizeText(body)))
	if err != nil {
		return SavedPrompt{}, fmt.Errorf("create saved prompt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SavedPrompt{}, fmt.Errorf("commit saved prompt: %w", err)
	}
	return p, nil
}

func (s *SavedPrompts) List(ctx context.Context, userID string) ([]SavedPrompt, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+savedPromptColumns+` FROM saved_prompts
		 WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list saved prompts: %w", err)
	}
	defer rows.Close()
	var out []SavedPrompt
	for rows.Next() {
		p, err := scanSavedPrompt(rows)
		if err != nil {
			return nil, fmt.Errorf("scan saved prompt: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Update applies the non-nil fields of a prompt; the returned row is the
// stored state. Unknown or foreign IDs surface as ErrNotFound.
func (s *SavedPrompts) Update(ctx context.Context, id, userID string, title, body *string) (SavedPrompt, error) {
	p, err := scanSavedPrompt(s.pool.QueryRow(ctx,
		`UPDATE saved_prompts SET
		   title = COALESCE($3, title),
		   body = COALESCE($4, body),
		   updated_at = now()
		 WHERE id = $1 AND user_id = $2
		 RETURNING `+savedPromptColumns,
		id, userID, sanitizePtr(title), sanitizePtr(body)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SavedPrompt{}, ErrNotFound
		}
		return SavedPrompt{}, fmt.Errorf("update saved prompt: %w", err)
	}
	return p, nil
}

func (s *SavedPrompts) Delete(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM saved_prompts WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete saved prompt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SavedPrompts) Count(ctx context.Context, userID string) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM saved_prompts WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count saved prompts: %w", err)
	}
	return n, nil
}

func sanitizePtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := SanitizeText(*s)
	return &v
}
