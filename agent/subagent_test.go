package agent

import (
	"context"
	"errors"
	"testing"
)

// mockLLMClient is a mock LLM client for testing
type mockLLMClient struct {
	responses []*Response
	callCount int
}

func (m *mockLLMClient) CreateMessage(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error) {
	if m.callCount >= len(m.responses) {
		return nil, errors.New("no more responses")
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

// mockExecutor is a mock tool executor for testing
type mockExecutor struct {
	results map[string]string
}

func (m *mockExecutor) Execute(name string, input map[string]interface{}) (string, error) {
	if result, ok := m.results[name]; ok {
		return result, nil
	}
	return "", errors.New("unknown tool")
}

func TestRunSubagent(t *testing.T) {
	// Setup mock client with responses
	client := &mockLLMClient{
		responses: []*Response{
			{
				Content: []ContentBlock{
					{Type: "text", Text: "Subagent completed the task"},
				},
				StopReason: "end_turn",
			},
		},
	}

	executor := &mockExecutor{}

	agent := &Agent{
		Client:   client,
		Executor: executor,
		System:   "test system",
		Tools:    []Tool{},
	}

	result, err := agent.RunSubagent(context.Background(), "test prompt", []Tool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Subagent completed the task" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRunSubagent_WithToolUse(t *testing.T) {
	// Setup mock client with tool use then text response
	client := &mockLLMClient{
		responses: []*Response{
			{
				Content: []ContentBlock{
					{Type: "tool_use", ID: "1", Name: "bash", Input: map[string]interface{}{"command": "ls"}},
				},
				StopReason: "tool_use",
			},
			{
				Content: []ContentBlock{
					{Type: "text", Text: "Found files: a.txt, b.txt"},
				},
				StopReason: "end_turn",
			},
		},
	}

	executor := &mockExecutor{
		results: map[string]string{
			"bash": "a.txt\nb.txt",
		},
	}

	agent := &Agent{
		Client:   client,
		Executor: executor,
		System:   "test system",
		Tools: []Tool{
			{Name: "bash", Description: "Run bash", InputSchema: InputSchema{Type: "object"}},
		},
	}

	result, err := agent.RunSubagent(context.Background(), "list files", agent.Tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Found files: a.txt, b.txt" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRunSubagent_MaxRounds(t *testing.T) {
	// Setup mock client that always returns tool_use (infinite loop scenario)
	callCount := 0
	client := &mockLLMClientWithCounter{
		createFunc: func(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error) {
			callCount++
			return &Response{
				Content: []ContentBlock{
					{Type: "tool_use", ID: "1", Name: "bash", Input: map[string]interface{}{"command": "echo"}},
				},
				StopReason: "tool_use",
			}, nil
		},
	}

	executor := &mockExecutor{
		results: map[string]string{
			"bash": "output",
		},
	}

	agent := &Agent{
		Client:   client,
		Executor: executor,
		System:   "test system",
		Tools: []Tool{
			{Name: "bash", Description: "Run bash", InputSchema: InputSchema{Type: "object"}},
		},
	}

	// Should stop after max rounds (default 30)
	_, err := agent.RunSubagent(context.Background(), "test", agent.Tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have stopped at max rounds
	if callCount > SubagentMaxRounds {
		t.Errorf("expected max %d calls, got %d", SubagentMaxRounds, callCount)
	}
}

// mockLLMClientWithCounter is a flexible mock that uses a function
type mockLLMClientWithCounter struct {
	createFunc func(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error)
}

func (m *mockLLMClientWithCounter) CreateMessage(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error) {
	return m.createFunc(ctx, system, messages, tools)
}