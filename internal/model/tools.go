package model

import "encoding/json"

// Tool type identifiers accepted by the Responses API.
// Responses API 接受的工具类型标识符。
const (
	// ToolTypeFunction identifies a function/tool the model may call.
	// ToolTypeFunction 标识模型可以调用的函数/工具。
	ToolTypeFunction = "function"
)

// ToolSpec describes a tool the model may invoke. It maps to the flat tool
// object format used by the OpenAI Responses API, for example:
//
//	{"type":"function","name":"get_weather","description":"...","parameters":{...}}
//
// Parameters must be a JSON Schema document describing the function arguments.
// It is stored as json.RawMessage so callers can pass pre-serialized schemas
// without a round-trip through an intermediate type.
// ToolSpec 描述模型可能调用的工具。它映射到 OpenAI Responses API 使用的扁平工具
// 对象格式，例如：
//
//	{"type":"function","name":"get_weather","description":"...","parameters":{...}}
//
// Parameters 必须是描述函数参数的 JSON Schema 文档。它以 json.RawMessage 形式存储，
// 以便调用者可以传递预序列化的模式，无需经过中间类型的往返转换。
type ToolSpec struct {
	Type        string
	Name        string
	Description string
	Parameters  json.RawMessage
}

// toolSpecWire is the on-the-wire representation of a ToolSpec with JSON tags.
// It is kept separate from ToolSpec so the public struct fields remain tag-free
// and easy to construct in Go, while serialization stays under explicit control.
// toolSpecWire 是带有 JSON 标签的 ToolSpec 线上表示形式。它与 ToolSpec 保持分离，
// 以便公共结构体字段保持无标签且易于在 Go 中构造，同时序列化保持显式控制。
type toolSpecWire struct {
	// Type is the tool type identifier.
	// Type 是工具类型标识符。
	Type string `json:"type"`
	// Name is the function name.
	// Name 是函数名称。
	Name string `json:"name,omitempty"`
	// Description is the human-readable description of the tool.
	// Description 是工具的人类可读描述。
	Description string `json:"description,omitempty"`
	// Parameters is the JSON Schema describing the function arguments.
	// Parameters 是描述函数参数的 JSON Schema。
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

// MarshalJSON produces the API-compatible flat JSON for a tool. This is the
// ToolSpec marshal implementation: it emits the Responses-API shape so that a
// []ToolSpec can be embedded directly in the request body.
// MarshalJSON 生成与 API 兼容的工具扁平 JSON。这是 ToolSpec 的 marshal 实现：
// 它发出 Responses-API 的形状，使得 []ToolSpec 可以直接嵌入请求体中。
func (t ToolSpec) MarshalJSON() ([]byte, error) {
	return json.Marshal(toolSpecWire{
		Type:        t.Type,
		Name:        t.Name,
		Description: t.Description,
		Parameters:  t.Parameters,
	})
}

// UnmarshalJSON parses an API tool object back into a ToolSpec.
// UnmarshalJSON 将 API 工具对象解析回 ToolSpec。
func (t *ToolSpec) UnmarshalJSON(data []byte) error {
	var w toolSpecWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	t.Type = w.Type
	t.Name = w.Name
	t.Description = w.Description
	t.Parameters = w.Parameters
	return nil
}

// NewFunctionTool is a convenience constructor that builds a function ToolSpec
// from a name, a human-readable description, and a JSON Schema describing the
// parameters. The returned spec is ready to be added to a Prompt's Tools slice.
// NewFunctionTool 是一个便捷构造函数，从名称、人类可读的描述和描述参数的 JSON Schema
// 构建函数 ToolSpec。返回的 spec 可以直接添加到 Prompt 的 Tools 切片中。
func NewFunctionTool(name, description string, params json.RawMessage) ToolSpec {
	return ToolSpec{
		Type:        ToolTypeFunction,
		Name:        name,
		Description: description,
		Parameters:  params,
	}
}
