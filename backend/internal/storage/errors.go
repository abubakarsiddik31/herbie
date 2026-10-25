package storage

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound  = errors.New("storage: not found")
	ErrDuplicate = errors.New("storage: duplicate")
)

// pgxErrNoRows aliases pgx.ErrNoRows so repos can map it to ErrNotFound
// without importing pgx in every file.
var pgxErrNoRows = pgx.ErrNoRows

func isDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
