package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditHandler replaces text in a file.
type EditHandler struct {
	WorkDir string
}

// NewEditHandler creates a new edit handler.
func NewEditHandler(workDir string) *EditHandler {
	return &EditHandler{WorkDir: workDir}
}

// Execute replaces exact text in a file.
func (h *EditHandler) Execute(input map[string]interface{}) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	oldText, ok := input["old_text"].(string)
	if !ok {
		return "", fmt.Errorf("old_text must be a string")
	}

	newText, ok := input["new_text"].(string)
	if !ok {
		return "", fmt.Errorf("new_text must be a string")
	}

	// Validate path is within workdir
	safePath, err := h.safePath(path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	// Read file content
	content, err := os.ReadFile(safePath)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	text := string(content)

	// Check if old_text exists
	if !strings.Contains(text, oldText) {
		return fmt.Sprintf("Error: Text not found in %s", path), nil
	}

	// Replace first occurrence
	newContent := strings.Replace(text, oldText, newText, 1)

	if err := os.WriteFile(safePath, []byte(newContent), 0644); err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	return fmt.Sprintf("Edited %s", path), nil
}

// safePath ensures the path doesn't escape the working directory.
func (h *EditHandler) safePath(p string) (string, error) {
	absPath := filepath.Join(h.WorkDir, p)
	absPath = filepath.Clean(absPath)

	absWorkDir, err := filepath.Abs(h.WorkDir)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absPath, absWorkDir) {
		return "", fmt.Errorf("path escapes workspace: %s", p)
	}

	return absPath, nil
}

// EditDefinition returns the tool definition for edit_file.
func EditDefinition() Definition {
	return Definition{
		Name:        "edit_file",
		Description: "Replace exact text in file.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {
					Type:        "string",
					Description: "The file path to edit",
				},
				"old_text": {
					Type:        "string",
					Description: "The exact text to replace",
				},
				"new_text": {
					Type:        "string",
					Description: "The replacement text",
				},
			},
			Required: []string{"path", "old_text", "new_text"},
		},
	}
}