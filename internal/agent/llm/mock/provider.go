package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

// Provider is a deterministic LLM used for local demos and tests.
// It produces structured RCA from evidence patterns without calling an external API.
type Provider struct {
	model string
}

// New creates a mock LLM provider.
func New(model string) *Provider {
	if model == "" {
		model = "mock-rca-v1"
	}
	return &Provider{model: model}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "mock" }

// Ensure Provider implements llm.Provider at compile time.
var _ llm.Provider = (*Provider)(nil)

// Generate returns a deterministic RCA or optional tool calls based on the request.
func (p *Provider) Generate(ctx context.Context, request llm.Request) (llm.Response, error) {
	start := time.Now()
	select {
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	default:
	}

	joined := joinMessages(request.Messages)

	// If tools are offered and we haven't collected enough evidence yet, request more tools.
	if len(request.Tools) > 0 && shouldCallMoreTools(joined, request.Tools) {
		calls := selectToolCalls(joined, request.Tools)
		if len(calls) > 0 {
			return llm.Response{
				ToolCalls: calls,
				Model:     p.model,
				Provider:  p.Name(),
				Duration:  time.Since(start),
				Usage:     llm.Usage{InputTokens: len(joined) / 4, OutputTokens: 50},
			}, nil
		}
	}

	rca := buildRCA(joined)
	data, err := json.Marshal(rca)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal rca: %w", err)
	}

	return llm.Response{
		Content:  string(data),
		Model:    p.model,
		Provider: p.Name(),
		Duration: time.Since(start),
		Usage: llm.Usage{
			InputTokens:  len(joined) / 4,
			OutputTokens: len(data) / 4,
		},
	}, nil
}

func joinMessages(messages []llm.Message) string {
	var b strings.Builder
	for _, m := range messages {
		b.WriteString(string(m.Role))
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
		if m.Name != "" {
			b.WriteString("tool=")
			b.WriteString(m.Name)
			b.WriteString("\n")
		}
	}
	return strings.ToLower(b.String())
}

func shouldCallMoreTools(joined string, tools []llm.ToolDefinition) bool {
	// Call extra tools once if deployment/code signals appear but code tools not yet used.
	hasDeploy := strings.Contains(joined, "deployment") || strings.Contains(joined, "dep-")
	hasCodeToolResult := strings.Contains(joined, "inspect_code") || strings.Contains(joined, "search_github_commits")
	hasConn := strings.Contains(joined, "connection") || strings.Contains(joined, "pool")
	if hasDeploy && hasConn && !hasCodeToolResult {
		for _, t := range tools {
			if t.Name == "inspect_code" || t.Name == "search_github_commits" {
				return true
			}
		}
	}
	return false
}

func selectToolCalls(joined string, tools []llm.ToolDefinition) []llm.ToolCall {
	wanted := []string{"search_github_commits", "inspect_code", "get_database_connections"}
	available := map[string]bool{}
	for _, t := range tools {
		available[t.Name] = true
	}

	var calls []llm.ToolCall
	for _, name := range wanted {
		if !available[name] {
			continue
		}
		if strings.Contains(joined, name) {
			continue
		}
		calls = append(calls, llm.ToolCall{
			ID:        "call_" + uuid.New().String()[:8],
			Name:      name,
			Arguments: json.RawMessage(`{}`),
		})
		if len(calls) >= 2 {
			break
		}
	}
	return calls
}

