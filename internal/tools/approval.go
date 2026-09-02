package tools

import "github.com/H-H1/actx/internal/protocol"

// ApprovalRequest represents a request for user approval before a tool
// ApprovalRequest 表示在工具执行敏感操作（如运行命令或应用补丁）之前，
// performs a sensitive operation such as running a command or applying a patch.
// 请求用户审批。
//
// The fields relevant to the specific tool are populated: shell tools fill in
// 与特定工具相关的字段会被填充：shell 工具填充 Command 和 Cwd，
// Command and Cwd, while patch tools fill in PatchContent.
// 而补丁工具填充 PatchContent。
type ApprovalRequest struct {
	// CallID identifies the tool call that requires approval.
	// CallID 标识需要审批的工具调用。
	CallID protocol.CallId
	// ToolName is the name of the tool requesting approval.
	// ToolName 是请求审批的工具名称。
	ToolName protocol.ToolName
	// Command is the command to be executed (populated for shell tools).
	// Command 是要执行的命令（为 shell 工具填充）。
	Command []string
	// Cwd is the working directory in which the command will run.
	// Cwd 是命令将运行的工作目录。
	Cwd string
	// PatchContent is the content of a patch to be applied (populated for
	// PatchContent 是要应用的补丁内容（为补丁工具填充）。
	// patch tools).
	PatchContent string
}

// ApprovalResult represents the user's response to an approval request.
// ApprovalResult 表示用户对审批请求的响应。
type ApprovalResult struct {
	// Approved indicates whether the user allowed the operation.
	// Approved 表示用户是否允许了该操作。
	Approved bool
	// Feedback is an optional message from the user explaining the decision
	// Feedback 是用户可选的消息，用于解释决定或为模型提供指导。
	// or providing guidance to the model.
	Feedback string
}
