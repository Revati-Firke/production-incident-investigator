package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/wiring"
	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	appinvestigation "github.com/Revati-Firke/production-incident-investigator/internal/application/investigation"
	"github.com/Revati-Firke/production-incident-investigator/internal/config"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/postgres"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/redis"
	"github.com/Revati-Firke/production-incident-investigator/internal/pkg/logger"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info("starting opspilot worker", "env", cfg.AppEnv, "llm_provider", cfg.LLMProvider)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dbPool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	redisClient, err := redis.NewClient(ctx, cfg.RedisURL)
	if err != nil {
		log.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Error("failed to close redis", "error", err)
		}
	}()

	incidentRepo := postgres.NewIncidentRepository(dbPool)
	investigationRepo := postgres.NewInvestigationRepository(dbPool)
	jobQueue := redis.NewJobQueue(redisClient)

	toolBundle, err := wiring.NewToolBundle(*cfg, dbPool, incidentRepo)
	if err != nil {
		log.Error("failed to bootstrap tools", "error", err)
		os.Exit(1)
	}

	if err := wiring.SeedKnowledgeBase(ctx, *cfg, toolBundle.RAG, log); err != nil {
		log.Warn("rag seed skipped or failed", "error", err)
	}

	agent, err := wiring.NewInvestigationAgent(*cfg, toolBundle)
	if err != nil {
		log.Error("failed to bootstrap investigation agent", "error", err)
		os.Exit(1)
	}

	incidentSvc := appincident.NewService(incidentRepo)
	processor := appinvestigation.NewProcessor(investigationRepo, incidentSvc, jobQueue, toolBundle.Service, agent)

	log.Info("worker ready, waiting for investigation jobs")

	if err := processor.Run(ctx); err != nil && err != context.Canceled {
		log.Error("worker stopped with error", "error", err)
		os.Exit(1)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := processor.Wait(shutdownCtx); err != nil {
		log.Warn("shutdown before in-flight jobs completed", "error", err)
	}

	log.Info("opspilot worker stopped")
}
