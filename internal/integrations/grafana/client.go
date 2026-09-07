package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
)

// Client fetches service health via Grafana API or mock.
type Client interface {
	Provider() string
	GetServiceHealth(ctx context.Context, service, environment string) (*integrations.ServiceHealth, error)
}

// MockClient returns deterministic health (Phase 3 behavior).
type MockClient struct{}

func NewMock() *MockClient { return &MockClient{} }

func (c *MockClient) Provider() string { return integrations.ProviderMock }

func (c *MockClient) GetServiceHealth(_ context.Context, service, environment string) (*integrations.ServiceHealth, error) {
	status := "degraded"
	pg := "healthy"
	if strings.Contains(service, "payment") {
		status = "unhealthy"
		pg = "unhealthy"
	}
	return &integrations.ServiceHealth{
		Service:     service,
		Environment: environment,
		Status:      status,
		Source:      integrations.ProviderMock,
		Dependencies: []map[string]string{
			{"name": "postgres", "status": pg},
			{"name": "redis", "status": "healthy"},
		},
	}, nil
}

// HTTPClient calls Grafana /api/health (and optionally datasources).
type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// Config configures Grafana HTTP client.
type Config struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// NewHTTP creates a Grafana HTTP client.
func NewHTTP(cfg Config) (*HTTPClient, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("GRAFANA_URL is required when GRAFANA_PROVIDER=grafana")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPClient{baseURL: base, token: cfg.Token, client: &http.Client{Timeout: timeout}}, nil
}

func (c *HTTPClient) Provider() string { return integrations.ProviderGrafana }

func (c *HTTPClient) GetServiceHealth(ctx context.Context, service, environment string) (*integrations.ServiceHealth, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/health", nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("grafana request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	status := "healthy"
	deps := []map[string]string{{"name": "grafana", "status": "healthy"}}
	if resp.StatusCode >= 300 {
		status = "unhealthy"
		deps[0]["status"] = "unhealthy"
	} else {
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		if db, ok := parsed["database"].(string); ok && db != "ok" {
			status = "degraded"
			deps = append(deps, map[string]string{"name": "grafana_database", "status": db})
		}
	}

	return &integrations.ServiceHealth{
		Service:      service,
		Environment:  environment,
		Status:       status,
		Dependencies: deps,
		Source:       integrations.ProviderGrafana,
	}, nil
}

// AlertPayload is a subset of Grafana webhook alert JSON.
type AlertPayload struct {
	Title       string            `json:"title"`
	Status      string            `json:"status"`
	Message     string            `json:"message"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"commonLabels"`
	Alerts      []struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		Status      string            `json:"status"`
	} `json:"alerts"`
}

// IncidentFromAlert maps a Grafana webhook into OpsPilot incident fields.
func IncidentFromAlert(raw json.RawMessage) (title, description, severity, service, environment string, err error) {
	var p AlertPayload
	if err = json.Unmarshal(raw, &p); err != nil {
		return "", "", "", "", "", fmt.Errorf("invalid grafana webhook: %w", err)
	}

	labels := p.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	if len(p.Alerts) > 0 && p.Alerts[0].Labels != nil {
		for k, v := range p.Alerts[0].Labels {
			if labels[k] == "" {
				labels[k] = v
			}
		}
	}

	title = firstNonEmpty(p.Title, labels["alertname"], "Grafana alert")
	description = firstNonEmpty(p.Description, p.Message, annotationsSummary(p), "Alert received from Grafana")
	service = firstNonEmpty(labels["service"], labels["job"], labels["container"], "unknown-service")
	environment = firstNonEmpty(labels["environment"], labels["env"], labels["namespace"], "production")
	severity = mapSeverity(labels["severity"], p.Status)
	return title, description, severity, service, environment, nil
}

func annotationsSummary(p AlertPayload) string {
	if len(p.Alerts) == 0 {
		return ""
	}
	a := p.Alerts[0].Annotations
	if a == nil {
		return ""
	}
	return firstNonEmpty(a["description"], a["summary"], a["message"])
}

func mapSeverity(label, status string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "critical", "high", "medium", "low":
		return strings.ToLower(label)
	}
	if strings.EqualFold(status, "firing") {
		return "high"
	}
	return "medium"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
