package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("validation")
)

func mapReadError(entity string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", entity, ErrNotFound)
	}
	return fmt.Errorf("%s read: %w", entity, err)
}

func mapWriteError(entity string, err error) error {
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%s unique constraint %s: %w", entity, pgErr.ConstraintName, ErrConflict)
		case "23503":
			return fmt.Errorf("%s foreign key constraint %s: %w", entity, pgErr.ConstraintName, ErrValidation)
		case "23514":
			return fmt.Errorf("%s check constraint %s: %w", entity, pgErr.ConstraintName, ErrValidation)
		}
	}

	return fmt.Errorf("%s write: %w", entity, err)
}
