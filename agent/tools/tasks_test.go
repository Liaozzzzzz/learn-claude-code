package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskManager_Create(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	task, err := tm.Create("Test task", "Test description")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.ID != 1 {
		t.Errorf("expected ID 1, got %d", task.ID)
	}

	if task.Subject != "Test task" {
		t.Errorf("expected subject 'Test task', got %q", task.Subject)
	}

	if task.Status != TaskPending {
		t.Errorf("expected status pending, got %s", task.Status)
	}

	// Verify file was created
	path := filepath.Join(tmpDir, "task_1.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("task file was not created")
	}
}

func TestTaskManager_Create_EmptySubject(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	_, err := tm.Create("", "description")
	if err == nil {
		t.Error("expected error for empty subject")
	}
}

func TestTaskManager_Get(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	// Create a task first
	created, _ := tm.Create("Test task", "Test description")

	// Get the task
	task, err := tm.Get(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if task.ID != created.ID {
		t.Errorf("expected ID %d, got %d", created.ID, task.ID)
	}
}

func TestTaskManager_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	_, err := tm.Get(999)
	if err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestTaskManager_Update_Status(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	created, _ := tm.Create("Test task", "")

	// Update to in_progress
	status := TaskInProgress
	updated, err := tm.Update(created.ID, UpdateOptions{Status: &status})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.Status != TaskInProgress {
		t.Errorf("expected status in_progress, got %s", updated.Status)
	}
}

func TestTaskManager_Update_Completed_ClearsDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	// Create two tasks
	task1, _ := tm.Create("Task 1", "")
	task2, _ := tm.Create("Task 2", "")

	// Task 2 is blocked by task 1
	tm.Update(task2.ID, UpdateOptions{AddBlockedBy: []int{task1.ID}})

	// Verify task 2 is blocked
	task2Check, _ := tm.Get(task2.ID)
	if len(task2Check.BlockedBy) != 1 || task2Check.BlockedBy[0] != task1.ID {
		t.Error("task 2 should be blocked by task 1")
	}

	// Complete task 1
	status := TaskCompleted
	tm.Update(task1.ID, UpdateOptions{Status: &status})

	// Verify task 2's blockedBy is cleared
	task2Check, _ = tm.Get(task2.ID)
	if len(task2Check.BlockedBy) != 0 {
		t.Errorf("task 2's blockedBy should be cleared, got %v", task2Check.BlockedBy)
	}
}

func TestTaskManager_Update_AddBlocks_Bidirectional(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	// Create two tasks
	task1, _ := tm.Create("Task 1", "")
	task2, _ := tm.Create("Task 2", "")

	// Task 1 blocks task 2
	tm.Update(task1.ID, UpdateOptions{AddBlocks: []int{task2.ID}})

	// Verify task 1's blocks list
	task1Check, _ := tm.Get(task1.ID)
	if len(task1Check.Blocks) != 1 || task1Check.Blocks[0] != task2.ID {
		t.Error("task 1 should block task 2")
	}

	// Verify task 2's blockedBy list (bidirectional)
	task2Check, _ := tm.Get(task2.ID)
	if len(task2Check.BlockedBy) != 1 || task2Check.BlockedBy[0] != task1.ID {
		t.Error("task 2 should be blocked by task 1")
	}
}

func TestTaskManager_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	task, _ := tm.Create("Test task", "")

	err := tm.Delete(task.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was deleted
	_, err = tm.Get(task.ID)
	if err == nil {
		t.Error("task should be deleted")
	}
}

func TestTaskManager_Delete_RemovesFromDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	// Create two tasks
	task1, _ := tm.Create("Task 1", "")
	task2, _ := tm.Create("Task 2", "")

	// Task 2 is blocked by task 1
	tm.Update(task2.ID, UpdateOptions{AddBlockedBy: []int{task1.ID}})

	// Delete task 1
	tm.Delete(task1.ID)

	// Verify task 2's blockedBy is updated
	task2Check, _ := tm.Get(task2.ID)
	if len(task2Check.BlockedBy) != 0 {
		t.Errorf("task 2's blockedBy should be empty, got %v", task2Check.BlockedBy)
	}
}

