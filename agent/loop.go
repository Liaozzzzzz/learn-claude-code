package agent

import (
	"context"
	"fmt"
)

// Run executes the agent loop until the LLM stops requesting tool use.
// The messages slice is modified in place to append the conversation history.
func (a *Agent) Run(ctx context.Context, messages *[]Message) error {
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

		fmt.Printf("response: %v\n\n", response)

		// If the model didn't call a tool, we're done
		if response.StopReason != "tool_use" {
			return nil
		}

		// Execute each tool call, collect results
		var results []ToolResultContent
		for _, block := range response.Content {
			if block.Type == "tool_use" {
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
