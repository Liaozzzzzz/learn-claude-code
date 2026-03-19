// Package tools provides worktree management for task isolation.
// Directory-level isolation for parallel task execution.
// Tasks are the control plane and worktrees are the execution plane.
//
//	.tasks/task_12.json
//	  {
//	    "id": 12,
//	    "subject": "Implement auth refactor",
//	    "status": "in_progress",
//	    "worktree": "auth-refactor"
//	  }
//
//	.worktrees/index.json
//	  {
//	    "worktrees": [
//	      {
//	        "name": "auth-refactor",
//	        "path": ".../.worktrees/auth-refactor",
//	        "branch": "wt/auth-refactor",
//	        "task_id": 12,
//	        "status": "active"
//	      }
//	    ]
//	  }
//
// Key insight: "Isolate by directory, coordinate by task ID."
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WorktreeStatus represents the status of a worktree.
type WorktreeStatus string

const (
	WorktreeActive  WorktreeStatus = "active"
	WorktreeRemoved WorktreeStatus = "removed"
	WorktreeKept    WorktreeStatus = "kept"
)

// WorktreeEntry represents a worktree in the index.
type WorktreeEntry struct {
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Branch    string         `json:"branch"`
	TaskID    *int           `json:"task_id,omitempty"`
	Status    WorktreeStatus `json:"status"`
	CreatedAt float64        `json:"created_at"`
	RemovedAt float64        `json:"removed_at,omitempty"`
	KeptAt    float64        `json:"kept_at,omitempty"`
}

// WorktreeIndex represents the worktree index file.
type WorktreeIndex struct {
	Worktrees []WorktreeEntry `json:"worktrees"`
}

// WorktreeEvent represents a lifecycle event.
type WorktreeEvent struct {
	Event     string         `json:"event"`
	Timestamp float64        `json:"ts"`
	Task      map[string]any `json:"task"`
	Worktree  map[string]any `json:"worktree"`
	Error     string         `json:"error,omitempty"`
}

// EventBus manages append-only lifecycle events.
type EventBus struct {
	path string
	mu   sync.Mutex
}

// NewEventBus creates a new event bus.
func NewEventBus(path string) *EventBus {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	// Create file if not exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.WriteFile(path, []byte{}, 0644)
	}
	return &EventBus{path: path}
}

// Emit appends an event to the log.
func (b *EventBus) Emit(event string, task map[string]any, worktree map[string]any, errMsg string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	evt := WorktreeEvent{
		Event:     event,
		Timestamp: float64(time.Now().UnixNano()) / 1e9,
		Task:      task,
		Worktree:  worktree,
	}
	if errMsg != "" {
		evt.Error = errMsg
	}

	data, _ := json.Marshal(evt)
	f, err := os.OpenFile(b.path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
	f.Write([]byte("\n"))
}

// ListRecent returns the most recent events.
func (b *EventBus) ListRecent(limit int) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	data, err := os.ReadFile(b.path)
	if err != nil {
		return "[]"
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return "[]"
	}

	// Get recent lines
	start := len(lines) - limit
	if start < 0 {
		start = 0
	}
	recent := lines[start:]

	var events []WorktreeEvent
	for _, line := range recent {
		if line == "" {
			continue
		}
		var evt WorktreeEvent
		if err := json.Unmarshal([]byte(line), &evt); err == nil {
			events = append(events, evt)
		}
	}

	data, _ = json.MarshalIndent(events, "", "  ")
	return string(data)
}

// WorktreeManager manages git worktrees with task binding.
type WorktreeManager struct {
	repoRoot    string
	dir         string
	indexPath   string
	tasks       *TaskManager
	events      *EventBus
	gitAvailable bool
	mu          sync.Mutex
}

// NewWorktreeManager creates a new worktree manager.
func NewWorktreeManager(repoRoot string, tasks *TaskManager, events *EventBus) *WorktreeManager {
	dir := filepath.Join(repoRoot, ".worktrees")
	os.MkdirAll(dir, 0755)

	indexPath := filepath.Join(dir, "index.json")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		data, _ := json.Marshal(WorktreeIndex{Worktrees: []WorktreeEntry{}})
		os.WriteFile(indexPath, data, 0644)
	}

	wm := &WorktreeManager{
		repoRoot:  repoRoot,
		dir:       dir,
		indexPath: indexPath,
		tasks:     tasks,
		events:    events,
	}
	wm.gitAvailable = wm.isGitRepo()
	return wm
}

