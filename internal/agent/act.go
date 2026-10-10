// Package agent implements the core ACTX agent loop. It provides the ACTX
// 包 agent 实现了核心 ACTX 代理循环。它提供 ACTX 结构体作为提交操作和接收事件的主要接口，
// struct as the main interface for submitting operations and receiving events,
// 由后台提交循环驱动代理的轮次周期。
// backed by a background submission loop that drives the agent turn cycle.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// ACTX is the high-level interface to the agent. Callers submit operations
// ACTX 是代理的高级接口。调用者通过 Submit 提交操作并通过 NextEvent 消费结果。
// via Submit and consume results via NextEvent. A background goroutine
// 后台 goroutine（提交循环）处理操作并发出事件。
// (the submission loop) processes operations and emits events.
type ACTX struct {
	txSub    chan protocol.Submission
	rxEvent  chan protocol.Event
	session  *Session
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	planMode bool
}

// SetPlanMode enables or disables plan mode. In plan mode, the agent
// only proposes actions without executing them.
// SetPlanMode 启用或禁用计划模式。在计划模式下，代理只提出行动建议而不执行。
func (c *ACTX) SetPlanMode(enabled bool) {
	c.planMode = enabled
	if c.session != nil {
		c.session.SetPlanMode(enabled)
	}
}

// IsPlanMode returns whether plan mode is currently active.
// IsPlanMode 返回计划模式是否当前处于活动状态。
func (c *ACTX) IsPlanMode() bool {
	return c.planMode
}

// ACTXArgs holds the parameters needed to spawn a ACTX instance.
// ACTXArgs 保存启动一个 ACTX 实例所需的参数。
type ACTXArgs struct {
	Config      *config.Config
	APIKey      string
	BaseURL     string
	ModelClient *model.ModelClient
	Registry    *tools.ToolRegistry
	Logger      *slog.Logger
}

