package agent

import (
	"context"
	"fmt"
)

// NagConfig configures the nag reminder feature.
type NagConfig struct {
	// ToolName is the tool to watch for (e.g., "todo")
	ToolName string
	// Threshold is the number of rounds without using the tool before nagging
	Threshold int
	// Message is the reminder message to inject
	Message string
}

// Run executes the agent loop until the LLM stops requesting tool use.
// The messages slice is modified in place to append the conversation history.
func (a *Agent) Run(ctx context.Context, messages *[]Message) error {
	return a.RunWithNag(ctx, messages, nil)
}

// RunWithNag executes the agent loop with optional nag reminder.
func (a *Agent) RunWithNag(ctx context.Context, messages *[]Message, nag *NagConfig) error {
	roundsWithoutTool := 0

	for {
		// Call the LLM
		response, err := a.Client.CreateMessage(ctx, a.System, *messages, a.Tools)
		// fmt.Printf("response:  %v\n", response)
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
		usedWatchedTool := false

		for _, block := range response.Content {
			if block.Type == "tool_use" {
				// Check if the watched tool was used
				if nag != nil && block.Name == nag.ToolName {
					usedWatchedTool = true
				}

				output, err := a.Executor.Execute(block.Name, block.Input)
				if err != nil {
					results = append(results, ToolResultContent{
						Type:      "tool_result",
						ToolUseID: block.ID,
						Content:   err.Error(),
						IsError:   true,
					})
					continue
				}

				results = append(results, ToolResultContent{
					Type:      "tool_result",
					ToolUseID: block.ID,
					Content:   output,
				})
			}
		}

		// Update rounds without tool counter
		if nag != nil {
			if usedWatchedTool {
				roundsWithoutTool = 0
			} else {
				roundsWithoutTool++
			}

			// Inject nag reminder if threshold reached
			if roundsWithoutTool >= nag.Threshold {
				// Add reminder as a separate user message before tool results
				*messages = append(*messages, Message{
					Role:    "user",
					Content: nag.Message,
				})
				roundsWithoutTool = 0 // Reset after nagging
			}
		}

		// Append tool results as user message
		*messages = append(*messages, Message{
			Role:    "user",
			Content: results,
		})
	}
}

// RunWithInput is a convenience method that creates a user message and runs the agent.
func (a *Agent) RunWithInput(ctx context.Context, userInput string) ([]Message, error) {
	messages := []Message{
		{Role: "user", Content: userInput},
	}

	if err := a.Run(ctx, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

// RunWithInputAndNag is a convenience method that creates a user message and runs the agent with nag reminder.
func (a *Agent) RunWithInputAndNag(ctx context.Context, userInput string, nag *NagConfig) ([]Message, error) {
	messages := []Message{
		{Role: "user", Content: userInput},
	}

	if err := a.RunWithNag(ctx, &messages, nag); err != nil {
		return nil, err
	}

	return messages, nil
}
