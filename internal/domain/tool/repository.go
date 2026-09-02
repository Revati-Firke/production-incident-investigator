package tool

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines persistence for tool executions.
type Repository interface {
	Create(ctx context.Context, exec *Execution) error
	Update(ctx context.Context, exec *Execution) error
	GetByID(ctx context.Context, id uuid.UUID) (*Execution, error)
	ListByIncidentID(ctx context.Context, incidentID uuid.UUID) ([]Execution, error)
}
