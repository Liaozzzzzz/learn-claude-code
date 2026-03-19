package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Valid message types
var ValidMsgTypes = map[string]bool{
	"message":                true,
	"broadcast":              true,
	"shutdown_request":       true,
	"shutdown_response":      true,
	"plan_approval_request":  true,
	"plan_approval_response": true,
}

// TeamMessage represents a message in a teammate's inbox.
type TeamMessage struct {
	Type      string  `json:"type"`
	From      string  `json:"from"`
	Content   string  `json:"content"`
	Timestamp float64 `json:"timestamp"`
}

// MessageBus manages JSONL inboxes for teammates.
type MessageBus struct {
	inboxDir string
	mu       sync.Mutex
}

// NewMessageBus creates a new message bus with the given inbox directory.
func NewMessageBus(inboxDir string) *MessageBus {
	os.MkdirAll(inboxDir, 0755)
	return &MessageBus{inboxDir: inboxDir}
}

// Send sends a message to a teammate's inbox.
func (b *MessageBus) Send(sender, to, content, msgType string) string {
	if !ValidMsgTypes[msgType] {
		return fmt.Sprintf("Error: Invalid type '%s'. Valid types: message, broadcast, shutdown_request, shutdown_response, plan_approval_response", msgType)
	}

	msg := TeamMessage{
		Type:      msgType,
		From:      sender,
		Content:   content,
		Timestamp: float64(time.Now().UnixNano()) / 1e9,
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	inboxPath := filepath.Join(b.inboxDir, to+".jsonl")
	f, err := os.OpenFile(inboxPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer f.Close()

	data, _ := json.Marshal(msg)
	f.Write(data)
	f.Write([]byte("\n"))

	return fmt.Sprintf("Sent %s to %s", msgType, to)
}

// ReadInbox reads and drains a teammate's inbox.
func (b *MessageBus) ReadInbox(name string) []TeamMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	inboxPath := filepath.Join(b.inboxDir, name+".jsonl")
	data, err := os.ReadFile(inboxPath)
	if err != nil {
		return nil
	}

	// Drain the inbox
	os.WriteFile(inboxPath, []byte{}, 0644)

	var messages []TeamMessage
	lines := splitLines(string(data))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var msg TeamMessage
		if err := json.Unmarshal([]byte(line), &msg); err == nil {
			messages = append(messages, msg)
		}
	}
	return messages
}

// ReadInboxJSON returns the inbox as JSON string.
func (b *MessageBus) ReadInboxJSON(name string) string {
	msgs := b.ReadInbox(name)
	if len(msgs) == 0 {
		return "[]"
	}
	data, _ := json.MarshalIndent(msgs, "", "  ")
	return string(data)
}

// Broadcast sends a message to all teammates except the sender.
func (b *MessageBus) Broadcast(sender, content string, teammates []string) string {
	count := 0
	for _, name := range teammates {
		if name != sender {
			b.Send(sender, name, content, "broadcast")
			count++
		}
	}
	return fmt.Sprintf("Broadcast to %d teammates", count)
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}


// TeamConfig represents the team configuration.
type TeamConfig struct {
	TeamName string        `json:"team_name"`
	Members  []TeamMember  `json:"members"`
}

// TeamMember represents a teammate.
type TeamMember struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"` // "idle", "working", "shutdown"
}

// TeammateManager manages persistent named agents.
type TeammateManager struct {
	dir         string
	configPath  string
	config      TeamConfig
	mu          sync.RWMutex
	bus         *MessageBus
	teammateRun func(name, role, prompt string) error
}

// NewTeammateManager creates a new teammate manager.
func NewTeammateManager(teamDir string, bus *MessageBus) *TeammateManager {
	os.MkdirAll(teamDir, 0755)
	tm := &TeammateManager{
		dir:        teamDir,
		configPath: filepath.Join(teamDir, "config.json"),
		bus:        bus,
	}
	tm.loadConfig()
	return tm
}

