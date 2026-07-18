package postgres

import (
	"context"
	"errors"
	"fmt"

	"bestelltool_be/internal/application/ports"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type idempotencyStore struct {
	q querier
}

func (s *idempotencyStore) Get(ctx context.Context, actionID string) (*ports.IdempotencyResult, error) {
	row := s.q.QueryRow(ctx, `
SELECT status_code, payload, error_text
FROM idempotency_outcomes
WHERE client_action_id = $1`, actionID)

	var out ports.IdempotencyResult
	if err := row.Scan(&out.StatusCode, &out.Payload, &out.ErrorText); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, mapReadError("idempotency outcome", err)
	}
	return &out, nil
}

func (s *idempotencyStore) Save(ctx context.Context, actionID string, result ports.IdempotencyResult) error {
	_, err := s.q.Exec(ctx, `
INSERT INTO idempotency_outcomes(client_action_id, status_code, payload, error_text)
VALUES ($1, $2, $3, $4)`, actionID, result.StatusCode, result.Payload, result.ErrorText)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		stored, getErr := s.Get(ctx, actionID)
		if getErr != nil {
			return fmt.Errorf("idempotency duplicate get existing: %w", getErr)
		}
		if stored == nil {
			return fmt.Errorf("idempotency duplicate without stored outcome: %w", ErrConflict)
		}
		return nil
	}

	return mapWriteError("idempotency outcome", err)
}
