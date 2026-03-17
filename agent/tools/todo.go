package tools

import (
	"fmt"
	"strings"
	"sync"
)

// TodoStatus represents the status of a todo item.
type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

// TodoItem represents a single todo item.
type TodoItem struct {
	ID     string     `json:"id"`
	Text   string     `json:"text"`
	Status TodoStatus `json:"status"`
}

// TodoManager manages a list of todo items.
type TodoManager struct {
	mu    sync.RWMutex
	items []TodoItem
}

// NewTodoManager creates a new TodoManager.
func NewTodoManager() *TodoManager {
	return &TodoManager{
		items: make([]TodoItem, 0),
	}
}

// Update replaces the todo list with new items.
// Returns the rendered todo list or an error.
func (m *TodoManager) Update(items []TodoItem) (string, error) {
	if len(items) > 20 {
		return "", fmt.Errorf("max 20 todos allowed")
	}

	var validated []TodoItem
	inProgressCount := 0

	for i, item := range items {
		// Set default ID if not provided
		id := item.ID
		if id == "" {
			id = fmt.Sprintf("%d", i+1)
		}

		// Validate text
		text := item.Text
		if text == "" {
			return "", fmt.Errorf("item %s: text required", id)
		}

		// Validate and normalize status
		status := TodoPending
		switch item.Status {
		case TodoPending, TodoInProgress, TodoCompleted:
			status = item.Status
		default:
			return "", fmt.Errorf("item %s: invalid status '%s'", id, item.Status)
		}

		if status == TodoInProgress {
			inProgressCount++
		}

		validated = append(validated, TodoItem{
			ID:     id,
			Text:   text,
			Status: status,
		})
	}

	if inProgressCount > 1 {
		return "", fmt.Errorf("only one task can be in_progress at a time")
	}

	m.mu.Lock()
	m.items = validated
	m.mu.Unlock()

	return m.Render(), nil
}

// Render returns a formatted string representation of the todo list.
func (m *TodoManager) Render() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.items) == 0 {
		return "No todos."
	}

	var lines []string
	for _, item := range m.items {
		var marker string
		switch item.Status {
		case TodoPending:
			marker = "[ ]"
		case TodoInProgress:
			marker = "[>]"
		case TodoCompleted:
			marker = "[x]"
		}
		lines = append(lines, fmt.Sprintf("%s #%s: %s", marker, item.ID, item.Text))
	}

	done := 0
	for _, t := range m.items {
		if t.Status == TodoCompleted {
			done++
		}
	}
	lines = append(lines, fmt.Sprintf("\n(%d/%d completed)", done, len(m.items)))

	return strings.Join(lines, "\n")
}

// Items returns a copy of the current todo items.
func (m *TodoManager) Items() []TodoItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TodoItem, len(m.items))
	copy(result, m.items)
	return result
}

// TodoHandler handles todo tool execution.
type TodoHandler struct {
	manager *TodoManager
}

// NewTodoHandler creates a new todo handler.
func NewTodoHandler(manager *TodoManager) *TodoHandler {
	return &TodoHandler{manager: manager}
}

// Execute processes the todo tool input.
func (h *TodoHandler) Execute(input map[string]interface{}) (string, error) {
	itemsRaw, ok := input["items"]
	if !ok {
		return "", fmt.Errorf("items is required")
	}

	itemsSlice, ok := itemsRaw.([]interface{})
	if !ok {
		return "", fmt.Errorf("items must be an array")
	}

	var items []TodoItem
	for _, itemRaw := range itemsSlice {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("each item must be an object")
		}

		item := TodoItem{}

		if id, ok := itemMap["id"].(string); ok {
			item.ID = id
		}
		if text, ok := itemMap["text"].(string); ok {
			item.Text = text
		}
		if status, ok := itemMap["status"].(string); ok {
			item.Status = TodoStatus(status)
		}

		items = append(items, item)
	}

	return h.manager.Update(items)
}

// Manager returns the underlying TodoManager.
func (h *TodoHandler) Manager() *TodoManager {
	return h.manager
}

// TodoDefinition returns the tool definition for todo.
func TodoDefinition() Definition {
	return Definition{
		Name:        "todo",
		Description: "Update task list. Track progress on multi-step tasks.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"items": {
					Type:        "array",
					Description: "List of todo items",
				},
			},
			Required: []string{"items"},
		},
	}
}