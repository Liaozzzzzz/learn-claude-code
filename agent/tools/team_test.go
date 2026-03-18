package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMessageBus_Send(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(dir)

	// Send a message
	result := bus.Send("lead", "alice", "hello", "message")
	if result != "Sent message to alice" {
		t.Errorf("Unexpected result: %s", result)
	}

	// Verify file was created
	inboxPath := filepath.Join(dir, "alice.jsonl")
	if _, err := os.Stat(inboxPath); os.IsNotExist(err) {
		t.Error("Inbox file was not created")
	}
}

func TestMessageBus_ReadInbox(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(dir)

	// Read empty inbox
	msgs := bus.ReadInbox("alice")
	if len(msgs) != 0 {
		t.Errorf("Expected empty inbox, got %d messages", len(msgs))
	}

	// Send messages
	bus.Send("lead", "alice", "hello", "message")
	bus.Send("lead", "alice", "world", "message")

	// Read inbox
	msgs = bus.ReadInbox("alice")
	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}

	// Verify inbox was drained
	msgs = bus.ReadInbox("alice")
	if len(msgs) != 0 {
		t.Errorf("Expected empty inbox after drain, got %d messages", len(msgs))
	}
}

func TestMessageBus_Broadcast(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(dir)

	teammates := []string{"alice", "bob", "lead"}
	result := bus.Broadcast("lead", "hello team", teammates)
	if result != "Broadcast to 2 teammates" {
		t.Errorf("Unexpected result: %s", result)
	}

	// Verify messages were sent
	msgs := bus.ReadInbox("alice")
	if len(msgs) != 1 {
		t.Errorf("Expected 1 message for alice, got %d", len(msgs))
	}
}

func TestMessageBus_InvalidType(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(dir)

	result := bus.Send("lead", "alice", "hello", "invalid_type")
	if result == "" || result[0:5] != "Error" {
		t.Errorf("Expected error for invalid type, got: %s", result)
	}
}

func TestTeammateManager_Spawn(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(filepath.Join(dir, "inbox"))
	tm := NewTeammateManager(dir, bus)

	// Spawn a teammate (without runner, just updates config)
	result := tm.Spawn("alice", "coder", "write a hello world")
	if result != "Spawned 'alice' (role: coder)" {
		t.Errorf("Unexpected result: %s", result)
	}

	// Check config
	member := tm.findMember("alice")
	if member == nil {
		t.Fatal("Member not found in config")
	}
	if member.Role != "coder" {
		t.Errorf("Expected role 'coder', got '%s'", member.Role)
	}
	if member.Status != "working" {
		t.Errorf("Expected status 'working', got '%s'", member.Status)
	}
}

func TestTeammateManager_ListAll(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(filepath.Join(dir, "inbox"))
	tm := NewTeammateManager(dir, bus)

	// Empty team
	result := tm.ListAll()
	if result != "No teammates." {
		t.Errorf("Expected 'No teammates.', got: %s", result)
	}

	// Add members
	tm.Spawn("alice", "coder", "task1")
	tm.Spawn("bob", "tester", "task2")

	result = tm.ListAll()
	if result == "" {
		t.Error("Expected non-empty result")
	}
	if !containsStr(result, "alice") || !containsStr(result, "bob") {
		t.Errorf("Expected both teammates in list, got: %s", result)
	}
}

func TestTeammateManager_MemberNames(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(filepath.Join(dir, "inbox"))
	tm := NewTeammateManager(dir, bus)

	tm.Spawn("alice", "coder", "task1")
	tm.Spawn("bob", "tester", "task2")

	names := tm.MemberNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 names, got %d", len(names))
	}
}

func TestTeammateManager_SpawnBusy(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(filepath.Join(dir, "inbox"))
	tm := NewTeammateManager(dir, bus)

	// Spawn teammate
	tm.Spawn("alice", "coder", "task1")

	// Try to spawn again while working
	result := tm.Spawn("alice", "tester", "task2")
	if result == "" || result[0:5] != "Error" {
		t.Errorf("Expected error for busy teammate, got: %s", result)
	}
}

func TestTeammateManager_SetStatus(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(filepath.Join(dir, "inbox"))
	tm := NewTeammateManager(dir, bus)

	tm.Spawn("alice", "coder", "task1")
	tm.SetStatus("alice", "idle")

	member := tm.findMember("alice")
	if member.Status != "idle" {
		t.Errorf("Expected status 'idle', got '%s'", member.Status)
	}
}

func TestSpawnTeammateHandler(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(filepath.Join(dir, "inbox"))
	tm := NewTeammateManager(dir, bus)
	h := NewSpawnTeammateHandler(tm)

	result, err := h.Execute(map[string]any{
		"name":  "alice",
		"role":  "coder",
		"prompt": "write code",
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestListTeammatesHandler(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(filepath.Join(dir, "inbox"))
	tm := NewTeammateManager(dir, bus)
	h := NewListTeammatesHandler(tm)

	result, err := h.Execute(map[string]any{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "No teammates." {
		t.Errorf("Expected 'No teammates.', got: %s", result)
	}
}

func TestSendMessageHandler(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(dir)
	h := NewSendMessageHandler(bus, "lead")

	result, err := h.Execute(map[string]any{
		"to":      "alice",
		"content": "hello",
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "Sent message to alice" {
		t.Errorf("Unexpected result: %s", result)
	}
}

func TestReadInboxHandler(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(dir)
	bus.Send("alice", "bob", "hello", "message")

	h := NewReadInboxHandler(bus, "bob")
	result, err := h.Execute(map[string]any{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == "" || result == "[]" {
		t.Errorf("Expected non-empty inbox, got: %s", result)
	}
}

func TestBroadcastHandler(t *testing.T) {
	dir := t.TempDir()
	bus := NewMessageBus(dir)
	h := NewBroadcastHandler(bus, "lead", func() []string { return []string{"alice", "bob"} })

	result, err := h.Execute(map[string]any{
		"content": "hello team",
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "Broadcast to 2 teammates" {
		t.Errorf("Unexpected result: %s", result)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}