package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ auth.UserStore = (*Users)(nil)

type Users struct{ pool *pgxpool.Pool }

func NewUsers(pool *pgxpool.Pool) *Users { return &Users{pool: pool} }

func (u *Users) Create(ctx context.Context, email, passwordHash string) (auth.UserRecord, error) {
	row := u.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email::text`,
		email, passwordHash)
	var rec auth.UserRecord
	if err := row.Scan(&rec.ID, &rec.Email); err != nil {
		if isDuplicate(err) {
			return auth.UserRecord{}, auth.ErrEmailTaken
		}
		return auth.UserRecord{}, fmt.Errorf("create user: %w", err)
	}
	return rec, nil
}

func (u *Users) ByEmail(ctx context.Context, email string) (auth.UserRecord, error) {
	rec, err := scanUser(u.pool.QueryRow(ctx,
		`SELECT id, email::text, password_hash FROM users WHERE email = $1`, email))
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
		`SELECT id, email::text, password_hash FROM users WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, fmt.Errorf("%w: %w", ErrNotFound, auth.ErrUserNotFound)
		}
		return auth.UserRecord{}, fmt.Errorf("user by id: %w", err)
	}
	return rec, nil
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
