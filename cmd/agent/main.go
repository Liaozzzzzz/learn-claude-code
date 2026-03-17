package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"learn-claude-code/agent"
	"learn-claude-code/agent/tools"
)

func main() {
	// Load .env file
	loadEnv(".env")

	// Create registry with todo tool
	registry, _ := tools.DefaultRegistryWithTodo()

	// Get child tools for subagent (excludes task to prevent recursion)
	childToolDefs := registry.GetChildToolDefinitions()

	// Create client from environment variables
	client := agent.NewOpenAIClientFromEnv()

	// Create agent first (will register task tool later)
	system := fmt.Sprintf("You are a coding agent at %s. Use the todo tool to plan multi-step tasks. Use the task tool to delegate exploration or subtasks. Prefer tools over prose.", mustGetwd())
	ag := agent.New(client, registry.AsExecutor(), system, nil)

	// Register task tool with a subagent run function
	taskHandler := tools.NewTaskHandler(func(ctx context.Context, prompt string) (string, error) {
		return ag.RunSubagent(ctx, prompt, agent.ToTools(childToolDefs))
	})
	registry.Register("task", tools.TaskDefinition(), taskHandler)

	// Now set the agent tools (all tools including task)
	agentTools := agent.ToTools(registry.Tools())
	ag.Tools = agentTools

	// Configure nag reminder for todo tool
	nagConfig := &agent.NagConfig{
		ToolName:  "todo",
		Threshold: 3,
		Message:   "<reminder>Update your todos.</reminder>",
	}

	// Interactive REPL
	history := []agent.Message{}
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Agent CLI (type 'q' or 'exit' to quit)")
	fmt.Println()

	for {
		fmt.Print("\033[36m>> \033[0m")
		if !scanner.Scan() {
			break
		}

		query := scanner.Text()
		if query == "" {
			continue
		}

		if query == "q" || query == "exit" {
			break
		}

		// Add user message to history
		history = append(history, agent.Message{
			Role:    "user",
			Content: query,
		})

		// Run agent with nag reminder
		if err := ag.RunWithNag(context.Background(), &history, nagConfig); err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
			continue
		}

		// Print assistant response
		if len(history) > 0 {
			lastMsg := history[len(history)-1]
			if content, ok := lastMsg.Content.([]agent.ContentBlock); ok {
				for _, block := range content {
					if block.Type == "text" {
						fmt.Println(block.Text)
					}
				}
			}
		}
		fmt.Println()
	}

	fmt.Println("Goodbye!")
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// loadEnv loads environment variables from a .env file
// It overrides existing environment variables
func loadEnv(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Override existing environment variable
			os.Setenv(key, value)
		}
	}
}
