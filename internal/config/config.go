package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// RAG / embeddings (Phase 5)
	RAGEnabled        bool
	RAGTopK           int
	RAGMinScore       float64
	RAGChunkSize      int
	RAGChunkOverlap   int
	RAGSeedOnStart    bool
	RAGSeedDir        string
	EmbeddingProvider string
	EmbeddingModel    string
	EmbeddingAPIKey   string
	EmbeddingBaseURL  string
	EmbeddingTimeout  time.Duration

	// Integrations (Phase 6) — providers default to mock
	LokiProvider        string
	LokiURL             string
	LokiToken           string
	PrometheusProvider  string
	PrometheusURL       string
	PrometheusToken     string
	GrafanaProvider     string
	GrafanaURL          string
	GrafanaToken        string
	GitHubProvider      string
	GitHubToken         string
	GitHubRepository    string
	GitHubBaseURL       string
	GitHubWriteEnabled  bool
	SlackProvider       string
	SlackBotToken       string
	SlackDefaultChannel string
	IntegrationTimeout  time.Duration

	// Phase 7+
	PublicURL string

	// Phase 9 — OpenTelemetry
	OTelEndpoint string
	OTelService  string

	// Phase 10 — Hardening
	APIKey               string
	CORSOrigins          []string
	GrafanaWebhookSecret string
	RateLimitPerMinute   int
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

		RAGEnabled:        getBoolEnv("RAG_ENABLED", true),
		RAGTopK:           getIntEnv("RAG_TOP_K", 5),
		RAGMinScore:       getFloatEnv("RAG_MIN_SCORE", 0),
		RAGChunkSize:      getIntEnv("RAG_CHUNK_SIZE", 800),
		RAGChunkOverlap:   getIntEnv("RAG_CHUNK_OVERLAP", 120),
		RAGSeedOnStart:    getBoolEnv("RAG_SEED_ON_START", true),
		RAGSeedDir:        getEnv("RAG_SEED_DIR", "data/knowledge"),
		EmbeddingProvider: getEnv("EMBEDDING_PROVIDER", "mock"),
		EmbeddingModel:    getEnv("EMBEDDING_MODEL", "mock-embed-v1"),
		EmbeddingAPIKey:   firstNonEmpty(os.Getenv("EMBEDDING_API_KEY"), os.Getenv("LLM_API_KEY")),
		EmbeddingBaseURL:  getEnv("EMBEDDING_BASE_URL", getEnv("LLM_BASE_URL", "https://api.openai.com/v1")),
		EmbeddingTimeout:  getDurationEnv("EMBEDDING_TIMEOUT", 60*time.Second),

		LokiProvider:        getEnv("LOKI_PROVIDER", "mock"),
		LokiURL:             os.Getenv("LOKI_URL"),
		LokiToken:           os.Getenv("LOKI_TOKEN"),
		PrometheusProvider:  getEnv("PROMETHEUS_PROVIDER", "mock"),
		PrometheusURL:       os.Getenv("PROMETHEUS_URL"),
		PrometheusToken:     os.Getenv("PROMETHEUS_TOKEN"),
		GrafanaProvider:     getEnv("GRAFANA_PROVIDER", "mock"),
		GrafanaURL:          os.Getenv("GRAFANA_URL"),
		GrafanaToken:        os.Getenv("GRAFANA_TOKEN"),
		GitHubProvider:      getEnv("GITHUB_PROVIDER", "mock"),
		GitHubToken:         os.Getenv("GITHUB_TOKEN"),
		GitHubRepository:    os.Getenv("GITHUB_REPOSITORY"),
		GitHubBaseURL:       getEnv("GITHUB_BASE_URL", "https://api.github.com"),
		GitHubWriteEnabled:  getBoolEnv("GITHUB_WRITE_ENABLED", false),
		SlackProvider:       getEnv("SLACK_PROVIDER", "mock"),
		SlackBotToken:       os.Getenv("SLACK_BOT_TOKEN"),
		SlackDefaultChannel: getEnv("SLACK_DEFAULT_CHANNEL", "#incidents"),
		IntegrationTimeout:  getDurationEnv("INTEGRATION_TIMEOUT", 15*time.Second),

		PublicURL:            getEnv("PUBLIC_URL", "http://localhost:8080"),
		OTelEndpoint:         os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		OTelService:          getEnv("OTEL_SERVICE_NAME", "opspilot"),
		APIKey:               os.Getenv("API_KEY"),
		CORSOrigins:          splitCSV(getEnv("CORS_ORIGINS", "http://localhost:5173,http://localhost:3000")),
		GrafanaWebhookSecret: os.Getenv("GRAFANA_WEBHOOK_SECRET"),
		RateLimitPerMinute:   getIntEnv("RATE_LIMIT_PER_MINUTE", 120),
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

func getBoolEnv(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getFloatEnv(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
