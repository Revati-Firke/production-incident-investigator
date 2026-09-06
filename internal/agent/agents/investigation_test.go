package agents_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/agents"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm/mock"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	toolmocks "github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/mocks"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

func TestInvestigationAgent_DatabaseExhaustionRCA(t *testing.T) {
	registry, err := toolmocks.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	provider := mock.New("mock-rca-v1")
	agent := agents.NewInvestigationAgent(provider, nil, registry)

	incidentID := uuid.New()
	invID := uuid.New()
	now := time.Now().UTC()

	output, _ := json.Marshal(map[string]any{
		"summary": "Found connection timeout errors",
		"data": map[string]any{
			"entries": []map[string]any{
				{"message": "database connection timeout after 30s"},
				{"message": "connection pool exhausted: 98/100 in use"},
			},
		},
	})
	metricsOut, _ := json.Marshal(map[string]any{
		"metrics": []map[string]any{
			{"name": "db_connections_active", "value": 98},
			{"name": "cpu_usage", "value": 42},
		},
	})
	deployOut, _ := json.Marshal(map[string]any{
		"deployments": []map[string]any{
			{"id": "dep-abc123", "version": "v2.4.1"},
		},
	})

	result, err := agent.Run(context.Background(), agents.Input{
		Incident: &domain.Incident{
			ID:          incidentID,
			Title:       "Payment API failures",
			Severity:    domain.SeverityCritical,
			Service:     "payment-service",
			Environment: "production",
			Description: "DB timeouts",
		},
		Investigation: &invdomain.Investigation{
			ID:         invID,
			IncidentID: incidentID,
			Status:     invdomain.StatusRunning,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		Evidence: []domaintool.Execution{
			{ID: uuid.New(), ToolName: "search_logs", Status: domaintool.StatusCompleted, Output: output},
			{ID: uuid.New(), ToolName: "query_metrics", Status: domaintool.StatusCompleted, Output: metricsOut},
			{ID: uuid.New(), ToolName: "get_recent_deployments", Status: domaintool.StatusCompleted, Output: deployOut},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.WaitingForAI {
		t.Fatal("did not expect waiting_for_ai with mock provider")
	}
	if result.RCA == nil {
		t.Fatal("expected RCA")
	}
	if result.RCA.Confidence < 0.8 {
		t.Errorf("confidence = %v, want >= 0.8", result.RCA.Confidence)
	}
	if result.RCA.RootCause == "" {
		t.Error("expected root_cause")
	}
	if len(result.RCA.Evidence) == 0 {
		t.Error("expected evidence")
	}
	if result.AgentRun == nil || result.AgentRun.Status != invdomain.AgentRunCompleted {
		t.Errorf("agent run status = %+v", result.AgentRun)
	}
}

func TestInvestigationAgent_RejectsInvalidEmptyEvidencePath(t *testing.T) {
	// Ensure registry tools exist for schema but evidence is thin; mock still returns valid RCA.
	registry := tools.NewRegistry()
	provider := mock.New("")
	agent := agents.NewInvestigationAgent(provider, nil, registry)

	result, err := agent.Run(context.Background(), agents.Input{
		Incident: &domain.Incident{
			ID: uuid.New(), Title: "Unknown issue", Severity: domain.SeverityMedium,
			Service: "other-service", Environment: "staging",
		},
		Investigation: &invdomain.Investigation{ID: uuid.New(), IncidentID: uuid.New()},
		Evidence:      nil,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.RCA == nil {
		t.Fatal("expected fallback RCA")
	}
	if result.RCA.Confidence > 0.7 {
		t.Errorf("expected lower confidence for weak evidence, got %v", result.RCA.Confidence)
	}
}
