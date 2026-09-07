package mocks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
)

// RegisterCore registers mock tools except knowledge (RAG-backed in Phase 5).
func RegisterCore(r *tools.Registry) error {
	all := []tools.Tool{
		&searchLogsTool{},
		&queryMetricsTool{},
		&serviceHealthTool{},
		&queryDatabaseTool{},
		&dbConnectionsTool{},
		&recentDeploymentsTool{},
		&searchCommitsTool{},
		&inspectCodeTool{},
		&createIssueTool{},
		&createPRTool{},
		&slackNotifyTool{},
	}
	for _, t := range all {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
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

// Re-export tool implementations with correct interface signatures below.

type searchLogsTool struct{ SearchLogsTool }
type queryMetricsTool struct{ QueryMetricsTool }
type serviceHealthTool struct{ ServiceHealthTool }
type queryDatabaseTool struct{ QueryDatabaseTool }
type dbConnectionsTool struct{ DBConnectionsTool }
type recentDeploymentsTool struct{ RecentDeploymentsTool }
type searchCommitsTool struct{ SearchCommitsTool }
type inspectCodeTool struct{ InspectCodeTool }
type createIssueTool struct{ CreateIssueTool }
type createPRTool struct{ CreatePRTool }
type searchRunbooksTool struct{ SearchRunbooksTool }
type searchIncidentsTool struct{ SearchIncidentsTool }
type slackNotifyTool struct{ SlackNotifyTool }

func (t *searchLogsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.SearchLogsTool.Execute(toolCtx, input)
}
func (t *queryMetricsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.QueryMetricsTool.Execute(toolCtx, input)
}
func (t *serviceHealthTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.ServiceHealthTool.Execute(toolCtx, input)
}
func (t *queryDatabaseTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.QueryDatabaseTool.Execute(toolCtx, input)
}
func (t *dbConnectionsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.DBConnectionsTool.Execute(toolCtx, input)
}
func (t *recentDeploymentsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.RecentDeploymentsTool.Execute(toolCtx, input)
}
func (t *searchCommitsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.SearchCommitsTool.Execute(toolCtx, input)
}
func (t *inspectCodeTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.InspectCodeTool.Execute(toolCtx, input)
}
func (t *createIssueTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.CreateIssueTool.Execute(toolCtx, input)
}
func (t *createPRTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.CreatePRTool.Execute(toolCtx, input)
}
func (t *searchRunbooksTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.SearchRunbooksTool.Execute(toolCtx, input)
}
func (t *searchIncidentsTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.SearchIncidentsTool.Execute(toolCtx, input)
}
func (t *slackNotifyTool) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return t.SlackNotifyTool.Execute(toolCtx, input)
}
