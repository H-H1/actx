// Package protocol defines the SQ/EQ (Submission Queue / Event Queue) communication
// 包 protocol 定义了客户端与 ACTX 代理之间的 SQ/EQ（提交队列/事件队列）通信类型。
// types between the client and the ACTX agent. The client submits Op operations
// 客户端提交操作（Op），并接收事件（Event）通知。
// and receives Event notifications.
package protocol

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SessionId uniquely identifies a ACTX session.
// SessionId 唯一标识一个 ACTX 会话。
type SessionId = string

// ThreadId uniquely identifies a conversation thread.
// ThreadId 唯一标识一个对话线程。
type ThreadId = string

// ToolName is the name of a tool callable by the model.
// ToolName 是模型可调用的工具名称。
type ToolName = string

// CallId identifies a specific function/tool call within a turn.
// CallId 标识一个轮次中的特定函数/工具调用。
type CallId = string

// TurnId identifies a single turn (user input → agent response cycle).
// TurnId 标识单个轮次（用户输入 → 代理响应循环）。
type TurnId = string

// SubmissionId correlates a Submission with its resulting Events.
// SubmissionId 将提交与其产生的事件关联起来。
type SubmissionId = string

// NewSubmissionId generates a unique submission ID.
// NewSubmissionId 生成一个唯一的提交 ID。
func NewSubmissionId() SubmissionId {
	return uuid.NewString()
}

// NewSessionId generates a unique session ID.
// NewSessionId 生成一个唯一的会话 ID。
func NewSessionId() SessionId {
	return uuid.NewString()
}

// NewThreadId generates a unique thread ID.
// NewThreadId 生成一个唯一的线程 ID。
func NewThreadId() ThreadId {
	return uuid.NewString()
}

// NewTurnId generates a unique turn ID.
// NewTurnId 生成一个唯一的轮次 ID。
func NewTurnId() TurnId {
	return uuid.NewString()
}

// NewCallId generates a unique tool call ID.
// NewCallId 生成一个唯一的工具调用 ID。
func NewCallId() CallId {
	return "call_" + uuid.NewString()
}

// Submission represents a user→agent request placed on the submission queue.
// Submission 表示放置在提交队列上的用户→代理请求。
type Submission struct {
	ID                  SubmissionId `json:"id"`
	Op                  Op           `json:"op"`
	ClientUserMessageID string       `json:"client_user_message_id,omitempty"`
}

// Event represents an agent→user notification placed on the event queue.
// Event 表示放置在事件队列上的代理→用户通知。
type Event struct {
	ID  SubmissionId `json:"id"`
	Msg EventMsg     `json:"msg"`
}

// Op is the operation submitted by the user to the agent.
// Op 是用户提交给代理的操作。
// It uses a tagged union pattern with JSON discrimination.
// 它使用带 JSON 区分的标记联合模式。
type Op struct {
	Type OpType `json:"type"`

	// UserInput fields
	// UserInput 字段
	Items             []ResponseItem  `json:"items,omitempty"`
	FinalOutputSchema json.RawMessage `json:"final_output_json_schema,omitempty"`
	AdditionalContext []ResponseItem  `json:"additional_context,omitempty"`

	// ExecApproval fields
	// ExecApproval 字段
	CallID   CallId `json:"call_id,omitempty"`
	Approved bool   `json:"approved,omitempty"`
	Feedback string `json:"feedback,omitempty"`

	// PatchApproval fields (reuses CallID, Approved)
	// PatchApproval 字段（复用 CallID、Approved）

	// Compact fields
	// Compact 字段
	Full bool `json:"full,omitempty"`

	// ThreadRollback fields
	// ThreadRollback 字段
	RollbackTo int `json:"rollback_to,omitempty"`
}

// OpType enumerates all operations the user can submit.
// OpType 枚举用户可提交的所有操作。
type OpType string

const (
	OpUserInput                OpType = "user_input"
	OpInterrupt                OpType = "interrupt"
	OpShutdown                 OpType = "shutdown"
	OpExecApproval             OpType = "exec_approval"
	OpPatchApproval            OpType = "patch_approval"
	OpCompact                  OpType = "compact"
	OpRefreshMcpServers        OpType = "refresh_mcp_servers"
	OpReloadUserConfig         OpType = "reload_user_config"
	OpThreadRollback           OpType = "thread_rollback"
	OpCleanBackgroundTerminals OpType = "clean_background_terminals"
	OpRunUserShellCommand      OpType = "run_user_shell_command"
	OpResolveElicitation       OpType = "resolve_elicitation"
	OpUserInputAnswer          OpType = "user_input_answer"
	OpRequestPermissionsResp   OpType = "request_permissions_response"
	OpDynamicToolResponse      OpType = "dynamic_tool_response"
)

