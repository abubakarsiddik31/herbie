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

// MCPServer represents a configured Model Context Protocol external server.
type MCPServer struct {
	ID        string          `json:"id"`
	UserID    string          `json:"userId"`
	Name      string          `json:"name"`
	URL       string          `json:"url"`
	Transport string          `json:"transport"` // "http" or "sse"
	Enabled   bool            `json:"enabled"`
	Headers   json.RawMessage `json:"headers"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type MCPServers struct {
	pool *pgxpool.Pool
}

func NewMCPServers(pool *pgxpool.Pool) *MCPServers {
	return &MCPServers{pool: pool}
}

const mcpServerColumns = `id, user_id::text, name, url, transport, enabled, headers, created_at, updated_at`

func scanMCPServer(row pgx.Row) (MCPServer, error) {
	var s MCPServer
	err := row.Scan(&s.ID, &s.UserID, &s.Name, &s.URL, &s.Transport, &s.Enabled, &s.Headers, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (s *MCPServers) Create(ctx context.Context, item MCPServer) (MCPServer, error) {
	if item.Headers == nil {
		item.Headers = json.RawMessage(`{}`)
	}
	if item.Transport == "" {
		item.Transport = "http"
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO mcp_servers (user_id, name, url, transport, enabled, headers)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+mcpServerColumns,
		item.UserID, item.Name, item.URL, item.Transport, item.Enabled, item.Headers,
	)
	out, err := scanMCPServer(row)
	if err != nil {
		if isDuplicate(err) {
			return MCPServer{}, ErrDuplicate
		}
		return MCPServer{}, fmt.Errorf("create mcp server: %w", err)
	}
	return out, nil
}

func (s *MCPServers) List(ctx context.Context, userID string) ([]MCPServer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+mcpServerColumns+` FROM mcp_servers
		 WHERE user_id = $1
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer rows.Close()

	var out []MCPServer
	for rows.Next() {
		item, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *MCPServers) ListEnabled(ctx context.Context, userID string) ([]MCPServer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+mcpServerColumns+` FROM mcp_servers
		 WHERE user_id = $1 AND enabled = true
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled mcp servers: %w", err)
	}
	defer rows.Close()

	var out []MCPServer
	for rows.Next() {
		item, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *MCPServers) ByID(ctx context.Context, id, userID string) (MCPServer, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+mcpServerColumns+` FROM mcp_servers
		 WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	out, err := scanMCPServer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return MCPServer{}, ErrNotFound
	}
	return out, err
}

func (s *MCPServers) Update(ctx context.Context, item MCPServer) (MCPServer, error) {
	if item.Headers == nil {
		item.Headers = json.RawMessage(`{}`)
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE mcp_servers
		 SET name = $3, url = $4, transport = $5, enabled = $6, headers = $7, updated_at = now()
		 WHERE id = $1 AND user_id = $2
		 RETURNING `+mcpServerColumns,
		item.ID, item.UserID, item.Name, item.URL, item.Transport, item.Enabled, item.Headers,
	)
	out, err := scanMCPServer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return MCPServer{}, ErrNotFound
	}
	if err != nil {
		if isDuplicate(err) {
			return MCPServer{}, ErrDuplicate
		}
		return MCPServer{}, fmt.Errorf("update mcp server: %w", err)
	}
	return out, nil
}

func (s *MCPServers) Delete(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete mcp server: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
