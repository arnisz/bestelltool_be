package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bestelltool_be/internal/domain"

	"github.com/jackc/pgx/v5"
)

type requestRepository struct {
	q querier
}

const requestSelectBase = `
SELECT id, technician_id, status, execution_state, execution_note, context_ref, context_label,
       wish_from, wish_until, note, version, created_at, updated_at
FROM requests
WHERE id = $1`

func (r *requestRepository) GetByID(ctx context.Context, id domain.RequestID) (*domain.Request, error) {
	return r.get(ctx, id, false)
}

func (r *requestRepository) GetForUpdate(ctx context.Context, id domain.RequestID) (*domain.Request, error) {
	return r.get(ctx, id, true)
}

func (r *requestRepository) get(ctx context.Context, id domain.RequestID, lock bool) (*domain.Request, error) {
	q := requestSelectBase
	if lock {
		q += " FOR UPDATE"
	}

	row := r.q.QueryRow(ctx, q, string(id))
	var req domain.Request
	var wishFrom, wishUntil *time.Time
	if err := row.Scan(
		&req.ID,
		&req.TechnicianID,
		&req.Status,
		&req.ExecutionState,
		&req.ExecutionNote,
		&req.ContextRef,
		&req.ContextLabel,
		&wishFrom,
		&wishUntil,
		&req.Note,
		&req.Version,
		&req.CreatedAt,
		&req.UpdatedAt,
	); err != nil {
		return nil, mapReadError("request", err)
	}
	req.WishFrom = wishFrom
	req.WishUntil = wishUntil

	rows, err := r.q.Query(ctx, `
SELECT resource_class_id
FROM request_resource_classes
WHERE request_id = $1
ORDER BY position`, string(id))
	if err != nil {
		return nil, fmt.Errorf("load request resource classes: %w", err)
	}
	defer rows.Close()

	classes := make([]domain.ResourceClassID, 0)
	for rows.Next() {
		var resourceClassID domain.ResourceClassID
		if err := rows.Scan(&resourceClassID); err != nil {
			return nil, fmt.Errorf("scan request resource class: %w", err)
		}
		classes = append(classes, resourceClassID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request resource classes: %w", err)
	}
	req.RequestedResourceClasses = classes

	return &req, nil
}

func (r *requestRepository) Save(ctx context.Context, req *domain.Request) error {
	if req == nil {
		return fmt.Errorf("request nil: %w", ErrValidation)
	}

	result, err := r.q.Exec(ctx, `
UPDATE requests
SET status = $1,
    execution_state = $2,
    execution_note = $3,
    context_ref = $4,
    context_label = $5,
    wish_from = $6,
    wish_until = $7,
    note = $8,
    version = $9,
    updated_at = $10
WHERE id = $11
  AND version = $12`,
		string(req.Status),
		string(req.ExecutionState),
		req.ExecutionNote,
		req.ContextRef,
		req.ContextLabel,
		optionalTime(req.WishFrom),
		optionalTime(req.WishUntil),
		req.Note,
		req.Version,
		req.UpdatedAt,
		string(req.ID),
		prevVersion(req.Version),
	)
	if err != nil {
		return mapWriteError("request", err)
	}
	if result.RowsAffected() == 0 {
		var found int
		err := r.q.QueryRow(ctx, "SELECT 1 FROM requests WHERE id = $1", string(req.ID)).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("request update: %w", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("check request existence: %w", err)
		}
		return fmt.Errorf("request update: %w", ErrConflict)
	}

	if _, err := r.q.Exec(ctx, "DELETE FROM request_resource_classes WHERE request_id = $1", string(req.ID)); err != nil {
		return mapWriteError("request resource classes delete", err)
	}
	for i, classID := range req.RequestedResourceClasses {
		if _, err := r.q.Exec(ctx, `
INSERT INTO request_resource_classes(request_id, position, resource_class_id)
VALUES ($1, $2, $3)`, string(req.ID), i, string(classID)); err != nil {
			return mapWriteError("request resource classes insert", err)
		}
	}

	return nil
}
