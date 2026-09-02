package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/config"
)

func TestLoad_RequiredFields(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("REDIS_URL")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when required env vars missing")
	}
}

func TestLoad_Success(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/opspilot?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("HTTP_PORT", "9090")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPPort != "9090" {
		t.Errorf("HTTPPort = %s, want 9090", cfg.HTTPPort)
	}
	if cfg.HTTPReadTimeout != 15*time.Second {
		t.Errorf("HTTPReadTimeout = %v", cfg.HTTPReadTimeout)
	}
}
