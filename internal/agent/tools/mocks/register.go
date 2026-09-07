package mocks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	inttools "github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/integrations"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/github"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/grafana"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/loki"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/prometheus"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/slack"
)

// RegisterLocal registers tools that stay mock-only (DB diagnostics + deployments).
func RegisterLocal(r *tools.Registry) error {
	all := []tools.Tool{
		&queryDatabaseTool{},
		&dbConnectionsTool{},
		&recentDeploymentsTool{},
	}
	for _, t := range all {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// RegisterIntegrationMocks registers Phase 6 tools backed by mock adapters.
func RegisterIntegrationMocks(r *tools.Registry) error {
	return RegisterIntegrations(r,
		loki.NewMock(),
		prometheus.NewMock(),
		grafana.NewMock(),
		github.NewMock(),
		slack.NewMock(),
	)
}

// RegisterIntegrations registers observability / GitHub / Slack tools.
func RegisterIntegrations(
	r *tools.Registry,
	logs loki.Client,
	metrics prometheus.Client,
	health grafana.Client,
	gh github.Client,
	sl slack.Client,
) error {
	all := []tools.Tool{
		inttools.NewSearchLogsTool(logs),
		inttools.NewQueryMetricsTool(metrics),
		inttools.NewServiceHealthTool(health),
		inttools.NewSearchCommitsTool(gh),
		inttools.NewInspectCodeTool(gh),
		inttools.NewCreateIssueTool(gh),
		inttools.NewCreatePRTool(gh),
		inttools.NewSlackNotifyTool(sl),
	}
	for _, t := range all {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// RegisterCore registers local + integration mocks (no knowledge tools).
func RegisterCore(r *tools.Registry) error {
	if err := RegisterLocal(r); err != nil {
		return err
	}
	return RegisterIntegrationMocks(r)
}

// RegisterKnowledgeMocks registers hardcoded knowledge tools (unit tests / RAG disabled).
func RegisterKnowledgeMocks(r *tools.Registry) error {
	for _, t := range []tools.Tool{&searchRunbooksTool{}, &searchIncidentsTool{}} {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// RegisterAll registers all mock tools into the registry.
func RegisterAll(r *tools.Registry) error {
	if err := RegisterCore(r); err != nil {
		return err
	}
	return RegisterKnowledgeMocks(r)
}

// NewRegistry creates a registry with all mock tools registered.
func NewRegistry() (*tools.Registry, error) {
	r := tools.NewRegistry()
	if err := RegisterAll(r); err != nil {
		return nil, err
	}
	return r, nil
}

func unmarshalInput(input json.RawMessage, dest any) error {
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(input, dest); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	return nil
}

type queryDatabaseTool struct{ QueryDatabaseTool }
type dbConnectionsTool struct{ DBConnectionsTool }
type recentDeploymentsTool struct{ RecentDeploymentsTool }
type searchRunbooksTool struct{ SearchRunbooksTool }
type searchIncidentsTool struct{ SearchIncidentsTool }

func (t *queryDatabaseTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.QueryDatabaseTool.Execute(toolCtx, input)
}
func (t *dbConnectionsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.DBConnectionsTool.Execute(toolCtx, input)
}
func (t *recentDeploymentsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.RecentDeploymentsTool.Execute(toolCtx, input)
}
func (t *searchRunbooksTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.SearchRunbooksTool.Execute(toolCtx, input)
}
func (t *searchIncidentsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.SearchIncidentsTool.Execute(toolCtx, input)
}
