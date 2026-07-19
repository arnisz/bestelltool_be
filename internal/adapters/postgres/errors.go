package postgres

import (
	"errors"
	"fmt"

	"bestelltool_be/internal/application/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound   = ports.ErrNotFound
	ErrConflict   = ports.ErrConflict
	ErrValidation = ports.ErrValidation
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

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
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
