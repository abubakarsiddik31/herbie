package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserTool is one user-defined HTTP API tool. Params and Headers stay raw
// JSON here; the chat package owns their shape (chat.ToolConfig).
type UserTool struct {
	ID              string
	UserID          string
	Name            string
	Description     string
	Method          string
	URLTemplate     string
	Params          json.RawMessage
	BodyTemplate    string
	Headers         json.RawMessage
	RequireApproval bool
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Tools struct{ pool *pgxpool.Pool }

func NewTools(pool *pgxpool.Pool) *Tools { return &Tools{pool: pool} }

const toolColumns = `id, user_id::text, name, description, method, url_template,
 params, body_template, headers, require_approval, enabled, created_at, updated_at`

func scanTool(row pgx.Row) (UserTool, error) {
	var t UserTool
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Description, &t.Method, &t.URLTemplate,
		&t.Params, &t.BodyTemplate, &t.Headers, &t.RequireApproval, &t.Enabled, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *Tools) Create(ctx context.Context, t UserTool) (UserTool, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO tools (user_id, name, description, method, url_template, params, body_template, headers, require_approval, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING `+toolColumns,
		t.UserID, t.Name, t.Description, t.Method, t.URLTemplate, t.Params, t.BodyTemplate,
		t.Headers, t.RequireApproval, t.Enabled)
	out, err := scanTool(row)
	if err != nil {
		if isDuplicate(err) {
			return UserTool{}, ErrDuplicate
		}
		return UserTool{}, fmt.Errorf("create tool: %w", err)
	}
	return out, nil
}

func (s *Tools) List(ctx context.Context, userID string) ([]UserTool, error) {
	return s.list(ctx, userID, false)
}

func (s *Tools) ListEnabled(ctx context.Context, userID string) ([]UserTool, error) {
	return s.list(ctx, userID, true)
}

func (s *Tools) list(ctx context.Context, userID string, enabledOnly bool) ([]UserTool, error) {
	q := `SELECT ` + toolColumns + ` FROM tools WHERE user_id = $1`
	if enabledOnly {
		q += ` AND enabled`
	}
	q += ` ORDER BY name`
	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	defer rows.Close()
	var out []UserTool
	for rows.Next() {
		t, err := scanTool(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Tools) ByID(ctx context.Context, id, userID string) (UserTool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+toolColumns+` FROM tools WHERE id = $1 AND user_id = $2`, id, userID)
	t, err := scanTool(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserTool{}, ErrNotFound
		}
		return UserTool{}, fmt.Errorf("tool by id: %w", err)
	}
	return t, nil
}

// Update replaces the editable fields of the row identified by t.ID and t.UserID.
func (s *Tools) Update(ctx context.Context, t UserTool) (UserTool, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE tools SET name=$3, description=$4, method=$5, url_template=$6, params=$7,
		 body_template=$8, headers=$9, require_approval=$10, enabled=$11, updated_at=now()
		 WHERE id=$1 AND user_id=$2
		 RETURNING `+toolColumns,
		t.ID, t.UserID, t.Name, t.Description, t.Method, t.URLTemplate, t.Params,
		t.BodyTemplate, t.Headers, t.RequireApproval, t.Enabled)
	out, err := scanTool(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserTool{}, ErrNotFound
		}
		if isDuplicate(err) {
			return UserTool{}, ErrDuplicate
		}
		return UserTool{}, fmt.Errorf("update tool: %w", err)
	}
	return out, nil
}

func (s *Tools) Delete(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM tools WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete tool: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Tools) Count(ctx context.Context, userID string) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM tools WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count tools: %w", err)
	}
	return n, nil
}
