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

var _ auth.AuthTokenStore = (*AuthTokens)(nil)

// AuthTokens stores single-use email verification and password reset tokens
// (SHA-256 hashes only).
type AuthTokens struct{ pool *pgxpool.Pool }

func NewAuthTokens(pool *pgxpool.Pool) *AuthTokens { return &AuthTokens{pool: pool} }

// Create replaces any outstanding token for the same user and purpose so
// only the newest link works, and opportunistically clears expired rows.
func (a *AuthTokens) Create(ctx context.Context, userID, tokenHash, purpose string, expiresAt time.Time) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin auth token: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`DELETE FROM auth_tokens WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`,
		userID, purpose); err != nil {
		return fmt.Errorf("clear prior auth tokens: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO auth_tokens (user_id, token_hash, purpose, expires_at) VALUES ($1, $2, $3, $4)`,
		userID, tokenHash, purpose, expiresAt); err != nil {
		return fmt.Errorf("insert auth token: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_tokens WHERE expires_at < now()`); err != nil {
		return fmt.Errorf("prune expired auth tokens: %w", err)
	}
	return tx.Commit(ctx)
}

func (a *AuthTokens) consume(ctx context.Context, tx pgx.Tx, tokenHash, purpose string, now time.Time) (string, error) {
	var userID string
	err := tx.QueryRow(ctx,
		`DELETE FROM auth_tokens
		 WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > $3
		 RETURNING user_id`, tokenHash, purpose, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("consume auth token: %w", err)
	}
	return userID, nil
}

// ConsumeVerify spends a verification token and marks the account verified
// in the same transaction.
func (a *AuthTokens) ConsumeVerify(ctx context.Context, tokenHash string, now time.Time) (string, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin verify: %w", err)
	}
	defer tx.Rollback(ctx)
	userID, err := a.consume(ctx, tx, tokenHash, auth.PurposeVerifyEmail, now)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET email_verified_at = COALESCE(email_verified_at, $2) WHERE id = $1`,
		userID, now); err != nil {
		return "", fmt.Errorf("mark verified: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit verify: %w", err)
	}
	return userID, nil
}

// ConsumeReset spends a reset token, replaces the password, marks the email
// verified, and revokes every session in the same transaction.
func (a *AuthTokens) ConsumeReset(ctx context.Context, tokenHash, passwordHash string, now time.Time) (string, error) {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin reset: %w", err)
	}
	defer tx.Rollback(ctx)
	userID, err := a.consume(ctx, tx, tokenHash, auth.PurposeResetPassword, now)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, $3) WHERE id = $1`,
		userID, passwordHash, now); err != nil {
		return "", fmt.Errorf("reset password: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID); err != nil {
		return "", fmt.Errorf("revoke sessions after reset: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit reset: %w", err)
	}
	return userID, nil
}
