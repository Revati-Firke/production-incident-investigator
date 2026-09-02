package investigation

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
)

var (
	ErrNotFound = errors.New("investigation not found")
	ErrNoJob    = errors.New("no job available")
)

// InvestigationStatus represents the state of an investigation.
type InvestigationStatus string

const (
	StatusPending      InvestigationStatus = "pending"
	StatusRunning      InvestigationStatus = "running"
	StatusCompleted    InvestigationStatus = "completed"
	StatusFailed       InvestigationStatus = "failed"
	StatusWaitingForAI InvestigationStatus = "waiting_for_ai"
)

// JobStatus represents the state of a background job.
type JobStatus string

const (
	JobPending    JobStatus = "pending"
	JobProcessing JobStatus = "processing"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
	JobDeadLetter JobStatus = "dead_letter"
)

// Investigation tracks the investigation lifecycle for an incident.
type Investigation struct {
	ID          uuid.UUID           `json:"id"`
	IncidentID  uuid.UUID           `json:"incident_id"`
	Status      InvestigationStatus `json:"status"`
	StartedAt   *time.Time          `json:"started_at,omitempty"`
	CompletedAt *time.Time          `json:"completed_at,omitempty"`
	Error       string              `json:"error,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// Job represents a durable investigation job.
type Job struct {
	ID              uuid.UUID  `json:"id"`
	IncidentID      uuid.UUID  `json:"incident_id"`
	InvestigationID uuid.UUID  `json:"investigation_id"`
	Status          JobStatus  `json:"status"`
	Attempts        int        `json:"attempts"`
	MaxAttempts     int        `json:"max_attempts"`
	Error           string     `json:"error,omitempty"`
	ScheduledAt     time.Time  `json:"scheduled_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CreateResult is returned when an incident is created with investigation queued.
type CreateResult struct {
	Incident      *incident.Incident `json:"incident"`
	Investigation *Investigation     `json:"investigation"`
	JobID         uuid.UUID          `json:"job_id"`
}
