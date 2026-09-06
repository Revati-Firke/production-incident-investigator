package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/memory"
	"github.com/Revati-Firke/production-incident-investigator/internal/agent/tools"
	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

const (
	maxAgentRounds = 4
	agentName      = "investigation-agent"
)

// Input is the investigation agent entrypoint.
type Input struct {
	Incident      *domain.Incident
	Investigation *invdomain.Investigation
	Evidence      []domaintool.Execution
}

// Result is the agent run outcome.
type Result struct {
	RCA          *invdomain.RootCauseAnalysis
	AgentRun     *invdomain.AgentRun
	ExtraTools   []domaintool.Execution
	WaitingForAI bool
}

// InvestigationAgent plans follow-up tools and produces structured RCA.
type InvestigationAgent struct {
	llm   llm.Provider
	tools *apptool.Service
	defs  []llm.ToolDefinition
}

// NewInvestigationAgent creates an investigation agent.
func NewInvestigationAgent(provider llm.Provider, toolSvc *apptool.Service, registry *tools.Registry) *InvestigationAgent {
	var defs []llm.ToolDefinition
	if registry != nil {
		for _, t := range registry.List() {
			if t.Permission() == tools.PermissionRequiresApproval {
				continue // never let LLM auto-invoke approval tools
			}
			defs = append(defs, llm.ToolDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.InputSchema(),
			})
		}
	}
	return &InvestigationAgent{llm: provider, tools: toolSvc, defs: defs}
}

// Run executes the investigation agent loop.
func (a *InvestigationAgent) Run(ctx context.Context, input Input) (*Result, error) {
	if input.Incident == nil || input.Investigation == nil {
		return nil, fmt.Errorf("incident and investigation are required")
	}
	if a.llm == nil {
		return nil, fmt.Errorf("llm provider is required")
	}

	start := time.Now()
	run := &invdomain.AgentRun{
		ID:              uuid.New(),
		InvestigationID: input.Investigation.ID,
		IncidentID:      input.Incident.ID,
		Status:          invdomain.AgentRunRunning,
		Provider:        a.llm.Name(),
		CreatedAt:       time.Now().UTC(),
	}

	mem := memory.New(input.Incident.ID, input.Investigation.ID)
	mem.Title = input.Incident.Title
	mem.Service = input.Incident.Service
	mem.Environment = input.Incident.Environment
	mem.Severity = string(input.Incident.Severity)
	mem.Description = input.Incident.Description
	for _, e := range input.Evidence {
		mem.AddToolExecution(e)
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: buildUserPrompt(mem)},
	}

	var extra []domaintool.Execution
	var lastResp llm.Response

	toolDefs := a.defs
	if a.tools == nil {
		toolDefs = nil
	}

	for round := 0; round < maxAgentRounds; round++ {
		resp, err := a.llm.Generate(ctx, llm.Request{
			Messages:           messages,
			Tools:              toolDefs,
			Temperature:        0.1,
			MaxTokens:          2000,
			ResponseFormatJSON: true,
		})
		if err != nil {
			run.Status = invdomain.AgentRunWaitingForAI
			run.Error = err.Error()
			completed := time.Now().UTC()
			run.CompletedAt = &completed
			duration := int(time.Since(start).Milliseconds())
			run.DurationMS = &duration
			slog.Warn("llm unavailable, investigation waiting for AI", "error", err)
			return &Result{AgentRun: run, ExtraTools: extra, WaitingForAI: true}, nil
		}

		lastResp = resp
		run.Model = resp.Model
		run.InputTokens += resp.Usage.InputTokens
		run.OutputTokens += resp.Usage.OutputTokens

		if len(resp.ToolCalls) == 0 {
			break
		}

		assistantMsg := llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		for _, call := range resp.ToolCalls {
			if a.tools == nil {
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					Name:       call.Name,
					ToolCallID: call.ID,
					Content:    `{"error":"tool service unavailable"}`,
				})
				continue
			}
			exec, err := a.tools.Execute(ctx, domaintool.ExecuteInput{
				IncidentID:      input.Incident.ID,
				InvestigationID: &input.Investigation.ID,
				ToolName:        call.Name,
				Input:           call.Arguments,
				Agent:           agentName,
				Service:         input.Incident.Service,
				Environment:     input.Incident.Environment,
			})
			content := ""
			if err != nil {
				content = fmt.Sprintf(`{"error":%q}`, err.Error())
				if exec != nil {
					extra = append(extra, *exec)
					mem.AddToolExecution(*exec)
				}
			} else {
				extra = append(extra, *exec)
				mem.AddToolExecution(*exec)
				if len(exec.Output) > 0 {
					content = string(exec.Output)
				} else {
					content = `{"status":"completed"}`
				}
			}
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Name:       call.Name,
				ToolCallID: call.ID,
				Content:    content,
			})
		}
	}

	rca, err := parseRCA(lastResp.Content)
	if err != nil {
		run.Status = invdomain.AgentRunFailed
		run.Error = err.Error()
		completed := time.Now().UTC()
		run.CompletedAt = &completed
		duration := int(time.Since(start).Milliseconds())
		run.DurationMS = &duration
		return &Result{AgentRun: run, ExtraTools: extra}, fmt.Errorf("invalid rca from llm: %w", err)
	}

	resultJSON, _ := json.Marshal(rca)
	run.Status = invdomain.AgentRunCompleted
	run.Result = resultJSON
	completed := time.Now().UTC()
	run.CompletedAt = &completed
	duration := int(time.Since(start).Milliseconds())
	run.DurationMS = &duration

	slog.Info("investigation agent completed",
		"incident_id", input.Incident.ID,
		"confidence", rca.Confidence,
		"provider", run.Provider,
		"duration_ms", duration,
	)

	return &Result{RCA: rca, AgentRun: run, ExtraTools: extra}, nil
}

