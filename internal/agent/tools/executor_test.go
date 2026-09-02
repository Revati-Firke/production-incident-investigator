package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools/mocks"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

type memStore struct {
	execs map[uuid.UUID]*domaintool.Execution
}

func newMemStore() *memStore {
	return &memStore{execs: make(map[uuid.UUID]*domaintool.Execution)}
}

func (m *memStore) Create(_ context.Context, exec *domaintool.Execution) error {
	m.execs[exec.ID] = exec
	return nil
}

func (m *memStore) Update(_ context.Context, exec *domaintool.Execution) error {
	m.execs[exec.ID] = exec
	return nil
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := tools.NewRegistry()
	tool := &mocks.SearchLogsTool{}
	if err := r.Register(&mockToolAdapter{SearchLogsTool: *tool}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(&mockToolAdapter{SearchLogsTool: *tool}); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestAuthorize_RequiresApproval(t *testing.T) {
	if err := tools.Authorize(tools.PermissionRequiresApproval, false); err == nil {
		t.Fatal("expected approval required error")
	}
	if err := tools.Authorize(tools.PermissionRequiresApproval, true); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestExecutor_ReadOnlyTool(t *testing.T) {
	registry, err := mocks.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	store := newMemStore()
	executor := tools.NewExecutor(registry, store, 0)

	incidentID := uuid.New()
	exec, err := executor.Execute(context.Background(), domaintool.ExecuteInput{
		IncidentID:  incidentID,
		ToolName:    "search_logs",
		Agent:       "test",
		Service:     "payment-service",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if exec.Status != domaintool.StatusCompleted {
		t.Errorf("status = %s, want completed", exec.Status)
	}
	if len(exec.Output) == 0 {
		t.Error("expected output")
	}
}

func TestExecutor_ApprovalRequiredBlocks(t *testing.T) {
	registry, err := mocks.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	store := newMemStore()
	executor := tools.NewExecutor(registry, store, 0)

	_, err = executor.Execute(context.Background(), domaintool.ExecuteInput{
		IncidentID: uuid.New(),
		ToolName:   "create_github_issue",
		Agent:      "test",
		Input:      json.RawMessage(`{"title":"test","body":"body"}`),
		Approved:   false,
	})
	if err == nil {
		t.Fatal("expected approval required error")
	}
}

func TestExecutor_ApprovalRequiredWithApproval(t *testing.T) {
	registry, err := mocks.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	store := newMemStore()
	executor := tools.NewExecutor(registry, store, 0)

	exec, err := executor.Execute(context.Background(), domaintool.ExecuteInput{
		IncidentID: uuid.New(),
		ToolName:   "create_github_issue",
		Agent:      "test",
		Input:      json.RawMessage(`{"title":"fix pool","body":"details"}`),
		Approved:   true,
		Service:    "payment-service",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if exec.Status != domaintool.StatusCompleted {
		t.Errorf("status = %s, want completed", exec.Status)
	}
}

func TestExecutor_UnknownTool(t *testing.T) {
	registry := tools.NewRegistry()
	executor := tools.NewExecutor(registry, newMemStore(), 0)

	_, err := executor.Execute(context.Background(), domaintool.ExecuteInput{
		IncidentID: uuid.New(),
		ToolName:   "nonexistent",
	})
	if err == nil {
		t.Fatal("expected tool not found error")
	}
}

type mockToolAdapter struct {
	mocks.SearchLogsTool
}

func (m *mockToolAdapter) Execute(ctx context.Context, input json.RawMessage, toolCtx tools.Context) (tools.Result, error) {
	return m.SearchLogsTool.Execute(toolCtx, input)
}
