package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// approvalResponse carries the result of an approval request back to the
// approvalResponse 将审批请求的结果带回给等待中的工具执行器。
// waiting tool executor.
type approvalResponse struct {
	Approved bool
	Feedback string
}

// Session holds all runtime state for a single ACTX conversation thread.
// Session 保存单个 ACTX 会话线程的所有运行时状态。
// It is not safe for concurrent use from multiple goroutines; all access
// 它不支持从多个 goroutine 并发使用；所有访问都通过提交循环进行协调。
// is mediated through the submission loop.
type Session struct {
	// id is the unique identifier for this session instance (UUID).
	// It is allocated by Spawn and used for logging and tracing.
	// id 是本会话实例的唯一标识符（UUID）。
	// 由 Spawn 分配，用于日志与追踪。
	id protocol.SessionId
	// threadID identifies the conversation thread; it survives across
	// multiple turns and supports rollback and App Server thread/start.
	// threadID 标识对话线程；跨多个轮次存活，
	// 支持回滚以及 App Server 的 thread/start。
	threadID protocol.ThreadId
	// config is the runtime configuration snapshot: model name, tools
	// mode, approval policy, sandbox policy, instructions, cwd, and
	// writable roots. runTurn reads it to build prompts; the fallback
	// logic may rewrite ToolsMode in place.
	// config 是运行时配置快照：模型名、工具模式、审批策略、沙箱策略、
	// 系统提示词、工作目录与可写根目录。runTurn 读取它构建提示词；
	// 降级逻辑可能会原地改写 ToolsMode。
	config *config.Config
	// client is the model API client; runTurn calls client.Stream to
	// issue SSE streaming requests.
	// client 是模型 API 客户端；runTurn 调用 client.Stream
	// 发起 SSE 流式请求。
	client *model.ModelClient
	// registry holds the tool executors; registry.Specs() produces the
	// tool descriptions exposed to the model.
	// registry 持有工具执行器；registry.Specs() 生成
	// 暴露给模型的工具描述。
	registry *tools.ToolRegistry
	// router dispatches tool calls (by ToolName) to the matching
	// executor in the registry.
	// router 按工具名把工具调用分发给注册表中
	// 对应的执行器。
	router *tools.ToolRouter
	// logger is the structured logger for session-level diagnostics
	// (tool execution, fallback switches, errors).
	// logger 是会话级诊断（工具执行、降级切换、错误）
	// 使用的结构化日志器。
	logger *slog.Logger
	// ctx is the session lifetime context. When cancelled it releases
	// every blocked channel operation: sendEvent drops, approval waits
	// return false.
	// ctx 是会话生命周期 context。取消它会解除所有阻塞的通道操作：
	// sendEvent 直接放弃，审批等待返回 false。
	ctx context.Context
	// txSub is the submission queue (SQ): external callers submit ops
	// here. NOTE: Session never reads/writes this field itself - the
	// ACTX submissionLoop consumes the shared channel; the field is a
	// redundant reference kept from Spawn.
	// txSub 是提交通道（SQ）：外部调用方在此提交操作。
	// 注意：Session 自身从不读写该字段 —— 由 ACTX 的
	// submissionLoop 消费这条共享通道；此字段只是 Spawn 时
	// 保留的冗余引用。
	txSub chan protocol.Submission
	// rxEvent is the event queue (EQ): the ONLY writer is
	// Session.sendEvent; consumers read it via ACTX.NextEvent/Events.
	// rxEvent 是事件通道（EQ）：唯一的写入方是 Session.sendEvent；
	// 消费方通过 ACTX.NextEvent/Events 读取。
	rxEvent chan protocol.Event
	// context manages the conversation history ([]ResponseItem) with
	// append/compact/rollback support; its first item is the system
	// message built from BaseInstructions.
	// context 管理对话历史（[]ResponseItem），支持追加/压缩/回滚；
	// 其首项是由 BaseInstructions 构建的 system 消息。
	context *ContextManager

	// mu guards turnCancel below: the submission loop writes it at turn
	// start/end while handleInterrupt may call cancelTurn concurrently.
	// mu 保护下面的 turnCancel：提交循环在轮次开始/结束时写入，
	// 而 handleInterrupt 可能并发调用 cancelTurn。
	mu sync.Mutex
	// turnCancel cancels the currently running turn; OpInterrupt
	// triggers it to abort in-flight model requests and tool executions.
	// turnCancel 取消当前正在运行的轮次；OpInterrupt 触发它，
	// 中止进行中的模型请求与工具执行。
	turnCancel context.CancelFunc
	// approvalCh (cap 1) carries approval verdicts: requestApproval
	// blocks reading it while handleApproval writes the user's y/n.
	// approvalCh（容量 1）传递审批裁决：requestApproval 阻塞读它，
	// handleApproval 写入用户的 y/n 结果。
	approvalCh chan approvalResponse
	// planMode makes executeToolCall return a "would execute" plan
	// instead of running tools for real.
	// planMode 使 executeToolCall 返回"将要执行"的计划描述，
	// 而不真正运行工具。
	planMode bool

	// toolsModeSwitched marks whether an automatic api->prompt tools
	// mode fallback has already happened, ensuring it occurs at most
	// once per session instead of retrying API mode every request.
	// toolsModeSwitched 标记是否已自动从 api 模式降级到 prompt 模式，
	// 保证单会话最多降级一次，避免每次请求都重试 API 模式。
	toolsModeSwitched bool
}

