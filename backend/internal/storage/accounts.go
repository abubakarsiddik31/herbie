package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExportFile is one original uploaded document included in an account export
// (and needed to remove the matching object-store data on deletion).
type ExportFile struct {
	DocumentID string    `json:"documentId"`
	Filename   string    `json:"filename"`
	Mime       string    `json:"mime"`
	SizeBytes  int64     `json:"sizeBytes"`
	Status     string    `json:"status"`
	ObjectKey  string    `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Accounts owns the account-level data export and deletion. Export streams
// JSON into the caller's writer so a large history is never held in memory.
type Accounts struct{ pool *pgxpool.Pool }

func NewAccounts(pool *pgxpool.Pool) *Accounts { return &Accounts{pool: pool} }

// exportTables is the explicit allowlist of exported collections. Queries
// return one JSON column per row. Secret-bearing columns (password hashes,
// token hashes, encrypted keys, credential/header payloads, webhook secrets)
// are deliberately omitted.
var exportTables = []struct {
	key   string
	query string
}{
	{"conversations", `SELECT row_to_json(t) FROM (
		SELECT * FROM conversations WHERE user_id = $1
	) t`},
	{"messages", `SELECT to_jsonb(t) - 'search_vector' FROM (
		SELECT * FROM messages WHERE user_id = $1
	) t`},
	{"projects", `SELECT row_to_json(t) FROM (
		SELECT * FROM projects WHERE user_id = $1
	) t`},
	{"documents", `SELECT row_to_json(t) FROM (
		SELECT id, filename, mime, size_bytes, status, chunk_count, created_at, project_id
		FROM documents WHERE user_id = $1
	) t`},
	{"memories", `SELECT row_to_json(t) FROM (
		SELECT * FROM user_memories WHERE user_id = $1
	) t`},
	{"savedPrompts", `SELECT row_to_json(t) FROM (
		SELECT * FROM saved_prompts WHERE user_id = $1
	) t`},
	{"bots", `SELECT row_to_json(t) FROM (
		SELECT * FROM bots WHERE user_id = $1
	) t`},
	{"botMemories", `SELECT row_to_json(t) FROM (
		SELECT * FROM bot_memories WHERE user_id = $1
	) t`},
	{"botRuns", `SELECT row_to_json(t) FROM (
		SELECT * FROM bot_runs WHERE user_id = $1
	) t`},
	{"botRoutines", `SELECT row_to_json(t) FROM (
		SELECT * FROM bot_routines WHERE user_id = $1
	) t`},
	{"botFiles", `SELECT row_to_json(t) FROM (
		SELECT * FROM bot_files WHERE user_id = $1
	) t`},
	{"workflows", `SELECT row_to_json(t) FROM (
		SELECT id, name, description, trigger_type, webhook_slug, nodes, edges,
		       expose_as_tool, tool_name, tool_description, is_active, created_at, updated_at
		FROM workflows WHERE user_id = $1
	) t`},
	{"workflowRuns", `SELECT row_to_json(t) FROM (
		SELECT id, workflow_id, status, trigger_source, error, duration_ms, created_at, finished_at
		FROM workflow_runs WHERE user_id = $1
	) t`},
	{"workflowCredentials", `SELECT row_to_json(t) FROM (
		SELECT id, name, type, created_at, updated_at
		FROM workflow_credentials WHERE user_id = $1
	) t`},
	{"tools", `SELECT row_to_json(t) FROM (
		SELECT id, name, description, method, url_template, params, body_template,
		       require_approval, enabled, created_at, updated_at
		FROM tools WHERE user_id = $1
	) t`},
	{"mcpServers", `SELECT row_to_json(t) FROM (
		SELECT id, name, url, transport, enabled, created_at, updated_at
		FROM mcp_servers WHERE user_id = $1
	) t`},
	{"providerKeys", `SELECT row_to_json(t) FROM (
		SELECT provider, created_at, updated_at FROM user_provider_keys WHERE user_id = $1
	) t`},
	{"oauthAccounts", `SELECT row_to_json(t) FROM (
		SELECT provider, created_at FROM oauth_accounts WHERE user_id = $1
	) t`},
	{"usage", `SELECT row_to_json(t) FROM (
		SELECT * FROM usage_events WHERE user_id = $1
	) t`},
	{"searches", `SELECT row_to_json(t) FROM (
		SELECT * FROM search_queries WHERE user_id = $1
	) t`},
	{"feedback", `SELECT row_to_json(t) FROM (
		SELECT * FROM message_feedback WHERE user_id = $1
	) t`},
	{"shares", `SELECT row_to_json(t) FROM (
		SELECT s.conversation_id, s.created_at
		FROM conversation_shares s JOIN conversations c ON c.id = s.conversation_id
		WHERE c.user_id = $1
	) t`},
}

func writeJSONArray(ctx context.Context, pool *pgxpool.Pool, w io.Writer, key, query, userID string) error {
	if _, err := fmt.Fprintf(w, `,"%s":[`, key); err != nil {
		return err
	}
	rows, err := pool.Query(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("export %s: %w", key, err)
	}
	defer rows.Close()
	first := true
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return fmt.Errorf("export %s row: %w", key, err)
		}
		if !first {
			if _, err := w.Write([]byte(",")); err != nil {
				return err
			}
		}
		first = false
		if _, err := w.Write(raw); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("export %s: %w", key, err)
	}
	_, err = w.Write([]byte("]"))
	return err
}

// WriteExport streams one JSON object for the account into w and returns the
// uploaded files whose originals the caller should append to the archive.
func (a *Accounts) WriteExport(ctx context.Context, userID string, w io.Writer) ([]ExportFile, error) {
	if _, err := fmt.Fprintf(w, `{"exportedAt":%q,"profile":`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	var profile []byte
	err := a.pool.QueryRow(ctx,
		`SELECT row_to_json(t) FROM (
		   SELECT id, email::text, role, email_verified_at, default_instructions,
		          default_top_p, default_max_output_tokens, default_reasoning, created_at
		   FROM users WHERE id = $1
		 ) t`, userID).Scan(&profile)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("export profile: %w", err)
	}
	if _, err := w.Write(profile); err != nil {
		return nil, err
	}
	for _, table := range exportTables {
		if err := writeJSONArray(ctx, a.pool, w, table.key, table.query, userID); err != nil {
			return nil, err
		}
	}
	if _, err := w.Write([]byte("}")); err != nil {
		return nil, err
	}

	rows, err := a.pool.Query(ctx,
		`SELECT id, filename, mime, size_bytes, status, object_key, created_at
		 FROM documents WHERE user_id = $1 AND object_key <> '' ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("export files: %w", err)
	}
	defer rows.Close()
	var files []ExportFile
	for rows.Next() {
		var f ExportFile
		if err := rows.Scan(&f.DocumentID, &f.Filename, &f.Mime, &f.SizeBytes, &f.Status, &f.ObjectKey, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan export file: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// Delete verifies the password (when set) and removes the account in one
// transaction. The users table is locked so two concurrent deletions can
// never remove the last admin.
func (a *Accounts) Delete(ctx context.Context, userID, password string) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete account: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock users: %w", err)
	}
	var hash *string
	var role string
	err = tx.QueryRow(ctx, `SELECT password_hash, role FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&hash, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load account: %w", err)
	}
	if hash != nil && auth.CheckPassword(*hash, password) != nil {
		return auth.ErrInvalidPassword
	}
	if role == "admin" {
		var admins int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&admins); err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if admins <= 1 {
			return auth.ErrLastAdmin
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return tx.Commit(ctx)
}
