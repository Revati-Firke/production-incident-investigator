package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

// InvestigationRepository implements investigation persistence.
type InvestigationRepository struct {
	pool *Pool
}

// NewInvestigationRepository creates a new investigation repository.
func NewInvestigationRepository(pool *Pool) *InvestigationRepository {
	return &InvestigationRepository{pool: pool}
}

// CreateWithJob atomically creates an investigation and its job.
func (r *InvestigationRepository) CreateWithJob(ctx context.Context, inv *invdomain.Investigation, job *invdomain.Job) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	insertInv := `
		INSERT INTO investigations (id, incident_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`

	if _, err := tx.Exec(ctx, insertInv,
		inv.ID, inv.IncidentID, string(inv.Status), inv.CreatedAt, inv.UpdatedAt,
	); err != nil {
		return fmt.Errorf("insert investigation: %w", err)
	}

	insertJob := `
		INSERT INTO investigation_jobs (
			id, incident_id, investigation_id, status, max_attempts,
			scheduled_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	if _, err := tx.Exec(ctx, insertJob,
		job.ID, job.IncidentID, job.InvestigationID, string(job.Status),
		job.MaxAttempts, job.ScheduledAt, job.CreatedAt, job.UpdatedAt,
	); err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	return tx.Commit(ctx)
}

// GetByIncidentID returns the investigation for an incident.
func (r *InvestigationRepository) GetByIncidentID(ctx context.Context, incidentID uuid.UUID) (*invdomain.Investigation, error) {
	query := `
		SELECT id, incident_id, status, started_at, completed_at, error, created_at, updated_at
		FROM investigations WHERE incident_id = $1`

	row := r.pool.QueryRow(ctx, query, incidentID)
	inv, err := scanInvestigation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invdomain.ErrNotFound
		}
		return nil, fmt.Errorf("get investigation: %w", err)
	}
	return inv, nil
}

// UpdateInvestigation updates an investigation record.
func (r *InvestigationRepository) UpdateInvestigation(ctx context.Context, inv *invdomain.Investigation) error {
	query := `
		UPDATE investigations
		SET status = $1, started_at = $2, completed_at = $3, error = $4, updated_at = $5
		WHERE id = $6`

	tag, err := r.pool.Exec(ctx, query,
		string(inv.Status), inv.StartedAt, inv.CompletedAt, nullIfEmpty(inv.Error),
		inv.UpdatedAt, inv.ID,
	)
	if err != nil {
		return fmt.Errorf("update investigation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return invdomain.ErrNotFound
	}
	return nil
}

// ClaimNextJob atomically claims the next pending job using SKIP LOCKED.
func (r *InvestigationRepository) ClaimNextJob(ctx context.Context) (*invdomain.Job, error) {
	query := `
		UPDATE investigation_jobs
		SET status = 'processing',
		    attempts = attempts + 1,
		    started_at = NOW(),
		    updated_at = NOW()
		WHERE id = (
			SELECT id FROM investigation_jobs
			WHERE status = 'pending' AND scheduled_at <= NOW()
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, incident_id, investigation_id, status, attempts, max_attempts,
		          error, scheduled_at, started_at, completed_at, created_at, updated_at`

	row := r.pool.QueryRow(ctx, query)
	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim job: %w", err)
	}
	return job, nil
}

// CompleteJob marks a job as completed.
func (r *InvestigationRepository) CompleteJob(ctx context.Context, jobID uuid.UUID) error {
	query := `
		UPDATE investigation_jobs
		SET status = 'completed', completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'processing'`

	tag, err := r.pool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return invdomain.ErrNotFound
	}
	return nil
}

// FailJob marks a job as failed, optionally requeueing for retry.
func (r *InvestigationRepository) FailJob(ctx context.Context, jobID uuid.UUID, jobErr string, retry bool) error {
	var query string
	if retry {
		query = `
			UPDATE investigation_jobs
			SET status = 'pending',
			    error = $1,
			    started_at = NULL,
			    scheduled_at = NOW() + INTERVAL '30 seconds',
			    updated_at = NOW()
			WHERE id = $2 AND status = 'processing'`
	} else {
		query = `
			UPDATE investigation_jobs
			SET status = 'dead_letter',
			    error = $1,
			    completed_at = NOW(),
			    updated_at = NOW()
			WHERE id = $2 AND status = 'processing'`
	}

	tag, err := r.pool.Exec(ctx, query, jobErr, jobID)
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return invdomain.ErrNotFound
	}
	return nil
}

func scanInvestigation(row pgx.Row) (*invdomain.Investigation, error) {
	var inv invdomain.Investigation
	var status string
	var errStr *string
	if err := row.Scan(
		&inv.ID, &inv.IncidentID, &status,
		&inv.StartedAt, &inv.CompletedAt, &errStr,
		&inv.CreatedAt, &inv.UpdatedAt,
	); err != nil {
		return nil, err
	}
	inv.Status = invdomain.InvestigationStatus(status)
	if errStr != nil {
		inv.Error = *errStr
	}
	return &inv, nil
}

func scanJob(row pgx.Row) (*invdomain.Job, error) {
	var job invdomain.Job
	var status string
	var errStr *string
	if err := row.Scan(
		&job.ID, &job.IncidentID, &job.InvestigationID, &status,
		&job.Attempts, &job.MaxAttempts, &errStr,
		&job.ScheduledAt, &job.StartedAt, &job.CompletedAt,
		&job.CreatedAt, &job.UpdatedAt,
	); err != nil {
		return nil, err
	}
	job.Status = invdomain.JobStatus(status)
	if errStr != nil {
		job.Error = *errStr
	}
	return &job, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
