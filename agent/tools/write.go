package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteHandler writes content to a file.
type WriteHandler struct {
	WorkDir string
}

// NewWriteHandler creates a new write handler.
func NewWriteHandler(workDir string) *WriteHandler {
	return &WriteHandler{WorkDir: workDir}
}

// Execute writes content to a file.
func (h *WriteHandler) Execute(input map[string]interface{}) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("content must be a string")
	}

	// Validate path is within workdir
	safePath, err := h.safePath(path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(safePath), 0755); err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	return fmt.Sprintf("Wrote %d bytes to %s", len(content), path), nil
}

// safePath ensures the path doesn't escape the working directory.
func (h *WriteHandler) safePath(p string) (string, error) {
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

// WriteDefinition returns the tool definition for write_file.
func WriteDefinition() Definition {
	return Definition{
		Name:        "write_file",
		Description: "Write content to file.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {
					Type:        "string",
					Description: "The file path to write to",
				},
				"content": {
					Type:        "string",
					Description: "The content to write",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}