// SetTeammateRun sets the function to run a teammate agent loop.
func (tm *TeammateManager) SetTeammateRun(fn func(name, role, prompt string) error) {
	tm.teammateRun = fn
}

func (tm *TeammateManager) loadConfig() {
	data, err := os.ReadFile(tm.configPath)
	if err != nil {
		tm.config = TeamConfig{TeamName: "default", Members: []TeamMember{}}
		return
	}
	json.Unmarshal(data, &tm.config)
}

func (tm *TeammateManager) saveConfig() {
	data, _ := json.MarshalIndent(tm.config, "", "  ")
	os.WriteFile(tm.configPath, data, 0644)
}

func (tm *TeammateManager) findMember(name string) *TeamMember {
	for i := range tm.config.Members {
		if tm.config.Members[i].Name == name {
			return &tm.config.Members[i]
		}
	}
	return nil
}

// Spawn spawns a new teammate or reactivates an idle one.
func (tm *TeammateManager) Spawn(name, role, prompt string) string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	member := tm.findMember(name)
	if member != nil {
		if member.Status != "idle" && member.Status != "shutdown" {
			return fmt.Sprintf("Error: '%s' is currently %s", name, member.Status)
		}
		member.Status = "working"
		member.Role = role
	} else {
		tm.config.Members = append(tm.config.Members, TeamMember{
			Name:   name,
			Role:   role,
			Status: "working",
		})
	}
	tm.saveConfig()

	// Run teammate in background if runner is set
	if tm.teammateRun != nil {
		go tm.teammateRun(name, role, prompt)
	}

	return fmt.Sprintf("Spawned '%s' (role: %s)", name, role)
}

// SetStatus updates a teammate's status.
func (tm *TeammateManager) SetStatus(name, status string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	member := tm.findMember(name)
	if member != nil {
		member.Status = status
		tm.saveConfig()
	}
}

// ListAll returns a string listing all teammates.
func (tm *TeammateManager) ListAll() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if len(tm.config.Members) == 0 {
		return "No teammates."
	}

	lines := []string{fmt.Sprintf("Team: %s", tm.config.TeamName)}
	for _, m := range tm.config.Members {
		lines = append(lines, fmt.Sprintf("  %s (%s): %s", m.Name, m.Role, m.Status))
	}
	return joinLines(lines)
}

// MemberNames returns all member names.
func (tm *TeammateManager) MemberNames() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	names := make([]string, len(tm.config.Members))
	for i, m := range tm.config.Members {
		names[i] = m.Name
	}
	return names
}

// GetBus returns the message bus.
func (tm *TeammateManager) GetBus() *MessageBus {
	return tm.bus
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}


// -- Tool Definitions --

// SpawnTeammateDefinition returns the tool definition for spawn_teammate.
func SpawnTeammateDefinition() Definition {
	return Definition{
		Name:        "spawn_teammate",
		Description: "Spawn a persistent teammate that runs in its own goroutine.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"name":  {Type: "string", Description: "Teammate name"},
				"role":  {Type: "string", Description: "Teammate role (e.g., coder, tester)"},
				"prompt": {Type: "string", Description: "Initial task prompt for the teammate"},
			},
			Required: []string{"name", "role", "prompt"},
		},
	}
}

// ListTeammatesDefinition returns the tool definition for list_teammates.
func ListTeammatesDefinition() Definition {
	return Definition{
		Name:        "list_teammates",
		Description: "List all teammates with name, role, and status.",
		InputSchema: InputSchema{Type: "object"},
	}
}

// SendMessageDefinition returns the tool definition for send_message.
func SendMessageDefinition() Definition {
	return Definition{
		Name:        "send_message",
		Description: "Send a message to a teammate's inbox.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"to":      {Type: "string", Description: "Recipient name"},
				"content": {Type: "string", Description: "Message content"},
				"msg_type": {Type: "string", Description: "Message type: message, broadcast, shutdown_request, shutdown_response, plan_approval_response"},
			},
			Required: []string{"to", "content"},
		},
	}
}

