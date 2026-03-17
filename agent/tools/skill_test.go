package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillLoader(t *testing.T) {
	// Create a temporary skills directory
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")

	// Create a test skill
	skillDir := filepath.Join(skillsDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent := `---
name: test-skill
description: A test skill for testing
---

# Test Skill

This is the body of the test skill.
It contains instructions for testing.
`

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write skill file: %v", err)
	}

	// Create a skill without frontmatter
	skillDir2 := filepath.Join(skillsDir, "no-frontmatter")
	if err := os.MkdirAll(skillDir2, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent2 := `This skill has no frontmatter.
It should use the directory name.
`
	skillPath2 := filepath.Join(skillDir2, "SKILL.md")
	if err := os.WriteFile(skillPath2, []byte(skillContent2), 0644); err != nil {
		t.Fatalf("Failed to write skill file: %v", err)
	}

	// Load skills
	loader := NewSkillLoader(skillsDir)

	// Test that skills were loaded
	if !loader.HasSkills() {
		t.Error("Expected HasSkills to return true")
	}

	// Test skill names
	names := loader.SkillNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(names))
	}

	// Test descriptions
	descriptions := loader.GetDescriptions()
	if descriptions == "" {
		t.Error("Expected non-empty descriptions")
	}

	// Test getting skill content
	content, err := loader.GetContent("test-skill")
	if err != nil {
		t.Errorf("Failed to get skill content: %v", err)
	}
	if content == "" {
		t.Error("Expected non-empty content")
	}
	if len(content) < 50 {
		t.Errorf("Expected longer content, got: %s", content)
	}

	// Test getting non-existent skill
	_, err = loader.GetContent("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent skill")
	}

	// Test skill without frontmatter
	content2, err := loader.GetContent("no-frontmatter")
	if err != nil {
		t.Errorf("Failed to get skill content: %v", err)
	}
	if content2 == "" {
		t.Error("Expected non-empty content")
	}
}

func TestSkillLoaderEmpty(t *testing.T) {
	// Test with non-existent directory
	loader := NewSkillLoader("/non/existent/path")

	if loader.HasSkills() {
		t.Error("Expected HasSkills to return false for non-existent directory")
	}

	if loader.GetDescriptions() != "(no skills available)" {
		t.Errorf("Expected '(no skills available)', got: %s", loader.GetDescriptions())
	}

	if len(loader.SkillNames()) != 0 {
		t.Error("Expected empty skill names")
	}
}

func TestSkillDefinition(t *testing.T) {
	def := SkillDefinition()

	if def.Name != "load_skill" {
		t.Errorf("Expected name 'load_skill', got: %s", def.Name)
	}

	if def.Description == "" {
		t.Error("Expected non-empty description")
	}

	if def.InputSchema.Type != "object" {
		t.Errorf("Expected type 'object', got: %s", def.InputSchema.Type)
	}

	if len(def.InputSchema.Required) != 1 {
		t.Errorf("Expected 1 required field, got: %d", len(def.InputSchema.Required))
	}
}

func TestSkillHandler(t *testing.T) {
	// Create a loader with a test skill
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, "skills")
	skillDir := filepath.Join(skillsDir, "handler-test")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill directory: %v", err)
	}

	skillContent := `---
name: handler-test
description: Test skill for handler
---

Handler test body.
`
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write skill file: %v", err)
	}

	loader := NewSkillLoader(skillsDir)
	handler := NewSkillHandler(loader)

	// Test valid input
	result, err := handler.Execute(map[string]interface{}{"name": "handler-test"})
	if err != nil {
		t.Errorf("Failed to execute handler: %v", err)
	}
	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Test invalid input (missing name)
	_, err = handler.Execute(map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for missing name")
	}

	// Test non-existent skill
	_, err = handler.Execute(map[string]interface{}{"name": "non-existent"})
	if err == nil {
		t.Error("Expected error for non-existent skill")
	}
}