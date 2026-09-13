package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/H-H1/actx/internal/protocol"
)

// API style identifiers selecting the wire format used by Stream.
// API 风格标识，选择 Stream 使用的传输格式。
const (
	// APIStyleChat routes requests to the /chat/completions endpoint
	// (OpenAI-compatible Chat Completions, including DeepSeek's /v1 base).
	// APIStyleChat 将请求路由到 /chat/completions 端点
	// （OpenAI 兼容的 Chat Completions，包括 DeepSeek 的 /v1 基地址）。
	APIStyleChat = "chat"
	// APIStyleResponses routes requests to the /responses endpoint
	// (OpenAI Responses API, including DeepSeek's Responses-compatible base).
	// APIStyleResponses 将请求路由到 /responses 端点
	// （OpenAI Responses API，包括 DeepSeek 的 Responses 兼容基地址）。
	APIStyleResponses = "responses"
)

// responsesRequest is the JSON body sent to the /responses endpoint.
// responsesRequest 是发送到 /responses 端点的 JSON 请求体。
type responsesRequest struct {
	// Model is the model identifier for the request.
	// Model 是请求的模型标识符。
	Model string `json:"model"`
	// Input is the list of input items (conversation history).
	// Input 是输入项列表（对话历史）。
	Input []responsesItem `json:"input"`
	// Instructions becomes the first system message server-side.
	// Instructions 会成为服务端的第一条 system 消息。
	Instructions string `json:"instructions,omitempty"`
	// Tools advertises callable functions to the model.
	// Tools 向模型声明可调用的函数。
	Tools []responsesTool `json:"tools,omitempty"`
	// Reasoning selects the reasoning effort when non-empty.
	// Reasoning 非空时选择推理投入程度。
	Reasoning *responsesReasoning `json:"reasoning,omitempty"`
	// Stream enables SSE streaming.
	// Stream 启用 SSE 流式返回。
	Stream bool `json:"stream"`
}

// responsesReasoning configures reasoning effort on the Responses API.
// responsesReasoning 配置 Responses API 的推理投入程度。
type responsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// responsesItem is one input item in the /responses request body.
// DeepSeek accepts messages with plain-string content, function_call /
// function_call_output items, and reasoning items whose content is a block
// array of reasoning_text.
// responsesItem 是 /responses 请求体中的一个输入项。
// DeepSeek 接受纯字符串 content 的 message、function_call /
// function_call_output 项，以及 content 为 reasoning_text 块数组的
// reasoning 项。
type responsesItem struct {
	// Type is the item type: message / function_call / function_call_output /
	// reasoning / custom_tool_call / custom_tool_call_output.
	// Type 是项类型：message / function_call / function_call_output /
	// reasoning / custom_tool_call / custom_tool_call_output。
	Type string `json:"type"`
	// Role is the message author (user / assistant / system / developer).
	// Role 是消息作者（user / assistant / system / developer）。
	Role string `json:"role,omitempty"`
	// Content carries the item body: a plain JSON string for message items,
	// or a block array ([{type, text}]) for reasoning items. The any type
	// keeps a single wire field for both shapes.
	// Content 携带项主体：message 项为纯 JSON 字符串，reasoning 项为
	// 块数组（[{type, text}]）。用 any 类型让两种形态共用同一线上字段。
	Content any `json:"content,omitempty"`
	// CallID pairs function_call and function_call_output items.
	// CallID 配对 function_call 与 function_call_output 项。
	CallID string `json:"call_id,omitempty"`
	// Name is the function name for function_call items.
	// Name 是 function_call 项的函数名称。
	Name string `json:"name,omitempty"`
	// Arguments is the JSON-encoded arguments for function_call items.
	// Arguments 是 function_call 项的 JSON 编码参数。
	Arguments string `json:"arguments,omitempty"`
	// Output is the tool result for function_call_output items. It is a
	// pointer so the field is always serialized for function_call_output
	// items even when the result is an empty string: DeepSeek's Responses
	// API rejects the request with "missing field `output`" when the field
	// is absent (a plain string + omitempty would drop it).
	// Output 是 function_call_output 项的工具结果。用指针类型保证
	// function_call_output 项即使结果为空字符串也始终序列化该字段：
	// DeepSeek Responses API 在字段缺失时报 "missing field `output`"
	// （普通 string + omitempty 会丢掉空值字段）。
	Output *string `json:"output,omitempty"`
}

