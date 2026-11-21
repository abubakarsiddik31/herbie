package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserProviderKey is one BYOK row: the encrypted key is opaque to storage.
type UserProviderKey struct {
	Provider  string
	Encrypted []byte
	UpdatedAt time.Time
}

type UserProviderKeys struct{ pool *pgxpool.Pool }

func NewUserProviderKeys(pool *pgxpool.Pool) *UserProviderKeys { return &UserProviderKeys{pool: pool} }

func (s *UserProviderKeys) Upsert(ctx context.Context, userID, provider string, encrypted []byte) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_provider_keys (user_id, provider, encrypted_key)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, provider)
		 DO UPDATE SET encrypted_key = EXCLUDED.encrypted_key, updated_at = now()`,
		userID, provider, encrypted)
	if err != nil {
		return fmt.Errorf("upsert provider key: %w", err)
	}
	return nil
}

func (s *UserProviderKeys) Get(ctx context.Context, userID, provider string) ([]byte, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT encrypted_key FROM user_provider_keys WHERE user_id = $1 AND provider = $2`,
		userID, provider).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get provider key: %w", err)
	}
	return raw, nil
}

func (s *UserProviderKeys) List(ctx context.Context, userID string) ([]UserProviderKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT provider, encrypted_key, updated_at FROM user_provider_keys WHERE user_id = $1 ORDER BY provider`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list provider keys: %w", err)
	}
	defer rows.Close()
	var out []UserProviderKey
	for rows.Next() {
		var k UserProviderKey
		if err := rows.Scan(&k.Provider, &k.Encrypted, &k.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan provider key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *UserProviderKeys) Delete(ctx context.Context, userID, provider string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM user_provider_keys WHERE user_id = $1 AND provider = $2`, userID, provider)
	if err != nil {
		return fmt.Errorf("delete provider key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
