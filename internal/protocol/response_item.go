package protocol

import "encoding/json"

// ResponseItemType discriminates the kind of a ResponseItem.
// ResponseItemType 区分 ResponseItem 的类型。
type ResponseItemType string

const (
	RITypeMessage              ResponseItemType = "message"
	RITypeFunctionCall         ResponseItemType = "function_call"
	RITypeFunctionCallOutput   ResponseItemType = "function_call_output"
	RITypeReasoning            ResponseItemType = "reasoning"
	RITypeCustomToolCall       ResponseItemType = "custom_tool_call"
	RITypeCustomToolCallOutput ResponseItemType = "custom_tool_call_output"
)

// ResponseItem represents an item in the conversation history sent to the model.
// ResponseItem 表示发送给模型的对话历史中的一个项目。
// It maps to the OpenAI Responses API input item format.
// 它映射到 OpenAI Responses API 的输入项格式。
type ResponseItem struct {
	// Type discriminates the kind of this item.
	// Type 区分此项目的类型。
	Type ResponseItemType `json:"type"`

	// Message fields
	// 消息字段
	// Role is the message author, e.g. "user", "assistant", or "system".
	// Role 是消息的作者，例如 "user"、"assistant" 或 "system"。
	Role string `json:"role,omitempty"`
	// Content holds the content blocks of the message.
	// Content 持有消息的内容块。
	Content []ContentBlock `json:"content,omitempty"`

	// FunctionCall fields
	// FunctionCall 字段
	// CallID is the ID of the function call.
	// CallID 是函数调用的 ID。
	CallID CallId `json:"call_id,omitempty"`
	// Name is the name of the called function.
	// Name 是被调用函数的名称。
	Name string `json:"name,omitempty"`
	// Arguments is the JSON-encoded call arguments.
	// Arguments 是 JSON 编码的调用参数。
	Arguments string `json:"arguments,omitempty"`

	// FunctionCallOutput fields
	// FunctionCallOutput 字段
	// Output is the output of the function call.
	// Output 是函数调用的输出。
	Output string `json:"output,omitempty"`

	// Reasoning fields
	// 推理字段
	// Summary is the summary of the reasoning.
	// Summary 是推理的摘要。
	Summary string `json:"summary,omitempty"`

	// ID of the item (for referencing in the API)
	// 项目的 ID（用于在 API 中引用）
	ID string `json:"id,omitempty"`

	// Status of the item
	// 项目的状态
	Status string `json:"status,omitempty"`
}

// ContentBlockType discriminates content block types within a message.
// ContentBlockType 区分消息中的内容块类型。
type ContentBlockType string

const (
	CTText       ContentBlockType = "text"
	CTInputText  ContentBlockType = "input_text"
	CTOutputText ContentBlockType = "output_text"
	CTImage      ContentBlockType = "input_image"
)

// ContentBlock represents a single content block within a message.
// ContentBlock 表示消息中的单个内容块。
type ContentBlock struct {
	// Type discriminates the content block type.
	// Type 区分内容块的类型。
	Type ContentBlockType `json:"type"`
	// Text is the text content (for text blocks).
	// Text 是文本内容（用于文本块）。
	Text string `json:"text,omitempty"`
	// ImageURL is the image URL (for image blocks).
	// ImageURL 是图像 URL（用于图像块）。
	ImageURL string `json:"image_url,omitempty"`
}

// NewUserMessage creates a user message ResponseItem with text content.
// NewUserMessage 创建一个带有文本内容的用户消息 ResponseItem。
func NewUserMessage(text string) ResponseItem {
	return ResponseItem{
		Type: RITypeMessage,
		Role: "user",
		Content: []ContentBlock{
			{Type: CTInputText, Text: text},
		},
	}
}

// NewAssistantMessage creates an assistant message ResponseItem with text content.
// NewAssistantMessage 创建一个带有文本内容的助手消息 ResponseItem。
func NewAssistantMessage(text string) ResponseItem {
	return ResponseItem{
		Type: RITypeMessage,
		Role: "assistant",
		Content: []ContentBlock{
			{Type: CTOutputText, Text: text},
		},
	}
}

// NewSystemMessage creates a system message ResponseItem.
// NewSystemMessage 创建一个系统消息 ResponseItem。
func NewSystemMessage(text string) ResponseItem {
	return ResponseItem{
		Type: RITypeMessage,
		Role: "system",
		Content: []ContentBlock{
			{Type: CTInputText, Text: text},
		},
	}
}

// NewFunctionCall creates a function call ResponseItem.
// NewFunctionCall 创建一个函数调用 ResponseItem。
func NewFunctionCall(callID CallId, name, arguments string) ResponseItem {
	return ResponseItem{
		Type:      RITypeFunctionCall,
		CallID:    callID,
		Name:      name,
		Arguments: arguments,
	}
}

// NewFunctionCallOutput creates a function call output ResponseItem.
// NewFunctionCallOutput 创建一个函数调用输出 ResponseItem。
func NewFunctionCallOutput(callID CallId, output string) ResponseItem {
	return ResponseItem{
		Type:   RITypeFunctionCallOutput,
		CallID: callID,
		Output: output,
	}
}

// NewReasoning creates a reasoning ResponseItem.
// NewReasoning 创建一个推理 ResponseItem。
func NewReasoning(summary string) ResponseItem {
	return ResponseItem{
		Type:    RITypeReasoning,
		Summary: summary,
	}
}

// IsFunctionCall returns true if the item is a function call.
// IsFunctionCall 如果该项目是函数调用则返回 true。
func (ri ResponseItem) IsFunctionCall() bool {
	return ri.Type == RITypeFunctionCall || ri.Type == RITypeCustomToolCall
}

// IsMessage returns true if the item is a message.
// IsMessage 如果该项目是消息则返回 true。
func (ri ResponseItem) IsMessage() bool {
	return ri.Type == RITypeMessage
}

// GetText returns the text content of a message item, or empty string.
// GetText 返回消息项目的文本内容，如果没有则返回空字符串。
func (ri ResponseItem) GetText() string {
	for _, c := range ri.Content {
		if c.Text != "" {
			return c.Text
		}
	}
	return ""
}

// MarshalJSON implements custom JSON marshaling for ResponseItem.
// MarshalJSON 实现 ResponseItem 的自定义 JSON 序列化。
func (ri ResponseItem) MarshalJSON() ([]byte, error) {
	type alias ResponseItem
	return json.Marshal(alias(ri))
}

// UnmarshalJSON implements custom JSON unmarshaling for ResponseItem.
// UnmarshalJSON 实现 ResponseItem 的自定义 JSON 反序列化。
func (ri *ResponseItem) UnmarshalJSON(data []byte) error {
	type alias ResponseItem
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*ri = ResponseItem(a)
	return nil
}
