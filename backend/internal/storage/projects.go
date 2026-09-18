package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Project struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Instructions string    `json:"instructions"`
	FilesCount   int       `json:"filesCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type ProjectPatch struct {
	Name         *string
	Description  *string
	Instructions *string
}

type Projects struct {
	pool *pgxpool.Pool
}

func NewProjects(pool *pgxpool.Pool) *Projects {
	return &Projects{pool: pool}
}

const projectColumns = `p.id, p.user_id::text, p.name, p.description, p.instructions,
	(SELECT COUNT(*) FROM documents d WHERE d.project_id = p.id) AS files_count,
	p.created_at, p.updated_at`

func scanProject(row pgx.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.Instructions,
		&p.FilesCount, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (pr *Projects) Create(ctx context.Context, p Project) (Project, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	row := pr.pool.QueryRow(ctx,
		`INSERT INTO projects (id, user_id, name, description, instructions)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id::text, name, description, instructions, 0, created_at, updated_at`,
		p.ID, p.UserID, p.Name, p.Description, p.Instructions)
	out, err := scanProject(row)
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return out, nil
}

func (pr *Projects) Get(ctx context.Context, id, userID string) (Project, error) {
	row := pr.pool.QueryRow(ctx,
		`SELECT `+projectColumns+` FROM projects p WHERE p.id = $1 AND p.user_id = $2`,
		id, userID)
	p, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (pr *Projects) List(ctx context.Context, userID string) ([]Project, error) {
	rows, err := pr.pool.Query(ctx,
		`SELECT `+projectColumns+` FROM projects p
		 WHERE p.user_id = $1
		 ORDER BY p.updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (pr *Projects) Update(ctx context.Context, id, userID string, patch ProjectPatch) (Project, error) {
	sets := []string{"updated_at = now()"}
	args := []any{id, userID}
	if patch.Name != nil {
		args = append(args, *patch.Name)
		sets = append(sets, fmt.Sprintf("name = $%d", len(args)))
	}
	if patch.Description != nil {
		args = append(args, *patch.Description)
		sets = append(sets, fmt.Sprintf("description = $%d", len(args)))
	}
	if patch.Instructions != nil {
		args = append(args, *patch.Instructions)
		sets = append(sets, fmt.Sprintf("instructions = $%d", len(args)))
	}
	query := fmt.Sprintf(
		`UPDATE projects SET %s WHERE id = $1 AND user_id = $2
		 RETURNING id, user_id::text, name, description, instructions,
		 (SELECT COUNT(*) FROM documents d WHERE d.project_id = projects.id),
		 created_at, updated_at`,
		strings.Join(sets, ", "))
	row := pr.pool.QueryRow(ctx, query, args...)
	out, err := scanProject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	return out, nil
}

func (pr *Projects) Delete(ctx context.Context, id, userID string) error {
	tag, err := pr.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
