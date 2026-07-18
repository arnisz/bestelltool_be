package domain

import (
	"fmt"
)

// ResourceClass represents a requestable resource category.
type ResourceClass struct {
	ID          ResourceClassID
	Name        string
	Description string
	Metadata    map[string]any
}

// NewResourceClass creates a resource class in a valid initial state.
func NewResourceClass(id ResourceClassID, name, description string, metadata map[string]any) (*ResourceClass, error) {
	if id == "" {
		return nil, fmt.Errorf("resource class id: %w", ErrRequiredField)
	}
	if name == "" {
		return nil, fmt.Errorf("resource class name: %w", ErrRequiredField)
	}

	return &ResourceClass{
		ID:          id,
		Name:        name,
		Description: description,
		Metadata:    metadata,
	}, nil
}
