package incident

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines persistence operations for incidents.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Incident, error)
	List(ctx context.Context, filter ListFilter) ([]Incident, int, error)
	ListEvents(ctx context.Context, incidentID uuid.UUID) ([]IncidentEvent, error)

	// CreateWithEvent atomically persists an incident and its creation event.
	CreateWithEvent(ctx context.Context, inc *Incident, event *IncidentEvent) error

	// TransitionWithEvent atomically updates status (with optimistic locking) and records an event.
	TransitionWithEvent(ctx context.Context, id uuid.UUID, expectedStatus, newStatus Status, event *IncidentEvent) error
}
