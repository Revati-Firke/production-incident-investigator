package investigation

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines persistence for investigations and jobs.
type Repository interface {
	CreateWithJob(ctx context.Context, inv *Investigation, job *Job) error
	GetByIncidentID(ctx context.Context, incidentID uuid.UUID) (*Investigation, error)
	UpdateInvestigation(ctx context.Context, inv *Investigation) error

	ClaimNextJob(ctx context.Context) (*Job, error)
	CompleteJob(ctx context.Context, jobID uuid.UUID) error
	FailJob(ctx context.Context, jobID uuid.UUID, jobErr string, retry bool) error

	CreateAgentRun(ctx context.Context, run *AgentRun) error
	ListAgentRuns(ctx context.Context, incidentID uuid.UUID) ([]AgentRun, error)
}

// JobNotifier notifies workers that a new job is available.
type JobNotifier interface {
	Notify(ctx context.Context, jobID uuid.UUID) error
	WaitForJob(ctx context.Context) error
}
