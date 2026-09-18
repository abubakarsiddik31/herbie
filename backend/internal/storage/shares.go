package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Shares struct{ pool *pgxpool.Pool }

func NewShares(pool *pgxpool.Pool) *Shares { return &Shares{pool: pool} }

// Share creates the conversation's public link unless one already exists.
// Created reports whether tokenHash is the new active link; when false the
// returned hash is the pre-existing one (whose raw token is unrecoverable).
// Unknown or foreign conversations are ErrNotFound.
func (s *Shares) Share(ctx context.Context, convID, userID, tokenHash string) (hash string, created bool, err error) {
	var owner string
	err = s.pool.QueryRow(ctx, `SELECT user_id::text FROM conversations WHERE id = $1`, convID).Scan(&owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, ErrNotFound
		}
		return "", false, fmt.Errorf("share lookup: %w", err)
	}
	if owner != userID {
		return "", false, ErrNotFound
	}
	res, err := s.pool.Exec(ctx,
		`INSERT INTO conversation_shares (conversation_id, token_hash) VALUES ($1, $2)
		 ON CONFLICT (conversation_id) DO NOTHING`, convID, tokenHash)
	if err != nil {
		return "", false, fmt.Errorf("share conversation: %w", err)
	}
	if res.RowsAffected() == 1 {
		return tokenHash, true, nil
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT token_hash FROM conversation_shares WHERE conversation_id = $1`, convID).Scan(&hash); err != nil {
		return "", false, fmt.Errorf("share conversation: %w", err)
	}
	return hash, false, nil
}

// Unshare revokes the conversation's public link. Missing links and
// foreign conversations are ErrNotFound.
func (s *Shares) Unshare(ctx context.Context, convID, userID string) error {
	res, err := s.pool.Exec(ctx,
		`DELETE FROM conversation_shares USING conversations
		 WHERE conversation_shares.conversation_id = conversations.id
		 AND conversation_shares.conversation_id = $1 AND conversations.user_id = $2`,
		convID, userID)
	if err != nil {
		return fmt.Errorf("unshare conversation: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SharedHash returns the active link's token hash, or "" when unshared.
func (s *Shares) SharedHash(ctx context.Context, convID, userID string) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT s.token_hash FROM conversation_shares s JOIN conversations c ON c.id = s.conversation_id
		 WHERE s.conversation_id = $1 AND c.user_id = $2`, convID, userID).Scan(&hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("share lookup: %w", err)
	}
	return hash, nil
}

// Resolve maps a link token hash to its conversation and owner.
func (s *Shares) Resolve(ctx context.Context, tokenHash string) (convID, userID string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT s.conversation_id::text, c.user_id::text FROM conversation_shares s
		 JOIN conversations c ON c.id = s.conversation_id WHERE s.token_hash = $1`,
		tokenHash).Scan(&convID, &userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", fmt.Errorf("resolve share: %w", err)
	}
	return convID, userID, nil
}