// Spawn creates a new ACTX instance and starts the background submission loop.
// Spawn 创建一个新的 ACTX 实例并启动后台提交循环。
// The caller is responsible for providing a configured ModelClient and a
// 调用者负责提供已配置的 ModelClient 以及已注册所有所需工具的 ToolRegistry。
// ToolRegistry with all desired tools registered.
func Spawn(args ACTXArgs) (*ACTX, error) {
	// Config is mandatory: it carries model name, policies, instructions,
	// cwd, writable roots, and the tools mode.
	// Config 是必需的：它携带模型名、策略、系统提示词、工作目录、
	// 可写根目录以及工具模式。
	if args.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	// If no pre-built ModelClient was supplied, construct one from the
	// API key; an API key is then mandatory.
	// 如果没有提供预构建的 ModelClient，则用 API 密钥构建一个；
	// 此时 API 密钥是必需的。
	if args.ModelClient == nil {
		if args.APIKey == "" {
			return nil, fmt.Errorf("either ModelClient or APIKey must be provided")
		}
		// Build an OpenAI-compatible streaming client bound to the
		// configured model and endpoint. The provider's api_style selects
		// the Chat Completions or Responses wire format.
		// 构建绑定到所配置模型与端点的 OpenAI 兼容流式客户端。
		// 提供商的 api_style 选择 Chat Completions 或 Responses 传输格式。
		args.ModelClient = model.NewModelClient(args.APIKey, args.Config.Model, args.BaseURL)
		args.ModelClient.SetAPIStyle(args.Config.APIStyle)
	}
	// Fall back to an empty registry if none was provided.
	// 如果未提供注册表，则回退到空注册表。
	if args.Registry == nil {
		args.Registry = tools.NewToolRegistry()
	}
	// Register built-in tools if registry is empty.
	// 如果注册表为空，注册内置工具。
	if len(args.Registry.List()) == 0 {
		// shell: execute commands under sandbox/approval constraints.
		// shell：在沙箱/审批约束下执行命令。
		args.Registry.Register(NewShellTool())
		// apply_patch: apply structured file edits.
		// apply_patch：应用结构化的文件补丁修改。
		args.Registry.Register(NewApplyPatchTool())
		// web_search: search the web when the feature is enabled.
		// web_search：特性启用时进行联网搜索。
		args.Registry.Register(NewWebSearchTool())
	}
	// Default to the standard structured logger.
	// 默认使用标准结构化日志器。
	if args.Logger == nil {
		args.Logger = slog.Default()
	}

	// Root context: cancelling it stops the submission loop and releases
	// every blocked channel operation inside the agent (event sends,
	// approval waits).
	// 根 context：取消它会停止提交循环，并解除代理内部所有阻塞的
	// 通道操作（事件发送、审批等待）。
	ctx, cancel := context.WithCancel(context.Background())

	// ACTX is the public facade: two channels plus a cancel handle.
	// txSub (cap 64) receives submissions; rxEvent (cap 256) emits events.
	// ACTX 是对外门面：两条通道加一个取消句柄。
	// txSub（容量 64）接收提交；rxEvent（容量 256）发出事件。
	ACTX := &ACTX{
		txSub:   make(chan protocol.Submission, 64),
		rxEvent: make(chan protocol.Event, 256),
		cancel:  cancel,
	}

	// Session holds the per-conversation runtime state. txSub and rxEvent
	// are SHARED with ACTX so both sides operate the same pipes.
	// Session 保存单条会话的运行时状态。txSub 与 rxEvent 与 ACTX
	// 共享，双方操作同一条管道。
	session := &Session{
		// Fresh unique identifiers for this session and its thread.
		// 为本会话及其线程生成新的唯一标识。
		id:       protocol.NewSessionId(),
		threadID: protocol.NewThreadId(),
		// Runtime configuration snapshot (policies, model, instructions).
		// 运行时配置快照（策略、模型、系统提示词）。
		config: args.Config,
		// Model API client used by runTurn for SSE streaming.
		// runTurn 用于 SSE 流式请求的模型 API 客户端。
		client: args.ModelClient,
		// Tool registry whose Specs() are exposed to the model.
		// 工具注册表，其 Specs() 会暴露给模型。
		registry: args.Registry,
		// Router that dispatches tool calls to executors.
		// 把工具调用分发给执行器的路由器。
		router: tools.NewToolRouter(args.Registry),
		// Structured logger for session-level diagnostics.
		// 会话级诊断用的结构化日志器。
		logger: args.Logger,
		// Session lifetime context (same root the facade cancels).
		// 会话生命周期 context（与门面取消的是同一个根）。
		ctx: ctx,
		// Shared channels wired into the ACTX facade.
		// 与 ACTX 门面接通的共享通道。
		txSub:   ACTX.txSub,
		rxEvent: ACTX.rxEvent,
		// Buffered approval response pipe (cap 1): requestApproval blocks
		// reading it until handleApproval writes a verdict.
		// 带缓冲的审批应答管道（容量 1）：requestApproval 阻塞读它，
		// 直到 handleApproval 写入裁决结果。
		approvalCh: make(chan approvalResponse, 1),
	}
	// ContextManager stores the conversation history; its first item is
	// the system message built from BaseInstructions.
	// ContextManager 存储对话历史；其首项是由 BaseInstructions
	// 构建的 system 消息。
	session.context = NewContextManager(args.Config.BaseInstructions)
	// Per-thread model I/O log under .actx/logs/<threadID>.log: one file per
	// conversation (window/task). Every submission of the thread appends to
	// the same file, with its own Turn banner inside; nil (and silently
	// skipped) if the file cannot be created.
	// 按线程划分的模型输入输出日志，位于 .actx/logs/<threadID>.log：
	// 每段对话（窗口/任务）一个文件。线程内的每次提交都追加到同一文件，
	// 各自带 Turn 横幅；文件无法创建时为 nil（写入被静默跳过）。
	session.turnLog = NewTurnLogger(session.threadID)
	// Log the exact wire request body of every model call into the thread
	// log (see TurnLogger.LogRequestBody).
	// 把每次模型调用的确切线上请求体记入线程日志（见 TurnLogger.LogRequestBody）。
	args.ModelClient.SetRequestTrace(session.turnLog.LogRequestBody)
	// Link the session into the facade.
	// 把 session 挂到门面上。
	ACTX.session = session

	// Emit session configured event so consumers learn the active model
	// and provider before the first turn starts.
	// 发出会话配置完成事件，让消费方在首轮开始前得知当前生效的
	// 模型与服务商。
	session.sendEvent(protocol.SessionConfiguredEvent(args.Config.Model, args.Config.ModelProvider))

	// Start the submission loop.
	// 启动提交循环。
	ACTX.wg.Add(1)
	go ACTX.submissionLoop(ctx)

	return ACTX, nil
}

