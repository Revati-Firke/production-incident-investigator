package wiring

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/agents"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm/mock"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm/openai"
	agentrag "github.com/Revati-Firke/production-incident-investigator/internal/agent/rag"
	ragtools "github.com/Revati-Firke/production-incident-investigator/internal/agent/rag/tools"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/mocks"
	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	apprag "github.com/Revati-Firke/production-incident-investigator/internal/application/rag"
	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	"github.com/Revati-Firke/production-incident-investigator/internal/config"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/postgres"
)

// ToolBundle holds wired tool components.
type ToolBundle struct {
	Registry *tools.Registry
	Executor *tools.Executor
	Service  *apptool.Service
	RAG      *apprag.Service
}

// NewToolService wires the tool registry, executor, and application service.
func NewToolService(cfg config.Config, pool *postgres.Pool, incidentRepo *postgres.IncidentRepository) (*tools.Executor, *apptool.Service, error) {
	bundle, err := NewToolBundle(cfg, pool, incidentRepo)
	if err != nil {
		return nil, nil, err
	}
	return bundle.Executor, bundle.Service, nil
}

// NewToolBundle wires tools (including Phase 5 RAG knowledge tools when enabled).
func NewToolBundle(cfg config.Config, pool *postgres.Pool, incidentRepo *postgres.IncidentRepository) (*ToolBundle, error) {
	registry := tools.NewRegistry()
	if err := mocks.RegisterCore(registry); err != nil {
		return nil, err
	}

	var ragSvc *apprag.Service
	if cfg.RAGEnabled {
		embedder, err := newEmbedder(cfg)
		if err != nil {
			return nil, fmt.Errorf("embedder: %w", err)
		}
		ragRepo := postgres.NewRAGRepository(pool)
		ragSvc = apprag.NewService(ragRepo, embedder, apprag.Config{
			ChunkSize:    cfg.RAGChunkSize,
			ChunkOverlap: cfg.RAGChunkOverlap,
			TopK:         cfg.RAGTopK,
			MinScore:     cfg.RAGMinScore,
		})
		if err := registry.Register(ragtools.NewSearchRunbooksTool(ragSvc, cfg.RAGTopK)); err != nil {
			return nil, err
		}
		if err := registry.Register(ragtools.NewSearchIncidentsTool(ragSvc, cfg.RAGTopK)); err != nil {
			return nil, err
		}
	} else if err := mocks.RegisterKnowledgeMocks(registry); err != nil {
		return nil, err
	}

	toolRepo := postgres.NewToolRepository(pool)
	incidentCtx := appincident.NewIncidentContext(incidentRepo)
	executor := tools.NewExecutor(registry, toolRepo, 30*time.Second)
	svc := apptool.NewService(executor, toolRepo, incidentCtx)
	return &ToolBundle{Registry: registry, Executor: executor, Service: svc, RAG: ragSvc}, nil
}

// SeedKnowledgeBase ingests seed markdown when configured and the store is empty.
func SeedKnowledgeBase(ctx context.Context, cfg config.Config, ragSvc *apprag.Service, log *slog.Logger) error {
	if ragSvc == nil || !cfg.RAGSeedOnStart {
		return nil
	}
	n, err := ragSvc.SeedFromDirectory(ctx, cfg.RAGSeedDir, false)
	if err != nil {
		return err
	}
	if log != nil {
		log.Info("rag seed complete", "ingested", n, "dir", cfg.RAGSeedDir, "embedder", ragSvc.EmbedderName())
	}
	return nil
}

// NewInvestigationAgent builds the investigation agent.
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

func newEmbedder(cfg config.Config) (agentrag.Embedder, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.EmbeddingProvider))
	if provider == "" || provider == "mock" {
		return agentrag.NewMockEmbedder(), nil
	}
	switch provider {
	case "openai":
		model := cfg.EmbeddingModel
		if model == "" || model == "mock-embed-v1" {
			model = "text-embedding-3-small"
		}
		return agentrag.NewOpenAIEmbedder(agentrag.OpenAIEmbedderConfig{
			APIKey:  cfg.EmbeddingAPIKey,
			Model:   model,
			BaseURL: cfg.EmbeddingBaseURL,
			Timeout: cfg.EmbeddingTimeout,
		})
	default:
		return nil, fmt.Errorf("unsupported EMBEDDING_PROVIDER %q (use mock or openai)", cfg.EmbeddingProvider)
	}
}
