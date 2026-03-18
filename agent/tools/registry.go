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
func DefaultRegistryWithTodoAndSkills(workDir, skillsDir string) (*Registry, *TodoManager, *SkillLoader, *BackgroundManager) {
	r := NewRegistryWithWorkDir(workDir)

	// Create and register todo tool
	todoManager := NewTodoManager()
	r.Register("todo", TodoDefinition(), NewTodoHandler(todoManager))

	// Create and register skill tool
	skillLoader := NewSkillLoader(skillsDir)
	if skillLoader.HasSkills() {
		r.Register("load_skill", SkillDefinition(), NewSkillHandler(skillLoader))
	}

	// Create tasks directory and manager
	tasksDir := filepath.Join(workDir, ".tasks")
	taskManager := NewTaskManager(tasksDir)

	// Register task tools
	r.Register("task_create", TaskCreateDefinition(), NewTaskCreateHandler(taskManager))
	r.Register("task_update", TaskUpdateDefinition(), NewTaskUpdateHandler(taskManager))
	r.Register("task_list", TaskListDefinition(), NewTaskListHandler(taskManager))
	r.Register("task_get", TaskGetDefinition(), NewTaskGetHandler(taskManager))
	r.Register("task_delete", TaskDeleteDefinition(), NewTaskDeleteHandler(taskManager))

	// Create and register background tools
	bgManager := NewBackgroundManager(workDir)
	r.Register("background_run", BackgroundRunDefinition(), NewBackgroundRunHandler(bgManager))
	r.Register("check_background", CheckBackgroundDefinition(), NewCheckBackgroundHandler(bgManager))

	// Register compact tool (handler is a placeholder - actual logic in agent loop)
	r.RegisterFunc("compact", Definition{
		Name:        "compact",
		Description: "Trigger manual conversation compression to reduce context size.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"focus": {
					Type:        "string",
					Description: "What to preserve in the summary",
				},
			},
		},
	}, func(input map[string]interface{}) (string, error) {
		return "Compressing...", nil
	})

	return r, todoManager, skillLoader, bgManager
}

// TaskManagerFromRegistry extracts the TaskManager from a registry that has tasks registered.
// This is a convenience function for callers that need access to the TaskManager.
func TaskManagerFromRegistry(workDir string) *TaskManager {
	tasksDir := filepath.Join(workDir, ".tasks")
	return NewTaskManager(tasksDir)
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

// DefaultRegistryWithTeam creates a registry with all tools including team features.
func DefaultRegistryWithTeam(workDir, skillsDir string) (*Registry, *TodoManager, *SkillLoader, *BackgroundManager, *MessageBus, *TeammateManager) {
	r, todoManager, skillLoader, bgManager := DefaultRegistryWithTodoAndSkills(workDir, skillsDir)

	// Create team directory and components
	teamDir := filepath.Join(workDir, ".team")
	inboxDir := filepath.Join(teamDir, "inbox")
	bus := NewMessageBus(inboxDir)
	teamManager := NewTeammateManager(teamDir, bus)

	// Register team tools
	r.Register("spawn_teammate", SpawnTeammateDefinition(), NewSpawnTeammateHandler(teamManager))
	r.Register("list_teammates", ListTeammatesDefinition(), NewListTeammatesHandler(teamManager))
	r.Register("send_message", SendMessageDefinition(), NewSendMessageHandler(bus, "lead"))
	r.Register("read_inbox", ReadInboxToolDefinition(), NewReadInboxHandler(bus, "lead"))
	r.Register("broadcast", BroadcastDefinition(), NewBroadcastHandler(bus, "lead", teamManager.MemberNames))

	return r, todoManager, skillLoader, bgManager, bus, teamManager
}
