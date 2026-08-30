package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return auth.UserRecord{}, auth.ErrEmailTaken
		}
		return auth.UserRecord{}, fmt.Errorf("create user: %w", err)
	}
	return rec, nil
}

func (u *Users) ByEmail(ctx context.Context, email string) (auth.UserRecord, error) {
	row := u.pool.QueryRow(ctx,
		`SELECT id, email::text, password_hash FROM users WHERE email = $1`, email)
	var rec auth.UserRecord
	if err := row.Scan(&rec.ID, &rec.Email, &rec.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, auth.ErrInvalidCredentials
		}
		return auth.UserRecord{}, fmt.Errorf("user by email: %w", err)
	}
	return rec, nil
}

func (u *Users) ByID(ctx context.Context, id string) (auth.UserRecord, error) {
	row := u.pool.QueryRow(ctx, `SELECT id, email::text, password_hash FROM users WHERE id = $1`, id)
	var rec auth.UserRecord
	if err := row.Scan(&rec.ID, &rec.Email, &rec.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.UserRecord{}, auth.ErrInvalidRefresh
		}
		return auth.UserRecord{}, fmt.Errorf("user by id: %w", err)
	}
	return rec, nil
}
