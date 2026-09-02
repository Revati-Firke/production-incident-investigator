package tools_test

import (
	"testing"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/mocks"
)

func TestMockRegistry_AllToolsRegistered(t *testing.T) {
	registry, err := mocks.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	tools := registry.List()
	if len(tools) != 13 {
		t.Fatalf("expected 13 tools, got %d", len(tools))
	}

	expected := map[string]bool{
		"search_logs": true, "query_metrics": true, "get_service_health": true,
		"query_database": true, "get_database_connections": true,
		"get_recent_deployments": true, "search_github_commits": true, "inspect_code": true,
		"create_github_issue": true, "create_pull_request": true,
		"search_runbooks": true, "search_previous_incidents": true,
		"send_slack_notification": true,
	}
	for _, tool := range tools {
		delete(expected, tool.Name())
	}
	if len(expected) > 0 {
		t.Errorf("missing tools: %v", expected)
	}
}
