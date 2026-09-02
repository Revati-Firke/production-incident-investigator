package tool

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// IncidentLookup provides incident context for tool execution.
type IncidentLookup interface {
	GetServiceEnvironment(ctx context.Context, incidentID uuid.UUID) (service, environment string, err error)
}

// Service provides tool listing and execution.
type Service struct {
	executor *tools.Executor
	repo     domaintool.Repository
	incident IncidentLookup
}

// NewService creates a tool application service.
func NewService(executor *tools.Executor, repo domaintool.Repository, incident IncidentLookup) *Service {
	return &Service{executor: executor, repo: repo, incident: incident}
}

// ListTools returns registered tool metadata.
func (s *Service) ListTools() []domaintool.ToolInfo {
	return s.executor.ListTools()
}

// Execute runs a tool in the context of an incident.
func (s *Service) Execute(ctx context.Context, input domaintool.ExecuteInput) (*domaintool.Execution, error) {
	service, environment, err := s.incident.GetServiceEnvironment(ctx, input.IncidentID)
	if err != nil {
		return nil, err
	}
	input.Service = service
	input.Environment = environment
	return s.executor.Execute(ctx, input)
}

// ListEvidence returns tool executions as incident evidence.
func (s *Service) ListEvidence(ctx context.Context, incidentID uuid.UUID) ([]domaintool.Execution, error) {
	if _, _, err := s.incident.GetServiceEnvironment(ctx, incidentID); err != nil {
		return nil, err
	}
	return s.repo.ListByIncidentID(ctx, incidentID)
}

// ExecuteBatch runs multiple read-only tools concurrently for an investigation.
func (s *Service) ExecuteBatch(ctx context.Context, incidentID uuid.UUID, investigationID uuid.UUID, toolNames []string, agent string) ([]domaintool.Execution, error) {
	service, environment, err := s.incident.GetServiceEnvironment(ctx, incidentID)
	if err != nil {
		return nil, err
	}

	results := make([]domaintool.Execution, 0, len(toolNames))
	for _, name := range toolNames {
		invID := investigationID
		exec, err := s.executor.Execute(ctx, domaintool.ExecuteInput{
			IncidentID:      incidentID,
			InvestigationID: &invID,
			ToolName:        name,
			Agent:           agent,
			Service:         service,
			Environment:     environment,
		})
		if err != nil {
			if exec != nil {
				results = append(results, *exec)
			}
			return results, fmt.Errorf("tool %s: %w", name, err)
		}
		results = append(results, *exec)
	}
	return results, nil
}