// responsesTool is the flat Responses API function tool definition.
// responsesTool 是扁平的 Responses API 函数工具定义。
type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// responsesStreamEvent is the union shape of every SSE event payload from
// the /responses stream; only the fields relevant to a given event type are
// populated. The event type is carried in the JSON "type" field.
// responsesStreamEvent 是 /responses 流中所有 SSE 事件负载的联合结构；
// 仅填充与具体事件类型相关的字段。事件类型由 JSON 的 "type" 字段携带。
type responsesStreamEvent struct {
	// Type is the SSE event type, e.g. "response.output_text.delta".
	// Type 是 SSE 事件类型，例如 "response.output_text.delta"。
	Type string `json:"type"`
	// Delta carries incremental text for "*delta" events.
	// Delta 为 "*delta" 事件携带增量文本。
	Delta string `json:"delta"`
	// Item holds the completed output item for output_item.done events.
	// Item 为 output_item.done 事件携带完成的输出项。
	Item *responsesOutputItem `json:"item"`
	// Response wraps usage and error info on terminal events.
	// Response 在终止事件上包装用量与错误信息。
	Response *responsesOutput `json:"response"`
}

// responsesOutput is the top-level response object carried by terminal
// events (response.completed / incomplete / failed).
// responsesOutput 是终止事件（response.completed / incomplete / failed）
// 携带的顶层响应对象。
type responsesOutput struct {
	// Usage reports token usage.
	// Usage 报告令牌用量。
	Usage *Usage `json:"usage"`
	// Error describes the failure for response.failed events.
	// Error 描述 response.failed 事件的失败原因。
	Error *responsesError `json:"error"`
}

// responsesError is the error detail on response.failed events.
// responsesError 是 response.failed 事件上的错误详情。
type responsesError struct {
	Message string `json:"message"`
}

// responsesOutputItem is an output item in a /responses stream event.
// responsesOutputItem 是 /responses 流事件中的输出项。
type responsesOutputItem struct {
	// Type is the item type: message / function_call / reasoning /
	// custom_tool_call / web_search_call.
	// Type 是项类型：message / function_call / reasoning /
	// custom_tool_call / web_search_call。
	Type string `json:"type"`
	// Role is the message author.
	// Role 是消息作者。
	Role string `json:"role,omitempty"`
	// Content holds output_text blocks for message items.
	// Content 为 message 项持有 output_text 块。
	Content []responsesContentBlock `json:"content,omitempty"`
	// Summary holds summary blocks for reasoning items.
	// Summary 为 reasoning 项持有摘要块。
	Summary []responsesContentBlock `json:"summary,omitempty"`
	// CallID identifies the function call.
	// CallID 标识函数调用。
	CallID string `json:"call_id,omitempty"`
	// Name is the function name.
	// Name 是函数名称。
	Name string `json:"name,omitempty"`
	// Arguments is the JSON-encoded call arguments.
	// Arguments 是 JSON 编码的调用参数。
	Arguments string `json:"arguments,omitempty"`
}

// responsesContentBlock is one content block of an output item.
// responsesContentBlock 是输出项的一个内容块。
type responsesContentBlock struct {
	// Type is the block type, e.g. "output_text" / "summary_text" /
	// "reasoning_text".
	// Type 是块类型，例如 "output_text" / "summary_text" / "reasoning_text"。
	Type string `json:"type,omitempty"`
	// Text is the block's text content.
	// Text 是块的文本内容。
	Text string `json:"text,omitempty"`
}

// SetAPIStyle selects the wire format used by Stream. An empty or unknown
// value falls back to the Chat Completions format.
// SetAPIStyle 选择 Stream 使用的传输格式。空值或未知值回退到
// Chat Completions 格式。
func (c *ModelClient) SetAPIStyle(style string) {
	if style != APIStyleResponses {
		style = APIStyleChat
	}
	c.apiStyle = style
}

