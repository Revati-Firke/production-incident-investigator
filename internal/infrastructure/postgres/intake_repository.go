package postgres

import (
	"context"
	"fmt"

	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

// IntakeRepository atomically persists incident and investigation intake records.
type IntakeRepository struct {
	pool *Pool
}

// NewIntakeRepository creates a new intake repository.
func NewIntakeRepository(pool *Pool) *IntakeRepository {
	return &IntakeRepository{pool: pool}
}

// Persist stores incident, event, investigation, and job in a single transaction.
func (r *IntakeRepository) Persist(
	ctx context.Context,
	inc *domain.Incident,
	event *domain.IncidentEvent,
	inv *invdomain.Investigation,
	job *invdomain.Job,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	insertIncident := `
		INSERT INTO incidents (
			id, title, description, severity, service, environment,
			alert_source, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	if _, err := tx.Exec(ctx, insertIncident,
		inc.ID, inc.Title, inc.Description, string(inc.Severity),
		inc.Service, inc.Environment, inc.AlertSource, string(inc.Status),
		inc.CreatedAt, inc.UpdatedAt,
	); err != nil {
		return fmt.Errorf("insert incident: %w", err)
	}

	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}

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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
