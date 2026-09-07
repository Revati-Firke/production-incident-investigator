package inttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/github"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/grafana"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/loki"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/prometheus"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/slack"
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

func resolveService(in baseInput, ctx tools.Context) string {
	if strings.TrimSpace(in.Service) != "" {
		return in.Service
	}
	return ctx.Service
}

func resolveEnvironment(in baseInput, ctx tools.Context) string {
	if strings.TrimSpace(in.Environment) != "" {
		return in.Environment
	}
	return ctx.Environment
}

// --- Logs ---

type SearchLogsTool struct{ Client loki.Client }

func NewSearchLogsTool(c loki.Client) *SearchLogsTool { return &SearchLogsTool{Client: c} }

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

func (t *SearchLogsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	var in struct {
		baseInput
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
		TimeRange string `json:"time_range"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return tools.Result{}, fmt.Errorf("invalid input: %w", err)
		}
	}
	window := time.Hour
	if in.TimeRange != "" {
		if d, err := time.ParseDuration(in.TimeRange); err == nil {
			window = d
		}
	}
	entries, err := t.Client.Search(ctx, integrations.LogSearchRequest{
		Service:     resolveService(in.baseInput, toolCtx),
		Environment: resolveEnvironment(in.baseInput, toolCtx),
		Query:       in.Query,
		Limit:       in.Limit,
		TimeRange:   window,
	})
	if err != nil {
		return tools.Result{}, err
	}
	rows := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, map[string]any{
			"timestamp": e.Timestamp.Format(time.RFC3339),
			"level":     e.Level,
			"message":   e.Message,
			"labels":    e.Labels,
		})
	}
	svc := resolveService(in.baseInput, toolCtx)
	return tools.Result{
		Summary: fmt.Sprintf("Found %d log entries for %s via %s", len(rows), svc, t.Client.Provider()),
		Data: map[string]any{
			"service":     svc,
			"environment": resolveEnvironment(in.baseInput, toolCtx),
			"provider":    t.Client.Provider(),
			"entries":     rows,
		},
	}, nil
}

// --- Metrics ---

type QueryMetricsTool struct{ Client prometheus.Client }

func NewQueryMetricsTool(c prometheus.Client) *QueryMetricsTool { return &QueryMetricsTool{Client: c} }

func (t *QueryMetricsTool) Name() string                      { return "query_metrics" }
func (t *QueryMetricsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *QueryMetricsTool) Description() string {
	return "Query infrastructure and application metrics"
}
func (t *QueryMetricsTool) InputSchema() any {
	return baseInputSchema(map[string]any{"metric": map[string]any{"type": "string"}})
}

func (t *QueryMetricsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	var in struct {
		baseInput
		Metric string `json:"metric"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return tools.Result{}, fmt.Errorf("invalid input: %w", err)
		}
	}
	svc := resolveService(in.baseInput, toolCtx)
	metrics, err := t.Client.Query(ctx, integrations.MetricsQueryRequest{
		Service:     svc,
		Environment: resolveEnvironment(in.baseInput, toolCtx),
		Metric:      in.Metric,
	})
	if err != nil {
		return tools.Result{}, err
	}
	rows := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		row := map[string]any{"name": m.Name, "value": m.Value, "unit": m.Unit}
		if m.Max > 0 {
			row["max"] = m.Max
		}
		rows = append(rows, row)
	}
	return tools.Result{
		Summary: fmt.Sprintf("Retrieved %d metrics for %s via %s", len(rows), svc, t.Client.Provider()),
		Data:    map[string]any{"service": svc, "provider": t.Client.Provider(), "metrics": rows},
	}, nil
}

// --- Health ---

type ServiceHealthTool struct{ Client grafana.Client }

func NewServiceHealthTool(c grafana.Client) *ServiceHealthTool { return &ServiceHealthTool{Client: c} }

func (t *ServiceHealthTool) Name() string                      { return "get_service_health" }
func (t *ServiceHealthTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *ServiceHealthTool) Description() string {
	return "Get health status of a service and its dependencies"
}
func (t *ServiceHealthTool) InputSchema() any { return baseInputSchema(nil) }

func (t *ServiceHealthTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	var in baseInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return tools.Result{}, fmt.Errorf("invalid input: %w", err)
		}
	}
	svc := resolveService(in, toolCtx)
	env := resolveEnvironment(in, toolCtx)
	h, err := t.Client.GetServiceHealth(ctx, svc, env)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: fmt.Sprintf("Service %s is %s via %s", h.Service, h.Status, t.Client.Provider()),
		Data: map[string]any{
			"service":      h.Service,
			"environment":  h.Environment,
			"status":       h.Status,
			"dependencies": h.Dependencies,
			"provider":     t.Client.Provider(),
			"source":       h.Source,
		},
	}, nil
}

// --- GitHub ---

type SearchCommitsTool struct{ Client github.Client }

