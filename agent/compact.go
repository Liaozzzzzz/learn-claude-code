// Package agent implements context compaction for long-running agent sessions.
//
// Three-layer compression pipeline so the agent can work forever:
//
//	Every turn:
//	+------------------+
//	| Tool call result |
//	+------------------+
//	        |
//	        v
//	[Layer 1: micro_compact]        (silent, every turn)
//	  Replace tool_result content older than last N
//	  with "[Previous: used {tool_name}]"
//	        |
//	        v
//	[Check: tokens > threshold?]
//	   |               |
//	   no              yes
//	   |               |
//	   v               v
//	continue    [Layer 2: auto_compact]
//	              Save full transcript to .transcripts/
//	              Ask LLM to summarize conversation.
//	              Replace all messages with [summary].
//	                    |
//	                    v
//	            [Layer 3: compact tool]
//	              Model calls compact -> immediate summarization.
//	              Same as auto, triggered manually.
//
// Key insight: "The agent can forget strategically and keep working forever."
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CompactConfig configures the context compaction behavior.
type CompactConfig struct {
	// Threshold is the token count that triggers auto_compact (default 50000)
	Threshold int
	// KeepRecent is the number of recent tool results to preserve in micro_compact (default 3)
	KeepRecent int
	// TranscriptDir is the directory to save transcripts (default ".transcripts")
	TranscriptDir string
	// WorkDir is the working directory for transcript storage
	WorkDir string
}

// DefaultCompactConfig returns a CompactConfig with sensible defaults.
func DefaultCompactConfig() CompactConfig {
	return CompactConfig{
		Threshold:     50000,
		KeepRecent:    3,
		TranscriptDir: ".transcripts",
		WorkDir:       ".",
	}
}

// Compactor handles context compaction operations.
type Compactor struct {
	config CompactConfig
	client LLMClient
	model  string
}

// NewCompactor creates a new Compactor with the given configuration.
func NewCompactor(client LLMClient, config CompactConfig) *Compactor {
	return &Compactor{
		config: config,
		client: client,
	}
}

// EstimateTokens provides a rough token count estimate (~4 chars per token).
func EstimateTokens(messages []Message) int {
	// Use JSON serialization for consistent measurement
	data, _ := json.Marshal(messages)
	return len(data) / 4
}

// MicroCompact (Layer 1) replaces old tool results with placeholders.
// This runs silently on every turn to reduce token usage gradually.
func MicroCompact(messages []Message, keepRecent int) []Message {
	if keepRecent <= 0 {
		keepRecent = 3
	}

	// Collect all tool result entries with their positions
	type toolResultEntry struct {
		msgIndex  int
		partIndex int
		result    *ToolResultContent
	}

	var toolResults []toolResultEntry
	for msgIdx, msg := range messages {
		if msg.Role == "user" {
			if parts, ok := msg.Content.([]ToolResultContent); ok {
				for partIdx := range parts {
					toolResults = append(toolResults, toolResultEntry{
						msgIndex:  msgIdx,
						partIndex: partIdx,
						result:    &parts[partIdx],
					})
				}
			}
		}
	}

	// Nothing to compact
	if len(toolResults) <= keepRecent {
		return messages
	}

	// Build tool name map from assistant messages
	toolNameMap := make(map[string]string)
	for _, msg := range messages {
		if msg.Role == "assistant" {
			if blocks, ok := msg.Content.([]ContentBlock); ok {
				for _, block := range blocks {
					if block.Type == "tool_use" {
						toolNameMap[block.ID] = block.Name
					}
				}
			}
		}
	}

	// Clear old results (keep last keepRecent)
	toClear := toolResults[:len(toolResults)-keepRecent]
	compacted := 0
	for _, entry := range toClear {
		content := entry.result.Content
		if str, ok := content.(string); ok && len(str) > 100 {
			toolID := entry.result.ToolUseID
			toolName := toolNameMap[toolID]
			if toolName == "" {
				toolName = "unknown"
			}
			entry.result.Content = fmt.Sprintf("[Previous: used %s]", toolName)
			compacted++
		}
	}

	if compacted > 0 {
		fmt.Printf("[micro_compact: %d tool results compressed]\n", compacted)
	}

	return messages
}

// AutoCompact (Layer 2) saves transcript and generates summary.
// This runs when token count exceeds the threshold.
func (c *Compactor) AutoCompact(ctx context.Context, messages []Message) ([]Message, error) {
	// Save full transcript to disk
	transcriptDir := filepath.Join(c.config.WorkDir, c.config.TranscriptDir)
	if err := os.MkdirAll(transcriptDir, 0755); err != nil {
		return nil, fmt.Errorf("create transcript dir: %w", err)
	}

	timestamp := time.Now().Unix()
	transcriptPath := filepath.Join(transcriptDir, fmt.Sprintf("transcript_%d.jsonl", timestamp))

	file, err := os.Create(transcriptPath)
	if err != nil {
		return nil, fmt.Errorf("create transcript file: %w", err)
	}

	encoder := json.NewEncoder(file)
	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			file.Close()
			return nil, fmt.Errorf("write transcript: %w", err)
		}
	}
	file.Close()

	fmt.Printf("[transcript saved: %s]\n", transcriptPath)

	// Ask LLM to summarize
	conversationData, _ := json.Marshal(messages)
	conversationText := string(conversationData)
	if len(conversationText) > 80000 {
		conversationText = conversationText[:80000]
	}

	summaryPrompt := fmt.Sprintf(
		"Summarize this conversation for continuity. Include: "+
			"1) What was accomplished, 2) Current state, 3) Key decisions made. "+
			"Be concise but preserve critical details.\n\n%s",
		conversationText,
	)

	summaryMsg := Message{
		Role:    "user",
		Content: summaryPrompt,
	}

	resp, err := c.client.CreateMessage(ctx, "", []Message{summaryMsg}, nil)
	if err != nil {
		return nil, fmt.Errorf("generate summary: %w", err)
	}

	var summaryText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			summaryText = block.Text
			break
		}
	}

	// Replace all messages with compressed summary
	return []Message{
		{
			Role: "user",
			Content: fmt.Sprintf("[Conversation compressed. Transcript: %s]\n\n%s",
				transcriptPath, summaryText),
		},
		{
			Role:    "assistant",
			Content: "Understood. I have the context from the summary. Continuing.",
		},
	}, nil
}

// ShouldAutoCompact checks if auto_compact should be triggered.
func (c *Compactor) ShouldAutoCompact(messages []Message) bool {
	return EstimateTokens(messages) > c.config.Threshold
}

// CompactTool is the handler for the "compact" tool (Layer 3).
// It triggers manual compaction when the model calls it.
type CompactTool struct {
	compactor *Compactor
}

// NewCompactTool creates a new compact tool definition.
func NewCompactTool() Tool {
	return Tool{
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
	}
}

// CompactResult represents the result of a compact operation.
type CompactResult struct {
	Triggered   bool
	NewMessages []Message
	Error       error
}
