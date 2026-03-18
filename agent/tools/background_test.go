package tools

import (
	"strings"
	"testing"
	"time"
)

func TestBackgroundManager_Run(t *testing.T) {
	bg := NewBackgroundManager(t.TempDir())

	// Run a quick command
	result := bg.Run("echo hello")
	if !strings.HasPrefix(result, "Background task") {
		t.Errorf("Expected task started message, got: %s", result)
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	// Check notifications were drained
	notif := bg.DrainNotifications()
	if notif == "" {
		t.Error("Expected notification after command completion")
	}
	if !strings.Contains(notif, "completed") {
		t.Errorf("Expected completed status in notification, got: %s", notif)
	}
}

func TestBackgroundManager_ConcurrentUniqueIDs(t *testing.T) {
	bg := NewBackgroundManager(t.TempDir())

	// Run many tasks concurrently to verify unique IDs
	const numTasks = 100
	idChan := make(chan string, numTasks)

	for i := 0; i < numTasks; i++ {
		go func() {
			result := bg.Run("echo test")
			// Extract task ID: "Background task XXXXXXXX started: ..."
			parts := strings.Fields(result)
			if len(parts) >= 3 {
				idChan <- parts[2]
			}
		}()
	}

	// Collect all IDs
	ids := make(map[string]bool)
	for i := 0; i < numTasks; i++ {
		id := <-idChan
		if ids[id] {
			t.Errorf("Duplicate task ID: %s", id)
		}
		ids[id] = true
	}

	if len(ids) != numTasks {
		t.Errorf("Expected %d unique IDs, got %d", numTasks, len(ids))
	}
}

func TestBackgroundManager_Check(t *testing.T) {
	bg := NewBackgroundManager(t.TempDir())

	// Check with no tasks
	result := bg.Check("")
	if result != "No background tasks." {
		t.Errorf("Expected no tasks message, got: %s", result)
	}

	// Run a command
	taskResult := bg.Run("sleep 0.1")

	// Extract task ID from result
	// Result format: "Background task XXXXXXXX started: ..."
	parts := strings.Split(taskResult, " ")
	if len(parts) < 3 {
		t.Fatalf("Unexpected result format: %s", taskResult)
	}
	taskID := parts[2]

	// Check specific task
	result = bg.Check(taskID)
	if !strings.Contains(result, "running") && !strings.Contains(result, "completed") {
		t.Errorf("Expected status in result, got: %s", result)
	}

	// Check all tasks
	result = bg.Check("")
	if !strings.Contains(result, taskID) {
		t.Errorf("Expected task ID in list, got: %s", result)
	}

	// Check unknown task
	result = bg.Check("unknown")
	if !strings.Contains(result, "Unknown task") {
		t.Errorf("Expected unknown task error, got: %s", result)
	}
}

func TestBackgroundManager_DrainNotifications(t *testing.T) {
	bg := NewBackgroundManager(t.TempDir())

	// Drain with no notifications
	notif := bg.DrainNotifications()
	if notif != "" {
		t.Errorf("Expected empty string, got: %s", notif)
	}

	// Run command and wait
	bg.Run("echo test")
	time.Sleep(200 * time.Millisecond)

	// Drain should return notifications
	notif = bg.DrainNotifications()
	if notif == "" {
		t.Error("Expected notifications after command")
	}

	// Second drain should be empty
	notif = bg.DrainNotifications()
	if notif != "" {
		t.Errorf("Expected empty after drain, got: %s", notif)
	}
}

func TestBackgroundRunHandler(t *testing.T) {
	bg := NewBackgroundManager(t.TempDir())
	h := NewBackgroundRunHandler(bg)

	result, err := h.Execute(map[string]interface{}{"command": "echo hello"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.HasPrefix(result, "Background task") {
		t.Errorf("Expected task started message, got: %s", result)
	}

	// Test missing command
	_, err = h.Execute(map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for missing command")
	}
}

func TestCheckBackgroundHandler(t *testing.T) {
	bg := NewBackgroundManager(t.TempDir())
	h := NewCheckBackgroundHandler(bg)

	// Check with no tasks
	result, err := h.Execute(map[string]interface{}{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "No background tasks." {
		t.Errorf("Expected no tasks message, got: %s", result)
	}

	// Check specific task
	result, err = h.Execute(map[string]interface{}{"task_id": "unknown"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !strings.Contains(result, "Unknown task") {
		t.Errorf("Expected unknown task error, got: %s", result)
	}
}

func TestBackgroundRunDefinition(t *testing.T) {
	def := BackgroundRunDefinition()
	if def.Name != "background_run" {
		t.Errorf("Expected name 'background_run', got: %s", def.Name)
	}
	if _, ok := def.InputSchema.Properties["command"]; !ok {
		t.Error("Expected 'command' property in input schema")
	}
}

func TestCheckBackgroundDefinition(t *testing.T) {
	def := CheckBackgroundDefinition()
	if def.Name != "check_background" {
		t.Errorf("Expected name 'check_background', got: %s", def.Name)
	}
	if _, ok := def.InputSchema.Properties["task_id"]; !ok {
		t.Error("Expected 'task_id' property in input schema")
	}
}