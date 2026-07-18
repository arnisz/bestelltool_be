package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"bestelltool_be/internal/domain"

	"github.com/jackc/pgx/v5"
)

type resourceRepository struct {
	q querier
}

const resourceSelectBase = `
SELECT id, resource_class_id, serial_number, status, block_reason, block_note,
       holder_id, location, valid_until, metadata, version
FROM resources
WHERE id = $1`

func (r *resourceRepository) GetByID(ctx context.Context, id domain.ResourceID) (*domain.Resource, error) {
	return r.get(ctx, id, false)
}

func (r *resourceRepository) GetForUpdate(ctx context.Context, id domain.ResourceID) (*domain.Resource, error) {
	return r.get(ctx, id, true)
}

func (r *resourceRepository) get(ctx context.Context, id domain.ResourceID, lock bool) (*domain.Resource, error) {
	q := resourceSelectBase
	if lock {
		q += " FOR UPDATE"
	}

	row := r.q.QueryRow(ctx, q, string(id))
	var res domain.Resource
	var blockReason *string
	var holderID *string
	var validUntil *time.Time
	var metadata []byte
	if err := row.Scan(
		&res.ID,
		&res.ResourceClassID,
		&res.SerialNumber,
		&res.Status,
		&blockReason,
		&res.BlockNote,
		&holderID,
		&res.Location,
		&validUntil,
		&metadata,
		&res.Version,
	); err != nil {
		return nil, mapReadError("resource", err)
	}
	if blockReason != nil {
		res.BlockReason = ptr(domain.BlockReason(*blockReason))
	}
	if holderID != nil {
		res.HolderID = ptr(domain.UserID(*holderID))
	}
	res.ValidUntil = validUntil

	m, err := unmarshalMetadata(metadata)
	if err != nil {
		return nil, fmt.Errorf("resource metadata: %w", err)
	}
	res.Metadata = m

	return &res, nil
}

func (r *resourceRepository) Save(ctx context.Context, res *domain.Resource) error {
	if res == nil {
		return fmt.Errorf("resource nil: %w", ErrValidation)
	}

	metadata, err := marshalMetadata(res.Metadata)
	if err != nil {
		return err
	}

	var blockReason any
	if res.BlockReason != nil {
		blockReason = string(*res.BlockReason)
	}
	var holderID any
	if res.HolderID != nil {
		holderID = string(*res.HolderID)
	}

	result, err := r.q.Exec(ctx, `
UPDATE resources
SET resource_class_id = $1,
    serial_number = $2,
    status = $3,
    block_reason = $4,
    block_note = $5,
    holder_id = $6,
    location = $7,
    valid_until = $8,
    metadata = $9,
    version = $10
WHERE id = $11
  AND version = $12`,
		string(res.ResourceClassID),
		res.SerialNumber,
		string(res.Status),
		blockReason,
		res.BlockNote,
		holderID,
		res.Location,
		optionalTime(res.ValidUntil),
		metadata,
		res.Version,
		string(res.ID),
		prevVersion(res.Version),
	)
	if err != nil {
		return mapWriteError("resource", err)
	}
	if result.RowsAffected() == 0 {
		var found int
		err := r.q.QueryRow(ctx, "SELECT 1 FROM resources WHERE id = $1", string(res.ID)).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("resource update: %w", ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("check resource existence: %w", err)
		}
		return fmt.Errorf("resource update: %w", ErrConflict)
	}

	return nil
}
