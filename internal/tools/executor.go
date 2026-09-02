// Package tools implements the tool execution system for the ACTX CLI agent.
// 包 tools 实现了 ACTX CLI 代理的工具执行系统。
//
// A ToolExecutor represents a single tool that the model can invoke. Executors
// ToolExecutor 表示模型可调用的单个工具。执行器注册在 ToolRegistry 中，
// are registered in a ToolRegistry, which the ToolRouter uses to dispatch
// ToolRouter 使用 ToolRegistry 将传入的工具调用分派到正确的处理程序。
// incoming tool calls to the correct handler. Each tool produces a ToolOutput
// 每个工具生成一个 ToolOutput，将其转换为 protocol.ResponseItem 供模型上下文使用。
// that is converted into a protocol.ResponseItem for the model's context.
package tools

import (
	"context"

	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
)

// ToolExecutor is the interface that every tool must implement.
// ToolExecutor 是每个工具必须实现的接口。
//
// Implementations are responsible for validating arguments, performing any
// 实现负责验证参数，通过提供的 ApprovalCb 执行所需的审批检查，
// required approval checks via the provided ApprovalCb, executing the tool
// 执行工具逻辑，并返回 ToolOutput。如果发生错误，执行器应返回
// logic, and returning a ToolOutput. If an error occurs the executor should
// *FunctionCallError，其 Kind 指示错误应报告给模型还是视为致命错误。
// return a *FunctionCallError whose Kind indicates whether the error should
// be reported back to the model or treated as fatal.
type ToolExecutor interface {
	// Name returns the unique name identifying this tool.
	// Name 返回标识此工具的唯一名称。
	Name() protocol.ToolName

	// Spec returns the tool specification sent to the model API so the model
	// Spec 返回发送给模型 API 的工具规范，以便模型知道如何调用此工具。
	// knows how to call this tool.
	Spec() model.ToolSpec

	// Handle executes the tool invocation and returns its output.
	// Handle 执行工具调用并返回其输出。
	Handle(ctx context.Context, invocation ToolInvocation) (ToolOutput, error)
}

// ToolInvocation carries all the data needed to execute a single tool call.
// ToolInvocation 携带执行单个工具调用所需的所有数据。
type ToolInvocation struct {
	// ToolName is the name of the tool to execute.
	// ToolName 是要执行的工具名称。
	ToolName protocol.ToolName

	// TurnID identifies the agent turn that produced this call.
	// TurnID 标识产生此调用的代理轮次。
	TurnID protocol.TurnId

	// CallID identifies this specific tool call within the turn.
	// CallID 标识轮次中的此特定工具调用。
	CallID protocol.CallId

	// Arguments is the raw JSON arguments string produced by the model.
	// Arguments 是模型生成的原始 JSON 参数字符串。
	Arguments string

	// Cwd is the working directory in which the tool should execute.
	// Cwd 是工具应执行的工作目录。
	Cwd string

	// SandboxPolicy controls filesystem and network restrictions for this
	// SandboxPolicy 控制此工具调用的文件系统和网络限制。
	// tool invocation.
	SandboxPolicy config.SandboxPolicy

	// WritableRoots lists additional directories the tool may write to
	// beyond the workspace root. Only effective when SandboxPolicy is
	// SandboxWorkspaceWrite.
	// WritableRoots 列出除工作区根目录外工具可以写入的其他目录。
	// 仅在 SandboxPolicy 为 SandboxWorkspaceWrite 时生效。
	WritableRoots []string

	// ApprovalPolicy controls when the user is asked to approve commands
	// ApprovalPolicy 控制用户何时被要求批准此工具调用的命令。
	// for this tool invocation.
	ApprovalPolicy config.ApprovalPolicy

	// ApprovalCb is an optional callback used to request user approval
	// ApprovalCb 是一个可选回调，用于在执行敏感操作之前请求用户审批。
	// before performing sensitive operations. If nil, the executor should
	// 如果为 nil，执行器应拒绝任何需要审批的操作。
	// deny any operation that requires approval.
	ApprovalCb func(approvalReq ApprovalRequest) bool
}

// ToolOutput is the interface implemented by all tool execution results.
// ToolOutput 是所有工具执行结果实现的接口。
//
// It provides a preview string for logging, a ResponseItem for the model
// 它提供用于日志记录的预览字符串、用于模型上下文的 ResponseItem，
// context, and an exit code indicating success or failure.
// 以及表示成功或失败的退出码。
type ToolOutput interface {
	// LogPreview returns a human-readable preview of the output suitable
	// LogPreview 返回输出的可读预览，适用于日志记录或 UI 显示。
	// for logging or display in the UI.
	LogPreview() string

	// ToResponseItem converts the output into a ResponseItem that will be
	// ToResponseItem 将输出转换为将被添加到模型对话上下文中的 ResponseItem。
	// added to the model's conversation context.
	ToResponseItem(callID protocol.CallId) protocol.ResponseItem

	// ExitCode returns the process exit code (0 indicates success).
	// ExitCode 返回进程退出码（0 表示成功）。
	ExitCode() int
}
