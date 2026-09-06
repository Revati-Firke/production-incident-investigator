package llm

import (
	"context"
	"encoding/json"
	"time"
)

// MessageRole identifies the speaker in a conversation.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message is a chat message exchanged with the LLM.
type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content,omitempty"`
	Name       string      `json:"name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
}

// ToolCall is a function call requested by the LLM.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDefinition describes a callable tool for the LLM.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// Request is a provider-agnostic LLM request.
type Request struct {
	Messages    []Message
	Tools       []ToolDefinition
	Temperature float64
	MaxTokens   int
	// ResponseFormatJSON asks the provider for structured JSON when supported.
	ResponseFormatJSON bool
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Response is a provider-agnostic LLM response.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
	Model     string
	Provider  string
	Duration  time.Duration
}

// Provider is the replaceable LLM abstraction.
type Provider interface {
	Name() string
	Generate(ctx context.Context, request Request) (Response, error)
}
