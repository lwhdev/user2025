package database

import "errors"

var (
	ErrNotFound      = errors.New("record not found")
	ErrDuplicate     = errors.New("duplicate record")
	ErrSchemaMissing = errors.New("database schema missing")
)
