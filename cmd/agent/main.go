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

	// Get working directory
	workDir, _ := os.Getwd()

	// Get skills directory (can be customized via environment variable)
	skillsDir := os.Getenv("SKILLS_DIR")
	if skillsDir == "" {
		// Default to skills directory relative to working directory
		skillsDir = "skills"
	}

	// Create registry with todo, skills, tasks, and background
	registry, _, skillLoader, bgManager := tools.DefaultRegistryWithTodoAndSkills(workDir, skillsDir)

	// Get child tools for subagent (excludes task to prevent recursion)
	childToolDefs := registry.GetChildToolDefinitions()

	// Create client from environment variables
	client := agent.NewOpenAIClientFromEnv()

	// Build system prompt with skill descriptions
	system := buildSystemPrompt(workDir, skillLoader)

	// Create agent first (will register task tool later)
	ag := agent.New(client, registry.AsExecutor(), system, nil)

	// Set background manager for notification draining
	ag.SetBackgroundManager(bgManager)

	// Register subagent task tool with a subagent run function
	subagentHandler := tools.NewTaskHandler(func(ctx context.Context, prompt string) (string, error) {
		return ag.RunSubagent(ctx, prompt, agent.ToTools(childToolDefs))
	})
	registry.Register("subagent", tools.TaskDefinition(), subagentHandler)

	// Now set the agent tools (all tools including task)
	agentTools := agent.ToTools(registry.Tools())
	ag.Tools = agentTools

	// Configure nag reminder for todo tool
	nagConfig := &agent.NagConfig{
		ToolName:  "todo",
		Threshold: 3,
		Message:   "<reminder>Update your todos.</reminder>",
	}

	// Configure context compaction
	compactConfig := &agent.CompactConfig{
		Threshold:     50000,
		KeepRecent:    3,
		TranscriptDir: ".transcripts",
		WorkDir:      workDir,
	}

	// Interactive REPL
	history := []agent.Message{}
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Agent CLI (type 'q' or 'exit' to quit)")
	if skillLoader.HasSkills() {
		fmt.Printf("Skills loaded: %s\n", strings.Join(skillLoader.SkillNames(), ", "))
	}
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

		// Run agent with nag reminder and context compaction
		if err := ag.RunWithNagAndCompact(context.Background(), &history, nagConfig, compactConfig); err != nil {
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

// buildSystemPrompt builds the system prompt with skill descriptions.
func buildSystemPrompt(workDir string, skillLoader *tools.SkillLoader) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are a coding agent at %s. ", workDir))
	sb.WriteString("Use the todo tool to plan multi-step tasks. ")
	sb.WriteString("Use task_create/task_update/task_list to track persistent tasks with dependencies. ")
	sb.WriteString("Use background_run for long-running commands (fire and forget). Use check_background to get results. ")
	sb.WriteString("Use the subagent tool to delegate exploration or subtasks. ")
	sb.WriteString("Prefer tools over prose. ")

	if skillLoader.HasSkills() {
		sb.WriteString("\n\nSkills available:\n")
		sb.WriteString(skillLoader.GetDescriptions())
		sb.WriteString("\n\nUse load_skill to access specialized knowledge before tackling unfamiliar topics.")
	}

	return sb.String()
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