// Submit sends an operation to the agent. It returns the submission ID that
// Submit 向代理发送一个操作。它返回将附加到结果事件中的提交 ID。
// will be attached to resulting events. This method blocks if the submission
// 如果提交队列已满，此方法会阻塞。
// queue is full.
func (c *ACTX) Submit(op protocol.Op) protocol.SubmissionId {
	// Wrap the op in a Submission carrying a fresh unique ID.
	// 把 Op 包装成带全新唯一 ID 的 Submission。
	sub := protocol.Submission{
		ID: protocol.NewSubmissionId(),
		Op: op,
	}
	// Enqueue; blocks if the 64-slot queue is full (backpressure).
	// 入队；若 64 格队列已满则阻塞（背压）。
	c.txSub <- sub
	// Return the ID so callers can correlate events with this submission.
	// 返回 ID，调用方可据此把事件与本次提交关联。
	return sub.ID
}

// NextEvent returns the next event from the agent. The ok value is false
// NextEvent 返回代理的下一个事件。当代理已关闭且不再产生更多事件时，
// when the agent has shut down and no more events will be produced.
func (c *ACTX) NextEvent() (protocol.Event, bool) {
	// Read one event; ok becomes false once the channel is closed and
	// drained, signalling agent shutdown.
	// 读取一条事件；通道关闭且读空后 ok 变为 false，
	// 表示代理已关闭。
	ev, ok := <-c.rxEvent
	return ev, ok
}

// Events returns the receive-only event channel for direct consumption.
// Events 返回只接收的事件通道，供直接消费使用。
func (c *ACTX) Events() <-chan protocol.Event {
	// Expose the receive side only; senders stay internal to the agent.
	// 仅暴露接收端；发送端保持代理内部私有。
	return c.rxEvent
}

// Shutdown gracefully stops the agent. It sends a shutdown operation and
// Shutdown 优雅地停止代理。它发送一个关闭操作并等待提交循环结束。
// waits for the submission loop to finish.
func (c *ACTX) Shutdown() {
	// Enqueue a shutdown op: the loop finishes any in-flight submission
	// before honouring it.
	// 入队一个关闭操作：循环会先完成正在处理的提交，再响应关闭。
	c.Submit(protocol.NewShutdownOp())
	// Block until submissionLoop returns and rxEvent has been closed.
	// 阻塞直到 submissionLoop 返回且 rxEvent 已被关闭。
	c.wg.Wait()
}

// SessionID returns the unique session identifier.
// SessionID 返回唯一的会话标识符。
func (c *ACTX) SessionID() protocol.SessionId {
	return c.session.id
}

// ThreadID returns the unique thread identifier.
// ThreadID 返回唯一的线程标识符。
func (c *ACTX) ThreadID() protocol.ThreadId {
	return c.session.threadID
}

// Session returns the underlying session for direct manipulation.
// Session 返回底层会话以供直接操作。
func (c *ACTX) Session() *Session {
	return c.session
}

