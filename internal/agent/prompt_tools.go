package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
)

// toolCallBlockStart 和 toolCallBlockEnd 标记模型输出中工具调用的起止。
const (
	toolCallBlockStart = "```tool"
	toolCallBlockEnd   = "```"
)

// fallbackKeywords 是触发自动降级到 prompt 模式的错误关键词。
// 当 API 返回 400 且错误信息包含这些关键词时，说明模型不支持 tools 参数。
var fallbackKeywords = []string{
	"tools",
	"function",
	"reasoning_content",
	"reasoning_content is not supported",
	"tool_calls",
	"function_calling",
	"not support",
	"unsupported parameter",
}

// shouldFallbackToPrompt 检查错误是否表明模型不支持 API 函数调用。
// 当返回 true 时，应自动从 api 模式降级到 prompt 模式。
func shouldFallbackToPrompt(err error) bool {
	var apiErr *model.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != 400 {
		return false
	}
	msg := strings.ToLower(apiErr.Message)
	for _, kw := range fallbackKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// ParsedToolCall represents a tool call parsed from the model's text output.
// ParsedToolCall 表示从模型文本中解析出的工具调用。
type ParsedToolCall struct {
	// Name is the name of the tool to invoke.
	// Name 是要调用的工具名称。
	Name string
	// Arguments is the raw JSON arguments string for the call.
	// Arguments 是该调用的原始 JSON 参数字符串。
	Arguments string
}

// BuildToolPrompt 生成注入系统提示词的工具描述文本。
// 当模型不支持 API 函数调用时，用此文本让模型通过文本输出调用工具。
func BuildToolPrompt(specs []model.ToolSpec) string {
	if len(specs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## 可用工具\n\n")
	b.WriteString("你可以使用以下工具完成任务。需要调用工具时，在回复中输出如下格式的代码块：\n\n")
	b.WriteString("```tool\n")
	b.WriteString(`{"name": "工具名", "arguments": {"参数名": "参数值"}}`)
	b.WriteString("\n```\n\n")
	b.WriteString("重要：arguments 必须是严格合法的 JSON：\n")
	b.WriteString("- 字符串值必须用双引号括起来，不允许裸露的标识符（如 {\"command\": dir} 是非法的，应写 {\"command\": \"dir\"}）。\n")
	b.WriteString("- 数组类型的参数（如 shell 的 command）必须写成 JSON 数组：{\"command\": [\"dir\"]}。\n")
	b.WriteString("示例（执行 shell 命令）：\n\n```tool\n")
	b.WriteString(`{"name": "shell", "arguments": {"command": ["dir"]}}`)
	b.WriteString("\n```\n\n")
	b.WriteString("每次只调用一个工具，等待工具执行结果后再决定下一步操作。\n\n")

	for i, spec := range specs {
		b.WriteString(fmt.Sprintf("### %d. %s\n", i+1, spec.Name))
		if spec.Description != "" {
			b.WriteString(spec.Description)
			b.WriteString("\n")
		}
		if len(spec.Parameters) > 0 {
			b.WriteString("参数 (JSON Schema):\n```json\n")
			b.Write(spec.Parameters)
			b.WriteString("\n```\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// toolBlockRe 匹配 ```tool ... ``` 代码块。
var toolBlockRe = regexp.MustCompile("(?s)```tool\\s*\n(.*?)```")

// ParseToolCallsFromText 从模型的文本回复中提取工具调用。
// 返回解析出的工具调用列表和去除工具调用块后的纯文本。
func ParseToolCallsFromText(text string) ([]ParsedToolCall, string) {
	matches := toolBlockRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, text
	}

	var calls []ParsedToolCall
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		var parsed struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			continue
		}
		if parsed.Name == "" {
			continue
		}
		calls = append(calls, ParsedToolCall{
			Name:      parsed.Name,
			Arguments: string(parsed.Arguments),
		})
	}

	// 从原文中移除工具调用块，保留纯文本部分。
	cleanText := toolBlockRe.ReplaceAllString(text, "")
	cleanText = strings.TrimSpace(cleanText)

	return calls, cleanText
}

// ParsedToolCallToResponseItem 将解析出的工具调用转换为 protocol.ResponseItem，
// 使其与 API 函数调用走相同的执行路径。
func (c ParsedToolCall) ToResponseItem(callID protocol.CallId) protocol.ResponseItem {
	return protocol.NewFunctionCall(callID, c.Name, c.Arguments)
}