func parseRCA(content string) (*invdomain.RootCauseAnalysis, error) {
	if content == "" {
		return nil, fmt.Errorf("empty llm content")
	}
	// Try direct parse; also accept fenced JSON.
	rca, err := invdomain.UnmarshalRootCause([]byte(content))
	if err == nil {
		return rca, nil
	}
	// Extract first JSON object if present.
	start := -1
	end := -1
	for i, c := range content {
		if c == '{' && start < 0 {
			start = i
		}
		if c == '}' {
			end = i
		}
	}
	if start >= 0 && end > start {
		return invdomain.UnmarshalRootCause([]byte(content[start : end+1]))
	}
	return nil, err
}

func buildUserPrompt(mem *memory.Memory) string {
	return fmt.Sprintf(`Investigate this production incident and return ONLY valid JSON matching the RCA schema.

Incident:
- title: %s
- service: %s
- environment: %s
- severity: %s
- description: %s

Collected tool evidence (facts only — do not invent additional telemetry):
%s

Rules:
1. Base conclusions only on the evidence above and any subsequent tool results.
2. Cite evidence with source names from tools (logs, metrics, deployment, etc.).
3. Include rejected_hypotheses with reasons grounded in evidence.
4. confidence must be between 0 and 1.
5. Never claim an action was performed.
`, mem.Title, mem.Service, mem.Environment, mem.Severity, mem.Description, mem.EvidenceDigest())
}

const systemPrompt = `You are OpsPilot, an autonomous production incident investigation agent.
You analyze tool-collected evidence and produce a structured root cause analysis.
You may request additional READ_ONLY tools when needed.
You must never invent logs, metrics, deployments, or database results.
Final answer must be JSON with fields:
summary, root_cause, confidence, severity, evidence[], rejected_hypotheses[], recommended_actions[], reasoning_summary.
Do not expose chain-of-thought; reasoning_summary must be a concise user-facing explanation.`
