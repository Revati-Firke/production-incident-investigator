package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Revati-Firke/production-incident-investigator/internal/agent/llm"
)

// Config configures the OpenAI-compatible chat completions client.
type Config struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

// Provider calls an OpenAI-compatible Chat Completions API.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New creates an OpenAI-compatible provider.
func New(cfg Config) (*Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY is required for openai provider")
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string { return "openai" }

// Ensure Provider implements llm.Provider at compile time.
var _ llm.Provider = (*Provider)(nil)

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Tools          []chatTool    `json:"tools,omitempty"`
	Temperature    float64       `json:"temperature,omitempty"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []chatCall `json:"tool_calls,omitempty"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type chatCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Generate sends a chat completion request.
func (p *Provider) Generate(ctx context.Context, request llm.Request) (llm.Response, error) {
	start := time.Now()

	reqBody := chatRequest{
		Model:       p.cfg.Model,
		Messages:    toChatMessages(request.Messages),
		Tools:       toChatTools(request.Tools),
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
	}
	if request.ResponseFormatJSON {
		reqBody.ResponseFormat = &respFormat{Type: "json_object"}
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return llm.Response{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return llm.Response{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai request: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 4<<20))
	if err != nil {
		return llm.Response{}, fmt.Errorf("read response: %w", err)
	}
	if httpResp.StatusCode >= 300 {
		return llm.Response{}, fmt.Errorf("openai status %d: %s", httpResp.StatusCode, truncate(string(body), 512))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return llm.Response{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("openai returned no choices")
	}

	msg := parsed.Choices[0].Message
	out := llm.Response{
		Content:  msg.Content,
		Model:    parsed.Model,
		Provider: p.Name(),
		Duration: time.Since(start),
		Usage: llm.Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
	}
	for _, tc := range msg.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(tc.Function.Arguments),
		})
	}
	return out, nil
}

func toChatMessages(messages []llm.Message) []chatMessage {
	out := make([]chatMessage, 0, len(messages))
	for _, m := range messages {
		cm := chatMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			args := string(tc.Arguments)
			if args == "" {
				args = "{}"
			}
			call := chatCall{ID: tc.ID, Type: "function"}
			call.Function.Name = tc.Name
			call.Function.Arguments = args
			cm.ToolCalls = append(cm.ToolCalls, call)
		}
		out = append(out, cm)
	}
	return out
}

func toChatTools(tools []llm.ToolDefinition) []chatTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]chatTool, 0, len(tools))
	for _, t := range tools {
		ct := chatTool{Type: "function"}
		ct.Function.Name = t.Name
		ct.Function.Description = t.Description
		ct.Function.Parameters = t.Parameters
		out = append(out, ct)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
