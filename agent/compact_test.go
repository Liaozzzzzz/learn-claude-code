package agent

import (
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name      string
		messages  []Message
		minTokens int
	}{
		{
			name: "empty messages",
			messages: []Message{},
			minTokens: 0,
		},
		{
			name: "simple message",
			messages: []Message{
				{Role: "user", Content: "Hello, world!"},
			},
			minTokens: 10,
		},
		{
			name: "multiple messages",
			messages: []Message{
				{Role: "user", Content: "Write a function"},
				{Role: "assistant", Content: "Here is the function..."},
				{Role: "user", Content: "Add error handling"},
			},
			minTokens: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := EstimateTokens(tt.messages)
			if tokens < tt.minTokens {
				t.Errorf("EstimateTokens() = %d, want at least %d", tokens, tt.minTokens)
			}
		})
	}
}

func TestMicroCompact(t *testing.T) {
	tests := []struct {
		name          string
		messages      []Message
		keepRecent    int
		wantCompacted int // number of tool results that should be compacted
	}{
		{
			name: "no tool results",
			messages: []Message{
				{Role: "user", Content: "Hello"},
			},
			keepRecent:    3,
			wantCompacted: 0,
		},
		{
			name: "few tool results - no compaction",
			messages: []Message{
				{Role: "user", Content: "List files"},
				{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "tool_use", ID: "tool1", Name: "bash"},
					},
				},
				{
					Role: "user",
					Content: []ToolResultContent{
						{Type: "tool_result", ToolUseID: "tool1", Content: "file1.txt\nfile2.txt"},
					},
				},
			},
			keepRecent:    3,
			wantCompacted: 0,
		},
		{
			name: "many tool results - should compact old ones",
			messages: []Message{
				{Role: "user", Content: "List files"},
				{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "tool_use", ID: "tool1", Name: "bash"},
					},
				},
				{
					Role: "user",
					Content: []ToolResultContent{
						{Type: "tool_result", ToolUseID: "tool1", Content: string(make([]byte, 200))}, // long content
					},
				},
				{Role: "user", Content: "Read file"},
				{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "tool_use", ID: "tool2", Name: "read"},
					},
				},
				{
					Role: "user",
					Content: []ToolResultContent{
						{Type: "tool_result", ToolUseID: "tool2", Content: string(make([]byte, 200))},
					},
				},
				{Role: "user", Content: "Write file"},
				{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "tool_use", ID: "tool3", Name: "write"},
					},
				},
				{
					Role: "user",
					Content: []ToolResultContent{
						{Type: "tool_result", ToolUseID: "tool3", Content: string(make([]byte, 200))},
					},
				},
				{Role: "user", Content: "Edit file"},
				{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "tool_use", ID: "tool4", Name: "edit"},
					},
				},
				{
					Role: "user",
					Content: []ToolResultContent{
						{Type: "tool_result", ToolUseID: "tool4", Content: string(make([]byte, 200))},
					},
				},
			},
			keepRecent:    2,
			wantCompacted: 2, // tool1 and tool2 should be compacted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MicroCompact(tt.messages, tt.keepRecent)

			// Count compacted tool results
			compacted := 0
			for _, msg := range result {
				if msg.Role == "user" {
					if parts, ok := msg.Content.([]ToolResultContent); ok {
						for _, part := range parts {
							if str, ok := part.Content.(string); ok {
								if len(str) > 0 && str[0] == '[' && str[len(str)-1] == ']' {
									compacted++
								}
							}
						}
					}
				}
			}

			if compacted != tt.wantCompacted {
				t.Errorf("MicroCompact() compacted %d results, want %d", compacted, tt.wantCompacted)
			}
		})
	}
}

func TestMicroCompactPreservesToolNames(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "List files"},
		{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "tool_use", ID: "tool1", Name: "bash"},
			},
		},
		{
			Role: "user",
			Content: []ToolResultContent{
				{Type: "tool_result", ToolUseID: "tool1", Content: string(make([]byte, 500))},
			},
		},
		{Role: "user", Content: "Read file"},
		{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "tool_use", ID: "tool2", Name: "read_file"},
			},
		},
		{
			Role: "user",
			Content: []ToolResultContent{
				{Type: "tool_result", ToolUseID: "tool2", Content: "recent result"},
			},
		},
	}

	result := MicroCompact(messages, 1)

	// Find the compacted tool result
	var compactedContent string
	for _, msg := range result {
		if msg.Role == "user" {
			if parts, ok := msg.Content.([]ToolResultContent); ok {
				for _, part := range parts {
					if str, ok := part.Content.(string); ok {
						if str[0] == '[' {
							compactedContent = str
						}
					}
				}
			}
		}
	}

	expected := "[Previous: used bash]"
	if compactedContent != expected {
		t.Errorf("MicroCompact() = %q, want %q", compactedContent, expected)
	}
}

func TestShouldAutoCompact(t *testing.T) {
	config := CompactConfig{
		Threshold:     100, // low threshold for testing
		KeepRecent:    3,
		TranscriptDir: ".transcripts",
	}

	compactor := NewCompactor(nil, config)

	// Small message should not trigger
	smallMessages := []Message{
		{Role: "user", Content: "Hi"},
	}
	if compactor.ShouldAutoCompact(smallMessages) {
		t.Error("ShouldAutoCompact() should return false for small messages")
	}

	// Large message should trigger
	largeContent := string(make([]byte, 500))
	largeMessages := []Message{
		{Role: "user", Content: largeContent},
	}
	if !compactor.ShouldAutoCompact(largeMessages) {
		t.Error("ShouldAutoCompact() should return true for large messages")
	}
}