// NewUserInputOp creates a user input operation.
// NewUserInputOp 创建一个用户输入操作。
func NewUserInputOp(items []ResponseItem) Op {
	return Op{Type: OpUserInput, Items: items}
}

// NewInterruptOp creates an interrupt operation.
// NewInterruptOp 创建一个中断操作。
func NewInterruptOp() Op {
	return Op{Type: OpInterrupt}
}

// NewShutdownOp creates a shutdown operation.
// NewShutdownOp 创建一个关闭操作。
func NewShutdownOp() Op {
	return Op{Type: OpShutdown}
}

// NewExecApprovalOp creates an exec approval response.
// NewExecApprovalOp 创建一个执行审批响应。
func NewExecApprovalOp(callID CallId, approved bool, feedback string) Op {
	return Op{Type: OpExecApproval, CallID: callID, Approved: approved, Feedback: feedback}
}

// NewPatchApprovalOp creates a patch approval response.
// NewPatchApprovalOp 创建一个补丁审批响应。
func NewPatchApprovalOp(callID CallId, approved bool) Op {
	return Op{Type: OpPatchApproval, CallID: callID, Approved: approved}
}

// NewCompactOp creates a compact operation.
// NewCompactOp 创建一个压缩操作。
func NewCompactOp(full bool) Op {
	return Op{Type: OpCompact, Full: full}
}

// EventMsg is a tagged union of all events the agent can emit to the user.
// EventMsg 是代理可向用户发出的所有事件的标记联合体。
type EventMsg struct {
	Type EventType `json:"type"`

	// Common fields used across multiple event types
	// 跨多个事件类型使用的公共字段
	TurnID   TurnId   `json:"turn_id,omitempty"`
	Message  string   `json:"message,omitempty"`
	CallID   CallId   `json:"call_id,omitempty"`
	ToolName ToolName `json:"tool_name,omitempty"`

	// ExecCommandBegin / End
	Command     []string `json:"command,omitempty"`
	Cwd         string   `json:"cwd,omitempty"`
	ExitCode    *int     `json:"exit_code,omitempty"`
	Output      string   `json:"output,omitempty"`
	OutputDelta string   `json:"output_delta,omitempty"`
	DurationMs  *int64   `json:"duration_ms,omitempty"`

	// PatchApply
	PatchContent string `json:"patch_content,omitempty"`

	// AgentMessageContentDelta
	Delta string `json:"delta,omitempty"`

	// TokenCount
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`

	// SessionConfigured
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`

	// Error
	ErrorMsg string `json:"error,omitempty"`

	// Raw JSON for extensible fields
	// 用于可扩展字段的原始 JSON
	Raw json.RawMessage `json:"-"`
}

// EventType enumerates all events the agent can emit.
// EventType 枚举代理可发出的所有事件。
type EventType string

const (
	EvSessionConfigured      EventType = "session_configured"
	EvTurnStarted            EventType = "turn_started"
	EvTurnComplete           EventType = "turn_complete"
	EvTurnAborted            EventType = "turn_aborted"
	EvAgentMessage           EventType = "agent_message"
	EvAgentMessageDelta      EventType = "agent_message_delta"
	EvAgentReasoning         EventType = "agent_reasoning"
	EvReasoningDelta         EventType = "reasoning_delta"
	EvUserMessage            EventType = "user_message"
	EvExecCommandBegin       EventType = "exec_command_begin"
	EvExecCommandOutputDelta EventType = "exec_command_output_delta"
	EvExecCommandEnd         EventType = "exec_command_end"
	EvPatchApplyBegin        EventType = "patch_apply_begin"
	EvPatchApplyUpdated      EventType = "patch_apply_updated"
	EvPatchApplyEnd          EventType = "patch_apply_end"
	EvExecApprovalRequest    EventType = "exec_approval_request"
	EvPatchApprovalRequest   EventType = "patch_approval_request"
	EvTokenCount             EventType = "token_count"
	EvContextCompacted       EventType = "context_compacted"
	EvStreamError            EventType = "stream_error"
	EvShutdownComplete       EventType = "shutdown_complete"
	EvMcpToolCallBegin       EventType = "mcp_tool_call_begin"
	EvMcpToolCallEnd         EventType = "mcp_tool_call_end"
	EvMcpStartupUpdate       EventType = "mcp_startup_update"
	EvMcpStartupComplete     EventType = "mcp_startup_complete"
)

// NewEvent creates an Event with the given submission ID and message.
// NewEvent 使用给定的提交 ID 和消息创建一个事件。
func NewEvent(submissionID SubmissionId, msg EventMsg) Event {
	return Event{ID: submissionID, Msg: msg}
}

