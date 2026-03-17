package agent

import (
	"context"
	"fmt"
	"strings"
)

const (
	// SubagentMaxRounds is the maximum number of rounds a subagent can run.
	SubagentMaxRounds = 30
)

// SubagentConfig configures the subagent behavior.
type SubagentConfig struct {
	// System is the system prompt for the subagent.
	System string
	// MaxRounds is the maximum number of tool-use rounds.
	MaxRounds int
}

// RunSubagent spawns a child agent with a fresh context.
// The child works in its own context, sharing the executor, then returns only a summary.
func (a *Agent) RunSubagent(ctx context.Context, prompt string, childTools []Tool) (string, error) {
	return a.RunSubagentWithConfig(ctx, prompt, childTools, nil)
}

// RunSubagentWithConfig spawns a child agent with custom configuration.
func (a *Agent) RunSubagentWithConfig(ctx context.Context, prompt string, childTools []Tool, config *SubagentConfig) (string, error) {
	// Default configuration
	maxRounds := SubagentMaxRounds
	system := "You are a coding subagent. Complete the given task, then summarize your findings."

	if config != nil {
		if config.MaxRounds > 0 {
			maxRounds = config.MaxRounds
		}
		if config.System != "" {
			system = config.System
		}
	}

	// Create child agent with filtered tools (no task tool to prevent recursion)
	childAgent := &Agent{
		Client:    a.Client,
		Executor:  a.Executor,
		System:    system,
		Tools:     childTools,
		MaxTokens: a.MaxTokens,
	}

	// Fresh message context for the child
	messages := []Message{
		{Role: "user", Content: prompt},
	}

	// Run the child agent loop
	var lastResponse *Response

	for i := 0; i < maxRounds; i++ {
		response, err := childAgent.Client.CreateMessage(ctx, childAgent.System, messages, childAgent.Tools)
		if err != nil {
			return "", fmt.Errorf("subagent LLM call failed: %w", err)
		}

		lastResponse = response

		// Append assistant turn
		messages = append(messages, Message{
			Role:    "assistant",
			Content: response.Content,
		})

		// If the model didn't call a tool, we're done
		if response.StopReason != "tool_use" {
			break
		}

		// Execute each tool call, collect results
		var results []ToolResultContent
		for _, block := range response.Content {
			if block.Type == "tool_use" {
				output, execErr := childAgent.Executor.Execute(block.Name, block.Input)
				if execErr != nil {
					results = append(results, ToolResultContent{
						Type:      "tool_result",
						ToolUseID: block.ID,
						Content:   execErr.Error(),
						IsError:   true,
					})
					continue
				}

				// Truncate output if too long
				if len(output) > 50000 {
					output = output[:50000] + "... (truncated)"
				}

				results = append(results, ToolResultContent{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   output,
				})
			}
		}

		// Append tool results as user message
		messages = append(messages, Message{
			Role:    "user",
			Content: results,
		})
	}

	// Extract final text summary from the last response
	if lastResponse != nil {
		var textParts []string
		for _, block := range lastResponse.Content {
			if block.Type == "text" && block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		}
		if len(textParts) > 0 {
			return strings.Join(textParts, "\n"), nil
		}
	}

	return "(no summary)", nil
}