func TestTaskManager_List(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	// Create multiple tasks
	tm.Create("Task 1", "")
	tm.Create("Task 2", "")
	tm.Create("Task 3", "")

	tasks, err := tm.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	// Verify sorted by ID
	for i, task := range tasks {
		if task.ID != i+1 {
			t.Errorf("expected task ID %d, got %d", i+1, task.ID)
		}
	}
}

func TestTaskManager_Render(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)

	// Empty tasks
	if tm.Render() != "No tasks." {
		t.Error("expected 'No tasks.' for empty tasks")
	}

	// Create tasks
	tm.Create("Task 1", "")
	status := TaskInProgress
	tm.Update(1, UpdateOptions{Status: &status})
	tm.Create("Task 2", "")
	tm.Update(2, UpdateOptions{AddBlockedBy: []int{1}})

	rendered := tm.Render()

	// Check markers
	if !containsSubstring(rendered, "[>] #1: Task 1") {
		t.Error("expected in_progress marker for task 1")
	}
	if !containsSubstring(rendered, "(blocked by: [1])") {
		t.Error("expected blocked by indicator for task 2")
	}
}

func TestTaskManager_NextID_Persists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create first manager and task
	tm1 := NewTaskManager(tmpDir)
	task1, _ := tm1.Create("Task 1", "")

	// Create second manager (simulates restart)
	tm2 := NewTaskManager(tmpDir)
	task2, _ := tm2.Create("Task 2", "")

	// IDs should be sequential
	if task2.ID != task1.ID+1 {
		t.Errorf("expected task ID %d, got %d", task1.ID+1, task2.ID)
	}
}

// --- Handler Tests ---

func TestTaskCreateHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)
	handler := NewTaskCreateHandler(tm)

	result, err := handler.Execute(map[string]interface{}{
		"subject":     "Test task",
		"description": "Test description",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSubstring(result, `"id": 1`) {
		t.Error("expected task ID in result")
	}
	if !containsSubstring(result, `"subject": "Test task"`) {
		t.Error("expected subject in result")
	}
}

func TestTaskCreateHandler_Execute_EmptySubject(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)
	handler := NewTaskCreateHandler(tm)

	_, err := handler.Execute(map[string]interface{}{
		"description": "Test description",
	})
	if err == nil {
		t.Error("expected error for missing subject")
	}
}

func TestTaskUpdateHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)
	tm.Create("Test task", "")
	handler := NewTaskUpdateHandler(tm)

	result, err := handler.Execute(map[string]interface{}{
		"task_id": float64(1),
		"status":  "in_progress",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSubstring(result, `"status": "in_progress"`) {
		t.Error("expected status update in result")
	}
}

func TestTaskListHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)
	tm.Create("Task 1", "")
	tm.Create("Task 2", "")
	handler := NewTaskListHandler(tm)

	result, err := handler.Execute(map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSubstring(result, "Task 1") || !containsSubstring(result, "Task 2") {
		t.Error("expected both tasks in result")
	}
}

func TestTaskGetHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)
	tm.Create("Test task", "Test description")
	handler := NewTaskGetHandler(tm)

	result, err := handler.Execute(map[string]interface{}{
		"task_id": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSubstring(result, "Test task") {
		t.Error("expected task subject in result")
	}
}

func TestTaskDeleteHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTaskManager(tmpDir)
	tm.Create("Test task", "")
	handler := NewTaskDeleteHandler(tm)

	result, err := handler.Execute(map[string]interface{}{
		"task_id": float64(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSubstring(result, "deleted") {
		t.Error("expected deletion message")
	}

	// Verify task is gone
	_, err = tm.Get(1)
	if err == nil {
		t.Error("task should be deleted")
	}
}