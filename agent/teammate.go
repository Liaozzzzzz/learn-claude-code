package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"learn-claude-code/agent/tools"
)

// TeammateRunner runs a teammate's agent loop.
type TeammateRunner struct {
	Client   LLMClient
	WorkDir  string
	Bus      *tools.MessageBus
	Manager  *tools.TeammateManager
	Registry *tools.Registry
	Tracker  *tools.RequestTracker
}

// NewTeammateRunner creates a new teammate runner.
func NewTeammateRunner(client LLMClient, workDir string, bus *tools.MessageBus, manager *tools.TeammateManager, registry *tools.Registry, tracker *tools.RequestTracker) *TeammateRunner {
	return &TeammateRunner{
		Client:   client,
		WorkDir:  workDir,
		Bus:      bus,
		Manager:  manager,
		Registry: registry,
		Tracker:  tracker,
	}
}

// Run executes the teammate's agent loop.
func (r *TeammateRunner) Run(name, role, prompt string) error {
	// Build teammate-specific tools
	teammateRegistry := r.buildTeammateTools(name)

	// Create agent with teammate tools
	sysPrompt := fmt.Sprintf("You are '%s', role: %s, at %s. Submit plans via plan_approval_submit before major work. Respond to shutdown_request with shutdown_response. Complete your task and mark yourself idle when done.", name, role, r.WorkDir)
	ag := New(r.Client, teammateRegistry.AsExecutor(), sysPrompt, ToTools(teammateRegistry.Tools()))

	// Run agent with initial prompt
	ctx := context.Background()
	messages := []Message{
		{Role: "user", Content: prompt},
	}

	// Run with inbox checking
	if err := r.runWithInbox(ctx, ag, name, &messages); err != nil {
		fmt.Printf("[%s] error: %v\n", name, err)
	}

	// Set status to idle when done
	r.Manager.SetStatus(name, "idle")
	return nil
}

// runWithInbox runs the agent loop with inbox checking before each LLM call.
func (r *TeammateRunner) runWithInbox(ctx context.Context, ag *Agent, name string, messages *[]Message) error {
	for {
		// Check inbox before each LLM call
		inbox := r.Bus.ReadInbox(name)
		if len(inbox) > 0 {
			inboxJSON, _ := json.MarshalIndent(inbox, "", "  ")
			*messages = append(*messages, Message{
				Role:    "user",
				Content: fmt.Sprintf("<inbox>%s</inbox>", string(inboxJSON)),
			})
			*messages = append(*messages, Message{
				Role:    "assistant",
				Content: "Noted inbox messages.",
			})
		}

		// Call LLM
		response, err := ag.Client.CreateMessage(ctx, ag.System, *messages, ag.Tools)
		if err != nil {
			return fmt.Errorf("LLM call failed: %w", err)
		}

		// Append assistant turn
		*messages = append(*messages, Message{
			Role:    "assistant",
			Content: response.Content,
		})

		// If the model didn't call a tool, we're done
		if response.StopReason != "tool_use" {
			return nil
		}

		// Execute each tool call, collect results
		var results []ToolResultContent
		for _, block := range response.Content {
			if block.Type == "tool_use" {
				output, err := ag.Executor.Execute(block.Name, block.Input)
				if err != nil {
					results = append(results, ToolResultContent{
						Type:      "tool_result",
						ToolUseID: block.ID,
						Content:   err.Error(),
						IsError:   true,
					})
					continue
				}
				fmt.Printf("  [%s] %s: %s\n", name, block.Name, truncate(output, 120))
				results = append(results, ToolResultContent{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   output,
				})
			}
		}

		// Append tool results as user message
		*messages = append(*messages, Message{
			Role:    "user",
			Content: results,
		})
	}
}

// buildTeammateTools builds tools for a teammate.
func (r *TeammateRunner) buildTeammateTools(name string) *tools.Registry {
	reg := tools.NewRegistry()

	// Base tools
	reg.Register("bash", tools.BashDefinition(), tools.NewBashHandler(r.WorkDir))
	reg.Register("read_file", tools.ReadDefinition(), tools.NewReadHandler(r.WorkDir))
	reg.Register("write_file", tools.WriteDefinition(), tools.NewWriteHandler(r.WorkDir))
	reg.Register("edit_file", tools.EditDefinition(), tools.NewEditHandler(r.WorkDir))

	// Communication tools
	reg.Register("send_message", tools.SendMessageDefinition(), tools.NewSendMessageHandler(r.Bus, name))
	reg.Register("read_inbox", tools.ReadInboxToolDefinition(), tools.NewReadInboxHandler(r.Bus, name))

	// Protocol tools for teammate
	if r.Tracker != nil {
		reg.Register("shutdown_response", tools.ShutdownResponseDefinition(), tools.NewShutdownResponseHandler(r.Tracker, r.Bus, name))
		reg.Register("plan_approval_submit", tools.PlanApprovalSubmitDefinition(), tools.NewPlanApprovalSubmitHandler(r.Tracker, r.Bus, name))
	}

	return reg
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}