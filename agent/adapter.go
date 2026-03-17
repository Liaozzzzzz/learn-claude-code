package agent

import (
	"learn-claude-code/agent/tools"
)

// ToTool converts a tools.Definition to agent.Tool.
func ToTool(def tools.Definition) Tool {
	props := make(map[string]Property, len(def.InputSchema.Properties))
	for k, v := range def.InputSchema.Properties {
		props[k] = Property{
			Type:        v.Type,
			Description: v.Description,
		}
	}

	return Tool{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: InputSchema{
			Type:       def.InputSchema.Type,
			Properties: props,
			Required:   def.InputSchema.Required,
		},
	}
}

// ToTools converts multiple tools.Definition to []agent.Tool.
func ToTools(defs []tools.Definition) []Tool {
	result := make([]Tool, len(defs))
	for i, def := range defs {
		result[i] = ToTool(def)
	}
	return result
}