// IsGitAvailable returns whether git is available.
func (wm *WorktreeManager) IsGitAvailable() bool {
	return wm.gitAvailable
}

func (wm *WorktreeManager) isGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = wm.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "true"
}

func (wm *WorktreeManager) runGit(args ...string) (string, error) {
	if !wm.gitAvailable {
		return "", fmt.Errorf("not in a git repository. worktree tools require git")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = wm.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = fmt.Sprintf("git %s failed", args[0])
		}
		return "", fmt.Errorf("%s", msg)
	}
	result := strings.TrimSpace(string(output))
	if result == "" {
		return "(no output)", nil
	}
	return result, nil
}

func (wm *WorktreeManager) loadIndex() (*WorktreeIndex, error) {
	data, err := os.ReadFile(wm.indexPath)
	if err != nil {
		return nil, err
	}
	var idx WorktreeIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

func (wm *WorktreeManager) saveIndex(idx *WorktreeIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(wm.indexPath, data, 0644)
}

func (wm *WorktreeManager) findEntry(name string) *WorktreeEntry {
	idx, err := wm.loadIndex()
	if err != nil {
		return nil
	}
	for i := range idx.Worktrees {
		if idx.Worktrees[i].Name == name {
			return &idx.Worktrees[i]
		}
	}
	return nil
}

func (wm *WorktreeManager) validateName(name string) error {
	matched, _ := regexp.MatchString(`^[A-Za-z0-9._-]{1,40}$`, name)
	if !matched {
		return fmt.Errorf("invalid worktree name. Use 1-40 chars: letters, numbers, ., _, -")
	}
	return nil
}

// Create creates a new worktree and optionally binds it to a task.
func (wm *WorktreeManager) Create(name string, taskID *int, baseRef string) (*WorktreeEntry, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if err := wm.validateName(name); err != nil {
		return nil, err
	}

	if wm.findEntry(name) != nil {
		return nil, fmt.Errorf("worktree '%s' already exists in index", name)
	}

	if taskID != nil && !wm.tasks.Exists(*taskID) {
		return nil, fmt.Errorf("task %d not found", *taskID)
	}

	if baseRef == "" {
		baseRef = "HEAD"
	}

	path := filepath.Join(wm.dir, name)
	branch := fmt.Sprintf("wt/%s", name)

	// Emit before event
	taskMap := map[string]any{}
	if taskID != nil {
		taskMap["id"] = *taskID
	}
	wm.events.Emit("worktree.create.before", taskMap, map[string]any{
		"name":    name,
		"baseRef": baseRef,
	}, "")

	// Run git worktree add
	_, err := wm.runGit("worktree", "add", "-b", branch, path, baseRef)
	if err != nil {
		wm.events.Emit("worktree.create.failed", taskMap, map[string]any{
			"name":    name,
			"baseRef": baseRef,
		}, err.Error())
		return nil, err
	}

	// Create entry
	entry := WorktreeEntry{
		Name:      name,
		Path:      path,
		Branch:    branch,
		TaskID:    taskID,
		Status:    WorktreeActive,
		CreatedAt: float64(time.Now().UnixNano()) / 1e9,
	}

	// Update index
	idx, _ := wm.loadIndex()
	idx.Worktrees = append(idx.Worktrees, entry)
	wm.saveIndex(idx)

	// Bind to task if specified
	if taskID != nil {
		wm.tasks.BindWorktree(*taskID, name, "")
	}

	// Emit after event
	wm.events.Emit("worktree.create.after", taskMap, map[string]any{
		"name":   name,
		"path":   path,
		"branch": branch,
		"status": "active",
	}, "")

	return &entry, nil
}

// List returns all worktrees.
func (wm *WorktreeManager) List() string {
	idx, err := wm.loadIndex()
	if err != nil || len(idx.Worktrees) == 0 {
		return "No worktrees in index."
	}

	var lines []string
	for _, wt := range idx.Worktrees {
		suffix := ""
		if wt.TaskID != nil {
			suffix = fmt.Sprintf(" task=%d", *wt.TaskID)
		}
		lines = append(lines, fmt.Sprintf("[%s] %s -> %s (%s)%s", wt.Status, wt.Name, wt.Path, wt.Branch, suffix))
	}
	return strings.Join(lines, "\n")
}

// Status returns the git status of a worktree.
func (wm *WorktreeManager) Status(name string) (string, error) {
	entry := wm.findEntry(name)
	if entry == nil {
		return "", fmt.Errorf("unknown worktree '%s'", name)
	}

	if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("worktree path missing: %s", entry.Path)
	}

	cmd := exec.Command("git", "status", "--short", "--branch")
	cmd.Dir = entry.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	result := strings.TrimSpace(string(output))
	if result == "" {
		return "Clean worktree", nil
	}
	return result, nil
}

