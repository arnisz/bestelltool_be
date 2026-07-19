package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bestelltool_be/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type allocationRepository struct {
	q querier
}

const allocationSelectBase = `
SELECT id, request_id, resource_id, status, planned_from, planned_until,
       return_requested_at, shipped_at, received_at, version, created_at, updated_at
FROM allocations
WHERE id = $1`

func (r *allocationRepository) GetByID(ctx context.Context, id domain.AllocationID) (*domain.Allocation, error) {
	return r.get(ctx, id, false)
}

func (r *allocationRepository) GetForUpdate(ctx context.Context, id domain.AllocationID) (*domain.Allocation, error) {
	return r.get(ctx, id, true)
}

func (r *allocationRepository) get(ctx context.Context, id domain.AllocationID, lock bool) (*domain.Allocation, error) {
	q := allocationSelectBase
	if lock {
		q += " FOR UPDATE"
	}

	row := r.q.QueryRow(ctx, q, string(id))
	var a domain.Allocation
	var returnRequestedAt, shippedAt, receivedAt *time.Time
	if err := row.Scan(
		&a.ID,
		&a.RequestID,
		&a.ResourceID,
		&a.Status,
		&a.PlannedFrom,
		&a.PlannedUntil,
		&returnRequestedAt,
		&shippedAt,
		&receivedAt,
		&a.Version,
		&a.CreatedAt,
		&a.UpdatedAt,
	); err != nil {
		return nil, mapReadError("allocation", err)
	}
	a.ReturnRequestedAt = returnRequestedAt
	a.ShippedAt = shippedAt
	a.ReceivedAt = receivedAt
	return &a, nil
}

func (r *allocationRepository) Save(ctx context.Context, a *domain.Allocation) error {
	if a == nil {
		return fmt.Errorf("allocation nil: %w", ErrValidation)
	}

	result, err := r.q.Exec(ctx, `
UPDATE allocations
SET request_id = $1,
    resource_id = $2,
    status = $3,
    planned_from = $4,
    planned_until = $5,
    return_requested_at = $6,
    shipped_at = $7,
    received_at = $8,
    version = $9,
    updated_at = $10
WHERE id = $11
  AND version = $12
  AND (
      request_id IS DISTINCT FROM $1 OR
      resource_id IS DISTINCT FROM $2 OR
      status IS DISTINCT FROM $3 OR
      planned_from IS DISTINCT FROM $4 OR
      planned_until IS DISTINCT FROM $5 OR
      return_requested_at IS DISTINCT FROM $6 OR
      shipped_at IS DISTINCT FROM $7 OR
      received_at IS DISTINCT FROM $8 OR
      updated_at IS DISTINCT FROM $10
  )`,
		string(a.RequestID),
		string(a.ResourceID),
		string(a.Status),
		a.PlannedFrom,
		a.PlannedUntil,
		optionalTime(a.ReturnRequestedAt),
		optionalTime(a.ShippedAt),
		optionalTime(a.ReceivedAt),
		a.Version,
		a.UpdatedAt,
		string(a.ID),
		prevVersion(a.Version),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_allocations_single_active_resource" {
			return fmt.Errorf("allocation active unique: %w", ErrConflict)
		}
		return mapWriteError("allocation", err)
	}
	if result.RowsAffected() == 0 {
		var found int
		err := r.q.QueryRow(ctx, "SELECT 1 FROM allocations WHERE id = $1", string(a.ID)).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("allocation update: %w", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("check allocation existence: %w", err)
		}
		return fmt.Errorf("allocation update: %w", ErrConflict)
	}

	return nil
}

// Create inserts a new allocation. Returns ErrConflict if the unique active-resource
// index fires (uq_allocations_single_active_resource).
func (r *allocationRepository) Create(ctx context.Context, a *domain.Allocation) error {
	if a == nil {
		return fmt.Errorf("allocation nil: %w", ErrValidation)
	}

	_, err := r.q.Exec(ctx, `
INSERT INTO allocations(id, request_id, resource_id, status, planned_from, planned_until,
    return_requested_at, shipped_at, received_at, version, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		string(a.ID),
		string(a.RequestID),
		string(a.ResourceID),
		string(a.Status),
		a.PlannedFrom,
		a.PlannedUntil,
		optionalTime(a.ReturnRequestedAt),
		optionalTime(a.ShippedAt),
		optionalTime(a.ReceivedAt),
		a.Version,
		a.CreatedAt,
		a.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_allocations_single_active_resource" {
			return fmt.Errorf("allocation active unique: %w", ErrConflict)
		}
		return mapWriteError("allocation", err)
	}

	return nil
}
