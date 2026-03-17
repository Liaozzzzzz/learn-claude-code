// Package tools provides tool implementations for the agent.
package tools

// Handler is the interface for tool execution.
type Handler interface {
	Execute(input map[string]interface{}) (string, error)
}

// HandlerFunc is an adapter to allow using functions as Handler.
type HandlerFunc func(input map[string]interface{}) (string, error)

// Execute implements Handler.
func (f HandlerFunc) Execute(input map[string]interface{}) (string, error) {
	return f(input)
}

// Definition represents a tool's schema definition.
type Definition struct {
	Name        string
	Description string
	InputSchema InputSchema
}

// InputSchema defines the schema for tool inputs.
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property defines a property in the input schema.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}
