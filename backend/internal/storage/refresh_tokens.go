package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ auth.RefreshStore = (*RefreshTokens)(nil)

type RefreshTokens struct{ pool *pgxpool.Pool }

func NewRefreshTokens(pool *pgxpool.Pool) *RefreshTokens { return &RefreshTokens{pool: pool} }

func (r *RefreshTokens) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokens) Get(ctx context.Context, tokenHash string) (auth.RefreshRecord, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT user_id, expires_at, revoked_at, replaced_by FROM refresh_tokens WHERE token_hash = $1`,
		tokenHash)
	var (
		rec        auth.RefreshRecord
		revokedAt  *time.Time
		replacedBy *string
	)
	if err := row.Scan(&rec.UserID, &rec.ExpiresAt, &revokedAt, &replacedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.RefreshRecord{}, auth.ErrInvalidRefresh
		}
		return auth.RefreshRecord{}, fmt.Errorf("get refresh token: %w", err)
	}
	rec.Revoked = revokedAt != nil || replacedBy != nil
	return rec, nil
}

func (r *RefreshTokens) Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rotate: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2 WHERE token_hash = $1 AND revoked_at IS NULL AND replaced_by IS NULL`,
		oldHash, newHash)
	if err != nil {
		return fmt.Errorf("revoke old token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrInvalidRefresh
	}
	var userID string
	if err := tx.QueryRow(ctx,
		`SELECT user_id FROM refresh_tokens WHERE token_hash = $1`, oldHash).Scan(&userID); err != nil {
		return fmt.Errorf("old token missing: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, newHash, expiresAt); err != nil {
		return fmt.Errorf("insert rotated token: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *RefreshTokens) RevokeFamily(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("revoke family: %w", err)
	}
	return nil
}