// Run executes a command in a worktree directory.
func (wm *WorktreeManager) Run(name, command string) (string, error) {
	dangerous := []string{"rm -rf /", "sudo", "shutdown", "reboot", "> /dev/"}
	for _, d := range dangerous {
		if strings.Contains(command, d) {
			return "", fmt.Errorf("dangerous command blocked")
		}
	}

	entry := wm.findEntry(name)
	if entry == nil {
		return "", fmt.Errorf("unknown worktree '%s'", name)
	}

	if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("worktree path missing: %s", entry.Path)
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = entry.Path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	result := string(output)
	if len(result) > 50000 {
		result = result[:50000] + "... (truncated)"
	}
	if result == "" {
		return "(no output)", nil
	}
	return result, nil
}

// Remove removes a worktree and optionally marks its task as completed.
func (wm *WorktreeManager) Remove(name string, force, completeTask bool) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	entry := wm.findEntry(name)
	if entry == nil {
		return fmt.Errorf("unknown worktree '%s'", name)
	}

	taskMap := map[string]any{}
	if entry.TaskID != nil {
		taskMap["id"] = *entry.TaskID
	}

	// Emit before event
	wm.events.Emit("worktree.remove.before", taskMap, map[string]any{
		"name": name,
		"path": entry.Path,
	}, "")

	// Run git worktree remove
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, entry.Path)

	_, err := wm.runGit(args...)
	if err != nil {
		wm.events.Emit("worktree.remove.failed", taskMap, map[string]any{
			"name": name,
			"path": entry.Path,
		}, err.Error())
		return err
	}

	// Complete task if requested
	if completeTask && entry.TaskID != nil {
		status := TaskCompleted
		wm.tasks.Update(*entry.TaskID, UpdateOptions{Status: &status})
		wm.tasks.UnbindWorktree(*entry.TaskID)
		task, _ := wm.tasks.Get(*entry.TaskID)
		wm.events.Emit("task.completed", map[string]any{
			"id":      *entry.TaskID,
			"subject": task.Subject,
			"status":  "completed",
		}, map[string]any{"name": name}, "")
	}

	// Update index
	idx, _ := wm.loadIndex()
	for i := range idx.Worktrees {
		if idx.Worktrees[i].Name == name {
			idx.Worktrees[i].Status = WorktreeRemoved
			idx.Worktrees[i].RemovedAt = float64(time.Now().UnixNano()) / 1e9
		}
	}
	wm.saveIndex(idx)

	// Emit after event
	wm.events.Emit("worktree.remove.after", taskMap, map[string]any{
		"name":   name,
		"path":   entry.Path,
		"status": "removed",
	}, "")

	return nil
}

// Keep marks a worktree as kept without removing it.
func (wm *WorktreeManager) Keep(name string) (*WorktreeEntry, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	entry := wm.findEntry(name)
	if entry == nil {
		return nil, fmt.Errorf("unknown worktree '%s'", name)
	}

	taskMap := map[string]any{}
	if entry.TaskID != nil {
		taskMap["id"] = *entry.TaskID
	}

	// Update index
	idx, _ := wm.loadIndex()
	var kept *WorktreeEntry
	for i := range idx.Worktrees {
		if idx.Worktrees[i].Name == name {
			idx.Worktrees[i].Status = WorktreeKept
			idx.Worktrees[i].KeptAt = float64(time.Now().UnixNano()) / 1e9
			kept = &idx.Worktrees[i]
		}
	}
	wm.saveIndex(idx)

	// Emit event
	wm.events.Emit("worktree.keep", taskMap, map[string]any{
		"name":   name,
		"path":   entry.Path,
		"status": "kept",
	}, "")

	return kept, nil
}

// --- Task Manager Extensions ---

// BindWorktree binds a task to a worktree.
func (m *TaskManager) BindWorktree(id int, worktree, owner string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.load(id)
	if err != nil {
		return nil, err
	}

	task.Worktree = worktree
	if owner != "" {
		task.Owner = owner
	}
	if task.Status == TaskPending {
		task.Status = TaskInProgress
	}

	if err := m.save(task); err != nil {
		return nil, err
	}
	return task, nil
}

