package incident

import (
	"time"

	"github.com/google/uuid"
)

// Incident represents a production incident.
type Incident struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Severity    Severity  `json:"severity"`
	Service     string    `json:"service"`
	Environment string    `json:"environment"`
	AlertSource string    `json:"alert_source"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IncidentEvent records a state change or notable event on an incident.
type IncidentEvent struct {
	ID         uuid.UUID `json:"id"`
	IncidentID uuid.UUID `json:"incident_id"`
	EventType  string    `json:"event_type"`
	FromStatus *Status   `json:"from_status,omitempty"`
	ToStatus   *Status   `json:"to_status,omitempty"`
	Message    string    `json:"message"`
	Metadata   []byte    `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// CreateInput holds validated input for creating an incident.
type CreateInput struct {
	Title       string
	Description string
	Severity    Severity
	Service     string
	Environment string
	AlertSource string
}

// ListFilter holds optional filters for listing incidents.
type ListFilter struct {
	Status      *Status
	Service     string
	Environment string
	Limit       int
	Offset      int
}

// TransitionInput holds input for a status transition.
type TransitionInput struct {
	IncidentID uuid.UUID
	ToStatus   Status
	Message    string
}
