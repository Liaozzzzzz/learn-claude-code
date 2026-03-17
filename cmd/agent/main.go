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

	registry := tools.DefaultRegistry()

	agentTools := agent.ToTools(registry.Tools())

	// Create client from environment variables
	client := agent.NewOpenAIClientFromEnv()

	// Create agent with system prompt
	system := fmt.Sprintf("You are a coding agent at %s. Use tools to solve tasks. Act, don't explain.", mustGetwd())
	ag := agent.New(client, registry.AsExecutor(), system, agentTools)

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

		// Run agent
		if err := ag.Run(context.Background(), &history); err != nil {
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
