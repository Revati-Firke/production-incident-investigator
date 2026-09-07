package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/agents"
	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	appremediation "github.com/Revati-Firke/production-incident-investigator/internal/application/remediation"
	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// defaultInvestigationTools are read-only tools run during automated triage.
var defaultInvestigationTools = []string{
	"search_logs",
	"query_metrics",
	"get_service_health",
	"get_recent_deployments",
	"search_runbooks",
}

const (
	defaultMaxAttempts = 3
	maxBackoff         = 30 * time.Second
)

// Service implements investigation business logic.
type Service struct {
	repo     invdomain.Repository
	notifier invdomain.JobNotifier
}

// NewService creates a new investigation service.
func NewService(repo invdomain.Repository, notifier invdomain.JobNotifier) *Service {
	return &Service{repo: repo, notifier: notifier}
}

// NewJob builds a new investigation and job for an incident.
func (s *Service) NewJob(incidentID uuid.UUID) (*invdomain.Investigation, *invdomain.Job) {
	now := time.Now().UTC()

	inv := &invdomain.Investigation{
		ID:         uuid.New(),
		IncidentID: incidentID,
		Status:     invdomain.StatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	job := &invdomain.Job{
		ID:              uuid.New(),
		IncidentID:      incidentID,
		InvestigationID: inv.ID,
		Status:          invdomain.JobPending,
		MaxAttempts:     defaultMaxAttempts,
		ScheduledAt:     now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	return inv, job
}

// Notify signals workers that a job is available.
func (s *Service) Notify(ctx context.Context, jobID uuid.UUID) error {
	return s.notifier.Notify(ctx, jobID)
}

// Enqueue creates an investigation and job, then notifies workers.
func (s *Service) Enqueue(ctx context.Context, incidentID uuid.UUID) (*invdomain.Investigation, uuid.UUID, error) {
	inv, job := s.NewJob(incidentID)

	if err := s.repo.CreateWithJob(ctx, inv, job); err != nil {
		return nil, uuid.Nil, fmt.Errorf("create investigation: %w", err)
	}

	if err := s.Notify(ctx, job.ID); err != nil {
		slog.Warn("failed to notify worker of new job", "job_id", job.ID, "error", err)
	}

	return inv, job.ID, nil
}

// GetByIncidentID returns the investigation for an incident.
func (s *Service) GetByIncidentID(ctx context.Context, incidentID uuid.UUID) (*invdomain.Investigation, error) {
	return s.repo.GetByIncidentID(ctx, incidentID)
}

// ListAgentRuns returns agent runs for an incident.
func (s *Service) ListAgentRuns(ctx context.Context, incidentID uuid.UUID) ([]invdomain.AgentRun, error) {
	return s.repo.ListAgentRuns(ctx, incidentID)
}

// Processor handles investigation job execution.
type Processor struct {
	repo         invdomain.Repository
	incidents    *appincident.Service
	notifier     invdomain.JobNotifier
	tools        *apptool.Service
	agent        *agents.InvestigationAgent
	remediations *appremediation.Service

	wg sync.WaitGroup
}

// NewProcessor creates a new investigation processor.
func NewProcessor(
	repo invdomain.Repository,
	incidents *appincident.Service,
	notifier invdomain.JobNotifier,
	tools *apptool.Service,
	agent *agents.InvestigationAgent,
	remediations *appremediation.Service,
) *Processor {
	return &Processor{
		repo:         repo,
		incidents:    incidents,
		notifier:     notifier,
		tools:        tools,
		agent:        agent,
		remediations: remediations,
	}
}

// ProcessNext claims and processes a single job. Returns nil when no jobs are available.
func (p *Processor) ProcessNext(ctx context.Context) error {
	return p.processOne(ctx)
}

// Run starts the worker loop until context is cancelled.
func (p *Processor) Run(ctx context.Context) error {
	backoff := time.Second

	for {
		err := p.processOne(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Error("job processing error", "error", err)
			if !sleepWithContext(ctx, backoff) {
				return ctx.Err()
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		backoff = time.Second

		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = p.notifier.WaitForJob(waitCtx)
		cancel()

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// Wait blocks until in-flight jobs complete or the context expires.
func (p *Processor) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Processor) processOne(ctx context.Context) error {
	p.wg.Add(1)
	defer p.wg.Done()

	job, err := p.repo.ClaimNextJob(ctx)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}

	slog.Info("processing investigation job",
		"job_id", job.ID,
		"incident_id", job.IncidentID,
		"attempt", job.Attempts,
	)

	if err := p.runInvestigation(ctx, job); err != nil {
		retry := job.Attempts < job.MaxAttempts
		if failErr := p.repo.FailJob(ctx, job.ID, err.Error(), retry); failErr != nil {
			return fmt.Errorf("fail job: %w", failErr)
		}
		if markErr := p.markInvestigationFailed(ctx, job.IncidentID, err.Error(), retry); markErr != nil {
			slog.Error("failed to update investigation status", "error", markErr)
		}
		if retry {
			_ = p.notifier.Notify(ctx, job.ID)
		}
		return err
	}

	if err := p.repo.CompleteJob(ctx, job.ID); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}

	return nil
}

func (p *Processor) runInvestigation(ctx context.Context, job *invdomain.Job) error {
	inv, err := p.repo.GetByIncidentID(ctx, job.IncidentID)
	if err != nil {
		return fmt.Errorf("get investigation: %w", err)
	}

	now := time.Now().UTC()
	inv.Status = invdomain.StatusRunning
	inv.StartedAt = &now
	inv.UpdatedAt = now
	inv.Error = ""
	if err := p.repo.UpdateInvestigation(ctx, inv); err != nil {
		return fmt.Errorf("update investigation: %w", err)
	}

	if _, err := p.incidents.Transition(ctx, domain.TransitionInput{
		IncidentID: job.IncidentID,
		ToStatus:   domain.StatusTriaging,
		Message:    "Investigation worker started triage",
	}); err != nil {
		return fmt.Errorf("transition to triaging: %w", err)
	}

	if _, err := p.incidents.Transition(ctx, domain.TransitionInput{
		IncidentID: job.IncidentID,
		ToStatus:   domain.StatusInvestigating,
		Message:    "Automated investigation in progress",
	}); err != nil {
		return fmt.Errorf("transition to investigating: %w", err)
	}

	var evidence []domaintool.Execution
	if p.tools != nil {
		batch, err := p.tools.ExecuteBatch(ctx, job.IncidentID, inv.ID, defaultInvestigationTools, "investigation-worker")
		if err != nil {
			slog.Warn("investigation tool batch partial failure", "error", err, "completed", len(batch))
		} else {
			slog.Info("investigation tools executed", "count", len(batch), "incident_id", job.IncidentID)
		}
		evidence = batch
	}

	if p.agent != nil {
		incident, err := p.incidents.GetByID(ctx, job.IncidentID)
		if err != nil {
			return fmt.Errorf("get incident for agent: %w", err)
		}

		result, err := p.agent.Run(ctx, agents.Input{
			Incident:      incident,
			Investigation: inv,
			Evidence:      evidence,
		})
		if err != nil {
			return fmt.Errorf("investigation agent: %w", err)
		}

		if result.AgentRun != nil {
			if saveErr := p.repo.CreateAgentRun(ctx, result.AgentRun); saveErr != nil {
				slog.Error("failed to persist agent run", "error", saveErr)
			}
		}

		if result.WaitingForAI {
			inv.Status = invdomain.StatusWaitingForAI
			inv.UpdatedAt = time.Now().UTC()
			if result.AgentRun != nil {
				inv.Error = result.AgentRun.Error
			}
			if err := p.repo.UpdateInvestigation(ctx, inv); err != nil {
				return fmt.Errorf("mark waiting_for_ai: %w", err)
			}
			slog.Warn("investigation waiting for AI", "incident_id", job.IncidentID)
			return nil
		}

		if result.RCA != nil {
			rcaJSON, err := json.Marshal(result.RCA)
			if err != nil {
				return fmt.Errorf("marshal rca: %w", err)
			}
			conf := result.RCA.Confidence
			inv.RootCause = rcaJSON
			inv.Confidence = &conf
			inv.ReasoningSummary = result.RCA.ReasoningSummary

			if _, err := p.incidents.Transition(ctx, domain.TransitionInput{
				IncidentID: job.IncidentID,
				ToStatus:   domain.StatusRootCauseIdentified,
				Message:    fmt.Sprintf("Root cause identified (confidence %.0f%%)", conf*100),
			}); err != nil {
				slog.Warn("failed to transition to ROOT_CAUSE_IDENTIFIED", "error", err)
			}

			if p.remediations != nil {
				if _, err := p.remediations.ProposeFromRCA(ctx, job.IncidentID, inv.ID, result.RCA); err != nil {
					slog.Warn("failed to auto-propose remediation", "error", err, "incident_id", job.IncidentID)
				}
			}
		}
	}

	completedAt := time.Now().UTC()
	inv.Status = invdomain.StatusCompleted
	inv.CompletedAt = &completedAt
	inv.UpdatedAt = completedAt
	if err := p.repo.UpdateInvestigation(ctx, inv); err != nil {
		return fmt.Errorf("complete investigation: %w", err)
	}

	slog.Info("investigation completed",
		"investigation_id", inv.ID,
		"incident_id", job.IncidentID,
	)

	return nil
}

func (p *Processor) markInvestigationFailed(ctx context.Context, incidentID uuid.UUID, jobErr string, retry bool) error {
	inv, err := p.repo.GetByIncidentID(ctx, incidentID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	inv.UpdatedAt = now
	inv.Error = jobErr
	if retry {
		inv.Status = invdomain.StatusPending
		inv.StartedAt = nil
		return p.repo.UpdateInvestigation(ctx, inv)
	}

	inv.Status = invdomain.StatusFailed
	inv.CompletedAt = &now
	return p.repo.UpdateInvestigation(ctx, inv)
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
