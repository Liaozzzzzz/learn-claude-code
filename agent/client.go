package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.siliconflow.cn/v1/chat/completions"
	defaultModel   = "Pro/zai-org/GLM-4.7"
	defaultTimeout = 120 * time.Second
)

// OpenAIClient implements LLMClient for OpenAI-compatible APIs (like SiliconFlow).
type OpenAIClient struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// OpenAIClientOption is a functional option for configuring the client.
type OpenAIClientOption func(*OpenAIClient)

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) OpenAIClientOption {
	return func(c *OpenAIClient) {
		c.BaseURL = url
	}
}

// WithModel sets the model to use.
func WithModel(model string) OpenAIClientOption {
	return func(c *OpenAIClient) {
		c.Model = model
	}
}

// WithTimeout sets the HTTP timeout.
func WithTimeout(timeout time.Duration) OpenAIClientOption {
	return func(c *OpenAIClient) {
		c.HTTPClient.Timeout = timeout
	}
}

// NewOpenAIClient creates a new OpenAI-compatible API client.
func NewOpenAIClient(apiKey string, opts ...OpenAIClientOption) *OpenAIClient {
	client := &OpenAIClient{
		APIKey:  apiKey,
		BaseURL: defaultBaseURL,
		Model:   defaultModel,
		HTTPClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

// NewOpenAIClientFromEnv creates a client from environment variables.
// Uses ANTHROPIC_API_KEY for the API key.
// Uses ANTHROPIC_BASE_URL for custom base URL (optional).
// Uses MODEL_ID for the model (optional).
func NewOpenAIClientFromEnv() *OpenAIClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	model := os.Getenv("MODEL_ID")
	if model == "" {
		model = defaultModel
	}

	opts := []OpenAIClientOption{WithModel(model)}
	if baseURL != "" {
		opts = append(opts, WithBaseURL(baseURL))
	}

	return NewOpenAIClient(apiKey, opts...)
}

// OpenAI request/response types
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string           `json:"type"`
	Function openAIFunctionDef `json:"function"`
}

type openAIFunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openAIRequest struct {
	Model    string         `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAITool   `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// CreateMessage sends a message to the OpenAI-compatible API.
func (c *OpenAIClient) CreateMessage(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error) {
	// Convert messages to OpenAI format
	openAIMessages := make([]openAIMessage, 0, len(messages)+1)

	// Add system message
	openAIMessages = append(openAIMessages, openAIMessage{
		Role:    "system",
		Content: system,
	})

	for _, msg := range messages {
		switch v := msg.Content.(type) {
		case string:
			openAIMessages = append(openAIMessages, openAIMessage{
				Role:    msg.Role,
				Content: v,
			})
		case []ToolResultContent:
			// In OpenAI format, each tool result is a separate message with role="tool"
			for _, tr := range v {
				content, _ := tr.Content.(string)
				openAIMessages = append(openAIMessages, openAIMessage{
					Role:       "tool",
					ToolCallID: tr.ToolUseID,
					Content:    content,
				})
			}
		case []ContentBlock:
			// This is assistant content with potential tool uses
			var toolCalls []openAIToolCall
			var textContent string
			for _, block := range v {
				if block.Type == "text" {
					textContent = block.Text
				} else if block.Type == "tool_use" {
					args, _ := json.Marshal(block.Input)
					toolCalls = append(toolCalls, openAIToolCall{
						ID:   block.ID,
						Type: "function",
						Function: openAIFunctionCall{
							Name:      block.Name,
							Arguments: string(args),
						},
					})
				}
			}
			openAIMessages = append(openAIMessages, openAIMessage{
				Role:      "assistant",
				Content:   textContent,
				ToolCalls: toolCalls,
			})
		default:
			openAIMessages = append(openAIMessages, openAIMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Convert tools to OpenAI format
	var openAITools []openAITool
	for _, t := range tools {
		params := map[string]interface{}{
			"type":       t.InputSchema.Type,
			"properties": t.InputSchema.Properties,
		}
		if len(t.InputSchema.Required) > 0 {
			params["required"] = t.InputSchema.Required
		}
		openAITools = append(openAITools, openAITool{
			Type: "function",
			Function: openAIFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}

	req := openAIRequest{
		Model:     c.Model,
		Messages:  openAIMessages,
		Tools:     openAITools,
		MaxTokens: 8000,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := apiResp.Choices[0]

	// Convert response to our format
	var content []ContentBlock

	// Add text content if present
	if str, ok := choice.Message.Content.(string); ok && str != "" {
		content = append(content, ContentBlock{
			Type: "text",
			Text: str,
		})
	}

	// Add tool calls if present
	for _, tc := range choice.Message.ToolCalls {
		var input map[string]interface{}
		json.Unmarshal([]byte(tc.Function.Arguments), &input)
		content = append(content, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	stopReason := choice.FinishReason
	if stopReason == "tool_calls" {
		stopReason = "tool_use"
	}

	return &Response{
		Content:    content,
		StopReason: stopReason,
	}, nil
}

// DefaultTools returns the default bash tool.
func DefaultTools() []Tool {
	return []Tool{
		{
			Name:        "bash",
			Description: "Run a shell command.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"command": {Type: "string", Description: "The shell command to run"},
				},
				Required: []string{"command"},
			},
		},
	}
}

// BashExecutor executes bash commands with safety checks.
type BashExecutor struct {
	WorkDir string
	Timeout time.Duration
}

// NewBashExecutor creates a new bash executor.
func NewBashExecutor(workDir string) *BashExecutor {
	return &BashExecutor{
		WorkDir: workDir,
		Timeout: defaultTimeout,
	}
}

// Execute runs a bash command.
func (e *BashExecutor) Execute(name string, input map[string]interface{}) (string, error) {
	if name != "bash" {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	command, ok := input["command"].(string)
	if !ok {
		return "", fmt.Errorf("command must be a string")
	}

	// Safety checks
	dangerous := []string{"rm -rf /", "sudo", "shutdown", "reboot", "> /dev/"}
	for _, d := range dangerous {
		if strings.Contains(command, d) {
			return "Error: Dangerous command blocked", nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.Timeout)
	defer cancel()

	// Use "sh -c" for compatibility across platforms
	cmd := execCommand(ctx, "sh", "-c", command)
	cmd.Dir = e.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + stderr.String())

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "Error: Timeout", nil
		}
	}

	// Limit output size
	if len(output) > 50000 {
		output = output[:50000]
	}

	if output == "" {
		return "(no output)", nil
	}

	return output, nil
}