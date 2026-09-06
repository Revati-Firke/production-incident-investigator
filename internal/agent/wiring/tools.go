package wiring

import (
	"fmt"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/agents"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm/mock"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm/openai"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/mocks"
	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	"github.com/Revati-Firke/production-incident-investigator/internal/config"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/postgres"
)

// ToolBundle holds wired tool components.
type ToolBundle struct {
	Registry *tools.Registry
	Executor *tools.Executor
	Service  *apptool.Service
}

// NewToolService wires the tool registry, executor, and application service.
func NewToolService(pool *postgres.Pool, incidentRepo *postgres.IncidentRepository) (*tools.Executor, *apptool.Service, error) {
	bundle, err := NewToolBundle(pool, incidentRepo)
	if err != nil {
		return nil, nil, err
	}
	return bundle.Executor, bundle.Service, nil
}

// NewToolBundle wires tools and returns the registry for agent use.
func NewToolBundle(pool *postgres.Pool, incidentRepo *postgres.IncidentRepository) (*ToolBundle, error) {
	registry, err := mocks.NewRegistry()
	if err != nil {
		return nil, err
	}

	toolRepo := postgres.NewToolRepository(pool)
	incidentCtx := appincident.NewIncidentContext(incidentRepo)
	executor := tools.NewExecutor(registry, toolRepo, 30*time.Second)
	svc := apptool.NewService(executor, toolRepo, incidentCtx)
	return &ToolBundle{Registry: registry, Executor: executor, Service: svc}, nil
}

// NewInvestigationAgent builds the Phase 4 investigation agent.
func NewInvestigationAgent(cfg config.Config, toolBundle *ToolBundle) (*agents.InvestigationAgent, error) {
	provider, err := newLLMProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("llm provider: %w", err)
	}
	return agents.NewInvestigationAgent(provider, toolBundle.Service, toolBundle.Registry), nil
}

func newLLMProvider(cfg config.Config) (llm.Provider, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.LLMProvider))
	if provider == "" || provider == "mock" {
		return mock.New(cfg.LLMModel), nil
	}
	switch provider {
	case "openai":
		p, err := openai.New(openai.Config{
			APIKey:  cfg.LLMAPIKey,
			Model:   cfg.LLMModel,
			BaseURL: cfg.LLMBaseURL,
			Timeout: cfg.LLMTimeout,
		})
		if err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unsupported LLM_PROVIDER %q (use mock or openai)", cfg.LLMProvider)
	}
}