// SetPlanMode enables or disables plan mode for this session.
// SetPlanMode 为此会话启用或禁用计划模式。
func (s *Session) SetPlanMode(enabled bool) {
	s.planMode = enabled
}

// sendEvent sends an event to the event channel. It is non-blocking when
// sendEvent 向事件通道发送事件。当通道有容量时不阻塞；
// the channel has capacity; if the channel is full the call blocks.
func (s *Session) sendEvent(msg protocol.EventMsg) {
	select {
	case s.rxEvent <- protocol.Event{Msg: msg}:
	case <-s.ctx.Done():
	}
}

// Context returns the session's context manager for inspection (e.g. item count).
// Context 返回会话的上下文管理器，用于检查（例如项数）。
func (s *Session) Context() *ContextManager {
	return s.context
}

// cancelTurn cancels the currently running turn, if any.
// cancelTurn 取消当前正在运行的轮次（如果有的话）。
func (s *Session) cancelTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turnCancel != nil {
		s.turnCancel()
		s.turnCancel = nil
	}
}

// handleApproval delivers an approval response to the waiting tool executor.
// handleApproval 将审批响应传递给等待中的工具执行器。
func (s *Session) handleApproval(op protocol.Op) {
	s.approvalCh <- approvalResponse{
		Approved: op.Approved,
		Feedback: op.Feedback,
	}
}

// requestApproval sends an approval request event and blocks until the
// requestApproval 发送审批请求事件并阻塞直到用户响应。
// user responds. This is called from within a tool execution.
func (s *Session) requestApproval(req tools.ApprovalRequest) bool {
	// Check approval policy - auto-approve if policy says so.
	// 检查审批策略 - 如果策略指示无需审批则自动批准。
	if !config.AskForApproval(s.config.ApprovalPolicy, req.Command) {
		return true
	}

	s.sendEvent(protocol.EventMsg{
		Type:    protocol.EvExecApprovalRequest,
		TurnID:  req.CallID,
		CallID:  req.CallID,
		Command: req.Command,
		Cwd:     req.Cwd,
	})

	select {
	case resp := <-s.approvalCh:
		return resp.Approved
	case <-s.ctx.Done():
		return false
	}
}

