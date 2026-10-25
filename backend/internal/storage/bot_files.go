package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BotFile is one file in an always-on bot's workspace: a slash-delimited path
// within the bot's namespace and its text content. One row per (bot_id, path);
// upserting the same path overwrites the content in place.
type BotFile struct {
	ID        string
	BotID     string
	UserID    string
	Path      string
	Content   string
	SizeBytes int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type BotFiles struct{ pool *pgxpool.Pool }

func NewBotFiles(pool *pgxpool.Pool) *BotFiles { return &BotFiles{pool: pool} }

// botFileColumns is the full column list used by Upsert and Get; List must
// NOT select content — it returns metadata with Content elided.
const botFileColumns = `id, bot_id, user_id, path, content, size_bytes, created_at, updated_at`

func scanBotFile(row interface{ Scan(...any) error }, file *BotFile) error {
	return row.Scan(&file.ID, &file.BotID, &file.UserID, &file.Path, &file.Content,
		&file.SizeBytes, &file.CreatedAt, &file.UpdatedAt)
}

// Upsert writes the file at (bot_id, path); re-upserting an existing path
// replaces its content and size and refreshes updated_at, keeping the
// original row's id and created_at.
func (f *BotFiles) Upsert(ctx context.Context, file BotFile) (BotFile, error) {
	row := f.pool.QueryRow(ctx, `
		INSERT INTO bot_files (bot_id, user_id, path, content, size_bytes)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (bot_id, path) DO UPDATE
		SET content = EXCLUDED.content,
		    size_bytes = EXCLUDED.size_bytes,
		    updated_at = now()
		RETURNING `+botFileColumns,
		file.BotID, file.UserID, file.Path, file.Content, len(file.Content))
	if err := scanBotFile(row, &file); err != nil {
		return BotFile{}, err
	}
	return file, nil
}

// Get returns the full file, content included.
func (f *BotFiles) Get(ctx context.Context, userID, botID, path string) (BotFile, error) {
	var file BotFile
	if err := scanBotFile(f.pool.QueryRow(ctx,
		`SELECT `+botFileColumns+` FROM bot_files WHERE bot_id=$1 AND user_id=$2 AND path=$3`,
		botID, userID, path), &file); err != nil {
		if errors.Is(err, pgxErrNoRows) {
			return BotFile{}, ErrNotFound
		}
		return BotFile{}, err
	}
	return file, nil
}

// List returns the bot's files ordered by path, WITHOUT content — a listing
// must not pull every file body, so Content is always empty here.
func (f *BotFiles) List(ctx context.Context, userID, botID string) ([]BotFile, error) {
	rows, err := f.pool.Query(ctx, `
		SELECT id, bot_id, user_id, path, size_bytes, created_at, updated_at
		FROM bot_files WHERE bot_id=$1 AND user_id=$2 ORDER BY path`, botID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BotFile
	for rows.Next() {
		var file BotFile
		if err := rows.Scan(&file.ID, &file.BotID, &file.UserID, &file.Path,
			&file.SizeBytes, &file.CreatedAt, &file.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, file)
	}
	return out, rows.Err()
}

// Delete removes the file at (bot_id, path); a missing path maps to
// ErrNotFound.
func (f *BotFiles) Delete(ctx context.Context, userID, botID, path string) error {
	tag, err := f.pool.Exec(ctx, `DELETE FROM bot_files WHERE bot_id=$1 AND user_id=$2 AND path=$3`,
		botID, userID, path)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns how many files the bot currently holds.
func (f *BotFiles) Count(ctx context.Context, botID string) (int, error) {
	var n int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM bot_files WHERE bot_id=$1`, botID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
