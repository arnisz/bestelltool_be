package postgres

import (
	"context"
	"fmt"

	"bestelltool_be/internal/domain"
)

type resourceClassRepository struct {
	q querier
}

func (r *resourceClassRepository) GetByID(ctx context.Context, id domain.ResourceClassID) (*domain.ResourceClass, error) {
	row := r.q.QueryRow(ctx, `
SELECT id, name, description, metadata
FROM resource_classes
WHERE id = $1`, string(id))

	var rc domain.ResourceClass
	var metadata []byte
	if err := row.Scan(
		&rc.ID,
		&rc.Name,
		&rc.Description,
		&metadata,
	); err != nil {
		return nil, mapReadError("resource class", err)
	}

	m, err := unmarshalMetadata(metadata)
	if err != nil {
		return nil, fmt.Errorf("resource class metadata: %w", err)
	}
	rc.Metadata = m

	return &rc, nil
}

// Create inserts a new resource class. Returns ErrConflict if the id already exists.
func (r *resourceClassRepository) Create(ctx context.Context, rc *domain.ResourceClass) error {
	if rc == nil {
		return fmt.Errorf("resource class nil: %w", ErrValidation)
	}

	metadata, err := marshalMetadata(rc.Metadata)
	if err != nil {
		return err
	}

	if _, err := r.q.Exec(ctx, `
INSERT INTO resource_classes(id, name, description, metadata)
VALUES ($1,$2,$3,$4)`,
		string(rc.ID),
		rc.Name,
		rc.Description,
		metadata,
	); err != nil {
		return mapWriteError("resource class", err)
	}

	return nil
}