// runTurn executes a single agent turn: a sampling loop that sends the
// runTurn 执行单个代理轮次：一个采样循环，将对话发送给模型，
// conversation to the model, processes the response, executes any tool
// 处理响应，执行任何工具调用，并重复直到模型产生最终消息。
// calls, and repeats until the model produces a final message.
func (s *Session) runTurn(parentCtx context.Context, turnID protocol.TurnId, submissionID protocol.SubmissionId) {
	turnCtx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	s.mu.Lock()
	s.turnCancel = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.turnCancel = nil
		s.mu.Unlock()
	}()

	// Emit any pending user messages.
	// 发出任何待处理的用户消息。
	for _, item := range s.context.PendingUserMessages() {
		s.sendEvent(protocol.EventMsg{
			Type:    protocol.EvUserMessage,
			Message: item.GetText(),
		})
	}

	for {
		select {
		case <-turnCtx.Done():
			s.sendEvent(protocol.StreamErrorEvent(turnID, "turn cancelled"))
			return
		default:
		}

		// Build the prompt from conversation history.
		// 从对话历史构建提示词。
		specs := s.registry.Specs()
		instructions := s.config.BaseInstructions + "\n\n" + s.config.UserInstructions

		var apiTools []model.ToolSpec
		if s.config.ToolsMode == "prompt" {
			// 提示词模式：不发送 tools 参数，改为注入工具描述到系统提示词。
			instructions += BuildToolPrompt(specs)
		} else {
			apiTools = specs
		}

		prompt := model.Prompt{
			Input:        s.context.Items(),
			Tools:        apiTools,
			Instructions: instructions,
			Model:        s.config.Model,
		}

		// Stream from the model.
		// 从模型流式获取响应。
		eventCh, err := s.client.Stream(turnCtx, prompt)
		if err != nil {
			// 自动降级：如果 api 模式遇到 tools/function 相关的 400 错误，
			// 切换到 prompt 模式并重试。
			if s.config.ToolsMode == "api" && !s.toolsModeSwitched && shouldFallbackToPrompt(err) {
				s.toolsModeSwitched = true
				s.config.ToolsMode = "prompt"
				s.logger.Info("auto-switching to prompt tools mode", "reason", err)
				s.sendEvent(protocol.EventMsg{
					Type:    protocol.EvUserMessage,
					Message: "[系统] 模型不支持函数调用，已自动切换到提示词工具模式",
				})
				continue
			}
			s.sendEvent(protocol.StreamErrorEvent(turnID, fmt.Sprintf("model request failed: %v", err)))
			return
		}

		// Process the SSE stream.
		// 处理 SSE 流。
		needsFollowUp, err := s.processStream(turnCtx, turnID, eventCh)
		if err != nil {
			s.sendEvent(protocol.StreamErrorEvent(turnID, err.Error()))
			return
		}

		if !needsFollowUp {
			return
		}
	}
}