// streamResponses sends a prompt to the /responses endpoint and returns a
// channel of ResponseEvent values parsed from the SSE stream. The channel
// closes when the stream ends (response.completed / incomplete / failed or
// EOF) or the context is cancelled. Contract mirrors Stream.
// streamResponses 向 /responses 端点发送提示，并返回从 SSE 流中解析出的
// ResponseEvent 值通道。流结束（response.completed / incomplete / failed
// 或 EOF）或 context 取消时通道关闭。契约与 Stream 一致。
func (c *ModelClient) streamResponses(ctx context.Context, prompt Prompt, model string) (<-chan ResponseEvent, error) {
	// Build the /responses request body from the prompt.
	// 从提示构建 /responses 请求体。
	reqBody := responsesRequest{
		Model:        model,
		Input:        buildResponsesInput(prompt.Input),
		Instructions: prompt.Instructions,
		Tools:        buildResponsesTools(prompt.Tools),
		Stream:       true,
	}
	if prompt.ReasoningEffort != "" {
		reqBody.Reasoning = &responsesReasoning{Effort: prompt.ReasoningEffort}
	}

	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	// The /responses endpoint hangs off the configured base URL; trim any
	// trailing slash to avoid "//".
	// /responses 端点挂在配置的基地址下；去除尾部斜杠避免 "//"。
	url := strings.TrimRight(c.baseURL, "/") + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, parseAPIError(resp)
	}

	events := make(chan ResponseEvent)
	scanner := NewSSEScanner(resp.Body)

	go func() {
		defer resp.Body.Close()
		defer close(events)

		for scanner.Next() {
			data := scanner.Data()
			if data == "" {
				continue
			}

			var ev responsesStreamEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}

			switch ev.Type {
			case "response.output_text.delta":
				if ev.Delta != "" {
					if !c.emit(ctx, events, ResponseEvent{Type: EventOutputTextDelta, DeltaText: ev.Delta}) {
						return
					}
				}

			case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
				// DeepSeek streams chain-of-thought via reasoning_text.delta;
				// OpenAI uses reasoning_summary_text.delta. Map both to the
				// internal EventReasoningDelta event.
				// DeepSeek 通过 reasoning_text.delta 流式输出思维链；
				// OpenAI 使用 reasoning_summary_text.delta。两者都映射到
				// 内部的 EventReasoningDelta 事件。
				if ev.Delta != "" {
					if !c.emit(ctx, events, ResponseEvent{Type: EventReasoningDelta, DeltaReasoning: ev.Delta}) {
						return
					}
				}

			case "response.output_item.done":
				if ev.Item == nil {
					continue
				}
				if item, ok := ev.Item.toResponseItem(); ok {
					if !c.emit(ctx, events, ResponseEvent{Type: EventOutputItemDone, Item: &item}) {
						return
					}
				}

			case "response.completed", "response.incomplete":
				// Terminal events carry usage; incomplete means the response
				// was truncated (e.g. max_output_tokens) but any output items
				// already delivered remain valid.
				// 终止事件携带用量；incomplete 表示响应被截断（例如达到
				// max_output_tokens），但已交付的输出项仍然有效。
				if ev.Response != nil && ev.Response.Usage != nil {
					if !c.emit(ctx, events, ResponseEvent{Type: EventCompleted, Usage: ev.Response.Usage}) {
						return
					}
				}
				// Terminal events end the stream: unlike Chat Completions
				// there is no [DONE] sentinel, and the server may keep the
				// SSE connection open afterwards. Stop reading here instead
				// of waiting for EOF or the HTTP client timeout.
				// 终止事件即流结束：与 Chat Completions 不同，这里没有
				// [DONE] 哨兵，且服务器之后可能保持 SSE 连接打开。
				// 在此停止读取，而不是等待 EOF 或 HTTP 客户端超时。
				return

			case "response.failed":
				msg := "responses stream failed"
				if ev.Response != nil && ev.Response.Error != nil && ev.Response.Error.Message != "" {
					msg = ev.Response.Error.Message
				}
				item := protocol.NewAssistantMessage(msg)
				if !c.emit(ctx, events, ResponseEvent{Type: EventFailed, Item: &item}) {
					return
				}
				// response.failed is also terminal; stop reading.
				// response.failed 同样是终止事件；停止读取。
				return

			case "error":
				msg := "responses stream error"
				item := protocol.NewAssistantMessage(msg)
				if !c.emit(ctx, events, ResponseEvent{Type: EventError, Item: &item}) {
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case <-ctx.Done():
			case events <- ResponseEvent{Type: EventError}:
			}
		}
	}()

	return events, nil
}

// emit sends ev on the channel unless the context is cancelled. It reports
// whether the send succeeded.
// emit 在 context 未取消时把 ev 发送到通道。返回发送是否成功。
func (c *ModelClient) emit(ctx context.Context, ch chan<- ResponseEvent, ev ResponseEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
	}
}

