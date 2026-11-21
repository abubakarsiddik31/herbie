package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ModelCatalogRow is one admin-managed model entry.
type ModelCatalogRow struct {
	ID           string
	Provider     string
	Label        string
	InputPerM    float64
	OutputPerM   float64
	Enabled      bool
	Capabilities []byte
	SortOrder    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ModelCatalog struct{ pool *pgxpool.Pool }

func NewModelCatalog(pool *pgxpool.Pool) *ModelCatalog { return &ModelCatalog{pool: pool} }

const modelCatalogColumns = `id, provider, label, input_per_m, output_per_m, enabled, capabilities, sort_order, created_at, updated_at`

func scanModelCatalog(row pgx.Row) (ModelCatalogRow, error) {
	var m ModelCatalogRow
	err := row.Scan(&m.ID, &m.Provider, &m.Label, &m.InputPerM, &m.OutputPerM, &m.Enabled, &m.Capabilities, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (s *ModelCatalog) List(ctx context.Context) ([]ModelCatalogRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+modelCatalogColumns+` FROM model_catalog ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("list model catalog: %w", err)
	}
	defer rows.Close()
	var out []ModelCatalogRow
	for rows.Next() {
		m, err := scanModelCatalog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan model catalog: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *ModelCatalog) Upsert(ctx context.Context, m ModelCatalogRow) (ModelCatalogRow, error) {
	if len(m.Capabilities) == 0 {
		m.Capabilities = []byte("{}")
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO model_catalog (id, provider, label, input_per_m, output_per_m, enabled, capabilities, sort_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (id) DO UPDATE SET provider = EXCLUDED.provider, label = EXCLUDED.label,
		   input_per_m = EXCLUDED.input_per_m, output_per_m = EXCLUDED.output_per_m,
		   enabled = EXCLUDED.enabled, capabilities = EXCLUDED.capabilities,
		   sort_order = EXCLUDED.sort_order, updated_at = now()
		 RETURNING `+modelCatalogColumns,
		m.ID, m.Provider, m.Label, m.InputPerM, m.OutputPerM, m.Enabled, SanitizeJSONBytes(m.Capabilities), m.SortOrder)
	out, err := scanModelCatalog(row)
	if err != nil {
		return ModelCatalogRow{}, fmt.Errorf("upsert model catalog: %w", err)
	}
	return out, nil
}

func (s *ModelCatalog) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM model_catalog WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete model catalog: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