// processStream consumes the model's SSE event stream, emitting agent events
// processStream 消费模型的 SSE 事件流，发出代理事件
// and executing tool calls. It returns true if the model made tool calls that
// 并执行工具调用。如果模型发出了需要后续请求的工具调用（即采样循环应继续），
// require a follow-up request (i.e., the sampling loop should continue).
func (s *Session) processStream(ctx context.Context, turnID protocol.TurnId, eventCh <-chan model.ResponseEvent) (bool, error) {
	needsFollowUp := false
	var pendingMessage strings.Builder

	for ev := range eventCh {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		switch ev.Type {
		case model.EventOutputTextDelta:
			// Stream the text delta to the client.
			// 将文本增量流式传输到客户端。
			s.sendEvent(protocol.AgentMessageDeltaEvent(turnID, ev.DeltaText))
			pendingMessage.WriteString(ev.DeltaText)

		case model.EventReasoningDelta:
			// Stream the reasoning delta to the client so the TUI/App
			// Server can render the model's thinking progress in real time.
			// 将推理增量流式传输到客户端，使 TUI/App Server 能够实时渲染模型的思考进度。
			if ev.DeltaReasoning != "" {
				s.sendEvent(protocol.ReasoningDeltaEvent(turnID, ev.DeltaReasoning))
			}

		case model.EventOutputItemDone:
			if ev.Item == nil {
				continue
			}
			item := *ev.Item

			// Handle function calls.
			// 处理函数调用。
			if item.IsFunctionCall() {
				needsFollowUp = true
				s.context.Append(item)

				// Execute the tool call.
				// 执行工具调用。
				output, err := s.executeToolCall(ctx, turnID, item)
				if err != nil {
					s.logger.Error("tool execution failed", "tool", item.Name, "error", err)
					// Send error output back to model.
					// 将错误输出发送回模型。
					errOutput := tools.JSONToolOutput{
						Data: map[string]string{"error": err.Error()},
						Exit: 1,
					}
					s.context.Append(errOutput.ToResponseItem(item.CallID))
				} else {
					s.context.Append(output.ToResponseItem(item.CallID))
				}
				continue
			}

			// Handle assistant messages.
			// 处理助手消息。
			if item.IsMessage() && item.Role == "assistant" {
				text := item.GetText()
				if text == "" {
					text = pendingMessage.String()
				}
				if text != "" {
					// 提示词模式：从助手消息文本中解析工具调用。
					if s.config.ToolsMode == "prompt" {
						calls, cleanText := ParseToolCallsFromText(text)
						if len(calls) > 0 {
							if cleanText != "" {
								s.context.Append(protocol.NewAssistantMessage(cleanText))
								s.sendEvent(protocol.AgentMessageEvent(turnID, cleanText))
							}
							for _, call := range calls {
								callID := protocol.NewCallId()
								callItem := call.ToResponseItem(callID)
								needsFollowUp = true
								s.context.Append(callItem)

								s.logger.Info("prompt-mode tool call", "tool", call.Name, "call_id", callID)

								output, err := s.executeToolCall(ctx, turnID, callItem)
								if err != nil {
									s.logger.Error("tool execution failed", "tool", call.Name, "error", err)
									errOutput := tools.JSONToolOutput{
										Data: map[string]string{"error": err.Error()},
										Exit: 1,
									}
									s.context.Append(errOutput.ToResponseItem(callID))
								} else {
									s.context.Append(output.ToResponseItem(callID))
								}
							}
							pendingMessage.Reset()
							continue
						}
					}

					s.context.Append(protocol.NewAssistantMessage(text))
					s.sendEvent(protocol.AgentMessageEvent(turnID, text))
					pendingMessage.Reset()
				}
			}

			// Handle reasoning items.
			// 处理推理项。
			if item.Type == protocol.RITypeReasoning {
				s.context.Append(item)
				s.sendEvent(protocol.EventMsg{
					Type:    protocol.EvAgentReasoning,
					TurnID:  turnID,
					Message: item.Summary,
				})
			}

		case model.EventCompleted:
			if ev.Usage != nil {
				s.sendEvent(protocol.TokenCountEvent(turnID, ev.Usage.InputTokens, ev.Usage.OutputTokens))
			}

		case model.EventError, model.EventFailed:
			msg := "model stream error"
			if ev.Item != nil {
				msg = ev.Item.GetText()
			}
			return false, fmt.Errorf("%s", msg)
		}
	}

	// If there's accumulated text that wasn't finalized via OutputItemDone,
	// 如果有未通过 OutputItemDone 最终确定的累积文本，
	// emit it as an agent message.
	// 将其作为代理消息发出。
	if pendingMessage.Len() > 0 {
		text := pendingMessage.String()

		// 提示词模式：从文本中解析工具调用。
		if s.config.ToolsMode == "prompt" {
			calls, cleanText := ParseToolCallsFromText(text)
			if len(calls) > 0 {
				// 保存去除工具块的纯文本（如有）。
				if cleanText != "" {
					s.context.Append(protocol.NewAssistantMessage(cleanText))
					s.sendEvent(protocol.AgentMessageEvent(turnID, cleanText))
				}
				// 执行每个解析出的工具调用。
				for _, call := range calls {
					callID := protocol.NewCallId()
					item := call.ToResponseItem(callID)
					needsFollowUp = true
					s.context.Append(item)

					s.logger.Info("prompt-mode tool call", "tool", call.Name, "call_id", callID)

					output, err := s.executeToolCall(ctx, turnID, item)
					if err != nil {
						s.logger.Error("tool execution failed", "tool", call.Name, "error", err)
						errOutput := tools.JSONToolOutput{
							Data: map[string]string{"error": err.Error()},
							Exit: 1,
						}
						s.context.Append(errOutput.ToResponseItem(callID))
					} else {
						s.context.Append(output.ToResponseItem(callID))
					}
				}
				return needsFollowUp, nil
			}
		}

		s.context.Append(protocol.NewAssistantMessage(text))
		s.sendEvent(protocol.AgentMessageEvent(turnID, text))
	}

	return needsFollowUp, nil
}