// toResponseItem converts a /responses output item into a protocol
// ResponseItem. Unsupported item types (web_search_call, custom_tool_call)
// return ok=false and are skipped.
// toResponseItem 把 /responses 输出项转换为 protocol ResponseItem。
// 不支持的项类型（web_search_call、custom_tool_call）返回 ok=false 并被跳过。
func (item *responsesOutputItem) toResponseItem() (protocol.ResponseItem, bool) {
	switch item.Type {
	case "message":
		// Concatenate all text blocks (normally a single output_text).
		// 拼接所有文本块（通常只有一个 output_text）。
		var b strings.Builder
		for _, blk := range item.Content {
			b.WriteString(blk.Text)
		}
		text := b.String()
		if text == "" {
			return protocol.ResponseItem{}, false
		}
		role := item.Role
		if role == "" {
			role = "assistant"
		}
		return protocol.ResponseItem{
			Type: protocol.RITypeMessage,
			Role: role,
			Content: []protocol.ContentBlock{
				{Type: protocol.CTOutputText, Text: text},
			},
		}, true

	case "function_call":
		return protocol.NewFunctionCall(
			protocol.CallId(item.CallID),
			item.Name,
			item.Arguments,
		), true

	case "reasoning":
		// Prefer the summary blocks; fall back to plaintext content blocks
		// (DeepSeek reasoning items carry reasoning_text content).
		// 优先使用摘要块；回退到明文内容块（DeepSeek 的 reasoning 项携带
		// reasoning_text 内容）。
		var b strings.Builder
		for _, blk := range item.Summary {
			b.WriteString(blk.Text)
		}
		if b.Len() == 0 {
			for _, blk := range item.Content {
				b.WriteString(blk.Text)
			}
		}
		if b.Len() == 0 {
			return protocol.ResponseItem{}, false
		}
		return protocol.NewReasoning(b.String()), true
	}
	return protocol.ResponseItem{}, false
}

// buildResponsesInput converts the conversation history into /responses
// input items. System messages are skipped (use the instructions parameter
// instead); reasoning items are echoed back as reasoning_text blocks, which
// DeepSeek's thinking models require in front of their function calls.
// buildResponsesInput 把对话历史转换为 /responses 输入项。跳过 system
// 消息（改用 instructions 参数）；reasoning 项以 reasoning_text 块回传，
// DeepSeek 思维链模型要求其 function_call 前带有对应推理内容。
func buildResponsesInput(items []protocol.ResponseItem) []responsesItem {
	out := make([]responsesItem, 0, len(items))
	for _, item := range items {
		switch {
		case item.IsMessage():
			// Skip system messages: instructions already carry them.
			// 跳过 system 消息：instructions 已携带它们。
			if item.Role == "system" {
				continue
			}
			role := item.Role
			if role == "" {
				role = "user"
			}
			out = append(out, responsesItem{
				Type:    "message",
				Role:    role,
				Content: item.GetText(),
			})

		case item.Type == protocol.RITypeFunctionCall:
			out = append(out, responsesItem{
				Type:      "function_call",
				CallID:    string(item.CallID),
				Name:      item.Name,
				Arguments: item.Arguments,
			})

		case item.Type == protocol.RITypeFunctionCallOutput || item.Type == protocol.RITypeCustomToolCallOutput:
			// Always serialize the output field, even when empty.
			// 始终序列化 output 字段，即使为空。
			output := item.Output
			out = append(out, responsesItem{
				Type:   "function_call_output",
				CallID: string(item.CallID),
				Output: &output,
			})

		case item.Type == protocol.RITypeReasoning:
			// Echo reasoning items back as reasoning_text blocks: DeepSeek's
			// Responses API validates that thinking-model function calls are
			// preceded by their reasoning, and rejects requests where the
			// reasoning is missing with a 400 ("The reasoning_text ...").
			// Empty reasoning items are still skipped.
			// 把 reasoning 项以 reasoning_text 块回传：DeepSeek 的
			// Responses API 校验思维链模型的 function_call 前须有对应
			// reasoning，缺失时以 400 拒绝（"The reasoning_text ..."）。
			// 空的 reasoning 项仍然跳过。
			if item.Summary == "" {
				continue
			}
			out = append(out, responsesItem{
				Type: "reasoning",
				Content: []responsesContentBlock{
					{Type: "reasoning_text", Text: item.Summary},
				},
			})
		}
	}
	return out
}

// buildResponsesTools converts ToolSpecs to the flat Responses API tool
// definition format.
// buildResponsesTools 将 ToolSpec 转换为扁平的 Responses API 工具定义格式。
func buildResponsesTools(specs []ToolSpec) []responsesTool {
	tools := make([]responsesTool, 0, len(specs))
	for _, s := range specs {
		tools = append(tools, responsesTool{
			Type:        "function",
			Name:        s.Name,
			Description: s.Description,
			Parameters:  s.Parameters,
		})
	}
	return tools
}
