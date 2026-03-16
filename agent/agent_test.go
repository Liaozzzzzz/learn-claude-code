package agent

import (
	"context"
	"errors"
	"testing"
)

// MockLLMClient implements LLMClient for testing.
type MockLLMClient struct {
	Responses []*Response
	CallCount int
}

func (m *MockLLMClient) CreateMessage(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error) {
	if m.CallCount >= len(m.Responses) {
		return nil, errors.New("no more responses")
	}
	resp := m.Responses[m.CallCount]
	m.CallCount++
	return resp, nil
}

// MockToolExecutor implements ToolExecutor for testing.
type MockToolExecutor struct {
	Results map[string]string
}

func (m *MockToolExecutor) Execute(name string, input map[string]interface{}) (string, error) {
	if result, ok := m.Results[name]; ok {
		return result, nil
	}
	return "mock output", nil
}

func TestAgent_Run_NoToolUse(t *testing.T) {
	client := &MockLLMClient{
		Responses: []*Response{
			{
				Content:    []ContentBlock{{Type: "text", Text: "Hello!"}},
				StopReason: "end_turn",
			},
		},
	}
	executor := &MockToolExecutor{}
	agent := New(client, executor, "test system", DefaultTools())

	messages := []Message{{Role: "user", Content: "hi"}}
	err := agent.Run(context.Background(), &messages)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if client.CallCount != 1 {
		t.Errorf("expected 1 LLM call, got %d", client.CallCount)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}

func TestAgent_Run_WithToolUse(t *testing.T) {
	client := &MockLLMClient{
		Responses: []*Response{
			{
				Content: []ContentBlock{
					{Type: "tool_use", ID: "tool_1", Name: "bash", Input: map[string]interface{}{"command": "echo hello"}},
				},
				StopReason: "tool_use",
			},
			{
				Content:    []ContentBlock{{Type: "text", Text: "Done!"}},
				StopReason: "end_turn",
			},
		},
	}
	executor := &MockToolExecutor{Results: map[string]string{"bash": "hello"}}
	agent := New(client, executor, "test system", DefaultTools())

	messages := []Message{{Role: "user", Content: "run echo"}}
	err := agent.Run(context.Background(), &messages)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if client.CallCount != 2 {
		t.Errorf("expected 2 LLM calls, got %d", client.CallCount)
	}

	// Should have: user message, assistant (tool_use), user (tool_result), assistant (end_turn)
	if len(messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(messages))
	}
}

func TestDefaultTools(t *testing.T) {
	tools := DefaultTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	if tools[0].Name != "bash" {
		t.Errorf("expected tool name 'bash', got %s", tools[0].Name)
	}

	if len(tools[0].InputSchema.Required) != 1 {
		t.Errorf("expected 1 required field, got %d", len(tools[0].InputSchema.Required))
	}
}

func TestBashExecutor_DangerousCommand(t *testing.T) {
	executor := NewBashExecutor(".")

	tests := []string{
		"rm -rf /",
		"sudo rm something",
		"shutdown now",
		"reboot",
	}

	for _, cmd := range tests {
		input := map[string]interface{}{"command": cmd}
		output, err := executor.Execute("bash", input)
		if err != nil {
			t.Errorf("unexpected error for command %q: %v", cmd, err)
		}
		if output != "Error: Dangerous command blocked" {
			t.Errorf("expected dangerous command blocked for %q, got %q", cmd, output)
		}
	}
}

func TestBashExecutor_Execute(t *testing.T) {
	executor := NewBashExecutor(".")

	// Use a simple command that works cross-platform
	output, err := executor.Execute("bash", map[string]interface{}{"command": "pwd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Just check that we got some output
	if output == "(no output)" {
		t.Logf("Warning: command produced no output (may be platform-specific)")
	}
}

func TestBashExecutor_UnknownTool(t *testing.T) {
	executor := NewBashExecutor(".")

	_, err := executor.Execute("unknown", map[string]interface{}{})
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}