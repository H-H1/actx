package tools

import "fmt"

// ErrorKind classifies how a tool error should be handled by the agent.
// ErrorKind 分类工具错误应由代理处理的方式。
type ErrorKind int

const (
	// ErrorKindRespondToModel indicates that the error should be reported
	// ErrorKindRespondToModel 表示错误应作为工具调用输出报告给模型，
	// back to the model as the tool call output so the model can adjust its
	// 以便模型可以调整其方法并可能重试。
	// approach and potentially retry.
	ErrorKindRespondToModel ErrorKind = iota
	// ErrorKindFatal indicates that the error is unrecoverable and the
	// ErrorKindFatal 表示错误不可恢复，当前轮次应中止。
	// current turn should be aborted.
	ErrorKindFatal
)

// String returns a human-readable representation of the error kind.
// String 返回错误类型的可读表示。
func (k ErrorKind) String() string {
	switch k {
	case ErrorKindRespondToModel:
		return "respond_to_model"
	case ErrorKindFatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// FunctionCallError is an error returned by a tool executor. The Kind field
// FunctionCallError 是工具执行器返回的错误。Kind 字段告诉代理
// tells the agent whether to feed the error back to the model or abort the
// 是将错误反馈给模型还是完全中止轮次。
// turn entirely.
type FunctionCallError struct {
	// Kind determines how the agent should handle this error.
	// Kind 决定代理应如何处理此错误。
	Kind ErrorKind
	// Message describes what went wrong.
	// Message 描述出了什么问题。
	Message string
}

// Error implements the error interface.
// Error 实现 error 接口。
func (e *FunctionCallError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Kind, e.Message)
}

// NewRespondToModelError creates a FunctionCallError that should be reported
// NewRespondToModelError 创建一个 FunctionCallError，应将其作为工具调用输出
// to the model as the tool call output, allowing the model to adjust and retry.
// 报告给模型，允许模型调整并重试。
func NewRespondToModelError(msg string) *FunctionCallError {
	return &FunctionCallError{
		Kind:    ErrorKindRespondToModel,
		Message: msg,
	}
}

// NewFatalError creates a fatal FunctionCallError that should abort the
// NewFatalError 创建一个应中止当前轮次的致命 FunctionCallError。
// current turn.
func NewFatalError(msg string) *FunctionCallError {
	return &FunctionCallError{
		Kind:    ErrorKindFatal,
		Message: msg,
	}
}
