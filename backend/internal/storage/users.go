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

var _ auth.UserStore = (*Users)(nil)

type Users struct{ pool *pgxpool.Pool }

func NewUsers(pool *pgxpool.Pool) *Users { return &Users{pool: pool} }

func (u *Users) Create(ctx context.Context, email, passwordHash string, role ...string) (auth.UserRecord, error) {
	userRole := "user"
	if len(role) > 0 && role[0] != "" {
		userRole = role[0]
	}
	row := u.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3)
		 RETURNING id, email::text, password_hash, role, email_verified_at`,
		email, passwordHash, userRole)
	rec, err := scanUser(row)
	if err != nil {
		if isDuplicate(err) {
			return auth.UserRecord{}, auth.ErrEmailTaken
		}
		return auth.UserRecord{}, fmt.Errorf("create user: %w", err)
	}
	return rec, nil
}

func (u *Users) ByEmail(ctx context.Context, email string) (auth.UserRecord, error) {
	rec, err := scanUser(u.pool.QueryRow(ctx,
		`SELECT id, email::text, password_hash, role, email_verified_at FROM users WHERE email = $1`, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, fmt.Errorf("%w: %w", ErrNotFound, auth.ErrUserNotFound)
		}
		return auth.UserRecord{}, fmt.Errorf("user by email: %w", err)
	}
	return rec, nil
}

func (u *Users) ByID(ctx context.Context, id string) (auth.UserRecord, error) {
	rec, err := scanUser(u.pool.QueryRow(ctx,
		`SELECT id, email::text, password_hash, role, email_verified_at FROM users WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, fmt.Errorf("%w: %w", ErrNotFound, auth.ErrUserNotFound)
		}
		return auth.UserRecord{}, fmt.Errorf("user by id: %w", err)
	}
	return rec, nil
}

func (u *Users) List(ctx context.Context) ([]auth.UserRecord, error) {
	rows, err := u.pool.Query(ctx, `SELECT id, email::text, password_hash, role, email_verified_at FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []auth.UserRecord
	for rows.Next() {
		rec, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (u *Users) Count(ctx context.Context) (int, error) {
	var n int
	err := u.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (u *Users) SetRole(ctx context.Context, userID, role string) error {
	if role != "admin" && role != "user" {
		return fmt.Errorf("invalid role %q: must be 'admin' or 'user'", role)
	}
	tag, err := u.pool.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, userID, role)
	if err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetEmailVerified records proven ownership of the account email.
func (u *Users) SetEmailVerified(ctx context.Context, userID string, at time.Time) error {
	tag, err := u.pool.Exec(ctx,
		`UPDATE users SET email_verified_at = COALESCE(email_verified_at, $2) WHERE id = $1`,
		userID, at)
	if err != nil {
		return fmt.Errorf("set email verified: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (u *Users) Instructions(ctx context.Context, userID string) (string, error) {
	var text string
	if err := u.pool.QueryRow(ctx, `SELECT default_instructions FROM users WHERE id = $1`, userID).Scan(&text); err != nil {
		return "", fmt.Errorf("user instructions: %w", err)
	}
	return text, nil
}

func (u *Users) SetInstructions(ctx context.Context, userID, text string) error {
	tag, err := u.pool.Exec(ctx, `UPDATE users SET default_instructions = $2 WHERE id = $1`, userID, text)
	if err != nil {
		return fmt.Errorf("set user instructions: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GenerationDefaults are the user-level fallback generation controls.
type GenerationDefaults struct {
	TopP            *float64
	MaxOutputTokens int
	Reasoning       string
}

// GenerationDefaults loads the user's fallback generation controls.
func (u *Users) GenerationDefaults(ctx context.Context, userID string) (GenerationDefaults, error) {
	var d GenerationDefaults
	err := u.pool.QueryRow(ctx,
		`SELECT default_top_p, default_max_output_tokens, default_reasoning FROM users WHERE id = $1`, userID).
		Scan(&d.TopP, &d.MaxOutputTokens, &d.Reasoning)
	if err != nil {
		return GenerationDefaults{}, fmt.Errorf("user generation defaults: %w", err)
	}
	return d, nil
}

// SetGenerationDefaults replaces the user's fallback generation controls.
func (u *Users) SetGenerationDefaults(ctx context.Context, userID string, d GenerationDefaults) error {
	tag, err := u.pool.Exec(ctx,
		`UPDATE users SET default_top_p = $2, default_max_output_tokens = $3, default_reasoning = $4 WHERE id = $1`,
		userID, d.TopP, d.MaxOutputTokens, SanitizeText(d.Reasoning))
	if err != nil {
		return fmt.Errorf("set user generation defaults: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