// ReadInboxToolDefinition returns the tool definition for read_inbox.
func ReadInboxToolDefinition() Definition {
	return Definition{
		Name:        "read_inbox",
		Description: "Read and drain your inbox.",
		InputSchema: InputSchema{Type: "object"},
	}
}

// BroadcastDefinition returns the tool definition for broadcast.
func BroadcastDefinition() Definition {
	return Definition{
		Name:        "broadcast",
		Description: "Send a message to all teammates.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"content": {Type: "string", Description: "Message content"},
			},
			Required: []string{"content"},
		},
	}
}


// -- Tool Handlers --

// SpawnTeammateHandler handles spawn_teammate tool calls.
type SpawnTeammateHandler struct {
	manager *TeammateManager
}

// NewSpawnTeammateHandler creates a new spawn_teammate handler.
func NewSpawnTeammateHandler(manager *TeammateManager) *SpawnTeammateHandler {
	return &SpawnTeammateHandler{manager: manager}
}

// Execute implements Handler.
func (h *SpawnTeammateHandler) Execute(input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	role, _ := input["role"].(string)
	prompt, _ := input["prompt"].(string)
	if name == "" || role == "" || prompt == "" {
		return "", fmt.Errorf("name, role, and prompt are required")
	}
	return h.manager.Spawn(name, role, prompt), nil
}

// ListTeammatesHandler handles list_teammates tool calls.
type ListTeammatesHandler struct {
	manager *TeammateManager
}

// NewListTeammatesHandler creates a new list_teammates handler.
func NewListTeammatesHandler(manager *TeammateManager) *ListTeammatesHandler {
	return &ListTeammatesHandler{manager: manager}
}

// Execute implements Handler.
func (h *ListTeammatesHandler) Execute(input map[string]any) (string, error) {
	return h.manager.ListAll(), nil
}

// SendMessageHandler handles send_message tool calls.
type SendMessageHandler struct {
	bus    *MessageBus
	sender string
}

// NewSendMessageHandler creates a new send_message handler.
func NewSendMessageHandler(bus *MessageBus, sender string) *SendMessageHandler {
	return &SendMessageHandler{bus: bus, sender: sender}
}

// Execute implements Handler.
func (h *SendMessageHandler) Execute(input map[string]any) (string, error) {
	to, _ := input["to"].(string)
	content, _ := input["content"].(string)
	msgType, _ := input["msg_type"].(string)
	if msgType == "" {
		msgType = "message"
	}
	if to == "" || content == "" {
		return "", fmt.Errorf("to and content are required")
	}
	return h.bus.Send(h.sender, to, content, msgType), nil
}

// ReadInboxHandler handles read_inbox tool calls.
type ReadInboxHandler struct {
	bus    *MessageBus
	name   string
}

// NewReadInboxHandler creates a new read_inbox handler.
func NewReadInboxHandler(bus *MessageBus, name string) *ReadInboxHandler {
	return &ReadInboxHandler{bus: bus, name: name}
}

// Execute implements Handler.
func (h *ReadInboxHandler) Execute(input map[string]any) (string, error) {
	return h.bus.ReadInboxJSON(h.name), nil
}

// BroadcastHandler handles broadcast tool calls.
type BroadcastHandler struct {
	bus      *MessageBus
	sender   string
	teamFunc func() []string
}

// NewBroadcastHandler creates a new broadcast handler.
func NewBroadcastHandler(bus *MessageBus, sender string, teamFunc func() []string) *BroadcastHandler {
	return &BroadcastHandler{bus: bus, sender: sender, teamFunc: teamFunc}
}

// Execute implements Handler.
func (h *BroadcastHandler) Execute(input map[string]any) (string, error) {
	content, _ := input["content"].(string)
	if content == "" {
		return "", fmt.Errorf("content is required")
	}
	teammates := h.teamFunc()
	return h.bus.Broadcast(h.sender, content, teammates), nil
}