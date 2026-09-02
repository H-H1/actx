# ACTX Agent 系统设计方案（基于 codex-go 实际代码）

> 本文基于 `d:\0Rust\codex\codex-go` 实际源码梳理，覆盖 10 个工程维度。
> 对未实现的能力明确标注「未实现」并给出可落地的补充方向，不编造不存在的机制。
> 所有代码引用均可点击跳转到具体文件与行号。

---

## 一、身份与目标（System Prompt）

### 角色定义

ACTX 是一个运行在终端 CLI 环境中的**编码代理**，通过阅读/编写代码、运行 shell 命令、应用补丁来完成软件工程任务。

### System Prompt 原文

系统提示词定义为 Go 常量，注入到每个模型请求中。

**英文版 `BaseInstructions`**（[internal/config/defaults.go#L116-L140](file:///d:/0Rust/codex/codex-go/internal/config/defaults.go#L116-L140)）：

```text
You are ACTX, a coding agent running in a terminal-based CLI environment.
You help the user accomplish software engineering tasks by reading and
writing code, running shell commands, and applying patches.

# Environment
You operate inside a sandboxed shell with the following characteristics:
- File system access is governed by the active sandbox policy
  (read-only, workspace-write, or full-access).
- Network access may be restricted depending on the sandbox policy.
- Commands run in the project working directory unless an explicit path
  is given.

# Operating principles
1. Understand before acting. Read relevant files and gather context
   before proposing changes.
2. Make minimal, focused changes. Prefer surgical edits over large
   rewrites.
3. Verify your work. Run tests, linters, or type-checkers when
   applicable.
4. Ask for approval when the sandbox policy or approval policy requires
   it. Never bypass approval gates.
5. Never fabricate file contents or command outputs. If you are unsure,
   read the file or run the command.
6. Follow the repository's existing conventions, style guides, and
   AGENTS.md instructions when present.

# Tool usage
- Use the shell tool to run commands. Prefer non-interactive commands.
- Use the apply_patch tool to modify files. Describe each change
  clearly.
- Use web search only when local information is insufficient and the
  feature is enabled.

When you complete a task, summarize the changes you made and any
follow-up steps the user may want to take.
```

**中文版 `BaseInstructionsZh`**（[internal/config/defaults.go#L142-L398](file:///d:/0Rust/codex/codex-go/internal/config/defaults.go#L142-L398)）补充了「个性/响应性/前置消息/规划/任务执行/验证你的工作」等更细化的行为规范，要求简洁直接、避免冗长解释、不主动提交 git、不添加版权头等。

### 角色 / 任务 / 边界 / 风格对照

| 维度 | Prompt 中的约束 | 代码位置 |
|------|----------------|----------|
| 你是谁（角色） | "You are ACTX, a coding agent" | [defaults.go#L116](file:///d:/0Rust/codex/codex-go/internal/config/defaults.go#L116) |
| 你要做什么（任务） | "help the user accomplish software engineering tasks" | [defaults.go#L116](file:///d:/0Rust/codex/codex-go/internal/config/defaults.go#L116) |
| 你不能做什么（边界） | "Never fabricate file contents or command outputs" / "Never bypass approval gates" | [defaults.go#L130-L131](file:///d:/0Rust/codex/codex-go/internal/config/defaults.go#L130-L131) |
| 你该怎么回答（风格） | 简洁、直接、分点、不冗长 | [defaults.go#L156-L158](file:///d:/0Rust/codex/codex-go/internal/config/defaults.go#L156-L158) |

### 加载与注入路径

| 步骤 | 实现位置 |
|------|----------|
| Config 字段 `BaseInstructions` | [config.go#L252-L254](file:///d:/0Rust/codex/codex-go/internal/config/config.go#L252-L254) |
| 用户附加指令 `UserInstructions`（追加在 BaseInstructions 之后） | [config.go#L256-L259](file:///d:/0Rust/codex/codex-go/internal/config/config.go#L256-L259) |
| Session 构造时把 BaseInstructions 作为第一项 system message | [act.go#L111](file:///d:/0Rust/codex/codex-go/internal/agent/act.go#L111) 与 [context.go#L27-L29](file:///d:/0Rust/codex/codex-go/internal/agent/context.go#L27-L29) |
| runTurn 拼装 instructions = Base + User (+ 工具描述) | [session.go#L159-L167](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L159-L167) |

---

## 二、工具与能力（Tools）

### 工具注册表

| 工具名 | 功能 | 是否幂等 | 调用方式 | 代码位置 |
|--------|------|---------|---------|----------|
| `shell` | 执行 shell 命令，支持 `timeout_ms` 参数 | 否 | 由 ApprovalPolicy 决定 | [session.go#L443-L467](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L443-L467) |
| `apply_patch` | 用 ACTX apply_patch 格式修改文件 | 否 | 默认需用户审批 | [session.go#L469-L479](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L469-L479) |
| `web_search` | 联网搜索（特性开关启用时） | 是 | 自动 | act.go Spawn 中注册 |
| MCP 工具 | 通过 MCP 协议接入外部工具 | 视具体工具 | 视配置 | [config.go#L188-L205](file:///d:/0Rust/codex/codex-go/internal/config/config.go#L188-L205) |

### 核心数据结构

```go
// internal/tools/registry.go
type ToolRegistry struct {
    mu        sync.RWMutex
    executors map[protocol.ToolName]ToolExecutor
    order     []protocol.ToolName   // 保证向模型暴露的顺序确定
}

type ToolExecutor interface {
    Name() protocol.ToolName
    Spec() model.ToolSpec
    Handle(ctx context.Context, inv ToolInvocation) (ToolOutput, error)
}

type ToolRouter struct{ registry *ToolRegistry }
func (rt *ToolRouter) Dispatch(ctx, inv) (ToolOutput, error)
```

### 双模式调用策略

| 模式 | 触发条件 | 实现 |
|------|---------|------|
| API 模式 | 默认；模型支持 function calling | `apiTools = specs`，作为 `tools` 参数发往 API |
| 提示词模式 | `config.ToolsMode == "prompt"` 或自动降级 | 调用 [BuildToolPrompt(specs)](file:///d:/0Rust/codex/codex-go/internal/agent/prompt_tools.go#L60) 把工具描述注入 system prompt，让模型以 ` ```tool ` 代码块输出调用 |

代码位置：[session.go#L161-L167](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L161-L167)。

### 审批策略（调用前确认）

`ApprovalPolicy` 四档（[config.go#L19-L35](file:///d:/0Rust/codex/codex-go/internal/config/config.go#L19-L35)）：

| 策略 | 行为 |
|------|------|
| `never` | 从不要求用户审批 |
| `on-failure` | 沙箱内自动执行，沙箱失败才升级审批 |
| `on-request` | 由模型决定何时请求审批（默认） |
| `on-untrusted` | 仅"已知安全"只读命令自动通过，其他都要审批 |

执行链路：`executeToolCall` → 构建 `ToolInvocation`（含 `ApprovalCb`）→ `ToolRouter.Dispatch` → 工具内部 `AskForApproval` 判断 → 必要时通过 `s.requestApproval` 发 `EvExecApprovalRequest` 事件并阻塞等待 `approvalCh`（[session.go#L98-L119](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L98-L119)）。

### 超时控制

- shell 工具的 JSON Schema 暴露 `timeout_ms` 参数（[session.go#L458-L462](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L458-L462)），由模型按需设置。
- 每个 turn 通过 `context.WithCancel` 支持中断（[session.go#L127-L137](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L127-L137)）。

---

## 三、记忆与上下文（Memory）

### 短期记忆（会话窗口）

`ContextManager`（[context.go#L14-L147](file:///d:/0Rust/codex/codex-go/internal/agent/context.go#L14-L147)）在内存中维护 `[]protocol.ResponseItem`，第一项是 system message，后续按时间序追加 user/assistant/function_call/function_call_output/reasoning 等。

```go
type ContextManager struct {
    mu      sync.RWMutex
    items   []protocol.ResponseItem
    version int   // 每次 Compact/Rollback/Clear 自增
}
```

**窗口压缩 `Compact(summary, keepRecent)`**（[context.go#L82-L110](file:///d:/0Rust/codex/codex-go/internal/agent/context.go#L82-L110)）：

1. 保留首条 system message
2. 追加一条 system message：`"Previous conversation summary:\n" + summary`
3. 保留尾部 `keepRecent` 条原始项
4. `version++`

> 注意：当前实现**按项数而非 token 数压缩**，没有显式 token 计数限制。`handleCompact` 在 [act.go#L260-L264](file:///d:/0Rust/codex/codex-go/internal/agent/act.go#L260-L264) 是 `TODO: implement compaction`，对外接口尚未接通。

### 上下文截断策略

| 优先级 | 保留内容 |
|--------|---------|
| 最高 | System Prompt（首项） |
| 高 | 最近 `keepRecent` 条对话（含本轮工具调用结果） |
| 中 | 由旧历史压缩成的摘要 system message |
| 最低（被丢弃） | 早于摘要的原始项 |

`Rollback(toIndex)`（[context.go#L115-L126](file:///d:/0Rust/codex/codex-go/internal/agent/context.go#L115-L126)）支持按索引回滚（对应 `OpThreadRollback`）。

### 长期记忆（跨会话）

`MemoryStore`（[memory.go#L18-L103](file:///d:/0Rust/codex/codex-go/internal/agent/memory.go#L18-L103)）是**基于 JSON 文件的简单 key-value 存储**，落盘到 `~/.ACTX/memory.json`：

```go
type MemoryEntry struct {
    Key     string `json:"key"`
    Value   string `json:"value"`
    Session string `json:"session,omitempty"`
}
```

提供 `Set/Get/Delete/All/Load/Save` 接口，文件不存在时静默返回 nil（[memory.go#L41-L47](file:///d:/0Rust/codex/codex-go/internal/agent/memory.go#L41-L47)）。

### 未实现

- **向量数据库 / RAG**：未实现。`MemoryStore` 没有 embedding、没有相似度检索。
- **基于 token 的窗口硬上限**：未实现，仅按项数 `keepRecent`。
- **自动摘要触发**：`handleCompact` 是 TODO，目前需手动调用。

---

## 四、安全与合规（Safety）

### 沙箱隔离

`SandboxType`（[sandbox/types.go#L31-L57](file:///d:/0Rust/codex/codex-go/internal/sandbox/types.go#L31-L57)）按平台选择：

| SandboxType | 平台 | 机制 |
|-------------|------|------|
| `none` | 全部 | 不沙箱，命令直接执行 |
| `macos-seatbelt` | macOS | `sandbox-exec(1)` |
| `linux-seccomp` | Linux | `actx-linux-sandbox` 辅助程序 |
| `windows-restricted-token` | Windows | 受限令牌 |

`SandboxPolicy`（[config.go#L98-L122](file:///d:/0Rust/codex/codex-go/internal/config/config.go#L98-L122)）四档：

| Policy | 文件系统 | 网络 |
|--------|---------|------|
| `read-only` | 只读 | 禁用 |
| `workspace-write` | 工作区根 + WritableRoots 可写 | 默认禁用 |
| `danger-full-access` | 全部可写 | 不限 |
| `none` | 未配置哨兵值，构建时替换为默认 |

### 审批门禁

如前节所述，`requestApproval`（[session.go#L98-L119](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L98-L119)）发出 `EvExecApprovalRequest` 后**阻塞**等待 `approvalCh`，未获批准的命令不会执行。这是 ACTX 的"刹车"。

### 未实现

| 能力 | 状态 |
|------|------|
| PII 脱敏 | 未实现 |
| 敏感词拦截 | 未实现 |
| SQL/命令注入检测 | 未实现 |
| 输出审计 | 未实现 |
| 速率限制（QPS/日调用量） | 未实现 |
| 高风险操作分类（删除/发送/扣费） | 仅靠 `ApprovalPolicy` 统一管控，未按操作类型分级 |

可补充方向：在 `ToolRouter.Dispatch` 前增加 `SecurityChecker` 接口，对 `ToolInvocation.Arguments` 做正则/规则匹配，命中危险模式则强制升级到 `ApprovalOnUntrusted`。

---

## 五、成本控制（Cost）

### 已实现：Token 用量统计

```go
// internal/model/client.go
type Usage struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
}
```

SSE 流末尾 `response.completed` 事件携带 `chunk.Usage`，被转成 `EventCompleted` 中的 `Usage`，再由 session 发出 `EvTokenCount` 事件（[session.go#L329-L333](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L329-L333)）：

```go
s.sendEvent(protocol.TokenCountEvent(turnID,
    ev.Usage.InputTokens, ev.Usage.OutputTokens))
```

TUI 在底部状态行渲染 `Tokens - Input: X, Output: Y`。

### 未实现

| 能力 | 状态 | 影响 |
|------|------|------|
| 单会话 Token 硬上限 | 未实现 | 长会话可能撞上下文上限才被模型 API 报错 |
| 单用户/单租户日封顶 | 未实现 | 多租户场景无法控成本 |
| 模型成本阈值（超预算自动降级） | 未实现 | 预算爆了无法自动切便宜模型 |
| 费用告警联动 | 未实现 | 异常消耗无告警 |

可补充方向：在 `ContextManager` 加 `tokenEstimate` 字段，每次 `Append` 累计；超过阈值时发 `EvContextCompacted` 触发自动 `Compact`。在 `ACTX` 层加日累计计数器，达 80% 切 `Models[1]`。

---

## 六、可观测性（Observability）

### 事件系统（24 种 EventType）

[protocol/types.go#L219-L245](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L219-L245) 定义了完整事件枚举：

```
EvSessionConfigured      EvTurnStarted          EvTurnComplete
EvTurnAborted            EvAgentMessage         EvAgentMessageDelta
EvAgentReasoning         EvReasoningDelta       EvUserMessage
EvExecCommandBegin       EvExecCommandOutputDelta EvExecCommandEnd
EvPatchApplyBegin        EvPatchApplyUpdated    EvPatchApplyEnd
EvExecApprovalRequest    EvPatchApprovalRequest EvTokenCount
EvContextCompacted       EvStreamError          EvShutdownComplete
EvMcpToolCallBegin       EvMcpToolCallEnd       EvMcpStartupUpdate
EvMcpStartupComplete
```

每个事件携带 `TurnID`/`CallID`/`Command`/`Output`/`Delta`/`InputTokens` 等结构化字段（[types.go#L175-L213](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L175-L213)）。

### 操作系统（15 种 OpType）

[types.go#L119-L135](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L119-L135) 定义用户可提交的全部操作：`user_input / interrupt / shutdown / exec_approval / patch_approval / compact / refresh_mcp_servers / reload_user_config / thread_rollback / clean_background_terminals / run_user_shell_command / resolve_elicitation / user_input_answer / request_permissions_response / dynamic_tool_response`。

### 链路 ID

| 维度 | ID | 生成 |
|------|----|------|
| 会话 | `SessionId` (UUID) | [types.go#L47](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L47) |
| 线程 | `ThreadId` (UUID) | [types.go#L53](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L53) |
| 轮次 | `TurnId` (UUID) | [types.go#L59](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L59) |
| 工具调用 | `CallId` (`call_<uuid>`) | [types.go#L65-L67](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L65-L67) |
| 提交 | `SubmissionId` (UUID) | [types.go#L41](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go#L41) |

所有事件携带 `SubmissionId`（关联到触发它的提交）和 `TurnID`，可拼成 `session→thread→turn→call` 四级链路。

### 日志

`Session.logger` 是 `*slog.Logger`（[session.go#L38](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L38)），在 `runTurn`/`executeToolCall`/`handleSubmission` 等关键路径打 `Info/Error` 日志。

### 未实现

| 能力 | 状态 |
|------|------|
| 全链路 Trace（OpenTelemetry） | 未实现，仅靠事件链 + slog |
| 错误快照（异常时自动保存上下文） | 未实现 |
| 关键指标聚合（成功率/P99/失败率） | 未实现 |
| SLA 监控与告警 | 未实现 |
| rollout 会话持久化 | 接口存在（`OpThreadRollback`）但持久化未完整接通 |

可补充方向：在 `sendEvent` 增加钩子，把每条 `Event` 写入 JSONL 文件 `~/.ACTX/trace/<session>.jsonl`，配合 `slog` 的 JSON handler 即可获得离线分析能力。

---

## 七、降级与容错（Fallback & Fault Tolerance）

### 工具模式自动降级

`shouldFallbackToPrompt`（[prompt_tools.go#L35-L50](file:///d:/0Rust/codex/codex-go/internal/agent/prompt_tools.go#L35-L50)）检测模型 API 返回的 400 错误，关键词命中即降级：

```go
var fallbackKeywords = []string{
    "tools", "function", "reasoning_content",
    "reasoning_content is not supported",
    "tool_calls", "function_calling",
    "not support", "unsupported parameter",
}
```

`runTurn` 中通过 `toolsModeSwitched` 标志保证**单次会话只降级一次**，避免反复试探（[session.go#L182-L191](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L182-L191)）：

```go
if s.config.ToolsMode == "api" && !s.toolsModeSwitched &&
   shouldFallbackToPrompt(err) {
    s.toolsModeSwitched = true
    s.config.ToolsMode = "prompt"
    s.sendEvent(... "[系统] 模型不支持函数调用，已自动切换到提示词工具模式")
    continue   // 重试本轮
}
```

### 错误分类

[tools/errors.go](file:///d:/0Rust/codex/codex-go/internal/tools/errors.go) 把工具错误分两类：

| ErrorKind | 行为 | 构造函数 |
|-----------|------|---------|
| `ErrorKindRespondToModel` | 把错误作为 `function_call_output` 喂回模型，让模型自己调整并可能重试 | `NewRespondToModelError` |
| `ErrorKindFatal` | 中止当前 turn | `NewFatalError` |

这是 ACTX 的"软重试"机制——不是代码层自动重试，而是**把错误反馈给模型让模型决策**。未知工具名直接 `NewFatalError`（[registry.go#L112](file:///d:/0Rust/codex/codex-go/internal/tools/registry.go#L112)）。

### 超时与取消

- `runTurn` 用 `context.WithCancel` 创建 turn ctx，`cancelTurn` 可中断当前轮次（[session.go#L77-L84](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L77-L84)）。
- shell 工具的 `timeout_ms` 参数透传给底层执行器。
- `requestApproval` 在 `s.ctx.Done()` 时返回 false（[session.go#L113-L118](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L113-L118)）。

### 未实现

| 能力 | 状态 |
|------|------|
| 模型降级链（GPT-4 → Claude → GPT-3.5） | 未实现；`Models` 字段是列表但仅在启动时取首项 |
| 工具调用自动重试（指数退避） | 未实现；靠 `ErrorKindRespondToModel` 让模型重试 |
| 熔断机制（连续失败禁用 N 分钟） | 未实现 |
| 整体超时（>60s 终止会话） | 未实现 |

可补充方向：在 `ModelClient.Stream` 外包一层 `ModelRouter`，按 `Models []string` 顺序尝试，前者失败（网络/5xx）切下一个；在 `ToolRouter` 加 `failureCount` map，达阈值返回 `ErrorKindFatal` 并发 `EvMcpStartupUpdate` 风格的禁用通知。

---

## 八、调度与并发（Concurrency & Throttling）

### 核心并发模型：SQ/EQ 双通道 + 后台 goroutine

`ACTX` 是顶层代理（[act.go#L25-L32](file:///d:/0Rust/codex/codex-go/internal/agent/act.go#L25-L32)）：

```go
type ACTX struct {
    txSub    chan protocol.Submission  // 提交队列 SQ，容量 64
    rxEvent  chan protocol.Event       // 事件队列 EQ，容量 256
    session  *Session
    cancel   context.CancelFunc
    wg       sync.WaitGroup
    planMode bool
}
```

`Spawn` 启动后台 goroutine `submissionLoop`（[act.go#L184-L199](file:///d:/0Rust/codex/codex-go/internal/agent/act.go#L184-L199)）：

```go
func (c *ACTX) submissionLoop(ctx context.Context) {
    defer c.wg.Done()
    defer close(c.rxEvent)
    for {
        select {
        case <-ctx.Done():
            return
        case sub, ok := <-c.txSub:
            if !ok { return }
            c.handleSubmission(ctx, sub)
        }
    }
}
```

`Submit(op)` 把 Op 发到 `txSub`（满则阻塞），`NextEvent()` 从 `rxEvent` 读结果。这是**单 Session 串行提交循环**——同一 Session 同一时刻只处理一个 Op，避免上下文竞争。

### 背压与流控

| 通道 | 容量 | 满时行为 |
|------|------|---------|
| `txSub` | 64 | `Submit` 阻塞调用方 |
| `rxEvent` | 256 | `sendEvent` 阻塞发送方（或 ctx 取消） |
| `approvalCh` | 1 | 审批响应投递阻塞 |

`sendEvent` 用 `select` 同时监听 `s.rxEvent <-` 和 `s.ctx.Done()`（[session.go#L62-L67](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L62-L67)），保证关闭时不死锁。

### 单 turn 内的采样循环

`runTurn` 内部 `for { ... }` 循环（[session.go#L148-L207](file:///d:/0Rust/codex/codex-go/internal/agent/session.go#L148-L207)）：每次 `client.Stream` → `processStream` → 若 `needsFollowUp`（有工具调用）则继续，否则返回。这是**串行的 ReAct 循环**，不并行执行工具。

### 未实现

| 能力 | 状态 |
|------|------|
| 多 Session 全局并发上限 | 未实现（单进程通常单 Session） |
| 单租户限流 | 未实现 |
| 请求优先级队列（VIP/普通/免费） | 未实现 |
| 负载感知路由（主模型延迟高切备用） | 未实现 |

可补充方向：在 `Spawn` 外加 `AgentPool`，用 `semaphore.NewWeighted(N)` 控制全局并发；`txSub` 改为优先级堆。但 ACTX 是 CLI 工具，单用户单会话场景下当前模型已足够。

---

## 九、缓存与复用（Caching）

### 配置分层合并（最成熟的"缓存"）

`Config` 字段注释（[config.go#L211-L215](file:///d:/0Rust/codex/codex-go/internal/config/config.go#L211-L215)）说明合并层级：

1. 代码内置默认值（`defaults.go` 的 `DefaultApprovalPolicy`、`DefaultSandboxPolicy`、`BaseInstructions`）
2. `~/.ACTX/config.toml`（用户主配置）
3. 项目目录 `.actx/config.json`（项目级覆盖，含 `model_providers`、`models` 列表、`base_url`）
4. 用户缓存（如 token 缓存）
5. CLI 标志覆盖（`--model`、`--approval-policy`、`--sandbox-policy` 等）

后层覆盖前层，最终拼出 `*Config` 传给 `Spawn`。

### 技能系统（Prompt 模板复用）

[skills/skills.go](file:///d:/0Rust/codex/codex-go/internal/skills/skills.go) 从 `~/.ACTX/skills/<name>/SKILL.md` 加载技能，YAML frontmatter 定义 `name`+`description`。用户在对话中提及 `$技能名` 时，对应 SKILL.md 全文被注入 system prompt，避免每次重复合成。

### 未实现

| 能力 | 状态 |
|------|------|
| 语义缓存（向量相似度匹配历史问答） | 未实现 |
| 工具结果缓存（天气/汇率 TTL 复用） | 未实现 |
| Prompt 模板 ID 化（预编译减少 token） | 未实现；技能系统是文件级注入，不是 ID 化传输 |
| MemoryStore 作缓存 | 不算；它是跨会话持久记忆，无 TTL/失效策略 |

可补充方向：在 `ToolRouter.Dispatch` 前加 `CacheChecker`，对幂等工具（`web_search`）按 `hash(Arguments)` 缓存结果，TTL 5 分钟。

---

## 十、异步与长时间任务（Async & Long-running Tasks）

### SSE 流式输出（核心异步能力）

`ModelClient.Stream`（[client.go](file:///d:/0Rust/codex/codex-go/internal/model/client.go)）开 goroutine 解析 SSE，通过 `events <- ResponseEvent` 通道推送，包含：

- `EventOutputTextDelta`：assistant 文本增量
- `EventReasoningDelta`：DeepSeek-R1/o1 思考增量（已补全，见 [client.go chatDelta.ReasoningContent](file:///d:/0Rust/codex/codex-go/internal/model/client.go)）
- `EventOutputItemDone`：完整 item（message/function_call/reasoning）

`processStream` 把这些转成 `EvAgentMessageDelta` / `EvReasoningDelta` / `EvExecCommandOutputDelta` 事件，TUI/App Server 实时渲染，**用户无需等待整轮完成就能看到进展**。

### 多执行模式

| 模式 | 入口 | 异步特征 |
|------|------|---------|
| TUI 交互 | `internal/tui` + `ACTX.Events()` channel | 实时流式渲染 agent_message_delta / reasoning_delta / exec_command_output_delta |
| `actx exec [prompt]` 非交互 | [cli/exec_helper.go#L15](file:///d:/0Rust/codex/codex-go/internal/cli/exec_helper.go#L15) | 单轮执行后输出，支持 `--json` 输出 JSONL 事件流 |
| App Server JSON-RPC | `internal/appserver` | 长连接，通过 `item/*` 通知推送增量 |

### SubAgent（异步子代理）

[agent/subagent.go](file:///d:/0Rust/codex/codex-go/internal/agent/subagent.go) 提供"独立运行单轮任务的轻量代理"：

```go
type SubAgent struct {
    id      string
    session *Session
    cancel  context.CancelFunc
}

func SpawnSubAgent(args SubAgentArgs) (*SubAgent, error)
func (sa *SubAgent) Run(parentCtx context.Context)   // 阻塞至单轮完成
func (sa *SubAgent) Cancel()                          // 取消
```

SubAgent 拥有独立 ContextManager 和独立 SessionID，可并发跑多个不互相干扰。

### 任务状态查询的替代方案

ACTX 没有 `task_id` 概念，但有等价能力：

- `TurnId`（UUID）唯一标识一轮任务
- 通过 `Events()` channel 实时收到 `EvTurnStarted` → 各类 delta → `EvTurnComplete`/`EvTurnAborted`
- App Server 把 `turnID` 暴露给客户端，客户端可查询 `thread/state` 拿到当前 `activeTurn` 与历史 items

### 未实现

| 能力 | 状态 |
|------|------|
| 任务队列（Celery/RabbitMQ） | 未实现；用 channel + goroutine 替代 |
| 进度百分比上报 | 未实现；只有事件流式增量 |
| Webhook 回调 | 未实现 |
| 长任务硬上限（>10min 强制 TIMEOUT） | 未实现 |
| 结果保留 7 天自动清理 | 未实现 |

可补充方向：在 `appserver` 加 `POST /task` 接收 prompt → 返回 `task_id` → 内部 `go SpawnSubAgent` → 完成后写入 `~/.ACTX/tasks/<id>.json`，再提供 `GET /task/<id>` 查询。这对 CLI 单用户场景不是必需。

---

## 附：代码对应位置速查表

| 工程维度 | 关键文件 | 关键符号 |
|---------|---------|---------|
| 身份与目标 | [defaults.go](file:///d:/0Rust/codex/codex-go/internal/config/defaults.go) | `BaseInstructions` / `BaseInstructionsZh` |
|  | [config.go](file:///d:/0Rust/codex/codex-go/internal/config/config.go) | `Config.BaseInstructions` / `UserInstructions` |
|  | [act.go](file:///d:/0Rust/codex/codex-go/internal/agent/act.go) | `Spawn` 中初始化 `ContextManager` |
| 工具与能力 | [tools/registry.go](file:///d:/0Rust/codex/codex-go/internal/tools/registry.go) | `ToolRegistry` / `ToolRouter.Dispatch` |
|  | [agent/session.go](file:///d:/0Rust/codex/codex-go/internal/agent/session.go) | `ShellToolSpec` / `ApplyPatchToolSpec` / `executeToolCall` |
|  | [agent/prompt_tools.go](file:///d:/0Rust/codex/codex-go/internal/agent/prompt_tools.go) | `BuildToolPrompt` / `shouldFallbackToPrompt` |
|  | [config/config.go](file:///d:/0Rust/codex/codex-go/internal/config/config.go) | `ApprovalPolicy` / `McpServerConfig` |
| 记忆与上下文 | [agent/context.go](file:///d:/0Rust/codex/codex-go/internal/agent/context.go) | `ContextManager` / `Compact` / `Rollback` |
|  | [agent/memory.go](file:///d:/0Rust/codex/codex-go/internal/agent/memory.go) | `MemoryStore` / `MemoryEntry` |
| 安全与合规 | [sandbox/types.go](file:///d:/0Rust/codex/codex-go/internal/sandbox/types.go) | `SandboxType` |
|  | [config/config.go](file:///d:/0Rust/codex/codex-go/internal/config/config.go) | `SandboxPolicy` |
|  | [agent/session.go](file:///d:/0Rust/codex/codex-go/internal/agent/session.go) | `requestApproval` / `EvExecApprovalRequest` |
| 成本控制 | [model/client.go](file:///d:/0Rust/codex/codex-go/internal/model/client.go) | `Usage` / `EventCompleted` |
|  | [protocol/types.go](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go) | `EvTokenCount` / `TokenCountEvent` |
| 可观测性 | [protocol/types.go](file:///d:/0Rust/codex/codex-go/internal/protocol/types.go) | 24 种 `EventType` / 15 种 `OpType` |
|  | [agent/act.go](file:///d:/0Rust/codex/codex-go/internal/agent/act.go) | `handleSubmission` 分发 |
| 降级与容错 | [agent/prompt_tools.go](file:///d:/0Rust/codex/codex-go/internal/agent/prompt_tools.go) | `fallbackKeywords` / `shouldFallbackToPrompt` |
|  | [tools/errors.go](file:///d:/0Rust/codex/codex-go/internal/tools/errors.go) | `ErrorKind` / `FunctionCallError` |
| 调度与并发 | [agent/act.go](file:///d:/0Rust/codex/codex-go/internal/agent/act.go) | `ACTX` / `submissionLoop` / `txSub` / `rxEvent` |
|  | [agent/session.go](file:///d:/0Rust/codex/codex-go/internal/agent/session.go) | `runTurn` 采样循环 |
| 缓存与复用 | [config/config.go](file:///d:/0Rust/codex/codex-go/internal/config/config.go) | `Config` 多层合并注释 |
|  | [skills/skills.go](file:///d:/0Rust/codex/codex-go/internal/skills/skills.go) | `Load` / `Skill` |
| 异步与长任务 | [model/client.go](file:///d:/0Rust/codex/codex-go/internal/model/client.go) | `Stream` SSE goroutine |
|  | [agent/subagent.go](file:///d:/0Rust/codex/codex-go/internal/agent/subagent.go) | `SubAgent` / `SpawnSubAgent` |
|  | [cli/exec_helper.go](file:///d:/0Rust/codex/codex-go/internal/cli/exec_helper.go) | `runExecWithConfig` |
|  | [appserver/bridge.go](file:///d:/0Rust/codex/codex-go/internal/appserver/bridge.go) | `bridgeEvent` 转 JSON-RPC 通知 |

---

## 总结

ACTX（codex-go）是一个**单进程、单会话、串行提交循环**的终端编码代理，定位是个人开发者工具而非多租户 SaaS。其工程化重点落在：

1. **身份清晰**：System Prompt 明确角色/边界/风格，中英双版
2. **工具完备**：3 个内置工具 + MCP 外接，双模式（API/提示词）兼容各家模型
3. **记忆分层**：内存 ContextManager + JSON MemoryStore，Compact 支持摘要压缩
4. **安全闸门**：沙箱 + 审批策略双重管控，未审批命令不执行
5. **可观测**：24 种结构化事件 + 链路 ID + slog 日志
6. **降级**：API→提示词模式自动切换 + 错误分类反馈模型重试

对多租户/高并发/成本治理/语义缓存等"企业级"能力，**当前实现未覆盖**，但代码结构（`ACTX`/`Session`/`ToolRouter`/`ContextManager`）留有扩展点，按文中"可补充方向"即可逐步补齐。
