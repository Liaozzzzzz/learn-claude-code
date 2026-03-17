package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Default timeout for bash commands.
const defaultBashTimeout = 120 * time.Second

// BashConfig holds configuration for the bash tool.
type BashConfig struct {
	WorkDir       string
	Timeout       time.Duration
	DangerousCmds []string
}

// BashHandler executes shell commands.
type BashHandler struct {
	config BashConfig
}

// NewBashHandler creates a new bash handler with default config.
func NewBashHandler(workDir string) *BashHandler {
	return &BashHandler{
		config: BashConfig{
			WorkDir: workDir,
			Timeout: defaultBashTimeout,
			DangerousCmds: []string{
				"rm -rf /",
				"sudo",
				"shutdown",
				"reboot",
				"> /dev/",
			},
		},
	}
}

// NewBashHandlerWithConfig creates a bash handler with custom config.
func NewBashHandlerWithConfig(config BashConfig) *BashHandler {
	if config.Timeout == 0 {
		config.Timeout = defaultBashTimeout
	}
	return &BashHandler{config: config}
}

// Execute runs a bash command.
func (h *BashHandler) Execute(input map[string]interface{}) (string, error) {
	command, ok := input["command"].(string)
	if !ok {
		return "", fmt.Errorf("command must be a string")
	}

	// Safety checks
	for _, d := range h.config.DangerousCmds {
		if strings.Contains(command, d) {
			return "Error: Dangerous command blocked", nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = h.config.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + stderr.String())

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "Error: Timeout", nil
		}
	}

	// Limit output size
	if len(output) > 50000 {
		output = output[:50000]
	}

	if output == "" {
		return "(no output)", nil
	}

	return output, nil
}

// BashDefinition returns the tool definition for bash.
func BashDefinition() Definition {
	return Definition{
		Name:        "bash",
		Description: "Run a shell command.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"command": {
					Type:        "string",
					Description: "The shell command to run",
				},
			},
			Required: []string{"command"},
		},
	}
}