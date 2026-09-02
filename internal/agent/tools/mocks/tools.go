package mocks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
)

func baseInputSchema(extra map[string]any) map[string]any {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"service":     map[string]any{"type": "string"},
			"environment": map[string]any{"type": "string"},
		},
	}
	for k, v := range extra {
		schema["properties"].(map[string]any)[k] = v
	}
	return schema
}

type baseInput struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
}

func resolveService(input baseInput, ctx tools.Context) string {
	if strings.TrimSpace(input.Service) != "" {
		return input.Service
	}
	return ctx.Service
}

func resolveEnvironment(input baseInput, ctx tools.Context) string {
	if strings.TrimSpace(input.Environment) != "" {
		return input.Environment
	}
	return ctx.Environment
}

// --- Observability ---

type SearchLogsTool struct{}

func (t *SearchLogsTool) Name() string                      { return "search_logs" }
func (t *SearchLogsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *SearchLogsTool) Description() string {
	return "Search application logs for errors and anomalies"
}
func (t *SearchLogsTool) InputSchema() any {
	return baseInputSchema(map[string]any{
		"query":      map[string]any{"type": "string"},
		"limit":      map[string]any{"type": "integer"},
		"time_range": map[string]any{"type": "string"},
	})
}

type searchLogsInput struct {
	baseInput
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
	TimeRange string `json:"time_range"`
}

func (t *SearchLogsTool) Execute(ctx tools.Context, input json.RawMessage) (tools.Result, error) {
	var in searchLogsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, fmt.Errorf("invalid input: %w", err)
	}
	svc := resolveService(in.baseInput, ctx)
	env := resolveEnvironment(in.baseInput, ctx)

	logs := mockLogs(svc, env)
	return tools.Result{
		Summary: fmt.Sprintf("Found %d log entries for %s", len(logs), svc),
		Data: map[string]any{
			"service":     svc,
			"environment": env,
			"entries":     logs,
		},
	}, nil
}

func mockLogs(service, environment string) []map[string]any {
	now := time.Now().UTC()
	switch {
	case strings.Contains(service, "payment"):
		return []map[string]any{
			{"timestamp": now.Add(-5 * time.Minute).Format(time.RFC3339), "level": "error", "message": "database connection timeout after 30s"},
			{"timestamp": now.Add(-4 * time.Minute).Format(time.RFC3339), "level": "error", "message": "failed to acquire connection from pool"},
			{"timestamp": now.Add(-3 * time.Minute).Format(time.RFC3339), "level": "warn", "message": "connection pool exhausted: 98/100 in use"},
		}
	case strings.Contains(service, "api"):
		return []map[string]any{
			{"timestamp": now.Add(-10 * time.Minute).Format(time.RFC3339), "level": "warn", "message": "P95 latency 3200ms exceeded threshold"},
			{"timestamp": now.Add(-8 * time.Minute).Format(time.RFC3339), "level": "error", "message": "upstream timeout calling inventory-service"},
		}
	default:
		return []map[string]any{
			{"timestamp": now.Add(-2 * time.Minute).Format(time.RFC3339), "level": "info", "message": fmt.Sprintf("%s running in %s", service, environment)},
		}
	}
}

type QueryMetricsTool struct{}

func (t *QueryMetricsTool) Name() string                      { return "query_metrics" }
func (t *QueryMetricsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *QueryMetricsTool) Description() string {
	return "Query infrastructure and application metrics"
}
func (t *QueryMetricsTool) InputSchema() any {
	return baseInputSchema(map[string]any{
		"metric": map[string]any{"type": "string"},
	})
}

type queryMetricsInput struct {
	baseInput
	Metric string `json:"metric"`
}

func (t *QueryMetricsTool) Execute(ctx tools.Context, input json.RawMessage) (tools.Result, error) {
	var in queryMetricsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, fmt.Errorf("invalid input: %w", err)
	}
	svc := resolveService(in.baseInput, ctx)

	metrics := mockMetrics(svc, in.Metric)
	return tools.Result{
		Summary: fmt.Sprintf("Retrieved %d metrics for %s", len(metrics), svc),
		Data:    map[string]any{"service": svc, "metrics": metrics},
	}, nil
}

func mockMetrics(service, metric string) []map[string]any {
	if strings.Contains(service, "payment") {
		return []map[string]any{
			{"name": "db_connections_active", "value": 98, "unit": "connections", "max": 100},
			{"name": "cpu_usage", "value": 42, "unit": "percent"},
			{"name": "error_rate", "value": 12.4, "unit": "percent"},
		}
	}
	return []map[string]any{
		{"name": coalesce(metric, "request_latency_p95"), "value": 3200, "unit": "ms"},
		{"name": "cpu_usage", "value": 38, "unit": "percent"},
		{"name": "error_rate", "value": 2.1, "unit": "percent"},
	}
}

