package tools

import (
	"context"
	"testing"
)

func TestTaskDefinition(t *testing.T) {
	def := TaskDefinition()

	if def.Name != "task" {
		t.Errorf("expected name 'task', got %q", def.Name)
	}

	if def.InputSchema.Type != "object" {
		t.Errorf("expected input schema type 'object', got %q", def.InputSchema.Type)
	}

	// Check required fields
	if len(def.InputSchema.Required) != 1 || def.InputSchema.Required[0] != "prompt" {
		t.Errorf("expected required field 'prompt', got %v", def.InputSchema.Required)
	}
}

func TestTaskHandler_Execute(t *testing.T) {
	// Test with valid prompt
	handler := NewTaskHandler(func(ctx context.Context, prompt string) (string, error) {
		return "subagent result: " + prompt, nil
	})

	result, err := handler.Execute(map[string]interface{}{
		"prompt": "test task",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "subagent result: test task" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestTaskHandler_Execute_EmptyPrompt(t *testing.T) {
	handler := NewTaskHandler(func(ctx context.Context, prompt string) (string, error) {
		return "should not be called", nil
	})

	_, err := handler.Execute(map[string]interface{}{
		"prompt": "",
	})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}
}

func TestTaskHandler_Execute_MissingPrompt(t *testing.T) {
	handler := NewTaskHandler(func(ctx context.Context, prompt string) (string, error) {
		return "should not be called", nil
	})

	_, err := handler.Execute(map[string]interface{}{})
	if err != ErrEmptyPrompt {
		t.Errorf("expected ErrEmptyPrompt, got %v", err)
	}
}

func TestTaskHandler_Execute_Truncation(t *testing.T) {
	// Create a result longer than 50000 chars
	longResult := make([]byte, 60000)
	for i := range longResult {
		longResult[i] = 'a'
	}

	handler := NewTaskHandler(func(ctx context.Context, prompt string) (string, error) {
		return string(longResult), nil
	})

	result, err := handler.Execute(map[string]interface{}{
		"prompt": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) > 50020 { // 50000 + "... (truncated)"
		t.Errorf("result should be truncated, got length %d", len(result))
	}

	if !containsSubstring(result, "... (truncated)") {
		t.Error("result should contain truncation indicator")
	}
}

func TestTaskError(t *testing.T) {
	err := &TaskError{Message: "test error"}
	if err.Error() != "test error" {
		t.Errorf("expected 'test error', got %q", err.Error())
	}
}