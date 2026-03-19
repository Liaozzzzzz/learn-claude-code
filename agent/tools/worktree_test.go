package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEventBus_EmitAndList(t *testing.T) {
	tmpDir := t.TempDir()
	eventsPath := filepath.Join(tmpDir, "events.jsonl")
	bus := NewEventBus(eventsPath)

	// Emit some events
	bus.Emit("test.event", map[string]any{"key": "value"}, map[string]any{"name": "test"}, "")

	// List events
	result := bus.ListRecent(10)
	if result == "[]" {
		t.Error("Expected events, got empty array")
	}
}

func TestEventBus_ListRecent_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	eventsPath := filepath.Join(tmpDir, "events.jsonl")
	// Create empty file
	os.WriteFile(eventsPath, []byte{}, 0644)
	bus := NewEventBus(eventsPath)

	result := bus.ListRecent(10)
	if result != "[]" {
		t.Errorf("Expected empty array, got %s", result)
	}
}

func TestWorktreeManager_List_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)

	result := wm.List()
	if result != "No worktrees in index." {
		t.Errorf("Expected 'No worktrees in index.', got %s", result)
	}
}

func TestWorktreeManager_ValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "test", false},
		{"valid with dash", "my-worktree", false},
		{"valid with underscore", "my_worktree", false},
		{"valid with dot", "my.worktree", false},
		{"valid alphanumeric", "abc123", false},
		{"empty", "", true},
		{"too long", "this-is-a-very-long-name-that-exceeds-forty-characters-limit", true},
		{"invalid chars", "invalid@name!", true},
		{"spaces", "has spaces", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wm := &WorktreeManager{}
			err := wm.validateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestWorktreeManager_IsGitAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)

	// In a non-git directory, should be false
	if wm.IsGitAvailable() {
		t.Log("Git available (running in git repo context)")
	} else {
		t.Log("Git not available (expected in temp dir)")
	}
}

func TestTaskManager_BindWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	taskMgr := NewTaskManager(tasksDir)
	task, err := taskMgr.Create("Test task", "Description")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Bind worktree
	updated, err := taskMgr.BindWorktree(task.ID, "my-worktree", "alice")
	if err != nil {
		t.Fatalf("BindWorktree failed: %v", err)
	}

	if updated.Worktree != "my-worktree" {
		t.Errorf("Worktree = %s, want my-worktree", updated.Worktree)
	}
	if updated.Owner != "alice" {
		t.Errorf("Owner = %s, want alice", updated.Owner)
	}
	if updated.Status != TaskInProgress {
		t.Errorf("Status = %s, want in_progress", updated.Status)
	}
}

func TestTaskManager_UnbindWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	taskMgr := NewTaskManager(tasksDir)
	task, _ := taskMgr.Create("Test task", "Description")
	taskMgr.BindWorktree(task.ID, "my-worktree", "")

	// Unbind worktree
	updated, err := taskMgr.UnbindWorktree(task.ID)
	if err != nil {
		t.Fatalf("UnbindWorktree failed: %v", err)
	}

	if updated.Worktree != "" {
		t.Errorf("Worktree = %s, want empty", updated.Worktree)
	}
}

func TestTaskManager_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	taskMgr := NewTaskManager(tasksDir)
	task, _ := taskMgr.Create("Test task", "Description")

	if !taskMgr.Exists(task.ID) {
		t.Error("Exists should return true for existing task")
	}
	if taskMgr.Exists(9999) {
		t.Error("Exists should return false for non-existing task")
	}
}

func TestWorktreeCreateHandler_Execute_InvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)
	handler := NewWorktreeCreateHandler(wm)

	_, err := handler.Execute(map[string]any{"name": ""})
	if err == nil {
		t.Error("Expected error for empty name")
	}
}

func TestWorktreeListHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)
	handler := NewWorktreeListHandler(wm)

	result, err := handler.Execute(map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result != "No worktrees in index." {
		t.Errorf("Expected 'No worktrees in index.', got %s", result)
	}
}

func TestWorktreeStatusHandler_Execute_UnknownWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)
	handler := NewWorktreeStatusHandler(wm)

	_, err := handler.Execute(map[string]any{"name": "unknown"})
	if err == nil {
		t.Error("Expected error for unknown worktree")
	}
}

func TestWorktreeRunHandler_Execute_DangerousCommand(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)
	handler := NewWorktreeRunHandler(wm)

	_, err := handler.Execute(map[string]any{"name": "test", "command": "sudo rm -rf /"})
	if err == nil {
		t.Error("Expected error for dangerous command")
	}
}

func TestWorktreeRemoveHandler_Execute_UnknownWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)
	handler := NewWorktreeRemoveHandler(wm)

	_, err := handler.Execute(map[string]any{"name": "unknown"})
	if err == nil {
		t.Error("Expected error for unknown worktree")
	}
}

func TestWorktreeKeepHandler_Execute_UnknownWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	eventsPath := filepath.Join(tmpDir, "events.jsonl")

	os.MkdirAll(tasksDir, 0755)
	taskMgr := NewTaskManager(tasksDir)
	eventBus := NewEventBus(eventsPath)

	wm := NewWorktreeManager(tmpDir, taskMgr, eventBus)
	handler := NewWorktreeKeepHandler(wm)

	_, err := handler.Execute(map[string]any{"name": "unknown"})
	if err == nil {
		t.Error("Expected error for unknown worktree")
	}
}

func TestWorktreeEventsHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	eventsPath := filepath.Join(tmpDir, "events.jsonl")
	eventBus := NewEventBus(eventsPath)
	handler := NewWorktreeEventsHandler(eventBus)

	result, err := handler.Execute(map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}
}

func TestTaskBindWorktreeHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	taskMgr := NewTaskManager(tasksDir)
	task, _ := taskMgr.Create("Test task", "Description")

	handler := NewTaskBindWorktreeHandler(taskMgr)

	result, err := handler.Execute(map[string]any{
		"task_id":  float64(task.ID),
		"worktree": "my-worktree",
		"owner":    "alice",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}

	// Verify binding
	got, _ := taskMgr.Get(task.ID)
	if got.Worktree != "my-worktree" {
		t.Errorf("Worktree = %s, want my-worktree", got.Worktree)
	}
}

func TestTaskBindWorktreeHandler_Execute_MissingRequired(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	taskMgr := NewTaskManager(tasksDir)
	handler := NewTaskBindWorktreeHandler(taskMgr)

	_, err := handler.Execute(map[string]any{"task_id": float64(1)})
	if err == nil {
		t.Error("Expected error for missing worktree")
	}

	_, err = handler.Execute(map[string]any{"worktree": "test"})
	if err == nil {
		t.Error("Expected error for missing task_id")
	}
}