// UnbindWorktree removes the worktree binding from a task.
func (m *TaskManager) UnbindWorktree(id int) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.load(id)
	if err != nil {
		return nil, err
	}

	task.Worktree = ""
	if err := m.save(task); err != nil {
		return nil, err
	}
	return task, nil
}

// Exists checks if a task exists.
func (m *TaskManager) Exists(id int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, err := m.load(id)
	return err == nil
}

// --- Tool Definitions ---

// WorktreeCreateDefinition returns the tool definition for worktree_create.
func WorktreeCreateDefinition() Definition {
	return Definition{
		Name:        "worktree_create",
		Description: "Create a git worktree and optionally bind it to a task. Use for isolated parallel work.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name":     {Type: "string", Description: "Worktree name (1-40 chars: letters, numbers, ., _, -)"},
				"task_id":  {Type: "integer", Description: "Optional task ID to bind to this worktree"},
				"base_ref": {Type: "string", Description: "Git ref to base the worktree on (default: HEAD)"},
			},
			Required: []string{"name"},
		},
	}
}

// WorktreeListDefinition returns the tool definition for worktree_list.
func WorktreeListDefinition() Definition {
	return Definition{
		Name:        "worktree_list",
		Description: "List all worktrees tracked in .worktrees/index.json.",
		InputSchema: InputSchema{Type: "object"},
	}
}

// WorktreeStatusDefinition returns the tool definition for worktree_status.
func WorktreeStatusDefinition() Definition {
	return Definition{
		Name:        "worktree_status",
		Description: "Show git status for a named worktree.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name": {Type: "string", Description: "Worktree name"},
			},
			Required: []string{"name"},
		},
	}
}

// WorktreeRunDefinition returns the tool definition for worktree_run.
func WorktreeRunDefinition() Definition {
	return Definition{
		Name:        "worktree_run",
		Description: "Run a shell command in a named worktree directory.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name":    {Type: "string", Description: "Worktree name"},
				"command": {Type: "string", Description: "Shell command to run"},
			},
			Required: []string{"name", "command"},
		},
	}
}

// WorktreeRemoveDefinition returns the tool definition for worktree_remove.
func WorktreeRemoveDefinition() Definition {
	return Definition{
		Name:        "worktree_remove",
		Description: "Remove a worktree and optionally mark its bound task completed.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name":           {Type: "string", Description: "Worktree name"},
				"force":          {Type: "boolean", Description: "Force removal even with uncommitted changes"},
				"complete_task":  {Type: "boolean", Description: "Mark the bound task as completed"},
			},
			Required: []string{"name"},
		},
	}
}

// WorktreeKeepDefinition returns the tool definition for worktree_keep.
func WorktreeKeepDefinition() Definition {
	return Definition{
		Name:        "worktree_keep",
		Description: "Mark a worktree as kept in lifecycle state without removing it.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name": {Type: "string", Description: "Worktree name"},
			},
			Required: []string{"name"},
		},
	}
}

// WorktreeEventsDefinition returns the tool definition for worktree_events.
func WorktreeEventsDefinition() Definition {
	return Definition{
		Name:        "worktree_events",
		Description: "List recent worktree/task lifecycle events from .worktrees/events.jsonl.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"limit": {Type: "integer", Description: "Number of recent events to return (default: 20, max: 200)"},
			},
		},
	}
}

// TaskBindWorktreeDefinition returns the tool definition for task_bind_worktree.
func TaskBindWorktreeDefinition() Definition {
	return Definition{
		Name:        "task_bind_worktree",
		Description: "Bind a task to a worktree name.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"task_id":   {Type: "integer", Description: "Task ID to bind"},
				"worktree":  {Type: "string", Description: "Worktree name to bind to"},
				"owner":     {Type: "string", Description: "Optional owner name"},
			},
			Required: []string{"task_id", "worktree"},
		},
	}
}

// --- Tool Handlers ---

// WorktreeCreateHandler handles worktree_create tool calls.
type WorktreeCreateHandler struct {
	manager *WorktreeManager
}

// NewWorktreeCreateHandler creates a new worktree_create handler.
func NewWorktreeCreateHandler(manager *WorktreeManager) *WorktreeCreateHandler {
	return &WorktreeCreateHandler{manager: manager}
}

// Execute implements Handler.
func (h *WorktreeCreateHandler) Execute(input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	var taskID *int
	if tid, ok := input["task_id"].(float64); ok {
		id := int(tid)
		taskID = &id
	}

	baseRef, _ := input["base_ref"].(string)

	entry, err := h.manager.Create(name, taskID, baseRef)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(entry, "", "  ")
	return string(data), nil
}

