package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	AppEnv   string
	HTTPPort string

	DatabaseURL string
	RedisURL    string

	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPIdleTimeout  time.Duration

	LogLevel string

	LLMProvider string
	LLMAPIKey   string
	LLMModel    string
	LLMBaseURL  string
	LLMTimeout  time.Duration
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:           getEnv("APP_ENV", "development"),
		HTTPPort:         getEnv("HTTP_PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		HTTPReadTimeout:  getDurationEnv("HTTP_READ_TIMEOUT", 15*time.Second),
		HTTPWriteTimeout: getDurationEnv("HTTP_WRITE_TIMEOUT", 15*time.Second),
		HTTPIdleTimeout:  getDurationEnv("HTTP_IDLE_TIMEOUT", 60*time.Second),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		LLMProvider:      getEnv("LLM_PROVIDER", "mock"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		LLMModel:         getEnv("LLM_MODEL", "mock-rca-v1"),
		LLMBaseURL:       getEnv("LLM_BASE_URL", "https://api.openai.com/v1"),
		LLMTimeout:       getDurationEnv("LLM_TIMEOUT", 60*time.Second),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
