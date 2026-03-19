// Package tools provides autonomous agent tools.
// Idle cycle with task board polling, auto-claiming unclaimed tasks.
//
// Teammate lifecycle:
//
//	+-------+
//	| spawn |
//	+---+---+
//	    |
//	    v
//	+-------+  tool_use    +-------+
//	| WORK  | <----------- |  LLM  |
//	+---+---+              +-------+
//	    |
//	    | stop_reason != tool_use
//	    v
//	+--------+
//	| IDLE   | poll every 5s for up to 60s
//	+---+----+
//	    |
//	    +---> check inbox -> message? -> resume WORK
//	    |
//	    +---> scan .tasks/ -> unclaimed? -> claim -> resume WORK
//	    |
//	    +---> timeout (60s) -> shutdown
//
// Key insight: "The agent finds work itself."
package tools

import (
	"fmt"
	"time"
)

// AutonomousConfig holds configuration for autonomous agent behavior.
type AutonomousConfig struct {
	PollInterval time.Duration // How often to poll for new tasks (default: 5s)
	IdleTimeout  time.Duration // How long to stay idle before shutdown (default: 60s)
}

// DefaultAutonomousConfig returns the default autonomous configuration.
func DefaultAutonomousConfig() *AutonomousConfig {
	return &AutonomousConfig{
		PollInterval: 5 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// -- Tool Definitions --

// IdleDefinition returns the tool definition for the idle tool.
func IdleDefinition() Definition {
	return Definition{
		Name:        "idle",
		Description: "Signal that you have no more work. Enters idle polling phase where you will auto-claim new tasks or receive messages.",
		InputSchema: InputSchema{Type: "object"},
	}
}

// ClaimTaskDefinition returns the tool definition for claim_task tool.
func ClaimTaskDefinition() Definition {
	return Definition{
		Name:        "claim_task",
		Description: "Claim a task from the task board by ID. Sets you as the owner and marks it in_progress.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"task_id": {
					Type:        "integer",
					Description: "The ID of the task to claim",
				},
			},
			Required: []string{"task_id"},
		},
	}
}

// -- Tool Handlers --

// IdleHandler handles idle tool calls.
type IdleHandler struct{}

// NewIdleHandler creates a new idle handler.
func NewIdleHandler() *IdleHandler {
	return &IdleHandler{}
}

// Execute implements Handler.
func (h *IdleHandler) Execute(input map[string]any) (string, error) {
	return "Entering idle phase. Will poll for new tasks.", nil
}

// ClaimTaskHandler handles claim_task tool calls.
type ClaimTaskHandler struct {
	manager *TaskManager
	owner   string
}

// NewClaimTaskHandler creates a new claim_task handler.
func NewClaimTaskHandler(manager *TaskManager, owner string) *ClaimTaskHandler {
	return &ClaimTaskHandler{manager: manager, owner: owner}
}

// Execute implements Handler.
func (h *ClaimTaskHandler) Execute(input map[string]any) (string, error) {
	taskID, ok := input["task_id"].(float64)
	if !ok {
		return "", fmt.Errorf("task_id is required and must be a number")
	}

	task, err := h.manager.Claim(int(taskID), h.owner)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Claimed task #%d: %s", task.ID, task.Subject), nil
}

// -- Identity Block for Context Compression --

// MakeIdentityBlock creates an identity block for re-injection after context compression.
func MakeIdentityBlock(name, role, teamName string) map[string]any {
	return map[string]any{
		"role":    "user",
		"content": fmt.Sprintf("<identity>You are '%s', role: %s, team: %s. Continue your work.</identity>", name, role, teamName),
	}
}

// -- Task Auto-Claim Helper --

// AutoClaimResult represents the result of an auto-claim operation.
type AutoClaimResult struct {
	Claimed bool
	Task    *Task
}

// TryAutoClaim attempts to claim an unclaimed task for the given owner.
func TryAutoClaim(manager *TaskManager, owner string) (*AutoClaimResult, error) {
	unclaimed, err := manager.ScanUnclaimed()
	if err != nil {
		return nil, err
	}

	if len(unclaimed) == 0 {
		return &AutoClaimResult{Claimed: false}, nil
	}

	// Claim the first unclaimed task
	task, err := manager.Claim(unclaimed[0].ID, owner)
	if err != nil {
		return nil, err
	}

	return &AutoClaimResult{Claimed: true, Task: task}, nil
}