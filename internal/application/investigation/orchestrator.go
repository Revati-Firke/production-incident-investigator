package investigation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

// Orchestrator coordinates incident creation and investigation enqueueing.
type Orchestrator struct {
	incidents      *appincident.Service
	investigations *Service
	intake         IntakeStore
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(incidents *appincident.Service, investigations *Service, intake IntakeStore) *Orchestrator {
	return &Orchestrator{
		incidents:      incidents,
		investigations: investigations,
		intake:         intake,
	}
}

// CreateIncident atomically creates an incident, investigation, and job, then notifies workers.
func (o *Orchestrator) CreateIncident(ctx context.Context, input domain.CreateInput) (*invdomain.CreateResult, error) {
	inc, event, err := o.incidents.PrepareCreate(input)
	if err != nil {
		return nil, err
	}

	inv, job := o.investigations.NewJob(inc.ID)

	if err := o.intake.Persist(ctx, inc, event, inv, job); err != nil {
		return nil, fmt.Errorf("persist intake: %w", err)
	}

	if err := o.investigations.Notify(ctx, job.ID); err != nil {
		// Job is durable in PostgreSQL; workers poll as a fallback.
		slog.Warn("failed to notify worker of new job", "job_id", job.ID, "error", err)
	}

	return &invdomain.CreateResult{
		Incident:      inc,
		Investigation: inv,
		JobID:         job.ID,
	}, nil
}

// GetInvestigation returns the investigation for an incident.
func (o *Orchestrator) GetInvestigation(ctx context.Context, incidentID uuid.UUID) (*invdomain.Investigation, error) {
	if _, err := o.incidents.GetByID(ctx, incidentID); err != nil {
		return nil, err
	}
	return o.investigations.GetByIncidentID(ctx, incidentID)
}
