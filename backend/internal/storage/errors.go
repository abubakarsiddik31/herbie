package storage

import "errors"

var (
	ErrNotFound  = errors.New("storage: not found")
	ErrDuplicate = errors.New("storage: duplicate")
)
