package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// TaskStatus represents the status of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
)

// Task represents a single task with dependencies.
type Task struct {
	ID          int        `json:"id"`
	Subject     string     `json:"subject"`
	Description string     `json:"description,omitempty"`
	Status      TaskStatus `json:"status"`
	BlockedBy   []int      `json:"blockedBy,omitempty"`
	Blocks      []int      `json:"blocks,omitempty"`
	Owner       string     `json:"owner,omitempty"`
	ActiveForm  string     `json:"activeForm,omitempty"`
}

// TaskManager manages tasks persisted as JSON files.
// Tasks survive context compression because they are stored outside the conversation.
type TaskManager struct {
	mu       sync.RWMutex
	dir      string
	nextID   int
	initOnce sync.Once
}

// NewTaskManager creates a new TaskManager with the given tasks directory.
func NewTaskManager(tasksDir string) *TaskManager {
	tm := &TaskManager{
		dir:    tasksDir,
		nextID: 1,
	}
	tm.initOnce.Do(func() {
		os.MkdirAll(tasksDir, 0755)
		tm.nextID = tm.maxID() + 1
	})
	return tm
}

// maxID returns the maximum task ID from existing task files.
func (m *TaskManager) maxID() int {
	files, err := filepath.Glob(filepath.Join(m.dir, "task_*.json"))
	if err != nil || len(files) == 0 {
		return 0
	}

	maxID := 0
	for _, f := range files {
		base := filepath.Base(f)
		var id int
		_, err := fmt.Sscanf(base, "task_%d.json", &id)
		if err == nil && id > maxID {
			maxID = id
		}
	}
	return maxID
}

// load reads a task from its JSON file.
func (m *TaskManager) load(id int) (*Task, error) {
	path := filepath.Join(m.dir, fmt.Sprintf("task_%d.json", id))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("task %d not found", id)
	}

	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("failed to parse task %d: %w", id, err)
	}
	return &task, nil
}

// save writes a task to its JSON file.
func (m *TaskManager) save(task *Task) error {
	path := filepath.Join(m.dir, fmt.Sprintf("task_%d.json", task.ID))
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// delete removes a task's JSON file.
func (m *TaskManager) delete(id int) error {
	path := filepath.Join(m.dir, fmt.Sprintf("task_%d.json", id))
	return os.Remove(path)
}

// Create creates a new task and returns it as JSON.
func (m *TaskManager) Create(subject, description string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}

	task := &Task{
		ID:          m.nextID,
		Subject:     subject,
		Description: description,
		Status:      TaskPending,
		BlockedBy:   []int{},
		Blocks:      []int{},
	}

	if err := m.save(task); err != nil {
		return nil, err
	}

	m.nextID++
	return task, nil
}

// Get retrieves a task by ID.
func (m *TaskManager) Get(id int) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.load(id)
}

// UpdateOptions contains optional parameters for updating a task.
type UpdateOptions struct {
	Status      *TaskStatus
	AddBlockedBy []int
	AddBlocks   []int
	Owner       *string
	ActiveForm  *string
}

// Update modifies a task's status and dependencies.
func (m *TaskManager) Update(id int, opts UpdateOptions) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.load(id)
	if err != nil {
		return nil, err
	}

	// Update status
	if opts.Status != nil {
		if *opts.Status != TaskPending && *opts.Status != TaskInProgress && *opts.Status != TaskCompleted {
			return nil, fmt.Errorf("invalid status: %s", *opts.Status)
		}
		task.Status = *opts.Status

		// When a task is completed, remove it from all other tasks' blockedBy
		if *opts.Status == TaskCompleted {
			m.clearDependency(id)
		}
	}

	// Add blockedBy dependencies
	if opts.AddBlockedBy != nil {
		for _, depID := range opts.AddBlockedBy {
			if !containsInt(task.BlockedBy, depID) {
				task.BlockedBy = append(task.BlockedBy, depID)
			}
		}
	}

	// Add blocks dependencies (bidirectional)
	if opts.AddBlocks != nil {
		for _, blockedID := range opts.AddBlocks {
			if !containsInt(task.Blocks, blockedID) {
				task.Blocks = append(task.Blocks, blockedID)
			}
			// Update the blocked task's blockedBy list
			blocked, err := m.load(blockedID)
			if err == nil && !containsInt(blocked.BlockedBy, id) {
				blocked.BlockedBy = append(blocked.BlockedBy, id)
				m.save(blocked)
			}
		}
	}

	// Update owner
	if opts.Owner != nil {
		task.Owner = *opts.Owner
	}

	// Update active form
	if opts.ActiveForm != nil {
		task.ActiveForm = *opts.ActiveForm
	}

	if err := m.save(task); err != nil {
		return nil, err
	}

	return task, nil
}

