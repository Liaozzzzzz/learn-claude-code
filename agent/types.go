// Package agent implements an AI agent loop pattern.
// The core pattern: while stop_reason == "tool_use", call LLM, execute tools, append results.
package agent

import "context"

// Message represents a message in the conversation.
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// TextContent represents text content in a message.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolUseContent represents a tool use block in a message.
type ToolUseContent struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToolResultContent represents a tool result block.
type ToolResultContent struct {
	Type      string      `json:"type"`
	ToolUseID string      `json:"tool_use_id"`
	Content   interface{} `json:"content"`
	IsError   bool        `json:"is_error,omitempty"`
}

// Tool represents a tool that the agent can use.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"input_schema"`
}

// InputSchema defines the schema for tool inputs.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property defines a property in the input schema.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Response represents the LLM response.
type Response struct {
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
}

// ContentBlock represents a block in the response content.
type ContentBlock struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id,omitempty"`
	Text  string                 `json:"text,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

// LLMClient defines the interface for LLM clients.
type LLMClient interface {
	CreateMessage(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error)
}

// ToolExecutor defines the interface for executing tools.
type ToolExecutor interface {
	Execute(name string, input map[string]interface{}) (string, error)
}

// BackgroundNotifier defines the interface for draining background task notifications.
// Returns notifications as a formatted string for injection into messages.
type BackgroundNotifier interface {
	DrainNotifications() string
}

// Agent represents an AI agent.
type Agent struct {
	Client            LLMClient
	Executor          ToolExecutor
	System            string
	Tools             []Tool
	MaxTokens         int
	BackgroundManager BackgroundNotifier
}

// New creates a new Agent with default configuration.
func New(client LLMClient, executor ToolExecutor, system string, tools []Tool) *Agent {
	return &Agent{
		Client:    client,
		Executor:  executor,
		System:    system,
		Tools:     tools,
		MaxTokens: 8000,
	}
}

// SetBackgroundManager sets the background manager for notification draining.
func (a *Agent) SetBackgroundManager(bm BackgroundNotifier) {
	a.BackgroundManager = bm
}