// submissionLoop is the background goroutine that processes submissions.
// submissionLoop 是处理提交的后台 goroutine。
// It reads from the submission channel and dispatches each Op to the
// appropriate handler.
//
// Turns run in a background goroutine so the loop keeps consuming the
// submission queue while a turn is in flight. This is essential for
// approvals: a running turn blocks inside requestApproval waiting for the
// user's verdict, and the verdict arrives as a queued op — if the loop were
// blocked inside the turn, handleApproval would never run (deadlock).
// User-input and compact ops are queued while a turn is running so turns
// stay strictly serial and Session access stays single-threaded; interrupt /
// approval / shutdown ops are processed immediately (they are safe:
// turnCancel is mutex-guarded, approvalCh is a channel).
// 轮次在后台 goroutine 中运行，因此循环在轮次进行期间仍持续消费提交队列。
// 这对审批至关重要：运行中的轮次在 requestApproval 内阻塞等待用户裁决，
// 而裁决以排队 Op 的形式到达——如果循环阻塞在轮次内部，
// handleApproval 将永远不会执行（死锁）。用户输入与压缩 Op 在轮次运行
// 期间排队，保持轮次严格串行、Session 访问单线程；
// 中断/审批/关闭 Op 立即处理（它们是安全的：turnCancel 有互斥锁保护，
// approvalCh 是通道）。
func (c *ACTX) submissionLoop(ctx context.Context) {
	// Signal WaitGroup completion on exit so Shutdown's Wait unblocks.
	// 退出时通知 WaitGroup 完成，使 Shutdown 的 Wait 解除阻塞。
	defer c.wg.Done()
	// Close the event channel so NextEvent consumers can detect shutdown
	// (ok=false) and stop polling.
	// 关闭事件通道，让 NextEvent 消费方能感知关闭（ok=false）
	defer close(c.rxEvent)
	// Flush and close the per-session model I/O log on loop exit.
	// 循环退出时刷新并关闭按会话划分的模型输入输出日志。
	defer c.session.turnLog.Close()

	// turnDone (cap 1) receives a signal when the running turn finishes.
	// turnDone（容量 1）在运行中的轮次结束时收到信号。
	turnDone := make(chan struct{}, 1)
	// turnIdle reports whether no turn is currently running.
	// turnIdle 报告当前是否没有轮次在运行。
	turnIdle := true
	// queued holds user-input/compact ops deferred while a turn runs.
	// queued 保存轮次运行期间被推迟的用户输入/压缩 Op。
	var queued []protocol.Submission

	// startTurn launches one queued submission as a turn goroutine.
	// startTurn 把一个排队的提交作为轮次 goroutine 启动。
	startTurn := func(sub protocol.Submission) {
		turnIdle = false
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.handleSubmission(ctx, sub)
			// Signal the loop; the buffered channel guarantees this never
			// blocks even if the loop is momentarily busy.
			// 通知循环；缓冲通道保证即使循环暂时忙碌也不会阻塞。
			turnDone <- struct{}{}
		}()
	}

	for {
		// Launch the next queued turn when idle.
		// 空闲时启动下一个排队的轮次。
		if turnIdle && len(queued) > 0 {
			sub := queued[0]
			queued = queued[1:]
			startTurn(sub)
		}

		select {
		// Context cancelled (Shutdown handled or parent cancelled):
		// stop draining and exit immediately.
		// context 被取消（已处理 Shutdown 或父级取消）：
		// 停止取数并立即退出。
		case <-ctx.Done():
			return
		// The running turn finished; the loop head starts the next one.
		// 运行中的轮次结束；循环头部会启动下一个。
		case <-turnDone:
			turnIdle = true
		// Take the next submission from the queue.
		// 从队列取出下一个提交。
		case sub, ok := <-c.txSub:
			// Channel closed by producer: exit.
			// 通道被生产方关闭：退出。
			if !ok {
				return
			}
			switch sub.Op.Type {
			case protocol.OpUserInput, protocol.OpCompact:
				// Defer while a turn runs; dispatched immediately otherwise.
				// 轮次运行期间推迟；否则立即分发。
				if turnIdle {
					startTurn(sub)
				} else {
					queued = append(queued, sub)
				}
			case protocol.OpExecApproval, protocol.OpPatchApproval:
				// Safe during a turn: handleApproval writes to the buffered
				// approvalCh, unblocking requestApproval.
				// 轮次期间安全：handleApproval 写入带缓冲的 approvalCh，
				// 解除 requestApproval 的阻塞。
				c.handleSubmission(ctx, sub)
			case protocol.OpInterrupt:
				// Safe during a turn: cancelTurn is mutex-guarded.
				// 轮次期间安全：cancelTurn 有互斥锁保护。
				c.handleSubmission(ctx, sub)
			default:
				c.handleSubmission(ctx, sub)
			}
		}
	}
}

// handleSubmission dispatches a single submission to the appropriate handler
// handleSubmission 根据 Op 类型将单个提交分发给相应的处理器。
// based on the Op type.
func (c *ACTX) handleSubmission(ctx context.Context, sub protocol.Submission) {
	// Route each op to its dedicated handler.
	// 把每个 Op 路由到专用处理器。
	switch sub.Op.Type {
	case protocol.OpUserInput:
		// Start a new agent turn for the submitted messages.
		// 为提交的消息启动新的代理轮次。
		c.handleUserInput(ctx, sub)
	case protocol.OpInterrupt:
		// Abort the currently running turn.
		// 中止当前正在运行的轮次。
		c.handleInterrupt(ctx, sub)
	case protocol.OpShutdown:
		// Stop the agent and release all waiters.
		// 停止代理并解除所有等待者。
		c.handleShutdown(ctx, sub)
	case protocol.OpExecApproval:
		// Deliver an exec approval verdict to the blocked tool executor.
		// 把执行审批的裁决投递给阻塞中的工具执行器。
		c.session.handleApproval(sub.Op)
	case protocol.OpPatchApproval:
		// Deliver a patch approval verdict the same way.
		// 以同样方式投递补丁审批的裁决。
		c.session.handleApproval(sub.Op)
	case protocol.OpCompact:
		// Compact (summarize) the conversation context.
		// 压缩（摘要化）对话上下文。
		c.handleCompact(ctx, sub)
	default:
		// Unknown ops are logged and dropped, not fatal.
		// 未知 Op 记录日志后丢弃，不视为致命错误。
		c.session.logger.Warn("unknown op type", "type", sub.Op.Type)
	}
}