func NewSearchCommitsTool(c github.Client) *SearchCommitsTool { return &SearchCommitsTool{Client: c} }

func (t *SearchCommitsTool) Name() string                      { return "search_github_commits" }
func (t *SearchCommitsTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *SearchCommitsTool) Description() string {
	return "Search recent GitHub commits for a service"
}
func (t *SearchCommitsTool) InputSchema() any {
	return baseInputSchema(map[string]any{"since": map[string]any{"type": "string"}})
}

func (t *SearchCommitsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	var in struct {
		baseInput
		Since string `json:"since"`
	}
	if len(input) > 0 {
		_ = json.Unmarshal(input, &in)
	}
	since := time.Now().UTC().Add(-24 * time.Hour)
	if in.Since != "" {
		if tParsed, err := time.Parse(time.RFC3339, in.Since); err == nil {
			since = tParsed
		}
	}
	commits, err := t.Client.SearchCommits(ctx, resolveService(in.baseInput, toolCtx), since)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: fmt.Sprintf("Found %d relevant commits via %s", len(commits), t.Client.Provider()),
		Data:    map[string]any{"provider": t.Client.Provider(), "commits": commits},
	}, nil
}

type InspectCodeTool struct{ Client github.Client }

func NewInspectCodeTool(c github.Client) *InspectCodeTool { return &InspectCodeTool{Client: c} }

func (t *InspectCodeTool) Name() string                      { return "inspect_code" }
func (t *InspectCodeTool) Permission() tools.PermissionLevel { return tools.PermissionReadOnly }
func (t *InspectCodeTool) Description() string {
	return "Read and analyze relevant source code files"
}
func (t *InspectCodeTool) InputSchema() any {
	return baseInputSchema(map[string]any{"path": map[string]any{"type": "string"}})
}

func (t *InspectCodeTool) Execute(ctx context.Context, input json.RawMessage, _ tools.Context) (tools.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if len(input) > 0 {
		_ = json.Unmarshal(input, &in)
	}
	findings, err := t.Client.InspectCode(ctx, in.Path)
	if err != nil {
		return tools.Result{}, err
	}
	summary := "Code inspection complete"
	if len(findings) > 0 {
		summary = findings[0].Finding
	}
	return tools.Result{
		Summary: summary,
		Data:    map[string]any{"provider": t.Client.Provider(), "files": findings},
	}, nil
}

type CreateIssueTool struct{ Client github.Client }

func NewCreateIssueTool(c github.Client) *CreateIssueTool { return &CreateIssueTool{Client: c} }

func (t *CreateIssueTool) Name() string                      { return "create_github_issue" }
func (t *CreateIssueTool) Permission() tools.PermissionLevel { return tools.PermissionRequiresApproval }
func (t *CreateIssueTool) Description() string {
	return "Create a GitHub issue for incident follow-up"
}
func (t *CreateIssueTool) InputSchema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"},
	}}
}

func (t *CreateIssueTool) Execute(ctx context.Context, input json.RawMessage, _ tools.Context) (tools.Result, error) {
	var in struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, err
	}
	issue, err := t.Client.CreateIssue(ctx, in.Title, in.Body)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: fmt.Sprintf("GitHub issue created via %s", t.Client.Provider()),
		Data:    issue,
	}, nil
}

type CreatePRTool struct{ Client github.Client }

func NewCreatePRTool(c github.Client) *CreatePRTool { return &CreatePRTool{Client: c} }

func (t *CreatePRTool) Name() string                      { return "create_pull_request" }
func (t *CreatePRTool) Permission() tools.PermissionLevel { return tools.PermissionRequiresApproval }
func (t *CreatePRTool) Description() string {
	return "Create a GitHub pull request with a proposed fix"
}
func (t *CreatePRTool) InputSchema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"title": map[string]any{"type": "string"}, "branch": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"},
	}}
}

func (t *CreatePRTool) Execute(ctx context.Context, input json.RawMessage, _ tools.Context) (tools.Result, error) {
	var in struct {
		Title  string `json:"title"`
		Branch string `json:"branch"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, err
	}
	pr, err := t.Client.CreatePullRequest(ctx, in.Title, in.Branch, in.Body)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: fmt.Sprintf("Pull request created via %s", t.Client.Provider()),
		Data:    pr,
	}, nil
}

// --- Slack ---

type SlackNotifyTool struct{ Client slack.Client }

func NewSlackNotifyTool(c slack.Client) *SlackNotifyTool { return &SlackNotifyTool{Client: c} }

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

func (t *SlackNotifyTool) Execute(ctx context.Context, input json.RawMessage, _ tools.Context) (tools.Result, error) {
	var in struct {
		Message string `json:"message"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.Result{}, err
	}
	msg, err := t.Client.PostMessage(ctx, in.Channel, in.Message)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{
		Summary: fmt.Sprintf("Slack notification sent via %s", t.Client.Provider()),
		Data:    msg,
	}, nil
}
