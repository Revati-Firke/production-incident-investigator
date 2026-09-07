package prometheus

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

// Client queries metrics via Prometheus or a deterministic mock.
type Client interface {
	Provider() string
	Query(ctx context.Context, req integrations.MetricsQueryRequest) ([]integrations.MetricSample, error)
}

// MockClient returns deterministic metrics (Phase 3 behavior).
type MockClient struct{}

func NewMock() *MockClient { return &MockClient{} }

func (c *MockClient) Provider() string { return integrations.ProviderMock }

func (c *MockClient) Query(_ context.Context, req integrations.MetricsQueryRequest) ([]integrations.MetricSample, error) {
	if strings.Contains(req.Service, "payment") {
		return []integrations.MetricSample{
			{Name: "db_connections_active", Value: 98, Unit: "connections", Max: 100},
			{Name: "cpu_usage", Value: 42, Unit: "percent"},
			{Name: "error_rate", Value: 12.4, Unit: "percent"},
		}, nil
	}
	name := req.Metric
	if name == "" {
		name = "request_latency_p95"
	}
	return []integrations.MetricSample{
		{Name: name, Value: 3200, Unit: "ms"},
		{Name: "cpu_usage", Value: 38, Unit: "percent"},
		{Name: "error_rate", Value: 2.1, Unit: "percent"},
	}, nil
}

// HTTPClient queries Prometheus instant queries.
type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// Config configures Prometheus HTTP client.
type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// NewHTTP creates a Prometheus HTTP client.
func NewHTTP(cfg Config) (*HTTPClient, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("PROMETHEUS_URL is required when PROMETHEUS_PROVIDER=prometheus")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPClient{baseURL: base, token: cfg.Token, client: &http.Client{Timeout: timeout}}, nil
}

func (c *HTTPClient) Provider() string { return integrations.ProviderProm }

func (c *HTTPClient) Query(ctx context.Context, req integrations.MetricsQueryRequest) ([]integrations.MetricSample, error) {
	queries := defaultQueries(req.Service, req.Metric)
	out := make([]integrations.MetricSample, 0, len(queries))
	for name, promQL := range queries {
		val, err := c.instant(ctx, promQL)
		if err != nil {
			return nil, err
		}
		sample := integrations.MetricSample{Name: name, Value: val}
		switch name {
		case "cpu_usage", "error_rate":
			sample.Unit = "percent"
		case "request_latency_p95":
			sample.Unit = "ms"
		case "db_connections_active":
			sample.Unit = "connections"
		}
		out = append(out, sample)
	}
	return out, nil
}

func (c *HTTPClient) instant(ctx context.Context, promQL string) (float64, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("query", promQL)
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("prometheus request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("prometheus status %d: %s", resp.StatusCode, string(body))
	}
	var parsed promQueryResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if parsed.Status != "success" || len(parsed.Data.Result) == 0 {
		return 0, nil
	}
	raw := parsed.Data.Result[0].Value
	if len(raw) < 2 {
		return 0, nil
	}
	switch v := raw[1].(type) {
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, err
		}
		return f, nil
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected prometheus value type %T", v)
	}
}

func defaultQueries(service, metric string) map[string]string {
	svc := escapeProm(service)
	if metric != "" {
		return map[string]string{
			metric: metric,
		}
	}
	return map[string]string{
		"request_latency_p95":   fmt.Sprintf(`histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{service="%s"}[5m])) by (le)) * 1000`, svc),
		"error_rate":            fmt.Sprintf(`sum(rate(http_requests_total{service="%s",status=~"5.."}[5m])) / sum(rate(http_requests_total{service="%s"}[5m])) * 100`, svc, svc),
		"cpu_usage":             fmt.Sprintf(`avg(rate(container_cpu_usage_seconds_total{pod=~"%s.*"}[5m])) * 100`, svc),
		"db_connections_active": fmt.Sprintf(`max(db_connections_active{service="%s"})`, svc),
	}
}

func escapeProm(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

type promQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Value []any `json:"value"`
		} `json:"result"`
	} `json:"data"`
}
