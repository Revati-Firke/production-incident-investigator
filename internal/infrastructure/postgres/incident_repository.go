package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
)

// IncidentRepository implements incident persistence with PostgreSQL.
type IncidentRepository struct {
	pool *Pool
}

// NewIncidentRepository creates a new incident repository.
func NewIncidentRepository(pool *Pool) *IncidentRepository {
	return &IncidentRepository{pool: pool}
}

// CreateWithEvent atomically inserts an incident and its creation event.
func (r *IncidentRepository) CreateWithEvent(ctx context.Context, inc *domain.Incident, event *domain.IncidentEvent) error {
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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// TransitionWithEvent atomically updates status with optimistic locking and records an event.
func (r *IncidentRepository) TransitionWithEvent(
	ctx context.Context,
	id uuid.UUID,
	expectedStatus, newStatus domain.Status,
	event *domain.IncidentEvent,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updateQuery := `
		UPDATE incidents
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND status = $3`

	tag, err := tx.Exec(ctx, updateQuery, string(newStatus), id, string(expectedStatus))
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConcurrentModification
	}

	if err := insertEvent(ctx, tx, event); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// GetByID returns an incident by ID.
func (r *IncidentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Incident, error) {
	query := `
		SELECT id, title, description, severity, service, environment,
		       alert_source, status, created_at, updated_at
		FROM incidents WHERE id = $1`

	row := r.pool.QueryRow(ctx, query, id)
	inc, err := scanIncident(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get incident: %w", err)
	}
	return inc, nil
}

// List returns incidents matching the filter and total count.
func (r *IncidentRepository) List(ctx context.Context, filter domain.ListFilter) ([]domain.Incident, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*filter.Status))
		argIdx++
	}
	if filter.Service != "" {
		conditions = append(conditions, fmt.Sprintf("service = $%d", argIdx))
		args = append(args, filter.Service)
		argIdx++
	}
	if filter.Environment != "" {
		conditions = append(conditions, fmt.Sprintf("environment = $%d", argIdx))
		args = append(args, filter.Environment)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM incidents " + where
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count incidents: %w", err)
	}

	listQuery := fmt.Sprintf(`
		SELECT id, title, description, severity, service, environment,
		       alert_source, status, created_at, updated_at
		FROM incidents %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, filter.Limit, filter.Offset)
	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	incidents := make([]domain.Incident, 0)
	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan incident: %w", err)
		}
		incidents = append(incidents, *inc)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate incidents: %w", err)
	}

	return incidents, total, nil
}

// ListEvents returns events for an incident ordered by time.
func (r *IncidentRepository) ListEvents(ctx context.Context, incidentID uuid.UUID) ([]domain.IncidentEvent, error) {
	query := `
		SELECT id, incident_id, event_type, from_status, to_status, message, metadata, created_at
		FROM incident_events
		WHERE incident_id = $1
		ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	events := make([]domain.IncidentEvent, 0)
	for rows.Next() {
		var e domain.IncidentEvent
		var fromStatus, toStatus *string
		if err := rows.Scan(
			&e.ID, &e.IncidentID, &e.EventType,
			&fromStatus, &toStatus, &e.Message, &e.Metadata, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if fromStatus != nil {
			s := domain.Status(*fromStatus)
			e.FromStatus = &s
		}
		if toStatus != nil {
			s := domain.Status(*toStatus)
			e.ToStatus = &s
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func insertEvent(ctx context.Context, tx pgx.Tx, event *domain.IncidentEvent) error {
	var fromStatus, toStatus *string
	if event.FromStatus != nil {
		s := string(*event.FromStatus)
		fromStatus = &s
	}
	if event.ToStatus != nil {
		s := string(*event.ToStatus)
		toStatus = &s
	}

	query := `
		INSERT INTO incident_events (
			id, incident_id, event_type, from_status, to_status, message, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := tx.Exec(ctx, query,
		event.ID, event.IncidentID, event.EventType,
		fromStatus, toStatus, event.Message, event.Metadata, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert incident event: %w", err)
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanIncident(row scannable) (*domain.Incident, error) {
	var inc domain.Incident
	var severity, status string
	if err := row.Scan(
		&inc.ID, &inc.Title, &inc.Description, &severity,
		&inc.Service, &inc.Environment, &inc.AlertSource, &status,
		&inc.CreatedAt, &inc.UpdatedAt,
	); err != nil {
		return nil, err
	}
	inc.Severity = domain.Severity(severity)
	inc.Status = domain.Status(status)
	return &inc, nil
}
