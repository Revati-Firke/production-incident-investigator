package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// ToolRepository implements tool execution persistence.
type ToolRepository struct {
	pool *Pool
}

// NewToolRepository creates a new tool repository.
func NewToolRepository(pool *Pool) *ToolRepository {
	return &ToolRepository{pool: pool}
}

// Create inserts a tool execution record.
func (r *ToolRepository) Create(ctx context.Context, exec *domaintool.Execution) error {
	query := `
		INSERT INTO tool_executions (
			id, incident_id, investigation_id, tool_name, permission_level,
			status, agent, input, output, error, duration_ms, created_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

	_, err := r.pool.Exec(ctx, query,
		exec.ID, exec.IncidentID, exec.InvestigationID,
		exec.ToolName, string(exec.PermissionLevel), string(exec.Status),
		exec.Agent, exec.Input, nullJSON(exec.Output), nullIfEmpty(exec.Error),
		exec.DurationMS, exec.CreatedAt, exec.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("insert tool execution: %w", err)
	}
	return nil
}

// Update updates a tool execution record.
func (r *ToolRepository) Update(ctx context.Context, exec *domaintool.Execution) error {
	query := `
		UPDATE tool_executions
		SET status = $1, output = $2, error = $3, duration_ms = $4, completed_at = $5
		WHERE id = $6`

	tag, err := r.pool.Exec(ctx, query,
		string(exec.Status), nullJSON(exec.Output), nullIfEmpty(exec.Error),
		exec.DurationMS, exec.CompletedAt, exec.ID,
	)
	if err != nil {
		return fmt.Errorf("update tool execution: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domaintool.ErrNotFound
	}
	return nil
}

// GetByID returns a tool execution by ID.
func (r *ToolRepository) GetByID(ctx context.Context, id uuid.UUID) (*domaintool.Execution, error) {
	query := `
		SELECT id, incident_id, investigation_id, tool_name, permission_level,
		       status, agent, input, output, error, duration_ms, created_at, completed_at
		FROM tool_executions WHERE id = $1`

	row := r.pool.QueryRow(ctx, query, id)
	exec, err := scanToolExecution(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domaintool.ErrNotFound
		}
		return nil, fmt.Errorf("get tool execution: %w", err)
	}
	return exec, nil
}

// ListByIncidentID returns tool executions for an incident.
func (r *ToolRepository) ListByIncidentID(ctx context.Context, incidentID uuid.UUID) ([]domaintool.Execution, error) {
	query := `
		SELECT id, incident_id, investigation_id, tool_name, permission_level,
		       status, agent, input, output, error, duration_ms, created_at, completed_at
		FROM tool_executions
		WHERE incident_id = $1
		ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, query, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list tool executions: %w", err)
	}
	defer rows.Close()

	execs := make([]domaintool.Execution, 0)
	for rows.Next() {
		exec, err := scanToolExecution(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tool execution: %w", err)
		}
		execs = append(execs, *exec)
	}
	return execs, rows.Err()
}

type toolScannable interface {
	Scan(dest ...any) error
}

func scanToolExecution(row toolScannable) (*domaintool.Execution, error) {
	var exec domaintool.Execution
	var perm, status string
	var output []byte
	var errStr *string

	if err := row.Scan(
		&exec.ID, &exec.IncidentID, &exec.InvestigationID,
		&exec.ToolName, &perm, &status, &exec.Agent,
		&exec.Input, &output, &errStr, &exec.DurationMS,
		&exec.CreatedAt, &exec.CompletedAt,
	); err != nil {
		return nil, err
	}

	exec.PermissionLevel = domaintool.PermissionLevel(perm)
	exec.Status = domaintool.ExecutionStatus(status)
	if len(output) > 0 {
		exec.Output = output
	}
	if errStr != nil {
		exec.Error = *errStr
	}
	return &exec, nil
}

func nullJSON(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	return data
}
