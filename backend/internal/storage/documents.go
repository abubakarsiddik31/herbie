package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Document is one uploaded file and its ingestion state. Status runs
// processing → ready, or processing → failed with Error set; the original
// object stays in the object store either way so re-ingestion never needs
// a re-upload.
type Document struct {
	ID         string
	UserID     string
	ObjectKey  string
	Filename   string
	Mime       string
	SizeBytes  int64
	Status     string // processing | ready | failed
	Error      string
	ChunkCount int
	CreatedAt  time.Time
}

type Documents struct{ pool *pgxpool.Pool }

func NewDocuments(pool *pgxpool.Pool) *Documents { return &Documents{pool: pool} }

const documentColumns = `id, user_id::text, object_key, filename, mime, size_bytes, status, COALESCE(error, ''), chunk_count, created_at`

func scanDocument(row pgx.Row) (Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.UserID, &d.ObjectKey, &d.Filename, &d.Mime, &d.SizeBytes,
		&d.Status, &d.Error, &d.ChunkCount, &d.CreatedAt)
	return d, err
}

func (d *Documents) Create(ctx context.Context, doc Document) (Document, error) {
	row := d.pool.QueryRow(ctx,
		`INSERT INTO documents (user_id, object_key, filename, mime, size_bytes) VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+documentColumns,
		doc.UserID, doc.ObjectKey, doc.Filename, doc.Mime, doc.SizeBytes)
	out, err := scanDocument(row)
	if err != nil {
		return Document{}, fmt.Errorf("create document: %w", err)
	}
	return out, nil
}

func (d *Documents) Get(ctx context.Context, id, userID string) (Document, error) {
	row := d.pool.QueryRow(ctx,
		`SELECT `+documentColumns+` FROM documents WHERE id = $1 AND user_id = $2`, id, userID)
	doc, err := scanDocument(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("get document: %w", err)
	}
	return doc, nil
}

func (d *Documents) List(ctx context.Context, userID string) ([]Document, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT `+documentColumns+` FROM documents WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("scan document: %w", err)
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

// SetStatus moves a document out of processing. On failure Error carries the
// model-visible reason; on success ChunkCount is the indexed chunk total.
func (d *Documents) SetStatus(ctx context.Context, id, userID, status, errMsg string, chunkCount int) error {
	tag, err := d.pool.Exec(ctx,
		`UPDATE documents SET status = $3, error = NULLIF($4, ''), chunk_count = $5
		 WHERE id = $1 AND user_id = $2`,
		id, userID, status, errMsg, chunkCount)
	if err != nil {
		return fmt.Errorf("set document status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *Documents) Delete(ctx context.Context, id, userID string) error {
	tag, err := d.pool.Exec(ctx,
		`DELETE FROM documents WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
