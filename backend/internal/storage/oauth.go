package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ auth.OAuthStore = (*OAuthIdentities)(nil)

type OAuthIdentities struct{ pool *pgxpool.Pool }

func NewOAuthIdentities(pool *pgxpool.Pool) *OAuthIdentities { return &OAuthIdentities{pool: pool} }

func scanUser(row pgx.Row) (auth.UserRecord, error) {
	var rec auth.UserRecord
	var hash *string
	var role *string
	if err := row.Scan(&rec.ID, &rec.Email, &hash, &role); err != nil {
		return auth.UserRecord{}, err
	}
	if hash != nil {
		rec.PasswordHash = *hash
		rec.HasPassword = true
	}
	if role != nil && *role != "" {
		rec.Role = *role
	} else {
		rec.Role = "user"
	}
	return rec, nil
}

func (o *OAuthIdentities) FindUserByProvider(ctx context.Context, provider, subject string) (auth.UserRecord, error) {
	rec, err := scanUser(o.pool.QueryRow(ctx,
		`SELECT u.id, u.email::text, u.password_hash, u.role FROM users u
		 JOIN oauth_accounts a ON a.user_id = u.id
		 WHERE a.provider = $1 AND a.provider_subject = $2`, provider, subject))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, auth.ErrOAuthUnlinked
		}
		return auth.UserRecord{}, fmt.Errorf("oauth account lookup: %w", err)
	}
	return rec, nil
}

func (o *OAuthIdentities) CreateOAuthUser(ctx context.Context, email string, role ...string) (auth.UserRecord, error) {
	userRole := "user"
	if len(role) > 0 && role[0] != "" {
		userRole = role[0]
	}
	rec, err := scanUser(o.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role) VALUES ($1, NULL, $2)
		 RETURNING id, email::text, password_hash, role`, email, userRole))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return auth.UserRecord{}, auth.ErrEmailTaken
		}
		return auth.UserRecord{}, fmt.Errorf("create oauth user: %w", err)
	}
	return rec, nil
}

func (o *OAuthIdentities) LinkProvider(ctx context.Context, userID, provider, subject string) error {
	_, err := o.pool.Exec(ctx,
		`INSERT INTO oauth_accounts (provider, provider_subject, user_id) VALUES ($1, $2, $3)`,
		provider, subject, userID)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return fmt.Errorf("link oauth account: %w", err)
	}
	existing, findErr := o.FindUserByProvider(ctx, provider, subject)
	if findErr != nil {
		return fmt.Errorf("link oauth account: %w", err)
	}
	if existing.ID == userID {
		return nil
	}
	return auth.ErrOAuthConflict
}

func (o *OAuthIdentities) StoreCode(ctx context.Context, userID, codeHash string, expiresAt time.Time) error {
	_, err := o.pool.Exec(ctx,
		`INSERT INTO oauth_codes (code_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		codeHash, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("store oauth code: %w", err)
	}
	return nil
}

func (o *OAuthIdentities) ConsumeCode(ctx context.Context, codeHash string) (string, error) {
	var userID string
	err := o.pool.QueryRow(ctx,
		`DELETE FROM oauth_codes WHERE code_hash = $1 AND expires_at > now() RETURNING user_id`,
		codeHash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", auth.ErrInvalidOAuthCode
		}
		return "", fmt.Errorf("consume oauth code: %w", err)
	}
	return userID, nil
}
