package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ auth.RefreshStore = (*RefreshTokens)(nil)

const (
	maxUserAgent = 300
	maxIP        = 64
)

type RefreshTokens struct{ pool *pgxpool.Pool }

func NewRefreshTokens(pool *pgxpool.Pool) *RefreshTokens { return &RefreshTokens{pool: pool} }

func trimSessionMeta(meta auth.SessionMeta) (string, string) {
	ua := strings.TrimSpace(meta.UserAgent)
	if len(ua) > maxUserAgent {
		ua = ua[:maxUserAgent]
	}
	ip := strings.TrimSpace(meta.IP)
	if len(ip) > maxIP {
		ip = ip[:maxIP]
	}
	return ua, ip
}

func (r *RefreshTokens) Create(ctx context.Context, userID, sessionID, tokenHash string, expiresAt time.Time, meta auth.SessionMeta) error {
	ua, ip := trimSessionMeta(meta)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, session_id, token_hash, expires_at, user_agent, ip, last_used_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		userID, sessionID, tokenHash, expiresAt, ua, ip)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokens) Get(ctx context.Context, tokenHash string) (auth.RefreshRecord, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT user_id, session_id, expires_at, revoked_at, replaced_by
		 FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
	var (
		rec        auth.RefreshRecord
		revokedAt  *time.Time
		replacedBy *string
	)
	if err := row.Scan(&rec.UserID, &rec.SessionID, &rec.ExpiresAt, &revokedAt, &replacedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.RefreshRecord{}, auth.ErrInvalidRefresh
		}
		return auth.RefreshRecord{}, fmt.Errorf("get refresh token: %w", err)
	}
	rec.Revoked = revokedAt != nil || replacedBy != nil
	return rec, nil
}

func (r *RefreshTokens) Rotate(ctx context.Context, oldHash, newHash string, expiresAt time.Time, meta auth.SessionMeta) error {
	ua, ip := trimSessionMeta(meta)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rotate: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2
		 WHERE token_hash = $1 AND revoked_at IS NULL AND replaced_by IS NULL`,
		oldHash, newHash)
	if err != nil {
		return fmt.Errorf("revoke old token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrInvalidRefresh
	}
	var userID, sessionID string
	if err := tx.QueryRow(ctx,
		`SELECT user_id, session_id FROM refresh_tokens WHERE token_hash = $1`, oldHash).
		Scan(&userID, &sessionID); err != nil {
		return fmt.Errorf("old token missing: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, session_id, token_hash, expires_at, user_agent, ip, last_used_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		userID, sessionID, newHash, expiresAt, ua, ip); err != nil {
		return fmt.Errorf("insert rotated token: %w", err)
	}
	return tx.Commit(ctx)
}

// RevokeSession kills one device's rotation family. Unknown or already-dead
// sessions report ErrNotFound so the API can answer 404 without leaking
// whether the session belongs to someone else.
func (r *RefreshTokens) RevokeSession(ctx context.Context, userID, sessionID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now()
		 WHERE user_id = $1 AND session_id = $2 AND revoked_at IS NULL`,
		userID, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrSessionNotFound
	}
	return nil
}

func (r *RefreshTokens) RevokeAll(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}
	return nil
}

// ListSessions returns one row per live rotation family, newest activity
// first. Rows already rotated out carry replaced_by and are skipped; the
// surviving row's metadata reflects that session's latest use.
func (r *RefreshTokens) ListSessions(ctx context.Context, userID string) ([]auth.Session, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT session_id, user_agent, ip, first_seen, last_used_at FROM (
		   SELECT session_id, user_agent, ip, last_used_at,
		          MIN(created_at) OVER (PARTITION BY session_id) AS first_seen,
		          ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at DESC) AS rn
		   FROM refresh_tokens
		   WHERE user_id = $1 AND revoked_at IS NULL AND replaced_by IS NULL AND expires_at > now()
		 ) s WHERE rn = 1 ORDER BY last_used_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []auth.Session
	for rows.Next() {
		var s auth.Session
		if err := rows.Scan(&s.ID, &s.UserAgent, &s.IP, &s.CreatedAt, &s.LastUsedAt); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