// WorktreeListHandler handles worktree_list tool calls.
type WorktreeListHandler struct {
	manager *WorktreeManager
}

// NewWorktreeListHandler creates a new worktree_list handler.
func NewWorktreeListHandler(manager *WorktreeManager) *WorktreeListHandler {
	return &WorktreeListHandler{manager: manager}
}

// Execute implements Handler.
func (h *WorktreeListHandler) Execute(input map[string]any) (string, error) {
	return h.manager.List(), nil
}

// WorktreeStatusHandler handles worktree_status tool calls.
type WorktreeStatusHandler struct {
	manager *WorktreeManager
}

// NewWorktreeStatusHandler creates a new worktree_status handler.
func NewWorktreeStatusHandler(manager *WorktreeManager) *WorktreeStatusHandler {
	return &WorktreeStatusHandler{manager: manager}
}

// Execute implements Handler.
func (h *WorktreeStatusHandler) Execute(input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	return h.manager.Status(name)
}

// WorktreeRunHandler handles worktree_run tool calls.
type WorktreeRunHandler struct {
	manager *WorktreeManager
}

// NewWorktreeRunHandler creates a new worktree_run handler.
func NewWorktreeRunHandler(manager *WorktreeManager) *WorktreeRunHandler {
	return &WorktreeRunHandler{manager: manager}
}

// Execute implements Handler.
func (h *WorktreeRunHandler) Execute(input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	command, _ := input["command"].(string)
	if name == "" || command == "" {
		return "", fmt.Errorf("name and command are required")
	}
	return h.manager.Run(name, command)
}

// WorktreeRemoveHandler handles worktree_remove tool calls.
type WorktreeRemoveHandler struct {
	manager *WorktreeManager
}

// NewWorktreeRemoveHandler creates a new worktree_remove handler.
func NewWorktreeRemoveHandler(manager *WorktreeManager) *WorktreeRemoveHandler {
	return &WorktreeRemoveHandler{manager: manager}
}

// Execute implements Handler.
func (h *WorktreeRemoveHandler) Execute(input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	force, _ := input["force"].(bool)
	completeTask, _ := input["complete_task"].(bool)

	err := h.manager.Remove(name, force, completeTask)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed worktree '%s'", name), nil
}

// WorktreeKeepHandler handles worktree_keep tool calls.
type WorktreeKeepHandler struct {
	manager *WorktreeManager
}

// NewWorktreeKeepHandler creates a new worktree_keep handler.
func NewWorktreeKeepHandler(manager *WorktreeManager) *WorktreeKeepHandler {
	return &WorktreeKeepHandler{manager: manager}
}

// Execute implements Handler.
func (h *WorktreeKeepHandler) Execute(input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}

	entry, err := h.manager.Keep(name)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(entry, "", "  ")
	return string(data), nil
}

// WorktreeEventsHandler handles worktree_events tool calls.
type WorktreeEventsHandler struct {
	events *EventBus
}

// NewWorktreeEventsHandler creates a new worktree_events handler.
func NewWorktreeEventsHandler(events *EventBus) *WorktreeEventsHandler {
	return &WorktreeEventsHandler{events: events}
}

// Execute implements Handler.
func (h *WorktreeEventsHandler) Execute(input map[string]any) (string, error) {
	limit := 20
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
	}
	return h.events.ListRecent(limit), nil
}

// TaskBindWorktreeHandler handles task_bind_worktree tool calls.
type TaskBindWorktreeHandler struct {
	manager *TaskManager
}

// NewTaskBindWorktreeHandler creates a new task_bind_worktree handler.
func NewTaskBindWorktreeHandler(manager *TaskManager) *TaskBindWorktreeHandler {
	return &TaskBindWorktreeHandler{manager: manager}
}

// Execute implements Handler.
func (h *TaskBindWorktreeHandler) Execute(input map[string]any) (string, error) {
	taskID, ok := input["task_id"].(float64)
	if !ok {
		return "", fmt.Errorf("task_id is required")
	}

	worktree, _ := input["worktree"].(string)
	if worktree == "" {
		return "", fmt.Errorf("worktree is required")
	}

	owner, _ := input["owner"].(string)

	task, err := h.manager.BindWorktree(int(taskID), worktree, owner)
	if err != nil {
		return "", err
	}

	data, _ := json.MarshalIndent(task, "", "  ")
	return string(data), nil
}