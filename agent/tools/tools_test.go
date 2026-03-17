package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	def := Definition{
		Name:        "test",
		Description: "A test tool",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"input": {Type: "string"},
			},
		},
	}

	r.Register("test", def, HandlerFunc(func(input map[string]interface{}) (string, error) {
		return "ok", nil
	}))

	if !r.HasTool("test") {
		t.Error("expected tool to be registered")
	}

	names := r.Names()
	if len(names) != 1 || names[0] != "test" {
		t.Errorf("expected [test], got %v", names)
	}
}

func TestRegistry_Execute(t *testing.T) {
	r := NewRegistry()

	r.RegisterFunc("echo", Definition{}, func(input map[string]interface{}) (string, error) {
		return input["msg"].(string), nil
	})

	result, err := r.Execute("echo", map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestRegistry_Execute_UnknownTool(t *testing.T) {
	r := NewRegistry()

	_, err := r.Execute("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestBashHandler_Execute(t *testing.T) {
	h := NewBashHandler(".")

	result, err := h.Execute(map[string]interface{}{"command": "echo hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestBashHandler_DangerousCommand(t *testing.T) {
	h := NewBashHandler(".")

	tests := []string{
		"rm -rf /",
		"sudo rm something",
		"shutdown now",
		"reboot",
	}

	for _, cmd := range tests {
		result, err := h.Execute(map[string]interface{}{"command": cmd})
		if err != nil {
			t.Errorf("unexpected error for command %q: %v", cmd, err)
		}
		if result != "Error: Dangerous command blocked" {
			t.Errorf("expected dangerous command blocked for %q, got %q", cmd, result)
		}
	}
}

func TestReadHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	h := NewReadHandler(tmpDir)

	result, err := h.Execute(map[string]interface{}{"path": "test.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != content {
		t.Errorf("expected %q, got %q", content, result)
	}
}

func TestReadHandler_Execute_WithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	h := NewReadHandler(tmpDir)

	result, err := h.Execute(map[string]interface{}{
		"path":  "test.txt",
		"limit": float64(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "line1\nline2\n... (1 more lines)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestWriteHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()

	h := NewWriteHandler(tmpDir)

	result, err := h.Execute(map[string]interface{}{
		"path":    "new.txt",
		"content": "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Wrote 11 bytes to new.txt"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "new.txt"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}

	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestEditHandler_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	h := NewEditHandler(tmpDir)

	result, err := h.Execute(map[string]interface{}{
		"path":      "test.txt",
		"old_text":  "world",
		"new_text":  "Go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Edited test.txt"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "hello Go" {
		t.Errorf("expected 'hello Go', got %q", string(data))
	}
}

func TestEditHandler_TextNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	h := NewEditHandler(tmpDir)

	result, err := h.Execute(map[string]interface{}{
		"path":      "test.txt",
		"old_text":  "notfound",
		"new_text":  "Go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Error: Text not found in test.txt" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()

	expectedTools := []string{"bash", "read_file", "write_file", "edit_file"}
	for _, name := range expectedTools {
		if !r.HasTool(name) {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestNewRegistryWithWorkDir(t *testing.T) {
	r := NewRegistryWithWorkDir("/tmp")

	if !r.HasTool("bash") {
		t.Error("expected bash tool")
	}

	tools := r.Tools()
	if len(tools) != 4 {
		t.Errorf("expected 4 tools, got %d", len(tools))
	}
}