// executeToolCall dispatches a function call to the appropriate tool executor
// executeToolCall 将函数调用分发到适当的工具执行器
// and returns the output. It handles approval requests for commands that
// 并返回输出。它处理需要用户同意的命令的审批请求。
// require user consent.
func (s *Session) executeToolCall(ctx context.Context, turnID protocol.TurnId, item protocol.ResponseItem) (tools.ToolOutput, error) {
	// In plan mode, don't actually execute tools - return a plan response.
	// 在计划模式下，不实际执行工具 - 返回计划响应。
	if s.planMode {
		return &tools.JSONToolOutput{
			Data: map[string]interface{}{
				"status":  "planned",
				"message": fmt.Sprintf("Would execute: %s with args: %s", item.Name, item.Arguments),
			},
			Exit: 0,
		}, nil
	}

	s.logger.Info("executing tool call", "tool", item.Name, "call_id", item.CallID)

	// Parse arguments if needed for cwd.
	// 根据需要解析参数以获取 cwd。
	var args struct {
		Command []string `json:"command"`
		Cwd     string   `json:"cwd,omitempty"`
	}
	_ = json.Unmarshal([]byte(item.Arguments), &args)

	cwd := args.Cwd
	if cwd == "" {
		cwd = s.config.Cwd
	}

	invocation := tools.ToolInvocation{
		ToolName:       item.Name,
		TurnID:         turnID,
		CallID:         item.CallID,
		Arguments:      item.Arguments,
		Cwd:            cwd,
		WritableRoots:  s.config.WritableRoots,
		SandboxPolicy:  s.config.SandboxPolicy,
		ApprovalPolicy: s.config.ApprovalPolicy,
		ApprovalCb: func(req tools.ApprovalRequest) bool {
			return s.requestApproval(req)
		},
	}

	return s.router.Dispatch(ctx, invocation)
}

// ShellToolSpec returns the tool spec for the shell tool.
// ShellToolSpec 返回 shell 工具的工具规范。
func ShellToolSpec() model.ToolSpec {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "The command to execute as an array of strings.",
			},
			"cwd": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for the command.",
			},
			"timeout_ms": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in milliseconds.",
			},
		},
		"required": []string{"command"},
	}
	paramsJSON, _ := json.Marshal(params)
	return model.NewFunctionTool("shell", "Execute a shell command.", paramsJSON)
}

// ApplyPatchToolSpec returns the tool spec for the apply_patch tool.
// ApplyPatchToolSpec 返回 apply_patch 工具的工具规范。
func ApplyPatchToolSpec() model.ToolSpec {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"input": map[string]interface{}{
				"type":        "string",
				"description": "The patch content in ACTX apply_patch format.",
			},
		},
		"required": []string{"input"},
	}
	paramsJSON, _ := json.Marshal(params)
	return model.NewFunctionTool("apply_patch", "Apply a file patch to modify files.", paramsJSON)
}

// durationSince returns the elapsed milliseconds since the given start time.
// durationSince 返回从给定开始时间经过的毫秒数。
func durationSince(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
