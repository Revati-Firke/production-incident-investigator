package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// ExecutionStore persists tool execution records.
type ExecutionStore interface {
	Create(ctx context.Context, exec *domaintool.Execution) error
	Update(ctx context.Context, exec *domaintool.Execution) error
}

// Executor runs tools with permission checks, timeouts, and audit logging.
type Executor struct {
	registry *Registry
	store    ExecutionStore
	timeout  time.Duration
}

// NewExecutor creates a tool executor.
func NewExecutor(registry *Registry, store ExecutionStore, timeout time.Duration) *Executor {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Executor{registry: registry, store: store, timeout: timeout}
}

// Execute runs a tool and persists the audit record.
func (e *Executor) Execute(ctx context.Context, req domaintool.ExecuteInput) (*domaintool.Execution, error) {
	tool, ok := e.registry.Get(req.ToolName)
	if !ok {
		return nil, domaintool.ErrToolNotFound
	}

	if err := Authorize(tool.Permission(), req.Approved); err != nil {
		return nil, err
	}

	if len(req.Input) == 0 {
		req.Input = json.RawMessage(`{}`)
	}
	if !json.Valid(req.Input) {
		return nil, fmt.Errorf("%w: invalid JSON", domaintool.ErrInvalidInput)
	}

	now := time.Now().UTC()
	status := domaintool.StatusRunning
	if tool.Permission() == PermissionRequiresApproval && !req.Approved {
		status = domaintool.StatusAwaitingApproval
	}

	exec := &domaintool.Execution{
		ID:              uuid.New(),
		IncidentID:      req.IncidentID,
		InvestigationID: req.InvestigationID,
		ToolName:        req.ToolName,
		PermissionLevel: ToDomainPermission(tool.Permission()),
		Status:          status,
		Agent:           req.Agent,
		Input:           req.Input,
		CreatedAt:       now,
	}

	if err := e.store.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("create execution record: %w", err)
	}

	if status == domaintool.StatusAwaitingApproval {
		return exec, domaintool.ErrApprovalRequired
	}

	runCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	start := time.Now()
	toolCtx := Context{
		IncidentID:  req.IncidentID.String(),
		Service:     req.Service,
		Environment: req.Environment,
	}

	result, err := tool.Execute(runCtx, req.Input, toolCtx)
	durationMS := int(time.Since(start).Milliseconds())
	exec.DurationMS = &durationMS
	completed := time.Now().UTC()
	exec.CompletedAt = &completed

	if err != nil {
		exec.Status = domaintool.StatusFailed
		exec.Error = err.Error()
		if updateErr := e.store.Update(ctx, exec); updateErr != nil {
			slog.Error("failed to update tool execution", "error", updateErr)
		}
		slog.Warn("tool execution failed",
			"tool", req.ToolName,
			"incident_id", req.IncidentID,
			"error", err,
		)
		return exec, fmt.Errorf("%w: %v", domaintool.ErrExecutionFailed, err)
	}

	output, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		exec.Status = domaintool.StatusFailed
		exec.Error = marshalErr.Error()
		_ = e.store.Update(ctx, exec)
		return exec, fmt.Errorf("marshal tool output: %w", marshalErr)
	}

	exec.Status = domaintool.StatusCompleted
	exec.Output = output
	if err := e.store.Update(ctx, exec); err != nil {
		return exec, fmt.Errorf("update execution record: %w", err)
	}

	slog.Info("tool executed",
		"tool", req.ToolName,
		"incident_id", req.IncidentID,
		"duration_ms", durationMS,
		"agent", req.Agent,
	)

	return exec, nil
}

// ListTools returns metadata for all registered tools.
func (e *Executor) ListTools() []domaintool.ToolInfo {
	tools := e.registry.List()
	info := make([]domaintool.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info = append(info, domaintool.ToolInfo{
			Name:            t.Name(),
			Description:     t.Description(),
			PermissionLevel: ToDomainPermission(t.Permission()),
			InputSchema:     t.InputSchema(),
		})
	}
	return info
}
