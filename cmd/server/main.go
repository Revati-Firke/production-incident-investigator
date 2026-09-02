package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	appinvestigation "github.com/Revati-Firke/production-incident-investigator/internal/application/investigation"
	"github.com/Revati-Firke/production-incident-investigator/internal/config"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/postgres"
	"github.com/Revati-Firke/production-incident-investigator/internal/infrastructure/redis"
	"github.com/Revati-Firke/production-incident-investigator/internal/pkg/logger"
	transporthttp "github.com/Revati-Firke/production-incident-investigator/internal/transport/http"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.LogLevel)
	log.Info("starting opspilot api", "env", cfg.AppEnv)

	ctx, cancel := context.WithCancel(context.Background())
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
	intakeRepo := postgres.NewIntakeRepository(dbPool)
	jobQueue := redis.NewJobQueue(redisClient)

	incidentSvc := appincident.NewService(incidentRepo)
	investigationSvc := appinvestigation.NewService(investigationRepo, jobQueue)
	orchestrator := appinvestigation.NewOrchestrator(incidentSvc, investigationSvc, intakeRepo)

	router := transporthttp.NewRouter(transporthttp.RouterDeps{
		Log:          log,
		Incidents:    incidentSvc,
		Orchestrator: orchestrator,
		Health: transporthttp.HealthDeps{
			Postgres: dbPool,
			Redis:    redisClient,
		},
	})

	server := transporthttp.NewServer(transporthttp.ServerConfig{
		Port:         cfg.HTTPPort,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}, router, log)

	go func() {
		if err := server.Start(); err != nil {
			log.Error("http server error", "error", err)
			cancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info("received shutdown signal", "signal", sig.String())
	case <-ctx.Done():
		log.Info("context cancelled, shutting down")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	log.Info("opspilot api stopped")
}
