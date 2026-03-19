package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"learn-claude-code/agent/tools"
)

// TeammateRunner runs a teammate's agent loop with autonomous behavior.
type TeammateRunner struct {
	Client   LLMClient
	WorkDir  string
	Bus      *tools.MessageBus
	Manager  *tools.TeammateManager
	Registry *tools.Registry
	Tracker  *tools.RequestTracker
	TaskMgr  *tools.TaskManager
	Config   *tools.AutonomousConfig
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
		Config:   tools.DefaultAutonomousConfig(),
	}
}

// NewAutonomousTeammateRunner creates a teammate runner with autonomous capabilities.
func NewAutonomousTeammateRunner(client LLMClient, workDir string, bus *tools.MessageBus, manager *tools.TeammateManager, registry *tools.Registry, tracker *tools.RequestTracker, taskMgr *tools.TaskManager) *TeammateRunner {
	return &TeammateRunner{
		Client:   client,
		WorkDir:  workDir,
		Bus:      bus,
		Manager:  manager,
		Registry: registry,
		Tracker:  tracker,
		TaskMgr:  taskMgr,
		Config:   tools.DefaultAutonomousConfig(),
	}
}

// Run executes the teammate's agent loop with autonomous behavior.
func (r *TeammateRunner) Run(name, role, prompt string) error {
	// Build teammate-specific tools
	teammateRegistry := r.buildTeammateTools(name)

	// Get team name for identity injection
	teamName := "default"
	if r.Manager != nil {
		teamName = r.Manager.TeamName()
	}

	// Create system prompt
	sysPrompt := fmt.Sprintf("You are '%s', role: %s, team: %s, at %s. Submit plans via plan_approval_submit before major work. Respond to shutdown_request with shutdown_response. Use idle tool when you have no more work. You will auto-claim new tasks.", name, role, teamName, r.WorkDir)
	ag := New(r.Client, teammateRegistry.AsExecutor(), sysPrompt, ToTools(teammateRegistry.Tools()))

	// Run agent with initial prompt
	ctx := context.Background()
	messages := []Message{
		{Role: "user", Content: prompt},
	}

	// Run autonomous loop
	if err := r.runAutonomous(ctx, ag, name, role, teamName, &messages); err != nil {
		fmt.Printf("[%s] error: %v\n", name, err)
	}

	// Set status to shutdown when done
	r.Manager.SetStatus(name, "shutdown")
	return nil
}

// runAutonomous implements the WORK/IDLE lifecycle.
func (r *TeammateRunner) runAutonomous(ctx context.Context, ag *Agent, name, role, teamName string, messages *[]Message) error {
	for {
		// -- WORK PHASE: standard agent loop --
		idleRequested, err := r.runWorkPhase(ctx, ag, name, messages)
		if err != nil {
			return err
		}

		// If no idle request and not tool_use, we're done
		if !idleRequested {
			return nil
		}

		// -- IDLE PHASE: poll for inbox messages and unclaimed tasks --
		resume, err := r.runIdlePhase(ctx, name, role, teamName, messages)
		if err != nil {
			return err
		}

		// If no resume, shutdown
		if !resume {
			return nil
		}

		// Resume work phase
		r.Manager.SetStatus(name, "working")
	}
}

// runWorkPhase runs the work phase until idle or done.
func (r *TeammateRunner) runWorkPhase(ctx context.Context, ag *Agent, name string, messages *[]Message) (bool, error) {
	for {
		// Check inbox before each LLM call
		inbox := r.Bus.ReadInbox(name)
		for _, msg := range inbox {
			// Check for shutdown request
			if msg.Type == "shutdown_request" {
				r.Manager.SetStatus(name, "shutdown")
				return false, nil
			}
			msgJSON, _ := json.Marshal(msg)
			*messages = append(*messages, Message{
				Role:    "user",
				Content: string(msgJSON),
			})
		}

		// Call LLM
		response, err := ag.Client.CreateMessage(ctx, ag.System, *messages, ag.Tools)
		if err != nil {
			return false, fmt.Errorf("LLM call failed: %w", err)
		}

		// Append assistant turn
		*messages = append(*messages, Message{
			Role:    "assistant",
			Content: response.Content,
		})

		// If the model didn't call a tool, we're done
		if response.StopReason != "tool_use" {
			return false, nil
		}

		// Execute each tool call, collect results
		var results []ToolResultContent
		idleRequested := false
		for _, block := range response.Content {
			if block.Type == "tool_use" {
				// Check for idle tool
				if block.Name == "idle" {
					idleRequested = true
					results = append(results, ToolResultContent{
						Type:      "tool_result",
						ToolUseID: block.ID,
						Content:   "Entering idle phase. Will poll for new tasks.",
					})
					continue
				}

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

		// If idle was requested, return
		if idleRequested {
			return true, nil
		}
	}
}

// runIdlePhase runs the idle phase, polling for work.
func (r *TeammateRunner) runIdlePhase(ctx context.Context, name, role, teamName string, messages *[]Message) (bool, error) {
	r.Manager.SetStatus(name, "idle")

	pollInterval := r.Config.PollInterval
	idleTimeout := r.Config.IdleTimeout
	polls := int(idleTimeout / pollInterval)

	for i := 0; i < polls; i++ {
		time.Sleep(pollInterval)

		// Check inbox
		inbox := r.Bus.ReadInbox(name)
		for _, msg := range inbox {
			// Check for shutdown request
			if msg.Type == "shutdown_request" {
				r.Manager.SetStatus(name, "shutdown")
				return false, nil
			}
			msgJSON, _ := json.Marshal(msg)
			*messages = append(*messages, Message{
				Role:    "user",
				Content: string(msgJSON),
			})
		}
		if len(inbox) > 0 {
			*messages = append(*messages, Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Received %d message(s). Resuming work.", len(inbox)),
			})
			return true, nil
		}

		// Scan for unclaimed tasks
		if r.TaskMgr != nil {
			result, err := tools.TryAutoClaim(r.TaskMgr, name)
			if err != nil {
				continue
			}
			if result.Claimed && result.Task != nil {
				// Inject identity if needed (after context compression)
				if len(*messages) <= 3 {
					identityBlock := tools.MakeIdentityBlock(name, role, teamName)
					*messages = append([]Message{{
						Role:    identityBlock["role"].(string),
						Content: identityBlock["content"],
					}}, *messages...)
					*messages = append(*messages, Message{
						Role:    "assistant",
						Content: fmt.Sprintf("I am %s. Continuing.", name),
					})
				}

				// Add claimed task to messages
				taskPrompt := fmt.Sprintf("<auto-claimed>Task #%d: %s\n%s</auto-claimed>", result.Task.ID, result.Task.Subject, result.Task.Description)
				*messages = append(*messages, Message{
					Role:    "user",
					Content: taskPrompt,
				})
				*messages = append(*messages, Message{
					Role:    "assistant",
					Content: fmt.Sprintf("Claimed task #%d. Working on it.", result.Task.ID),
				})
				fmt.Printf("  [%s] auto-claimed task #%d\n", name, result.Task.ID)
				return true, nil
			}
		}
	}

	// Timeout - shutdown
	return false, nil
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

	// Autonomous tools
	reg.Register("idle", tools.IdleDefinition(), tools.NewIdleHandler())
	if r.TaskMgr != nil {
		reg.Register("claim_task", tools.ClaimTaskDefinition(), tools.NewClaimTaskHandler(r.TaskMgr, name))
	}

	return reg
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
