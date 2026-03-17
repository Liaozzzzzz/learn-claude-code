// Package tools provides tool implementations for the agent.
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Skill represents a loaded skill with its metadata and body.
type Skill struct {
	Name        string
	Description string
	Body        string
	Path        string
}

// SkillLoader loads and manages skills from a directory.
// It implements a two-layer skill injection pattern:
//   - Layer 1 (cheap): skill names in system prompt (~100 tokens/skill)
//   - Layer 2 (on demand): full skill body in tool_result
type SkillLoader struct {
	skillsDir string
	skills    map[string]*Skill
}

// NewSkillLoader creates a new SkillLoader and loads all skills from the directory.
func NewSkillLoader(skillsDir string) *SkillLoader {
	loader := &SkillLoader{
		skillsDir: skillsDir,
		skills:    make(map[string]*Skill),
	}
	loader.loadAll()
	return loader
}

// loadAll scans the skills directory for SKILL.md files with YAML frontmatter.
func (l *SkillLoader) loadAll() {
	skillsDir := l.skillsDir
	if skillsDir == "" {
		return
	}

	// Check if skills directory exists
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return
	}

	// Find all SKILL.md files
	err := filepath.Walk(skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() != "SKILL.md" {
			return nil
		}

		// Read and parse the skill file
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		skill := l.parseSkill(string(content), path)
		if skill != nil {
			l.skills[skill.Name] = skill
		}
		return nil
	})
	_ = err // Ignore errors
}

// parseSkill parses a SKILL.md file with YAML frontmatter.
// The format is:
//
//	---
//	name: skill-name
//	description: A short description
//	---
//
//	Skill body content...
func (l *SkillLoader) parseSkill(content, path string) *Skill {
	// Parse YAML frontmatter between --- delimiters
	re := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)`)
	match := re.FindStringSubmatch(content)
	if match == nil {
		// No frontmatter, use directory name as skill name
		dir := filepath.Dir(path)
		name := filepath.Base(dir)
		return &Skill{
			Name:        name,
			Description: "No description",
			Body:        strings.TrimSpace(content),
			Path:        path,
		}
	}

	// Parse frontmatter
	meta := make(map[string]string)
	for _, line := range strings.Split(match[1], "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			meta[key] = val
		}
	}

	// Get skill name from frontmatter or directory
	name := meta["name"]
	if name == "" {
		dir := filepath.Dir(path)
		name = filepath.Base(dir)
	}

	description := meta["description"]
	if description == "" {
		description = "No description"
	}

	body := strings.TrimSpace(match[2])

	return &Skill{
		Name:        name,
		Description: description,
		Body:        body,
		Path:        path,
	}
}

// GetDescriptions returns Layer 1: short descriptions for the system prompt.
func (l *SkillLoader) GetDescriptions() string {
	if len(l.skills) == 0 {
		return "(no skills available)"
	}

	// Sort skill names for consistent output
	names := make([]string, 0, len(l.skills))
	for name := range l.skills {
		names = append(names, name)
	}
	sort.Strings(names)

	var lines []string
	for _, name := range names {
		skill := l.skills[name]
		lines = append(lines, fmt.Sprintf("  - %s: %s", name, skill.Description))
	}
	return strings.Join(lines, "\n")
}

// GetContent returns Layer 2: full skill body for tool_result.
func (l *SkillLoader) GetContent(name string) (string, error) {
	skill, ok := l.skills[name]
	if !ok {
		available := make([]string, 0, len(l.skills))
		for n := range l.skills {
			available = append(available, n)
		}
		sort.Strings(available)
		return "", fmt.Errorf("unknown skill '%s'. Available: %s", name, strings.Join(available, ", "))
	}
	return fmt.Sprintf("<skill name=\"%s\">\n%s\n</skill>", name, skill.Body), nil
}

// HasSkills returns true if there are any loaded skills.
func (l *SkillLoader) HasSkills() bool {
	return len(l.skills) > 0
}

// SkillNames returns all loaded skill names.
func (l *SkillLoader) SkillNames() []string {
	names := make([]string, 0, len(l.skills))
	for name := range l.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SkillDefinition returns the tool definition for the skill loader.
func SkillDefinition() Definition {
	return Definition{
		Name:        "load_skill",
		Description: "Load specialized knowledge by name. Use this tool to access expertise for specific tasks.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name": {
					Type:        "string",
					Description: "Skill name to load",
				},
			},
			Required: []string{"name"},
		},
	}
}

// SkillHandler handles the load_skill tool.
type SkillHandler struct {
	loader *SkillLoader
}

// NewSkillHandler creates a new SkillHandler.
func NewSkillHandler(loader *SkillLoader) *SkillHandler {
	return &SkillHandler{loader: loader}
}

// Execute implements Handler.
func (h *SkillHandler) Execute(input map[string]interface{}) (string, error) {
	name, ok := input["name"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid 'name' parameter")
	}
	return h.loader.GetContent(name)
}
