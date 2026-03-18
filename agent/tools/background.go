package tools

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// BackgroundTimeout is the default timeout for background commands.
const BackgroundTimeout = 300 * time.Second

// BackgroundTask represents a running or completed background task.
type BackgroundTask struct {
	ID      string
	Command string
	Status  string // "running", "completed", "timeout", "error"
	Result  string
}

// BackgroundManager manages background task execution.
type BackgroundManager struct {
	mu                 sync.Mutex
	tasks              map[string]*BackgroundTask
	notificationQueue  []Notification
	workDir            string
}

// Notification represents a completed task notification.
type Notification struct {
	TaskID  string
	Status  string
	Command string
	Result  string
}

// NewBackgroundManager creates a new background manager.
func NewBackgroundManager(workDir string) *BackgroundManager {
	return &BackgroundManager{
		tasks:     make(map[string]*BackgroundTask),
		workDir:   workDir,
	}
}

// Run starts a command in a background goroutine and returns the task ID immediately.
func (m *BackgroundManager) Run(command string) string {
	taskID := generateTaskID()
	task := &BackgroundTask{
		ID:      taskID,
		Command: command,
		Status:  "running",
	}

	m.mu.Lock()
	m.tasks[taskID] = task
	m.mu.Unlock()

	go m.execute(taskID, command)

	return fmt.Sprintf("Background task %s started: %s", taskID, truncate(command, 80))
}

// execute runs the command and stores the result.
func (m *BackgroundManager) execute(taskID, command string) {
	ctx, cancel := context.WithTimeout(context.Background(), BackgroundTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = m.workDir

	output, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))

	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			task.Status = "timeout"
			task.Result = "Error: Timeout (300s)"
		} else {
			task.Status = "error"
			task.Result = fmt.Sprintf("Error: %v", err)
		}
	} else {
		task.Status = "completed"
		if result == "" {
			task.Result = "(no output)"
		} else {
			task.Result = truncate(result, 50000)
		}
	}

	// Add to notification queue
	m.notificationQueue = append(m.notificationQueue, Notification{
		TaskID:  taskID,
		Status:  task.Status,
		Command: truncate(command, 80),
		Result:  truncate(task.Result, 500),
	})
}

// Check returns the status of a task, or lists all tasks if taskID is empty.
func (m *BackgroundManager) Check(taskID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if taskID != "" {
		task, ok := m.tasks[taskID]
		if !ok {
			return fmt.Sprintf("Error: Unknown task %s", taskID)
		}
		return fmt.Sprintf("[%s] %s\n%s", task.Status, truncate(task.Command, 60), task.Result)
	}

	if len(m.tasks) == 0 {
		return "No background tasks."
	}

	var lines []string
	for id, task := range m.tasks {
		lines = append(lines, fmt.Sprintf("%s: [%s] %s", id, task.Status, truncate(task.Command, 60)))
	}
	return strings.Join(lines, "\n")
}

// DrainNotifications returns and clears all pending completion notifications as a formatted string.
func (m *BackgroundManager) DrainNotifications() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.notificationQueue) == 0 {
		return ""
	}

	var lines []string
	for _, n := range m.notificationQueue {
		lines = append(lines, fmt.Sprintf("[bg:%s] %s: %s", n.TaskID, n.Status, n.Result))
	}
	m.notificationQueue = m.notificationQueue[:0]
	return strings.Join(lines, "\n")
}

// generateTaskID generates a short unique task ID using crypto/rand.
func generateTaskID() string {
	b := make([]byte, 4)
	crand.Read(b)
	return hex.EncodeToString(b)
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}


// -- Tool Definitions and Handlers --

// BackgroundRunDefinition returns the tool definition for background_run.
func BackgroundRunDefinition() Definition {
	return Definition{
		Name:        "background_run",
		Description: "Run a command in background. Returns task_id immediately. Use check_background to get results.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"command": {
					Type:        "string",
					Description: "The shell command to run in background",
				},
			},
			Required: []string{"command"},
		},
	}
}

// CheckBackgroundDefinition returns the tool definition for check_background.
func CheckBackgroundDefinition() Definition {
	return Definition{
		Name:        "check_background",
		Description: "Check background task status. Omit task_id to list all tasks.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"task_id": {
					Type:        "string",
					Description: "The task ID to check (optional)",
				},
			},
		},
	}
}

// BackgroundRunHandler handles background_run tool calls.
type BackgroundRunHandler struct {
	manager *BackgroundManager
}

// NewBackgroundRunHandler creates a new background_run handler.
func NewBackgroundRunHandler(manager *BackgroundManager) *BackgroundRunHandler {
	return &BackgroundRunHandler{manager: manager}
}

// Execute implements Handler.
func (h *BackgroundRunHandler) Execute(input map[string]interface{}) (string, error) {
	command, ok := input["command"].(string)
	if !ok {
		return "", fmt.Errorf("command must be a string")
	}
	return h.manager.Run(command), nil
}

// CheckBackgroundHandler handles check_background tool calls.
type CheckBackgroundHandler struct {
	manager *BackgroundManager
}

// NewCheckBackgroundHandler creates a new check_background handler.
func NewCheckBackgroundHandler(manager *BackgroundManager) *CheckBackgroundHandler {
	return &CheckBackgroundHandler{manager: manager}
}

// Execute implements Handler.
func (h *CheckBackgroundHandler) Execute(input map[string]interface{}) (string, error) {
	var taskID string
	if v, ok := input["task_id"].(string); ok {
		taskID = v
	}
	return h.manager.Check(taskID), nil
}