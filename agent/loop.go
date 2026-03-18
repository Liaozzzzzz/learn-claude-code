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
	return a.RunWithNagAndCompact(ctx, messages, nag, nil)
}

// RunWithNagAndCompact executes the agent loop with nag reminder and context compaction.
func (a *Agent) RunWithNagAndCompact(ctx context.Context, messages *[]Message, nag *NagConfig, compactConfig *CompactConfig) error {
	// Initialize compactor if config provided
	var compactor *Compactor
	if compactConfig != nil {
		compactor = NewCompactor(a.Client, *compactConfig)
	}

	roundsWithoutTool := 0

	for {
		// Drain background task notifications and inject as system message before LLM call
		if a.BackgroundManager != nil {
			notifText := a.BackgroundManager.DrainNotifications()
			if notifText != "" && len(*messages) > 0 {
				*messages = append(*messages, Message{
					Role:    "user",
					Content: fmt.Sprintf("<background-results>\n%s\n</background-results>", notifText),
				})
				*messages = append(*messages, Message{
					Role:    "assistant",
					Content: "Noted background results.",
				})
			}
		}

		// Layer 1: micro_compact before each LLM call
		if compactConfig != nil {
			*messages = MicroCompact(*messages, compactConfig.KeepRecent)
		}

		// Layer 2: auto_compact if token estimate exceeds threshold
		if compactor != nil && compactor.ShouldAutoCompact(*messages) {
			fmt.Println("[auto_compact triggered]")
			newMsgs, err := compactor.AutoCompact(ctx, *messages)
			if err != nil {
				fmt.Printf("[auto_compact error: %v]\n", err)
			} else {
				*messages = newMsgs
			}
		}

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
		manualCompact := false

		for _, block := range response.Content {
			if block.Type == "tool_use" {
				// Check if the watched tool was used
				if nag != nil && block.Name == nag.ToolName {
					usedWatchedTool = true
				}

				// Layer 3: Check for manual compact tool
				if block.Name == "compact" {
					manualCompact = true
					results = append(results, ToolResultContent{
						Type:      "tool_result",
						ToolUseID: block.ID,
						Content:   "Compressing...",
					})
					continue
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

		// Layer 3: manual compact triggered by the compact tool
		if manualCompact && compactor != nil {
			fmt.Println("[manual compact]")
			newMsgs, err := compactor.AutoCompact(ctx, *messages)
			if err != nil {
				fmt.Printf("[manual compact error: %v]\n", err)
			} else {
				*messages = newMsgs
			}
		}
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
