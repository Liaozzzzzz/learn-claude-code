package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadHandler reads file contents.
type ReadHandler struct {
	WorkDir string
}

// NewReadHandler creates a new read handler.
func NewReadHandler(workDir string) *ReadHandler {
	return &ReadHandler{WorkDir: workDir}
}

// Execute reads a file's contents.
func (h *ReadHandler) Execute(input map[string]interface{}) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path must be a string")
	}

	limit, _ := input["limit"].(float64) // JSON numbers are float64

	// Validate path is within workdir
	safePath, err := h.safePath(path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	content, err := os.ReadFile(safePath)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	text := string(content)
	lines := strings.Split(text, "\n")

	// Apply limit if specified
	if limit > 0 && int(limit) < len(lines) {
		remaining := len(lines) - int(limit)
		lines = lines[:int(limit)]
		lines = append(lines, fmt.Sprintf("... (%d more lines)", remaining))
	}

	result := strings.Join(lines, "\n")

	// Limit output size
	if len(result) > 50000 {
		result = result[:50000]
	}

	return result, nil
}

// safePath ensures the path doesn't escape the working directory.
func (h *ReadHandler) safePath(p string) (string, error) {
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

// ReadDefinition returns the tool definition for read_file.
func ReadDefinition() Definition {
	return Definition{
		Name:        "read_file",
		Description: "Read file contents.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"path": {
					Type:        "string",
					Description: "The file path to read",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of lines to read",
				},
			},
			Required: []string{"path"},
		},
	}
}