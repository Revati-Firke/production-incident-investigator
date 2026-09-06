package mock_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm/mock"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

func TestMockProvider_ConnectionExhaustion(t *testing.T) {
	p := mock.New("mock-rca-v1")
	resp, err := p.Generate(context.Background(), llm.Request{
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: "logs show database connection timeout and pool 98/100 after deployment",
		}},
		ResponseFormatJSON: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	rca, err := invdomain.UnmarshalRootCause([]byte(resp.Content))
	if err != nil {
		t.Fatalf("parse rca: %v", err)
	}
	if rca.Confidence < 0.8 {
		t.Errorf("confidence = %v", rca.Confidence)
	}
	if len(rca.RejectedHypotheses) == 0 {
		t.Error("expected rejected hypotheses")
	}
}

func TestMockProvider_MayRequestTools(t *testing.T) {
	p := mock.New("")
	resp, err := p.Generate(context.Background(), llm.Request{
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: "connection pool and deployment dep-abc123 without inspect_code yet",
		}},
		Tools: []llm.ToolDefinition{
			{Name: "inspect_code", Description: "read code", Parameters: map[string]any{}},
			{Name: "search_github_commits", Description: "commits", Parameters: map[string]any{}},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		// Not required every time; ensure response is still usable JSON if no tool calls.
		if resp.Content == "" {
			t.Fatal("expected tool calls or content")
		}
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(resp.Content), &raw); err != nil {
			t.Fatalf("content not json: %v", err)
		}
	}
}