// Delete removes a task by ID.
func (m *TaskManager) Delete(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove this task from other tasks' dependencies
	files, _ := filepath.Glob(filepath.Join(m.dir, "task_*.json"))
	for _, f := range files {
		task, err := m.loadFromFile(f)
		if err != nil {
			continue
		}
		modified := false

		// Remove from blockedBy
		if containsInt(task.BlockedBy, id) {
			task.BlockedBy = removeFromSlice(task.BlockedBy, id)
			modified = true
		}

		// Remove from blocks
		if containsInt(task.Blocks, id) {
			task.Blocks = removeFromSlice(task.Blocks, id)
			modified = true
		}

		if modified {
			m.save(task)
		}
	}

	return m.delete(id)
}

// loadFromFile reads a task from a file path (without lock).
func (m *TaskManager) loadFromFile(path string) (*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// clearDependency removes the completed task ID from all other tasks' blockedBy lists.
func (m *TaskManager) clearDependency(completedID int) {
	files, _ := filepath.Glob(filepath.Join(m.dir, "task_*.json"))
	for _, f := range files {
		task, err := m.loadFromFile(f)
		if err != nil {
			continue
		}
		if containsInt(task.BlockedBy, completedID) {
			task.BlockedBy = removeFromSlice(task.BlockedBy, completedID)
			m.save(task)
		}
	}
}

// List returns all tasks.
func (m *TaskManager) List() ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := filepath.Glob(filepath.Join(m.dir, "task_*.json"))
	if err != nil {
		return nil, err
	}

	var tasks []*Task
	for _, f := range files {
		task, err := m.loadFromFile(f)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	// Sort by ID
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	return tasks, nil
}

// Render returns a formatted string representation of all tasks.
func (m *TaskManager) Render() string {
	tasks, err := m.List()
	if err != nil || len(tasks) == 0 {
		return "No tasks."
	}

	var lines []string
	for _, t := range tasks {
		var marker string
		switch t.Status {
		case TaskPending:
			marker = "[ ]"
		case TaskInProgress:
			marker = "[>]"
		case TaskCompleted:
			marker = "[x]"
		}

		blocked := ""
		if len(t.BlockedBy) > 0 {
			blocked = fmt.Sprintf(" (blocked by: %v)", t.BlockedBy)
		}
		lines = append(lines, fmt.Sprintf("%s #%d: %s%s", marker, t.ID, t.Subject, blocked))
	}

	return strings.Join(lines, "\n")
}

// Helper functions
func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func removeFromSlice(slice []int, val int) []int {
	var result []int
	for _, v := range slice {
		if v != val {
			result = append(result, v)
		}
	}
	return result
}

// --- Tool Definitions and Handlers ---

// TaskCreateDefinition returns the tool definition for task_create.
func TaskCreateDefinition() Definition {
	return Definition{
		Name:        "task_create",
		Description: "Create a structured task for tracking progress. Use for complex multi-step tasks.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"subject": {
					Type:        "string",
					Description: "Brief, actionable task title (imperative form, e.g., 'Fix authentication bug')",
				},
				"description": {
					Type:        "string",
					Description: "Detailed description of what needs to be done",
				},
				"activeForm": {
					Type:        "string",
					Description: "Present continuous form shown in spinner when in_progress (e.g., 'Fixing authentication bug')",
				},
			},
			Required: []string{"subject"},
		},
	}
}

// TaskUpdateDefinition returns the tool definition for task_update.
func TaskUpdateDefinition() Definition {
	return Definition{
		Name:        "task_update",
		Description: "Update a task's status or dependencies. Mark tasks in_progress before starting, completed when done.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"task_id": {
					Type:        "integer",
					Description: "The ID of the task to update",
				},
				"status": {
					Type:        "string",
					Description: "New status: 'pending', 'in_progress', or 'completed'",
				},
				"addBlockedBy": {
					Type:        "array",
					Description: "Task IDs that must complete before this task can start",
				},
				"addBlocks": {
					Type:        "array",
					Description: "Task IDs that this task blocks",
				},
				"owner": {
					Type:        "string",
					Description: "Agent or person assigned to this task",
				},
			},
			Required: []string{"task_id"},
		},
	}
}

// TaskListDefinition returns the tool definition for task_list.
func TaskListDefinition() Definition {
	return Definition{
		Name:        "task_list",
		Description: "List all tasks with status summary. Use to see available work and check progress.",
		InputSchema: InputSchema{
			Type:       "object",
			Properties: map[string]Property{},
		},
	}
}

// TaskGetDefinition returns the tool definition for task_get.
func TaskGetDefinition() Definition {
	return Definition{
		Name:        "task_get",
		Description: "Get full details of a task by ID. Use to understand requirements before starting work.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"task_id": {
					Type:        "integer",
					Description: "The ID of the task to retrieve",
				},
			},
			Required: []string{"task_id"},
		},
	}
}

