package tools

import (
	"context"
)

// TaskDefinition returns the tool definition for the subagent tool.
func TaskDefinition() Definition {
	return Definition{
		Name:        "subagent",
		Description: "Spawn a subagent with fresh context. It shares the filesystem but not conversation history. Use for exploration or subtasks that benefit from isolation.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"prompt": {
					Type:        "string",
					Description: "The task prompt for the subagent.",
				},
				"description": {
					Type:        "string",
					Description: "Short description of the task (optional).",
				},
			},
			Required: []string{"prompt"},
		},
	}
}

// SubagentRunFunc is a function that runs a subagent.
type SubagentRunFunc func(ctx context.Context, prompt string) (string, error)

// TaskHandler handles the task tool execution.
type TaskHandler struct {
	runSubagent SubagentRunFunc
}

// NewTaskHandler creates a new task handler with a subagent run function.
func NewTaskHandler(runSubagent SubagentRunFunc) *TaskHandler {
	return &TaskHandler{
		runSubagent: runSubagent,
	}
}

// Execute processes the task tool input.
func (h *TaskHandler) Execute(input map[string]interface{}) (string, error) {
	prompt, ok := input["prompt"].(string)
	if !ok || prompt == "" {
		return "", ErrEmptyPrompt
	}

	// Run subagent with fresh context
	result, err := h.runSubagent(context.Background(), prompt)
	if err != nil {
		return "", err
	}

	// Truncate result if too long
	if len(result) > 50000 {
		result = result[:50000] + "... (truncated)"
	}

	return result, nil
}

// Errors
var ErrEmptyPrompt = &TaskError{Message: "prompt is required"}

// TaskError represents a task tool error.
type TaskError struct {
	Message string
}

func (e *TaskError) Error() string {
	return e.Message
}