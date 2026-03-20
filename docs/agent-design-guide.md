# AI Agent 系统渐进式设计文档

> 本文档按功能模块划分，从最基础的 Agent 循环开始，逐步添加高级功能。

---

## 目录

1. [Phase 1: 核心 Agent 循环](#phase-1-核心-agent-循环)
2. [Phase 2: LLM 客户端](#phase-2-llm-客户端)
3. [Phase 3: 基础工具系统](#phase-3-基础工具系统)
4. [Phase 4: 子代理系统](#phase-4-子代理系统)
5. [Phase 5: 上下文压缩](#phase-5-上下文压缩)
6. [Phase 6: 任务管理系统](#phase-6-任务管理系统)
7. [Phase 7: 后台任务执行](#phase-7-后台任务执行)
8. [Phase 8: 团队管理系统](#phase-8-团队管理系统)
9. [Phase 9: 协议系统](#phase-9-协议系统)
10. [Phase 10: Worktree 隔离](#phase-10-worktree-隔离)
11. [Phase 11: 自主代理](#phase-11-自主代理)
12. [入口程序：cmd/agent/main.go](#入口程序cmdagentmaingo)

---

## Phase 1: 核心 Agent 循环

**目标**: 实现最基础的 Agent 循环模式

**核心模式**:

```
while stop_reason == "tool_use":
    call LLM
    execute tools
    append results
```

### 1.1 核心类型定义

**文件**: `agent/types.go`

```go
// Message 表示对话中的一条消息
type Message struct {
    Role    string      `json:"role"`    // "user", "assistant", "tool"
    Content interface{} `json:"content"` // string 或 []ContentBlock
}

// Tool 表示 Agent 可用的工具
type Tool struct {
    Name        string      `json:"name"`
    Description string      `json:"description"`
    InputSchema InputSchema `json:"input_schema"`
}

// Response 表示 LLM 的响应
type Response struct {
    Content    []ContentBlock `json:"content"`
    StopReason string         `json:"stop_reason"` // "end_turn", "tool_use"
}

// LLMClient 定义 LLM 客户端接口
type LLMClient interface {
    CreateMessage(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error)
}

// ToolExecutor 定义工具执行器接口
type ToolExecutor interface {
    Execute(name string, input map[string]interface{}) (string, error)
}
```

### 1.2 Agent 结构

**文件**: `agent/types.go`

```go
type Agent struct {
    Client    LLMClient      // LLM 客户端
    Executor  ToolExecutor   // 工具执行器
    System    string         // 系统提示词
    Tools     []Tool         // 可用工具列表
    MaxTokens int            // 最大输出 token
}
```

### 1.3 基础循环实现

**文件**: `agent/loop.go`

```go
func (a *Agent) Run(ctx context.Context, messages *[]Message) error {
    for {
        // 1. 调用 LLM
        response, err := a.Client.CreateMessage(ctx, a.System, *messages, a.Tools)
        if err != nil {
            return fmt.Errorf("LLM call failed: %w", err)
        }

        // 2. 追加 assistant 消息
        *messages = append(*messages, Message{
            Role:    "assistant",
            Content: response.Content,
        })

        // 3. 如果没有工具调用，结束循环
        if response.StopReason != "tool_use" {
            return nil
        }

        // 4. 执行工具调用，收集结果
        var results []ToolResultContent
        for _, block := range response.Content {
            if block.Type == "tool_use" {
                output, err := a.Executor.Execute(block.Name, block.Input)
                if err != nil {
                    results = append(results, ToolResultContent{
                        Type:      "tool_result",
                        ToolUseID: block.ID,
                        Content:   err.Error(),
                        IsError:   true,
                    })
                    continue
                }
                results = append(results, ToolResultContent{
                    Type:      "tool_result",
                    ToolUseID: block.ID,
                    Content:   output,
                })
            }
        }

        // 5. 追加工具结果作为 user 消息
        *messages = append(*messages, Message{
            Role:    "user",
            Content: results,
        })
    }
}
```

**验收标准**:

- [ ] Agent 能与 LLM 进行基本对话
- [ ] Agent 能调用工具并返回结果
- [ ] 循环在 `stop_reason != "tool_use"` 时终止

---

## Phase 2: LLM 客户端

**目标**: 实现 OpenAI 兼容 API 客户端

### 2.1 客户端结构

**文件**: `agent/client.go`

```go
type OpenAIClient struct {
    APIKey     string
    BaseURL    string
    Model      string
    HTTPClient *http.Client
}

// 从环境变量创建客户端
func NewOpenAIClientFromEnv() *OpenAIClient {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    baseURL := os.Getenv("ANTHROPIC_BASE_URL")  // 可选
    model := os.Getenv("MODEL_ID")              // 可选
    // ...
}
```

### 2.2 消息格式转换

OpenAI 格式与内部格式的差异:

- System 消息作为独立消息，而非参数
- Tool results 使用 `role: "tool"` 而非 `role: "user"`
- Tool calls 使用 `tool_calls` 数组

**文件**: `agent/client.go`

```go
func (c *OpenAIClient) CreateMessage(ctx context.Context, system string, messages []Message, tools []Tool) (*Response, error) {
    // 1. 构建请求
    openAIMessages := make([]openAIMessage, 0, len(messages)+1)

    // 添加 system 消息
    openAIMessages = append(openAIMessages, openAIMessage{
        Role:    "system",
        Content: system,
    })

    // 2. 转换消息格式
    for _, msg := range messages {
        switch v := msg.Content.(type) {
        case string:
            // 简单文本消息
        case []ToolResultContent:
            // 工具结果 -> role: "tool"
        case []ContentBlock:
            // Assistant 消息，可能包含 tool_calls
        }
    }

    // 3. 发送请求并解析响应
    // ...
}
```

**验收标准**:

- [ ] 支持 OpenAI 兼容 API
- [ ] 正确处理流式响应（可选）
- [ ] 错误处理和重试

---

## Phase 3: 基础工具系统

**目标**: 实现可扩展的工具注册和执行系统

### 3.1 工具接口

**文件**: `agent/tools/types.go`

```go
// Handler 工具执行接口
type Handler interface {
    Execute(input map[string]interface{}) (string, error)
}

// HandlerFunc 函数适配器
type HandlerFunc func(input map[string]interface{}) (string, error)

// Definition 工具定义
type Definition struct {
    Name        string
    Description string
    InputSchema InputSchema
}
```

### 3.2 工具注册表

**文件**: `agent/tools/registry.go`

```go
type Registry struct {
    handlers    map[string]Handler
    definitions map[string]Definition
}

func (r *Registry) Register(name string, def Definition, handler Handler) {
    r.definitions[name] = def
    r.handlers[name] = handler
}

func (r *Registry) Execute(name string, input map[string]interface{}) (string, error) {
    handler, ok := r.handlers[name]
    if !ok {
        return "", fmt.Errorf("unknown tool: %s", name)
    }
    return handler.Execute(input)
}

// 适配为 ToolExecutor 接口
func (r *Registry) AsExecutor() *RegistryExecutor {
    return &RegistryExecutor{Registry: r}
}
```

### 3.3 基础工具实现

**文件**: `agent/tools/bash.go`, `agent/tools/read.go`, `agent/tools/write.go`, `agent/tools/edit.go`

每个工具遵循相同模式:

```go
// 定义
func BashDefinition() Definition {
    return Definition{
        Name:        "bash",
        Description: "Execute a bash command",
        InputSchema: InputSchema{
            Type: "object",
            Properties: map[string]Property{
                "command": {Type: "string", Description: "The command to execute"},
            },
            Required: []string{"command"},
        },
    }
}

// 处理器
type BashHandler struct {
    workDir string
}

func NewBashHandler(workDir string) *BashHandler {
    return &BashHandler{workDir: workDir}
}

func (h *BashHandler) Execute(input map[string]interface{}) (string, error) {
    command, _ := input["command"].(string)
    // 执行命令...
}
```

**验收标准**:

- [ ] 实现 bash, read_file, write_file, edit_file 工具
- [ ] 工具在指定工作目录下执行
- [ ] 输出截断（避免上下文溢出）

---

## Phase 4: 子代理系统

**目标**: 实现隔离上下文的子代理

**关键洞察**: 子代理共享文件系统但不共享对话历史，最后只返回摘要。

### 4.1 子代理配置

**文件**: `agent/subagent.go`

```go
type SubagentConfig struct {
    System   string // 子代理系统提示词
    MaxRounds int   // 最大轮次（默认 30）
}

func (a *Agent) RunSubagent(ctx context.Context, prompt string, childTools []Tool) (string, error) {
    // 1. 创建子代理（共享 Client 和 Executor）
    childAgent := &Agent{
        Client:    a.Client,
        Executor:  a.Executor,
        System:    "You are a coding subagent. Complete the given task, then summarize your findings.",
        Tools:     childTools,  // 排除 task 工具防止递归
        MaxTokens: a.MaxTokens,
    }

    // 2. 新的消息上下文
    messages := []Message{
        {Role: "user", Content: prompt},
    }

    // 3. 运行子代理循环
    for i := 0; i < maxRounds; i++ {
        response, err := childAgent.Client.CreateMessage(ctx, childAgent.System, messages, childAgent.Tools)
        // ... 执行工具 ...

        if response.StopReason != "tool_use" {
            break
        }
    }

    // 4. 提取最终文本摘要
    return extractSummary(lastResponse), nil
}
```

### 4.2 工具过滤

**文件**: `agent/tools/registry.go`

```go
// 子代理可用工具（排除 task 防止递归）
var ChildToolNames = []string{"bash", "read_file", "write_file", "edit_file", "todo"}

func (r *Registry) GetChildToolDefinitions() []Definition {
    var defs []Definition
    for _, name := range ChildToolNames {
        if def, ok := r.definitions[name]; ok {
            defs = append(defs, def)
        }
    }
    return defs
}
```

**验收标准**:

- [ ] 子代理使用独立的上下文
- [ ] 子代理无法调用 task 工具
- [ ] 返回结果摘要而非完整历史

---

## Phase 5: 上下文压缩

**目标**: 让 Agent 能够无限运行

**三层压缩流水线**:

```
每个 turn:
    [Layer 1: micro_compact]     静默执行，压缩旧的 tool_result
           |
           v
    [检查: tokens > threshold?]
           |               |
          no              yes
           |               |
           v               v
       continue    [Layer 2: auto_compact]
                         保存完整转录到 .transcripts/
                         让 LLM 生成摘要
                         用摘要替换所有消息
                               |
                               v
                       [Layer 3: compact tool]
                         手动触发压缩
```

### 5.1 Micro Compact (Layer 1)

**文件**: `agent/compact.go`

```go
// 每轮静默执行，替换旧的 tool_result 为占位符
func MicroCompact(messages []Message, keepRecent int) []Message {
    // 保留最近 N 个 tool_result
    // 其他的替换为 "[Previous: used {tool_name}]"
}
```

### 5.2 Auto Compact (Layer 2)

**文件**: `agent/compact.go`

```go
func (c *Compactor) AutoCompact(ctx context.Context, messages []Message) ([]Message, error) {
    // 1. 保存完整转录到磁盘
    transcriptPath := ".transcripts/transcript_{timestamp}.jsonl"

    // 2. 让 LLM 生成摘要
    summaryPrompt := "Summarize this conversation for continuity..."

    // 3. 用摘要替换所有消息
    return []Message{
        {Role: "user", Content: "[Conversation compressed. Transcript: ...]\n\n{summary}"},
        {Role: "assistant", Content: "Understood. I have the context from the summary."},
    }, nil
}
```

### 5.3 整合到循环中

**文件**: `agent/loop.go`

```go
func (a *Agent) RunWithNagAndCompact(ctx context.Context, messages *[]Message, nag *NagConfig, compactConfig *CompactConfig) error {
    for {
        // Layer 1: micro_compact
        if compactConfig != nil {
            *messages = MicroCompact(*messages, compactConfig.KeepRecent)
        }

        // Layer 2: auto_compact
        if compactor != nil && compactor.ShouldAutoCompact(*messages) {
            newMsgs, _ := compactor.AutoCompact(ctx, *messages)
            *messages = newMsgs
        }

        // ... LLM call ...

        // Layer 3: manual compact (tool triggered)
    }
}
```

**验收标准**:

- [ ] token 估算准确（~4 chars/token）
- [ ] 转储的转录文件可恢复上下文
- [ ] 摘要保留关键决策和状态

---

## Phase 6: 任务管理系统

**目标**: 实现持久化的任务跟踪

**关键洞察**: 任务存储在 `.tasks/` 目录，不受上下文压缩影响。

### 6.1 任务模型

**文件**: `agent/tools/tasks.go`

```go
type TaskStatus string

const (
    TaskPending    TaskStatus = "pending"
    TaskInProgress TaskStatus = "in_progress"
    TaskCompleted  TaskStatus = "completed"
)

type Task struct {
    ID          int        `json:"id"`
    Subject     string     `json:"subject"`
    Description string     `json:"description,omitempty"`
    Status      TaskStatus `json:"status"`
    BlockedBy   []int      `json:"blockedBy,omitempty"` // 依赖的任务
    Blocks      []int      `json:"blocks,omitempty"`    // 被阻塞的任务
    Owner       string     `json:"owner,omitempty"`     // 分配给谁
    Worktree    string     `json:"worktree,omitempty"`  // 关联的 worktree
}
```

### 6.2 任务管理器

**文件**: `agent/tools/tasks.go`

```go
type TaskManager struct {
    mu     sync.RWMutex
    dir    string  // .tasks/
    nextID int
}

// CRUD 操作
func (m *TaskManager) Create(subject, description string) (*Task, error)
func (m *TaskManager) Get(id int) (*Task, error)
func (m *TaskManager) Update(id int, opts UpdateOptions) (*Task, error)
func (m *TaskManager) Delete(id int) error
func (m *TaskManager) List() ([]*Task, error)

// 自动领取
func (m *TaskManager) ScanUnclaimed() ([]*Task, error)
func (m *TaskManager) Claim(id int, owner string) (*Task, error)
```

### 6.3 任务工具

```go
task_create  // 创建任务
task_update  // 更新状态/依赖
task_list    // 列出所有任务
task_get     // 获取任务详情
task_delete  // 删除任务
```

**验收标准**:

- [ ] 任务持久化到 JSON 文件
- [ ] 依赖关系正确维护
- [ ] 完成的任务自动从其他任务的 blockedBy 中移除

---

## Phase 7: 后台任务执行

**目标**: 实现异步命令执行和结果通知

### 7.1 后台管理器

**文件**: `agent/tools/background.go`

```go
type BackgroundTask struct {
    ID      string  // 随机生成
    Command string
    Status  string  // "running", "completed", "timeout", "error"
    Result  string
}

type BackgroundManager struct {
    tasks             map[string]*BackgroundTask
    notificationQueue []Notification  // 完成通知队列
}

// 启动后台任务
func (m *BackgroundManager) Run(command string) string {
    taskID := generateTaskID()
    go m.execute(taskID, command)  // 异步执行
    return fmt.Sprintf("Background task %s started: %s", taskID, command)
}

// 检查任务状态
func (m *BackgroundManager) Check(taskID string) string

// 获取并清空通知队列
func (m *BackgroundManager) DrainNotifications() string
```

### 7.2 整合到 Agent 循环

**文件**: `agent/loop.go`

```go
func (a *Agent) Run(...) {
    for {
        // 在 LLM 调用前注入后台通知
        if a.BackgroundManager != nil {
            notifText := a.BackgroundManager.DrainNotifications()
            if notifText != "" {
                *messages = append(*messages, Message{
                    Role: "user",
                    Content: fmt.Sprintf("<background-results>\n%s\n</background-results>", notifText),
                })
            }
        }
        // ...
    }
}
```

**验收标准**:

- [ ] 后台命令异步执行
- [ ] 超时处理（默认 300s）
- [ ] 通知在下一轮循环中注入

---

## Phase 8: 团队管理系统

**目标**: 实现多 Agent 协作

### 8.1 消息总线

**文件**: `agent/tools/team.go`

```go
type TeamMessage struct {
    Type      string  `json:"type"`      // "message", "broadcast", "shutdown_request", ...
    From      string  `json:"from"`
    Content   string  `json:"content"`
    Timestamp float64 `json:"timestamp"`
}

type MessageBus struct {
    inboxDir string  // .team/inbox/
}

// 发送消息到收件箱
func (b *MessageBus) Send(sender, to, content, msgType string) string {
    // 追加到 {to}.jsonl
}

// 读取并清空收件箱
func (b *MessageBus) ReadInbox(name string) []TeamMessage
```

### 8.2 队友管理器

**文件**: `agent/tools/team.go`

```go
type TeamMember struct {
    Name   string `json:"name"`
    Role   string `json:"role"`
    Status string `json:"status"`  // "idle", "working", "shutdown"
}

type TeammateManager struct {
    config      TeamConfig
    bus         *MessageBus
    teammateRun func(name, role, prompt string) error  // 运行函数
}

// 生成队友
func (tm *TeammateManager) Spawn(name, role, prompt string) string {
    // 更新配置
    // 在后台运行
    go tm.teammateRun(name, role, prompt)
}
```

### 8.3 队友工具

```go
spawn_teammate  // 生成队友
list_teammates  // 列出队友
send_message    // 发送消息
read_inbox      // 读取收件箱
broadcast       // 广播消息
```

**验收标准**:

- [ ] 队友在独立 goroutine 中运行
- [ ] 消息通过 JSONL 文件传递
- [ ] 状态持久化到 config.json

---

## Phase 9: 协议系统

**目标**: 实现结构化的请求-响应协议

### 9.1 请求追踪器

**文件**: `agent/tools/protocols.go`

```go
type RequestStatus string

const (
    StatusPending   RequestStatus = "pending"
    StatusApproved  RequestStatus = "approved"
    StatusRejected  RequestStatus = "rejected"
)

type RequestTracker struct {
    shutdownRequests map[string]*ShutdownRequest
    planRequests     map[string]*PlanRequest
}

func generateRequestID() string  // 生成唯一 ID
```

### 9.2 关机协议

```
Lead                              Teammate
+---------------------+          +---------------------+
| shutdown_request     | -------> | receives request    |
| {request_id: abc}    |          | decides: approve?   |
+---------------------+          +---------------------+
                                      |
+---------------------+          +-------v-------------+
| check_status         | <------- | shutdown_response   |
| {status: approved}   |          | {request_id: abc,   |
+---------------------+          |  approve: true}     |
                                 +---------------------+
```

### 9.3 计划审批协议

```
Teammate                          Lead
+---------------------+          +---------------------+
| plan_approval_submit| -------> | reviews plan text   |
| {plan: "..."}        |          | approve/reject?     |
+---------------------+          +---------------------+
                                      |
+---------------------+          +-------v-------------+
| receives response    | <------- | plan_approval_review|
| {approve: true}      |          | {request_id, approve}|
+---------------------+          +---------------------+
```

**验收标准**:

- [ ] request_id 正确关联请求和响应
- [ ] 状态机正确转换
- [ ] 消息通过 MessageBus 传递

---

## Phase 10: Worktree 隔离

**目标**: 实现目录级别的任务隔离

**关键洞察**: 任务是控制平面，worktree 是执行平面。

### 10.1 Worktree 结构

**文件**: `agent/tools/worktree.go`

```go
type WorktreeEntry struct {
    Name      string         `json:"name"`
    Path      string         `json:"path"`
    Branch    string         `json:"branch"`     // wt/{name}
    TaskID    *int           `json:"task_id,omitempty"`
    Status    WorktreeStatus `json:"status"`     // "active", "removed", "kept"
    CreatedAt float64        `json:"created_at"`
}

type WorktreeManager struct {
    repoRoot string
    dir      string  // .worktrees/
    tasks    *TaskManager
    events   *EventBus
}
```

### 10.2 生命周期事件

```go
type WorktreeEvent struct {
    Event     string  `json:"event"`     // "worktree.create.before", ...
    Timestamp float64 `json:"ts"`
    Task      map[string]any
    Worktree  map[string]any
    Error     string
}

type EventBus struct {
    path string  // .worktrees/events.jsonl
}
```

### 10.3 Worktree 工具

```go
worktree_create  // 创建 worktree，可选绑定任务
worktree_list    // 列出所有 worktree
worktree_status  // 查看 git 状态
worktree_run     // 在 worktree 中执行命令
worktree_remove  // 删除 worktree，可选完成任务
worktree_keep    // 保留 worktree
worktree_events  // 查看生命周期事件
task_bind_worktree  // 绑定任务到 worktree
```

**验收标准**:

- [ ] Git worktree 正确创建和删除
- [ ] 任务与 worktree 正确绑定
- [ ] 生命周期事件记录到 events.jsonl

---

## Phase 11: 自主代理

**目标**: 实现能自己找工作的 Agent

### 11.1 生命周期

```
+-------+
| spawn |
+---+---+
    |
    v
+-------+  tool_use    +-------+
| WORK  | <----------> |  LLM  |
+---+---+              +-------+
    |
    | idle tool
    v
+--------+  poll every 5s
| IDLE   | -------------> check inbox
+---+----+ -------------> scan .tasks/ for unclaimed
    |                    -------------> timeout (60s) -> shutdown
    +---> message? -> resume WORK
    +---> unclaimed? -> claim -> resume WORK
```

### 11.2 自主配置

**文件**: `agent/tools/autonomous.go`

```go
type AutonomousConfig struct {
    PollInterval time.Duration  // 轮询间隔（默认 5s）
    IdleTimeout  time.Duration  // 空闲超时（默认 60s）
}
```

### 11.3 Teammate Runner

**文件**: `agent/teammate.go`

```go
type TeammateRunner struct {
    Client   LLMClient
    WorkDir  string
    Bus      *MessageBus
    Manager  *TeammateManager
    Registry *Registry
    Tracker  *RequestTracker
    TaskMgr  *TaskManager
    Config   *AutonomousConfig
}

func (r *TeammateRunner) Run(name, role, prompt string) error {
    // 1. 构建队友专属工具
    registry := r.buildTeammateTools(name)

    // 2. 创建 Agent
    ag := New(r.Client, registry.AsExecutor(), sysPrompt, tools)

    // 3. 运行自主循环
    r.runAutonomous(ctx, ag, name, role, teamName, &messages)
}

func (r *TeammateRunner) runWorkPhase(...) (bool, error) {
    // 返回 true 表示请求 idle
}

func (r *TeammateRunner) runIdlePhase(...) (bool, error) {
    // 返回 true 表示恢复工作
}
```

### 11.4 自动领取任务

**文件**: `agent/tools/autonomous.go`

```go
func TryAutoClaim(manager *TaskManager, owner string) (*AutoClaimResult, error) {
    unclaimed, _ := manager.ScanUnclaimed()
    if len(unclaimed) == 0 {
        return &AutoClaimResult{Claimed: false}, nil
    }
    // 领取第一个无主任务
    task, _ := manager.Claim(unclaimed[0].ID, owner)
    return &AutoClaimResult{Claimed: true, Task: task}, nil
}
```

### 11.5 身份注入

上下文压缩后需要重新注入身份:

```go
func MakeIdentityBlock(name, role, teamName string) map[string]any {
    return map[string]any{
        "role": "user",
        "content": fmt.Sprintf("<identity>You are '%s', role: %s, team: %s.</identity>", name, role, teamName),
    }
}
```

**验收标准**:

- [ ] Agent 在 idle 时轮询检查新任务
- [ ] 自动领取无主任务
- [ ] 空闲超时后自动关闭
- [ ] 响应关机请求

---

## 完整架构图

```
                         +------------------+
                         |   cmd/agent      |
                         |   (main.go)      |
                         +--------+---------+
                                  |
              +-------------------+-------------------+
              |                   |                   |
              v                   v                   v
     +--------+-------+  +--------+-------+  +--------+-------+
     |    Agent       |  |    Client      |  |   Registry     |
     |  (loop.go)     |  |  (client.go)   |  | (registry.go)  |
     +--------+-------+  +--------+-------+  +--------+-------+
              |                   |                   |
              |                   |                   |
    +---------+---------+         |         +---------+---------+
    |         |         |         |         |         |         |
    v         v         v         v         v         v         v
+-------+ +-------+ +-------+ +-------+ +-------+ +-------+ +-------+
|compact| |nag    | |inbox  | |OpenAI | |bash   | |read   | |write  |
|       | |       | |       | | API   | |       | |       | |       |
+-------+ +-------+ +-------+ +-------+ +-------+ +-------+ +-------+
                                          |         |         |
                                          v         v         v
                                    +-------+ +-------+ +-------+
                                    |task   | |team   | |worktree|
                                    |       | |       | |       |
                                    +-------+ +-------+ +-------+
```

---

## 快速开始

```go
// 1. 创建客户端
client := agent.NewOpenAIClientFromEnv()

// 2. 创建工具注册表
registry := tools.NewRegistryWithWorkDir(workDir)
registry.Register("bash", tools.BashDefinition(), tools.NewBashHandler(workDir))
registry.Register("read_file", tools.ReadDefinition(), tools.NewReadHandler(workDir))
// ...

// 3. 创建 Agent
ag := agent.New(client, registry.AsExecutor(), systemPrompt, agent.ToTools(registry.Tools()))

// 4. 运行
messages := []agent.Message{{Role: "user", Content: "Hello!"}}
ag.Run(context.Background(), &messages)
```

---

## 入口程序：cmd/agent/main.go

**目标**: 整合所有组件，提供交互式 REPL 入口

### 入口程序结构

**文件**: `cmd/agent/main.go`

入口程序负责初始化所有组件并将它们连接在一起。

### 1. 初始化阶段

```go
func main() {
    // 1. 加载 .env 文件
    loadEnv(".env")

    // 2. 获取工作目录和技能目录
    workDir, _ := os.Getwd()
    skillsDir := os.Getenv("SKILLS_DIR")
    if skillsDir == "" {
        skillsDir = "skills"
    }

    // 3. 创建完整组件注册表（包含所有工具和自主队友）
    registry, _, skillLoader, bgManager, bus, teamManager, tracker, taskManager :=
        tools.DefaultRegistryWithAutonomousTeammates(workDir, skillsDir)

    // 4. 获取子代理可用工具（排除 task 防止递归）
    childToolDefs := registry.GetChildToolDefinitions()

    // 5. 创建 LLM 客户端
    client := agent.NewOpenAIClientFromEnv()

    // 6. 构建系统提示词（包含技能描述）
    system := buildSystemPrompt(workDir, skillLoader)

    // 7. 创建 Agent
    ag := agent.New(client, registry.AsExecutor(), system, nil)
}
```

### 2. 组件连接

```go
// 设置后台管理器（用于通知提取）
ag.SetBackgroundManager(bgManager)

// 设置收件箱检查器（用于团队通信）
ag.SetInboxChecker(bus, "lead")

// 设置自主队友运行器
teamManager.SetTeammateRun(func(name, role, prompt string) error {
    runner := agent.NewAutonomousTeammateRunner(client, workDir, bus, teamManager, registry, tracker, taskManager)
    return runner.Run(name, role, prompt)
})

// 注册子代理 task 工具
subagentHandler := tools.NewTaskHandler(func(ctx context.Context, prompt string) (string, error) {
    return ag.RunSubagent(ctx, prompt, agent.ToTools(childToolDefs))
})
registry.Register("subagent", tools.TaskDefinition(), subagentHandler)

// 设置 Agent 工具列表（包含所有工具）
agentTools := agent.ToTools(registry.Tools())
ag.Tools = agentTools
```

### 3. 配置项

```go
// Nag 提醒配置（用于 todo 工具）
nagConfig := &agent.NagConfig{
    ToolName:  "todo",
    Threshold: 3,
    Message:   "<reminder>Update your todos.</reminder>",
}

// 上下文压缩配置
compactConfig := &agent.CompactConfig{
    Threshold:  50000,
    KeepRecent: 3,
    TranscriptDir: ".transcripts",
    WorkDir:   workDir,
}
```

### 4. 交互式 REPL 循环

```go
history := []agent.Message{}
scanner := bufio.NewScanner(os.Stdin)

fmt.Println("Agent CLI (type 'q' or 'exit' to quit)")
fmt.Println("  /team  - list teammates")
fmt.Println("  /inbox - check lead inbox")
fmt.Println("  /tasks - list tasks")
if skillLoader.HasSkills() {
    fmt.Printf("Skills loaded: %s\n", strings.Join(skillLoader.SkillNames(), ", "))
}

for {
    fmt.Print("\033[36m>> \033[0m")
    if !scanner.Scan() {
        break
    }

    query := scanner.Text()
    if query == "" || query == "q" || query == "exit" {
        continue // 空输入或退出
    }

    // 处理特殊命令
    if query == "/team" {
        fmt.Println(teamManager.ListAll())
        continue
    }
    if query == "/inbox" {
        fmt.Println(bus.ReadInboxJSON("lead"))
        continue
    }
    if query == "/tasks" {
        fmt.Println(taskManager.Render())
        continue
    }

    // 添加用户消息到历史
    history = append(history, agent.Message{
        Role:    "user",
        Content: query,
    })

    // 运行 Agent（带 Nag 提醒和上下文压缩）
    if err := ag.RunWithNagAndCompact(context.Background(), &history, nagConfig, compactConfig); err != nil {
        fmt.Printf("\033[31mError: %v\033[0m\n", err)
        continue
    }

    // 打印助手响应
    if len(history) > 0 {
        lastMsg := history[len(history)-1]
        if content, ok := lastMsg.Content.([]agent.ContentBlock); ok {
            for _, block := range content {
                if block.Type == "text" {
                    fmt.Println(block.Text)
                }
            }
        }
    }
}
```

### 5. 系统提示词构建

```go
func buildSystemPrompt(workDir string, skillLoader *tools.SkillLoader) string {
    var sb strings.Builder

    // 基础角色定义
    sb.WriteString(fmt.Sprintf("You are a team lead at %s. Teammates are autonomous -- they find work themselves. ", workDir))

    // 工具使用说明
    sb.WriteString("Use the todo tool to plan multi-step tasks. ")
    sb.WriteString("Use task_create/task_update/task_list to track persistent tasks with dependencies. ")
    sb.WriteString("Use background_run for long-running commands (fire and forget). Use check_background to get results. ")
    sb.WriteString("Use spawn_teammate to spawn autonomous teammates that run in parallel. Use send_message and read_inbox to communicate. ")
    sb.WriteString("Use shutdown_request to request a teammate to shut down gracefully. Use check_shutdown_status to track the request. ")
    sb.WriteString("Use list_pending_plans to see pending plan approval requests. Use plan_approval_review to approve or reject plans. ")
    sb.WriteString("Use the subagent tool to delegate exploration or subtasks. ")
    sb.WriteString("Prefer tools over prose. ")

    // 可选技能说明
    if skillLoader.HasSkills() {
        sb.WriteString("\n\nSkills available:\n")
        sb.WriteString(skillLoader.GetDescriptions())
        sb.WriteString("\n\nUse load_skill to access specialized knowledge before tackling unfamiliar topics.")
    }

    return sb.String()
}
```

### 6. .env 文件加载

```go
func loadEnv(filename string) {
    data, err := os.ReadFile(filename)
    if err != nil {
        return
    }

    lines := strings.Split(string(data), "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        parts := strings.SplitN(line, "=", 2)
        if len(parts) == 2 {
            key := strings.TrimSpace(parts[0])
            value := strings.TrimSpace(parts[1])
            os.Setenv(key, value)  // 覆盖已有环境变量
        }
    }
}
```

**验收标准**:

- [ ] 所有组件正确初始化并连接
- [ ] REPL 支持特殊命令（/team, /inbox, /tasks）
- [ ] Nag 提醒和上下文压缩正常工作
- [ ] 自主队友可以生成和运行
- [ ] 子代理工具正确注册

---

## 完整目录结构

```
agent/
├── types.go        # 核心类型定义
├── adapter.go      # 工具适配器
├── client.go       # OpenAI 客户端
├── loop.go         # Agent 循环
├── subagent.go     # 子代理
├── compact.go      # 上下文压缩
├── teammate.go     # 自主队友运行器
└── tools/
    ├── types.go        # 工具接口
    ├── registry.go     # 工具注册表
    ├── bash.go         # Bash 工具
    ├── read.go         # 文件读取
    ├── write.go        # 文件写入
    ├── edit.go         # 文件编辑
    ├── todo.go         # Todo 工具
    ├── skill.go        # 技能加载
    ├── task.go         # 子代理任务
    ├── tasks.go        # 任务管理
    ├── background.go   # 后台任务
    ├── team.go         # 团队管理
    ├── protocols.go    # 协议系统
    ├── autonomous.go   # 自主代理
    └── worktree.go     # Worktree 管理
cmd/
└── agent/
    └── main.go         # 入口程序（REPL + 组件初始化）
```