type ServiceHealthTool struct{}

func (t *ServiceHealthTool) Name() string                      { return "get_service_health" }
func (t *ServiceHealthTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *ServiceHealthTool) Description() string {
	return "Get health status of a service and its dependencies"
}
func (t *ServiceHealthTool) InputSchema() any { return baseInputSchema(nil) }

func (t *ServiceHealthTool) Execute(ctx tools.Context, input json.RawMessage) (tools.Result, error) {
	var in baseInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, fmt.Errorf("invalid input: %w", err)
	}
	svc := resolveService(in, ctx)
	env := resolveEnvironment(in, ctx)

	status := "degraded"
	if strings.Contains(svc, "payment") {
		status = "unhealthy"
	}

	return tools.Result{
		Summary: fmt.Sprintf("Service %s is %s", svc, status),
		Data: map[string]any{
			"service":     svc,
			"environment": env,
			"status":      status,
			"dependencies": []map[string]string{
				{"name": "postgres", "status": ternary(strings.Contains(svc, "payment"), "unhealthy", "healthy")},
				{"name": "redis", "status": "healthy"},
			},
		},
	}, nil
}

// --- Database ---

type QueryDatabaseTool struct{}

func (t *QueryDatabaseTool) Name() string                      { return "query_database" }
func (t *QueryDatabaseTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *QueryDatabaseTool) Description() string               { return "Run a read-only database diagnostic query" }
func (t *QueryDatabaseTool) InputSchema() any {
	return baseInputSchema(map[string]any{"query": map[string]any{"type": "string"}})
}

func (t *QueryDatabaseTool) Execute(ctx tools.Context, input json.RawMessage) (tools.Result, error) {
	svc := ctx.Service
	active, max := 45, 100
	if strings.Contains(svc, "payment") {
		active, max = 98, 100
	}
	slowQueries := 0
	if strings.Contains(svc, "payment") {
		slowQueries = 12
	}
	return tools.Result{
		Summary: fmt.Sprintf("DB connections %d/%d", active, max),
		Data: map[string]any{
			"connections_active": active,
			"connections_max":    max,
			"slow_queries":       slowQueries,
		},
	}, nil
}

type DBConnectionsTool struct{}

func (t *DBConnectionsTool) Name() string                      { return "get_database_connections" }
func (t *DBConnectionsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *DBConnectionsTool) Description() string               { return "Get current database connection pool usage" }
func (t *DBConnectionsTool) InputSchema() any                  { return baseInputSchema(nil) }

func (t *DBConnectionsTool) Execute(ctx tools.Context, _ json.RawMessage) (tools.Result, error) {
	active, max := 45, 100
	if strings.Contains(ctx.Service, "payment") {
		active, max = 98, 100
	}
	return tools.Result{
		Summary: fmt.Sprintf("%d of %d connections in use", active, max),
		Data:    map[string]any{"active": active, "max": max, "utilization_pct": float64(active) / float64(max) * 100},
	}, nil
}

// --- Deployment ---

type RecentDeploymentsTool struct{}

func (t *RecentDeploymentsTool) Name() string                      { return "get_recent_deployments" }
func (t *RecentDeploymentsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *RecentDeploymentsTool) Description() string               { return "List recent deployments for a service" }
func (t *RecentDeploymentsTool) InputSchema() any {
	return baseInputSchema(map[string]any{"limit": map[string]any{"type": "integer"}})
}

func (t *RecentDeploymentsTool) Execute(ctx tools.Context, _ json.RawMessage) (tools.Result, error) {
	now := time.Now().UTC()
	return tools.Result{
		Summary: "1 recent deployment found",
		Data: map[string]any{
			"deployments": []map[string]any{
				{
					"id":          "dep-abc123",
					"service":     ctx.Service,
					"version":     "v2.4.1",
					"commit":      "a1b2c3d",
					"deployed_at": now.Add(-8 * time.Minute).Format(time.RFC3339),
					"deployer":    "ci-pipeline",
				},
			},
		},
	}, nil
}

// --- GitHub ---

type SearchCommitsTool struct{}

func (t *SearchCommitsTool) Name() string                      { return "search_github_commits" }
func (t *SearchCommitsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *SearchCommitsTool) Description() string               { return "Search recent GitHub commits for a service" }
func (t *SearchCommitsTool) InputSchema() any {
	return baseInputSchema(map[string]any{"since": map[string]any{"type": "string"}})
}

func (t *SearchCommitsTool) Execute(ctx tools.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{
		Summary: "Found 1 relevant commit",
		Data: map[string]any{
			"commits": []map[string]any{
				{"sha": "a1b2c3d", "message": "refactor: update repository connection handling", "author": "dev@example.com"},
			},
		},
	}, nil
}