func buildRCA(joined string) *invdomain.RootCauseAnalysis {
	switch {
	case strings.Contains(joined, "connection") && (strings.Contains(joined, "98") || strings.Contains(joined, "pool") || strings.Contains(joined, "timeout")):
		return &invdomain.RootCauseAnalysis{
			Summary:    "Payment API failures caused by database connection pool exhaustion",
			RootCause:  "Connections introduced by the latest repository implementation are not released correctly",
			Confidence: 0.91,
			Severity:   "critical",
			Evidence: []invdomain.EvidenceItem{
				{ID: "E-001", Source: "logs", Finding: "Repeated database connection timeout errors"},
				{ID: "E-002", Source: "metrics", Finding: "98/100 database connections consumed"},
				{ID: "E-003", Source: "deployment", Finding: "Issue started shortly after deployment v2.4.1"},
			},
			RejectedHypotheses: []invdomain.RejectedHypothesis{
				{Hypothesis: "CPU saturation", Reason: "CPU remained below 50% according to metric evidence"},
				{Hypothesis: "External dependency failure", Reason: "No upstream timeout pattern dominating evidence"},
			},
			RecommendedActions: []string{
				"Inspect connection lifecycle in repository layer",
				"Patch repository to release connections on error paths",
				"Run integration tests for connection pool under load",
				"Deploy fix to staging before production",
			},
			ReasoningSummary: "Logs and metrics show connection pool exhaustion correlating with a recent deployment that changed connection handling.",
		}
	case strings.Contains(joined, "memory") || strings.Contains(joined, "oom") || strings.Contains(joined, "heap"):
		return &invdomain.RootCauseAnalysis{
			Summary:    "Service instability caused by progressive memory growth",
			RootCause:  "Suspected memory leak after recent change leading to elevated RSS and GC pressure",
			Confidence: 0.84,
			Severity:   "high",
			Evidence: []invdomain.EvidenceItem{
				{ID: "E-001", Source: "metrics", Finding: "Memory utilization trending upward during incident window"},
				{ID: "E-002", Source: "logs", Finding: "GC and allocation pressure messages observed"},
			},
			RejectedHypotheses: []invdomain.RejectedHypothesis{
				{Hypothesis: "Database connection exhaustion", Reason: "Connection utilization remained within normal bounds"},
			},
			RecommendedActions: []string{
				"Capture heap profile",
				"Review recent allocations in changed packages",
				"Roll back suspect deployment if memory continues rising",
			},
			ReasoningSummary: "Memory metrics dominate the evidence set; other resource signals remain secondary.",
		}
	case strings.Contains(joined, "latency") || strings.Contains(joined, "3200"):
		return &invdomain.RootCauseAnalysis{
			Summary:    "Elevated API latency driven by slow upstream dependency calls",
			RootCause:  "Upstream timeout and elevated P95 latency on the critical request path",
			Confidence: 0.82,
			Severity:   "high",
			Evidence: []invdomain.EvidenceItem{
				{ID: "E-001", Source: "metrics", Finding: "P95 latency above 3 seconds"},
				{ID: "E-002", Source: "logs", Finding: "Upstream timeout messages observed"},
			},
			RejectedHypotheses: []invdomain.RejectedHypothesis{
				{Hypothesis: "CPU saturation", Reason: "CPU utilization remained moderate"},
			},
			RecommendedActions: []string{
				"Inspect upstream dependency health",
				"Review timeout and retry configuration",
				"Add circuit breaker if missing",
			},
			ReasoningSummary: "Latency metrics and upstream timeout logs align more strongly than compute saturation signals.",
		}
	default:
		return &invdomain.RootCauseAnalysis{
			Summary:    "Incident under investigation with incomplete conclusive evidence",
			RootCause:  "Probable service degradation; stronger evidence needed for a precise root cause",
			Confidence: 0.55,
			Severity:   "medium",
			Evidence: []invdomain.EvidenceItem{
				{ID: "E-001", Source: "health", Finding: "Service reported degraded or unhealthy status"},
			},
			RejectedHypotheses: []invdomain.RejectedHypothesis{},
			RecommendedActions: []string{
				"Collect additional logs and metrics",
				"Compare with recent deployments",
				"Escalate if impact continues",
			},
			ReasoningSummary: "Available evidence indicates degradation but does not yet uniquely identify a single root cause.",
		}
	}
}
