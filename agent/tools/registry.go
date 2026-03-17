package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

// RegistryExecutor adapts a Registry to implement the agent.ToolExecutor interface.
type RegistryExecutor struct {
	*Registry
}

// Execute implements agent.ToolExecutor.
func (e *RegistryExecutor) Execute(name string, input map[string]interface{}) (string, error) {
	return e.Registry.Execute(name, input)
}

// Registry holds tool definitions and their handlers.
type Registry struct {
	handlers    map[string]Handler
	definitions map[string]Definition
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers:    make(map[string]Handler),
		definitions: make(map[string]Definition),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(name string, def Definition, handler Handler) {
	r.definitions[name] = def
	r.handlers[name] = handler
}

// RegisterFunc adds a tool with a handler function.
func (r *Registry) RegisterFunc(name string, def Definition, handler func(input map[string]interface{}) (string, error)) {
	r.Register(name, def, HandlerFunc(handler))
}

// Execute runs a tool by name.
func (r *Registry) Execute(name string, input map[string]interface{}) (string, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return handler.Execute(input)
}

// Tools returns all tool definitions in the registry.
func (r *Registry) Tools() []Definition {
	tools := make([]Definition, 0, len(r.definitions))
	for _, def := range r.definitions {
		tools = append(tools, def)
	}
	return tools
}

// HasTool checks if a tool exists in the registry.
func (r *Registry) HasTool(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// AsExecutor returns a RegistryExecutor that implements agent.ToolExecutor.
func (r *Registry) AsExecutor() *RegistryExecutor {
	return &RegistryExecutor{Registry: r}
}

// DefaultRegistry creates a registry with all standard tools.
// It uses the current working directory as the work directory.
func DefaultRegistry() *Registry {
	workDir, _ := os.Getwd()
	return NewRegistryWithWorkDir(workDir)
}

// NewRegistryWithWorkDir creates a registry with all standard tools
// using the specified working directory.
func NewRegistryWithWorkDir(workDir string) *Registry {
	// Resolve to absolute path
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		absWorkDir = workDir
	}

	r := NewRegistry()

	// Register bash tool
	r.Register("bash", BashDefinition(), NewBashHandler(absWorkDir))

	// Register read_file tool
	r.Register("read_file", ReadDefinition(), NewReadHandler(absWorkDir))

	// Register write_file tool
	r.Register("write_file", WriteDefinition(), NewWriteHandler(absWorkDir))

	// Register edit_file tool
	r.Register("edit_file", EditDefinition(), NewEditHandler(absWorkDir))

	return r
}

// DefaultRegistryWithTodo creates a registry with all standard tools including todo.
func DefaultRegistryWithTodo() (*Registry, *TodoManager) {
	workDir, _ := os.Getwd()
	return NewRegistryWithWorkDirAndTodo(workDir)
}

// DefaultRegistryWithTodoAndSkills creates a registry with all standard tools including todo and skills.
func DefaultRegistryWithTodoAndSkills(workDir, skillsDir string) (*Registry, *TodoManager, *SkillLoader) {
	r := NewRegistryWithWorkDir(workDir)

	// Create and register todo tool
	todoManager := NewTodoManager()
	r.Register("todo", TodoDefinition(), NewTodoHandler(todoManager))

	// Create and register skill tool
	skillLoader := NewSkillLoader(skillsDir)
	if skillLoader.HasSkills() {
		r.Register("load_skill", SkillDefinition(), NewSkillHandler(skillLoader))
	}

	return r, todoManager, skillLoader
}

// NewRegistryWithWorkDirAndTodo creates a registry with all standard tools including todo.
func NewRegistryWithWorkDirAndTodo(workDir string) (*Registry, *TodoManager) {
	r := NewRegistryWithWorkDir(workDir)

	// Create and register todo tool
	todoManager := NewTodoManager()
	r.Register("todo", TodoDefinition(), NewTodoHandler(todoManager))

	return r, todoManager
}

// BaseToolNames returns the names of base tools (excluding task).
var BaseToolNames = []string{"bash", "read_file", "write_file", "edit_file"}

// ChildToolNames returns the names of tools available to subagents.
// This excludes task to prevent recursive spawning.
var ChildToolNames = []string{"bash", "read_file", "write_file", "edit_file", "todo"}

// GetChildToolDefinitions returns definitions for subagent tools (without task).
func (r *Registry) GetChildToolDefinitions() []Definition {
	var defs []Definition
	for _, name := range ChildToolNames {
		if def, ok := r.definitions[name]; ok {
			defs = append(defs, def)
		}
	}
	return defs
}