type InspectCodeTool struct{}

func (t *InspectCodeTool) Name() string                      { return "inspect_code" }
func (t *InspectCodeTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *InspectCodeTool) Description() string               { return "Read and analyze relevant source code files" }
func (t *InspectCodeTool) InputSchema() any {
	return baseInputSchema(map[string]any{"path": map[string]any{"type": "string"}})
}

func (t *InspectCodeTool) Execute(_ tools.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{
		Summary: "Suspicious connection handling in repository layer",
		Data: map[string]any{
			"files": []map[string]any{
				{
					"path":    "internal/repository/payment.go",
					"finding": "Connection acquired but not released in error path (line 142)",
				},
			},
		},
	}, nil
}

type CreateIssueTool struct{}

func (t *CreateIssueTool) Name() string                      { return "create_github_issue" }
func (t *CreateIssueTool) Permission() tools.PermissionLevel { return tools.PermissionRequiresApproval }
func (t *CreateIssueTool) Description() string               { return "Create a GitHub issue for incident follow-up" }
func (t *CreateIssueTool) InputSchema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"},
	}}
}

func (t *CreateIssueTool) Execute(_ tools.Context, input json.RawMessage) (tools.Result, error) {
	var in struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: "GitHub issue created (mock)",
		Data:    map[string]any{"issue_number": 42, "url": "https://github.com/example/repo/issues/42", "title": in.Title},
	}, nil
}

type CreatePRTool struct{}

func (t *CreatePRTool) Name() string                      { return "create_pull_request" }
func (t *CreatePRTool) Permission() tools.PermissionLevel { return tools.PermissionRequiresApproval }
func (t *CreatePRTool) Description() string {
	return "Create a GitHub pull request with a proposed fix"
}
func (t *CreatePRTool) InputSchema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "branch": map[string]any{"type": "string"},
	}}
}

func (t *CreatePRTool) Execute(_ tools.Context, input json.RawMessage) (tools.Result, error) {
	var in struct {
		Title  string `json:"title"`
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: "Pull request created (mock)",
		Data:    map[string]any{"pr_number": 17, "url": "https://github.com/example/repo/pull/17", "title": in.Title},
	}, nil
}

// --- Knowledge ---

type SearchRunbooksTool struct{}

func (t *SearchRunbooksTool) Name() string                      { return "search_runbooks" }
func (t *SearchRunbooksTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *SearchRunbooksTool) Description() string {
	return "Search runbooks and troubleshooting documentation"
}
func (t *SearchRunbooksTool) InputSchema() any {
	return baseInputSchema(map[string]any{"query": map[string]any{"type": "string"}})
}

func (t *SearchRunbooksTool) Execute(_ tools.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{
		Summary: "Found 1 relevant runbook",
		Data: map[string]any{
			"documents": []map[string]any{
				{"title": "Database Connection Pool Exhaustion", "excerpt": "Check connection leak in repository layer. Restart pods as temporary mitigation."},
			},
		},
	}, nil
}

type SearchIncidentsTool struct{}

func (t *SearchIncidentsTool) Name() string                      { return "search_previous_incidents" }
func (t *SearchIncidentsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *SearchIncidentsTool) Description() string {
	return "Search historical incidents for similar patterns"
}
func (t *SearchIncidentsTool) InputSchema() any {
	return baseInputSchema(map[string]any{"query": map[string]any{"type": "string"}})
}

func (t *SearchIncidentsTool) Execute(_ tools.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{
		Summary: "Found 1 similar historical incident",
		Data: map[string]any{
			"incidents": []map[string]any{
				{"id": "INC-1001", "title": "Payment DB pool exhaustion", "resolution": "Fixed connection leak in repository", "confidence": 0.89},
			},
		},
	}, nil
}

// --- Communication ---

type SlackNotifyTool struct{}

func (t *SlackNotifyTool) Name() string                      { return "send_slack_notification" }
func (t *SlackNotifyTool) Permission() tools.PermissionLevel { return tools.PermissionAutonomous }
func (t *SlackNotifyTool) Description() string {
	return "Send a Slack notification about incident status"
}
func (t *SlackNotifyTool) InputSchema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"message": map[string]any{"type": "string"}, "channel": map[string]any{"type": "string"},
	}}
}

func (t *SlackNotifyTool) Execute(_ tools.Context, input json.RawMessage) (tools.Result, error) {
	var in struct {
		Message string `json:"message"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: "Slack notification sent (mock)",
		Data:    map[string]any{"channel": coalesce(in.Channel, "#incidents"), "message_id": "msg-mock-001"},
	}, nil
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
