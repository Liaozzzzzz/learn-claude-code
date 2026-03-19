package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdleHandler_Execute(t *testing.T) {
	handler := NewIdleHandler()

	result, err := handler.Execute(map[string]any{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}
}

func TestClaimTaskHandler_Execute(t *testing.T) {
	// Create temp tasks directory
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	manager := NewTaskManager(tasksDir)

	// Create a task
	task, err := manager.Create("Test task", "Description")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	handler := NewClaimTaskHandler(manager, "bob")

	// Claim the task
	result, err := handler.Execute(map[string]any{"task_id": float64(task.ID)})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result == "" {
		t.Error("Result should not be empty")
	}

	// Verify task was claimed
	got, err := manager.Get(task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Owner != "bob" {
		t.Errorf("Owner = %s, want bob", got.Owner)
	}
	if got.Status != TaskInProgress {
		t.Errorf("Status = %s, want in_progress", got.Status)
	}
}

func TestClaimTaskHandler_AlreadyOwned(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	manager := NewTaskManager(tasksDir)
	task, _ := manager.Create("Test task", "Description")

	// Claim once
	manager.Claim(task.ID, "alice")

	handler := NewClaimTaskHandler(manager, "bob")

	// Try to claim again
	_, err := handler.Execute(map[string]any{"task_id": float64(task.ID)})
	if err == nil {
		t.Error("Expected error for already owned task")
	}
}

func TestTaskManager_ScanUnclaimed(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	manager := NewTaskManager(tasksDir)

	// Create tasks
	task1, _ := manager.Create("Task 1", "")
	task2, _ := manager.Create("Task 2", "")
	task3, _ := manager.Create("Task 3", "")

	// Claim task2
	manager.Claim(task2.ID, "alice")

	// Add blocker to task3
	manager.Update(task3.ID, UpdateOptions{AddBlockedBy: []int{task1.ID}})

	// Scan unclaimed
	unclaimed, err := manager.ScanUnclaimed()
	if err != nil {
		t.Fatalf("ScanUnclaimed failed: %v", err)
	}

	// Only task1 should be unclaimed
	if len(unclaimed) != 1 {
		t.Errorf("ScanUnclaimed returned %d tasks, want 1", len(unclaimed))
	}
	if len(unclaimed) > 0 && unclaimed[0].ID != task1.ID {
		t.Errorf("Unclaimed task ID = %d, want %d", unclaimed[0].ID, task1.ID)
	}
}

func TestTaskManager_Claim(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	manager := NewTaskManager(tasksDir)
	task, _ := manager.Create("Test task", "")

	// Claim the task
	claimed, err := manager.Claim(task.ID, "bob")
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if claimed.Owner != "bob" {
		t.Errorf("Owner = %s, want bob", claimed.Owner)
	}
	if claimed.Status != TaskInProgress {
		t.Errorf("Status = %s, want in_progress", claimed.Status)
	}
}

func TestTaskManager_Claim_NotPending(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	manager := NewTaskManager(tasksDir)
	task, _ := manager.Create("Test task", "")

	// Complete the task first
	status := TaskCompleted
	manager.Update(task.ID, UpdateOptions{Status: &status})

	// Try to claim
	_, err := manager.Claim(task.ID, "bob")
	if err == nil {
		t.Error("Expected error for non-pending task")
	}
}

func TestTryAutoClaim(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, ".tasks")
	os.MkdirAll(tasksDir, 0755)

	manager := NewTaskManager(tasksDir)

	// No tasks
	result, err := TryAutoClaim(manager, "bob")
	if err != nil {
		t.Fatalf("TryAutoClaim failed: %v", err)
	}
	if result.Claimed {
		t.Error("Should not claim when no tasks")
	}

	// Create a task
	task, _ := manager.Create("Test task", "Description")

	result, err = TryAutoClaim(manager, "bob")
	if err != nil {
		t.Fatalf("TryAutoClaim failed: %v", err)
	}
	if !result.Claimed {
		t.Error("Should have claimed a task")
	}
	if result.Task == nil {
		t.Fatal("Task should not be nil")
	}
	if result.Task.ID != task.ID {
		t.Errorf("Task ID = %d, want %d", result.Task.ID, task.ID)
	}
	if result.Task.Owner != "bob" {
		t.Errorf("Owner = %s, want bob", result.Task.Owner)
	}
}

func TestMakeIdentityBlock(t *testing.T) {
	block := MakeIdentityBlock("alice", "coder", "my-team")

	role, ok := block["role"].(string)
	if !ok || role != "user" {
		t.Errorf("role = %v, want 'user'", block["role"])
	}

	content, ok := block["content"].(string)
	if !ok {
		t.Error("content should be string")
	}
	if content == "" {
		t.Error("content should not be empty")
	}
}

func TestDefaultAutonomousConfig(t *testing.T) {
	config := DefaultAutonomousConfig()

	if config.PollInterval <= 0 {
		t.Error("PollInterval should be positive")
	}
	if config.IdleTimeout <= 0 {
		t.Error("IdleTimeout should be positive")
	}
}