// TaskDeleteDefinition returns the tool definition for task_delete.
func TaskDeleteDefinition() Definition {
	return Definition{
		Name:        "task_delete",
		Description: "Delete a task that is no longer needed or was created in error.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"task_id": {
					Type:        "integer",
					Description: "The ID of the task to delete",
				},
			},
			Required: []string{"task_id"},
		},
	}
}

// TaskCreateHandler handles task_create tool execution.
type TaskCreateHandler struct {
	manager *TaskManager
}

// NewTaskCreateHandler creates a new task_create handler.
func NewTaskCreateHandler(manager *TaskManager) *TaskCreateHandler {
	return &TaskCreateHandler{manager: manager}
}

// Execute processes the task_create tool input.
func (h *TaskCreateHandler) Execute(input map[string]interface{}) (string, error) {
	subject, _ := input["subject"].(string)
	description, _ := input["description"].(string)
	activeForm, _ := input["activeForm"].(string)

	if subject == "" {
		return "", fmt.Errorf("subject is required")
	}

	task, err := h.manager.Create(subject, description)
	if err != nil {
		return "", err
	}

	if activeForm != "" {
		task, _ = h.manager.Update(task.ID, UpdateOptions{ActiveForm: &activeForm})
	}

	data, _ := json.MarshalIndent(task, "", "  ")
	return string(data), nil
}

// TaskUpdateHandler handles task_update tool execution.
type TaskUpdateHandler struct {
	manager *TaskManager
}

// NewTaskUpdateHandler creates a new task_update handler.
func NewTaskUpdateHandler(manager *TaskManager) *TaskUpdateHandler {
	return &TaskUpdateHandler{manager: manager}
}

// Execute processes the task_update tool input.
func (h *TaskUpdateHandler) Execute(input map[string]interface{}) (string, error) {
	taskID, ok := input["task_id"].(float64)
	if !ok {
		return "", fmt.Errorf("task_id is required and must be a number")
	}
	id := int(taskID)

	opts := UpdateOptions{}

	if status, ok := input["status"].(string); ok && status != "" {
		s := TaskStatus(status)
		opts.Status = &s
	}

	if blockedBy, ok := input["addBlockedBy"].([]interface{}); ok {
		for _, v := range blockedBy {
			if f, ok := v.(float64); ok {
				opts.AddBlockedBy = append(opts.AddBlockedBy, int(f))
			}
		}
	}

	if blocks, ok := input["addBlocks"].([]interface{}); ok {
		for _, v := range blocks {
			if f, ok := v.(float64); ok {
				opts.AddBlocks = append(opts.AddBlocks, int(f))
			}
		}
	}

	if owner, ok := input["owner"].(string); ok {
		opts.Owner = &owner
	}

	task, err := h.manager.Update(id, opts)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(task, "", "  ")
	return string(data), nil
}

// TaskListHandler handles task_list tool execution.
type TaskListHandler struct {
	manager *TaskManager
}

// NewTaskListHandler creates a new task_list handler.
func NewTaskListHandler(manager *TaskManager) *TaskListHandler {
	return &TaskListHandler{manager: manager}
}

// Execute processes the task_list tool input.
func (h *TaskListHandler) Execute(input map[string]interface{}) (string, error) {
	return h.manager.Render(), nil
}

// TaskGetHandler handles task_get tool execution.
type TaskGetHandler struct {
	manager *TaskManager
}

// NewTaskGetHandler creates a new task_get handler.
func NewTaskGetHandler(manager *TaskManager) *TaskGetHandler {
	return &TaskGetHandler{manager: manager}
}

// Execute processes the task_get tool input.
func (h *TaskGetHandler) Execute(input map[string]interface{}) (string, error) {
	taskID, ok := input["task_id"].(float64)
	if !ok {
		return "", fmt.Errorf("task_id is required and must be a number")
	}

	task, err := h.manager.Get(int(taskID))
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(task, "", "  ")
	return string(data), nil
}

// TaskDeleteHandler handles task_delete tool execution.
type TaskDeleteHandler struct {
	manager *TaskManager
}

// NewTaskDeleteHandler creates a new task_delete handler.
func NewTaskDeleteHandler(manager *TaskManager) *TaskDeleteHandler {
	return &TaskDeleteHandler{manager: manager}
}

// Execute processes the task_delete tool input.
func (h *TaskDeleteHandler) Execute(input map[string]interface{}) (string, error) {
	taskID, ok := input["task_id"].(float64)
	if !ok {
		return "", fmt.Errorf("task_id is required and must be a number")
	}

	if err := h.manager.Delete(int(taskID)); err != nil {
		return "", err
	}

	return fmt.Sprintf("Task %d deleted", int(taskID)), nil
}