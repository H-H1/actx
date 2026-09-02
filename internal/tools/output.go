package tools

import (
	"encoding/json"
	"fmt"

	"github.com/H-H1/actx/internal/protocol"
)

// Compile-time assertions that the output types implement ToolOutput.
// 编译时断言输出类型实现了 ToolOutput 接口。
var (
	_ ToolOutput = JSONToolOutput{}
	_ ToolOutput = ExecOutput{}
)

// JSONToolOutput is a ToolOutput that wraps arbitrary JSON-serializable data.
// JSONToolOutput 是一个包装任意可 JSON 序列化数据的 ToolOutput。
// It is suitable for tools whose results are naturally structured data rather
// 它适用于结果本质上是结构化数据而非命令输出的工具。
// than command output.
type JSONToolOutput struct {
	// Data is the value to be JSON-serialized for the model.
	// Data 是要为模型进行 JSON 序列化的值。
	Data interface{}
	// Exit is the exit code (0 for success).
	// Exit 是退出码（0 表示成功）。
	Exit int
}

// LogPreview returns a pretty-printed JSON representation of the data.
// LogPreview 返回数据的格式化 JSON 表示。
func (o JSONToolOutput) LogPreview() string {
	b, err := json.MarshalIndent(o.Data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", o.Data)
	}
	return string(b)
}

// ToResponseItem converts the output to a function_call_output ResponseItem
// ToResponseItem 将输出转换为包含数据紧凑 JSON 序列化的 function_call_output ResponseItem。
// containing the compact JSON serialization of the data.
func (o JSONToolOutput) ToResponseItem(callID protocol.CallId) protocol.ResponseItem {
	b, err := json.Marshal(o.Data)
	if err != nil {
		return protocol.NewFunctionCallOutput(
			callID,
			fmt.Sprintf("error serializing tool output: %v", err),
		)
	}
	return protocol.NewFunctionCallOutput(callID, string(b))
}

// ExitCode returns the exit code of the tool execution.
// ExitCode 返回工具执行的退出码。
func (o JSONToolOutput) ExitCode() int {
	return o.Exit
}

// ExecOutput is a ToolOutput representing the result of a shell command
// ExecOutput 是表示 shell 命令执行结果的 ToolOutput。
// execution.
type ExecOutput struct {
	// Stdout is the captured standard output of the command.
	// Stdout 是命令捕获的标准输出。
	Stdout string
	// Stderr is the captured standard error of the command.
	// Stderr 是命令捕获的标准错误。
	Stderr string
	// Exit is the process exit code. The field is named Exit rather than
	// Exit 是进程退出码。字段命名为 Exit 而非 ExitCode，以避免与 ExitCode() 接口方法冲突。
	// ExitCode to avoid a collision with the ExitCode() interface method.
	Exit int
	// DurationMs is the wall-clock execution time in milliseconds.
	// DurationMs 是以毫秒为单位的墙上时钟执行时间。
	DurationMs int64
}

// LogPreview returns a combined view of stdout and stderr for logging. When
// LogPreview 返回 stdout 和 stderr 的组合视图用于日志记录。当退出码非零时，
// the exit code is non-zero it is appended so failures are easy to spot.
// 会附加退出码，以便容易发现失败。
func (o ExecOutput) LogPreview() string {
	return combineOutput(o.Stdout, o.Stderr, o.Exit)
}

// ToResponseItem converts the exec output to a function_call_output ResponseItem.
// ToResponseItem 将执行输出转换为 function_call_output ResponseItem。
// The exit code is included when non-zero so the model can reason about failures.
// 当退出码非零时会包含退出码，以便模型能够推理失败原因。
func (o ExecOutput) ToResponseItem(callID protocol.CallId) protocol.ResponseItem {
	return protocol.NewFunctionCallOutput(callID, combineOutput(o.Stdout, o.Stderr, o.Exit))
}

// ExitCode returns the process exit code.
// ExitCode 返回进程退出码。
func (o ExecOutput) ExitCode() int {
	return o.Exit
}

// combineOutput joins stdout and stderr and optionally appends the exit code
// combineOutput 合并 stdout 和 stderr，并在退出码非零时可选地附加退出码。
// when it is non-zero.
func combineOutput(stdout, stderr string, exit int) string {
	var output string
	switch {
	case stderr == "":
		output = stdout
	case stdout == "":
		output = stderr
	default:
		output = stdout + "\n" + stderr
	}
	if exit != 0 {
		if output == "" {
			return fmt.Sprintf("[exit code: %d]", exit)
		}
		return fmt.Sprintf("%s\n[exit code: %d]", output, exit)
	}
	return output
}