// AgentMessageEvent creates an agent message event.
// AgentMessageEvent 创建一个代理消息事件。
func AgentMessageEvent(turnID TurnId, message string) EventMsg {
	return EventMsg{Type: EvAgentMessage, TurnID: turnID, Message: message}
}

// AgentMessageDeltaEvent creates a streaming delta event for agent messages.
// AgentMessageDeltaEvent 创建代理消息的流式增量事件。
func AgentMessageDeltaEvent(turnID TurnId, delta string) EventMsg {
	return EventMsg{Type: EvAgentMessageDelta, TurnID: turnID, Delta: delta}
}

// ReasoningDeltaEvent creates a streaming reasoning delta event.
// ReasoningDeltaEvent 创建流式推理增量事件。
// The Delta field carries a fragment of the model's reasoning_content
// stream (DeepSeek-R1, o1-style models) so clients can render thinking
// progress in real time before the final answer is produced.
// Delta 字段携带模型 reasoning_content 流的一个片段（DeepSeek-R1、o1 系列模型），
// 以便客户端在最终答案产出前实时渲染思考进度。
func ReasoningDeltaEvent(turnID TurnId, delta string) EventMsg {
	return EventMsg{Type: EvReasoningDelta, TurnID: turnID, Delta: delta}
}

// ExecCommandBeginEvent creates a command execution begin event.
// ExecCommandBeginEvent 创建命令执行开始事件。
func ExecCommandBeginEvent(turnID TurnId, callID CallId, command []string, cwd string) EventMsg {
	return EventMsg{Type: EvExecCommandBegin, TurnID: turnID, CallID: callID, Command: command, Cwd: cwd}
}

// ExecCommandEndEvent creates a command execution end event.
// ExecCommandEndEvent 创建命令执行结束事件。
func ExecCommandEndEvent(turnID TurnId, callID CallId, exitCode int, output string, durationMs int64) EventMsg {
	return EventMsg{Type: EvExecCommandEnd, TurnID: turnID, CallID: callID, ExitCode: &exitCode, Output: output, DurationMs: &durationMs}
}

// ExecApprovalRequestEvent creates an approval request for a command.
// ExecApprovalRequestEvent 创建命令的审批请求。
func ExecApprovalRequestEvent(turnID TurnId, callID CallId, command []string, cwd string) EventMsg {
	return EventMsg{Type: EvExecApprovalRequest, TurnID: turnID, CallID: callID, Command: command, Cwd: cwd}
}

// PatchApprovalRequestEvent creates an approval request for a patch.
// PatchApprovalRequestEvent 创建补丁的审批请求。
func PatchApprovalRequestEvent(turnID TurnId, callID CallId, patchContent string) EventMsg {
	return EventMsg{Type: EvPatchApprovalRequest, TurnID: turnID, CallID: callID, PatchContent: patchContent}
}

// TokenCountEvent creates a token count event.
// TokenCountEvent 创建一个令牌计数事件。
func TokenCountEvent(turnID TurnId, input, output int) EventMsg {
	return EventMsg{Type: EvTokenCount, TurnID: turnID, InputTokens: input, OutputTokens: output}
}

// SessionConfiguredEvent creates a session configured event.
// SessionConfiguredEvent 创建一个会话已配置事件。
func SessionConfiguredEvent(model, provider string) EventMsg {
	return EventMsg{Type: EvSessionConfigured, Model: model, Provider: provider}
}

// TurnStartedEvent creates a turn started event.
// TurnStartedEvent 创建一个轮次已启动事件。
func TurnStartedEvent(turnID TurnId) EventMsg {
	return EventMsg{Type: EvTurnStarted, TurnID: turnID}
}

// TurnCompleteEvent creates a turn complete event.
// TurnCompleteEvent 创建一个轮次已完成事件。
func TurnCompleteEvent(turnID TurnId) EventMsg {
	return EventMsg{Type: EvTurnComplete, TurnID: turnID}
}

// StreamErrorEvent creates a stream error event.
// StreamErrorEvent 创建一个流错误事件。
func StreamErrorEvent(turnID TurnId, errMsg string) EventMsg {
	return EventMsg{Type: EvStreamError, TurnID: turnID, ErrorMsg: errMsg}
}

// ShutdownCompleteEvent creates a shutdown complete event.
// ShutdownCompleteEvent 创建一个关闭完成事件。
func ShutdownCompleteEvent() EventMsg {
	return EventMsg{Type: EvShutdownComplete}
}

// Timestamp returns the current time as Unix milliseconds.
// Timestamp 返回当前时间的 Unix 毫秒数。
func Timestamp() int64 {
	return time.Now().UnixMilli()
}