// handleUserInput processes a user input operation by starting a new turn.
// handleUserInput 通过启动新轮次来处理用户输入操作。
func (c *ACTX) handleUserInput(ctx context.Context, sub protocol.Submission) {
	// Allocate a fresh turn ID for this round of the sampling loop.
	// 为本轮采样循环分配新的轮次 ID。
	turnID := protocol.NewTurnId()
	// Mark the turn start in the model I/O log BEFORE the user items so
	// the banner groups the messages that triggered the turn. The banner
	// carries the submission ID alongside the turn ID so individual
	// submissions stay traceable inside the shared thread log file.
	// 在用户消息之前于模型输入输出日志中标记轮次开始，
	// 使横幅把触发本轮的消息归入同一组。横幅在轮次 ID 之外携带
	// 提交 ID，使共享的线程日志文件内每次提交仍可单独追溯。
	c.session.turnLog.LogTurnStart(turnID, sub.ID, c.session.config.Model)

	// Add user messages to context.
	// 将用户消息添加上下文。
	// They land in PendingUserMessages and are also echoed as
	// EvUserMessage events at the start of runTurn.
	// 它们进入 PendingUserMessages，并会在 runTurn 开始时
	// 以 EvUserMessage 事件回显。
	for _, item := range sub.Op.Items {
		c.session.context.Append(item)
		// Log user input here (not in LogRequest) so every context item
		// is written exactly once at append time; request headers then
		// never duplicate history.
		// 在此记录用户输入（而非 LogRequest），使每个上下文条目
		// 在追加时只记录一次；请求头不再重复历史。
		c.session.turnLog.LogItem(item)
	}

	// Announce turn start so the UI can flip its "running" state.
	// 宣告轮次开始，让 UI 切换到"运行中"状态。
	c.session.sendEvent(protocol.TurnStartedEvent(turnID))

	// Run the turn (sampling loop): stream from the model, execute tool
	// calls, repeat until a final assistant message. This call blocks
	// until the turn completes.
	// 运行轮次（采样循环）：从模型流式获取响应，执行工具调用，
	// 重复直到产出最终助手消息。此调用阻塞直到轮次完成。
	c.session.runTurn(ctx, turnID, sub.ID)

	// Announce turn completion so the UI resets its "running" state.
	// 宣告轮次完成，让 UI 复位"运行中"状态。
	c.session.sendEvent(protocol.TurnCompleteEvent(turnID))
}

// handleInterrupt aborts the current turn.
// handleInterrupt 中止当前轮次。
func (c *ACTX) handleInterrupt(ctx context.Context, sub protocol.Submission) {
	// Log the interrupt for observability.
	// 记录中断以便观测。
	c.session.logger.Info("interrupt received")
	// Cancel the current turn context if any. runTurn watches turnCtx
	// and aborts the in-flight model request / tool execution.
	// 如果有当前轮次上下文，则取消它。runTurn 监视 turnCtx，
	// 会中止进行中的模型请求/工具执行。
	c.session.cancelTurn()
}

// handleShutdown stops the agent.
// handleShutdown 停止代理。
func (c *ACTX) handleShutdown(ctx context.Context, sub protocol.Submission) {
	// Tell consumers shutdown finished; the TUI uses this to quit.
	// 通知消费方关闭完成；TUI 借此退出。
	c.session.sendEvent(protocol.ShutdownCompleteEvent())
	// Cancel the root context: submissionLoop exits its select loop and
	// closes rxEvent, releasing all blocked senders/readers.
	// 取消根 context：submissionLoop 退出其 select 循环并关闭
	// rxEvent，解除所有阻塞的发送方/读取方。
	c.cancel()
}

// handleCompact triggers context compaction.
// handleCompact 触发上下文压缩。
func (c *ACTX) handleCompact(ctx context.Context, sub protocol.Submission) {
	// Log the compaction request and its "full" flag for observability.
	// 记录压缩请求及其 full 标志以便观测。
	c.session.logger.Info("compact requested", "full", sub.Op.Full)
	// TODO: implement compaction
	// TODO: 实现压缩功能
	// Intended behaviour: summarize older history via the model, then
	// call ContextManager.Compact(summary, keepRecent) so the window
	// shrinks while the recent context stays intact.
	// 预期行为：通过模型对较早历史生成摘要，然后调用
	// ContextManager.Compact(summary, keepRecent)，
	// 在保留近期上下文的同时收缩窗口。
}
