package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"

	agentrag "github.com/Revati-Firke/production-incident-investigator/internal/agent/rag"
	apprag "github.com/Revati-Firke/production-incident-investigator/internal/application/rag"
	"github.com/Revati-Firke/production-incident-investigator/internal/config"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/postgres"
	"github.com/Revati-Firke/production-incident-investigator/internal/pkg/logger"
)

func main() {
	_ = godotenv.Load()

	dir := flag.String("dir", "", "directory of markdown/text seed documents")
	force := flag.Bool("force", false, "ingest even when documents already exist")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if *dir == "" {
		*dir = cfg.RAGSeedDir
	}

	log := logger.New(cfg.LogLevel)
	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("postgres", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	embedder, err := newEmbedder(*cfg)
	if err != nil {
		log.Error("embedder", "error", err)
		os.Exit(1)
	}

	svc := apprag.NewService(postgres.NewRAGRepository(pool), embedder, apprag.Config{
		ChunkSize:    cfg.RAGChunkSize,
		ChunkOverlap: cfg.RAGChunkOverlap,
		TopK:         cfg.RAGTopK,
		MinScore:     cfg.RAGMinScore,
	})

	n, err := svc.SeedFromDirectory(ctx, *dir, *force)
	if err != nil {
		log.Error("seed failed", "error", err)
		os.Exit(1)
	}
	log.Info("knowledge ingest complete", "ingested", n, "dir", *dir, "embedder", svc.EmbedderName())
}

func newEmbedder(cfg config.Config) (agentrag.Embedder, error) {
	switch cfg.EmbeddingProvider {
	case "", "mock":
		return agentrag.NewMockEmbedder(), nil
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
		return nil, fmt.Errorf("unsupported EMBEDDING_PROVIDER %q", cfg.EmbeddingProvider)
	}
}
