package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
)

// Client searches logs via Loki or a deterministic mock.
type Client interface {
	Provider() string
	Search(ctx context.Context, req integrations.LogSearchRequest) ([]integrations.LogEntry, error)
}

// MockClient returns deterministic incident-shaped logs (Phase 3 behavior).
type MockClient struct{}

func NewMock() *MockClient { return &MockClient{} }

func (c *MockClient) Provider() string { return integrations.ProviderMock }

func (c *MockClient) Search(_ context.Context, req integrations.LogSearchRequest) ([]integrations.LogEntry, error) {
	now := time.Now().UTC()
	svc := req.Service
	env := req.Environment
	switch {
	case strings.Contains(svc, "payment"):
		return []integrations.LogEntry{
			{Timestamp: now.Add(-5 * time.Minute), Level: "error", Message: "database connection timeout after 30s", Labels: map[string]string{"service": svc, "env": env}},
			{Timestamp: now.Add(-4 * time.Minute), Level: "error", Message: "failed to acquire connection from pool", Labels: map[string]string{"service": svc, "env": env}},
			{Timestamp: now.Add(-3 * time.Minute), Level: "warn", Message: "connection pool exhausted: 98/100 in use", Labels: map[string]string{"service": svc, "env": env}},
		}, nil
	case strings.Contains(svc, "api"):
		return []integrations.LogEntry{
			{Timestamp: now.Add(-10 * time.Minute), Level: "warn", Message: "P95 latency 3200ms exceeded threshold", Labels: map[string]string{"service": svc}},
			{Timestamp: now.Add(-8 * time.Minute), Level: "error", Message: "upstream timeout calling inventory-service", Labels: map[string]string{"service": svc}},
		}, nil
	default:
		return []integrations.LogEntry{
			{Timestamp: now.Add(-2 * time.Minute), Level: "info", Message: fmt.Sprintf("%s running in %s", svc, env), Labels: map[string]string{"service": svc}},
		}, nil
	}
}

// HTTPClient queries Loki query_range API.
type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// Config configures a Loki HTTP client.
type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// NewHTTP creates a Loki HTTP client.
func NewHTTP(cfg Config) (*HTTPClient, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("LOKI_URL is required when LOKI_PROVIDER=loki")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPClient{
		baseURL: base,
		token:   cfg.Token,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *HTTPClient) Provider() string { return integrations.ProviderLoki }

func (c *HTTPClient) Search(ctx context.Context, req integrations.LogSearchRequest) ([]integrations.LogEntry, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	window := req.TimeRange
	if window <= 0 {
		window = time.Hour
	}
	end := time.Now().UTC()
	start := end.Add(-window)

	logQL := buildLogQL(req.Service, req.Environment, req.Query)
	u, err := url.Parse(c.baseURL + "/loki/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", logQL)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("direction", "backward")
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("loki request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("loki status %d: %s", resp.StatusCode, truncate(string(body), 300))
	}

	var parsed lokiRangeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode loki: %w", err)
	}
	entries := make([]integrations.LogEntry, 0)
	for _, stream := range parsed.Data.Result {
		for _, v := range stream.Values {
			if len(v) < 2 {
				continue
			}
			ts, _ := strconv.ParseInt(v[0], 10, 64)
			msg := v[1]
			level := detectLevel(msg)
			entries = append(entries, integrations.LogEntry{
				Timestamp: time.Unix(0, ts).UTC(),
				Level:     level,
				Message:   msg,
				Labels:    stream.Stream,
			})
		}
	}
	return entries, nil
}

func buildLogQL(service, env, query string) string {
	selector := `{job=~".+"}`
	parts := make([]string, 0, 2)
	if service != "" {
		parts = append(parts, fmt.Sprintf(`service="%s"`, escapeLabel(service)))
	}
	if env != "" {
		parts = append(parts, fmt.Sprintf(`environment="%s"`, escapeLabel(env)))
	}
	if len(parts) > 0 {
		selector = "{" + strings.Join(parts, ",") + "}"
	}
	q := strings.TrimSpace(query)
	if q == "" {
		q = "error|Error|ERROR|timeout|Timeout|exception"
	}
	return selector + " |~ `" + strings.ReplaceAll(q, "`", "") + "`"
}

func escapeLabel(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func detectLevel(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "error"), strings.Contains(lower, "fatal"):
		return "error"
	case strings.Contains(lower, "warn"):
		return "warn"
	default:
		return "info"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

type lokiRangeResponse struct {
	Data struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}
