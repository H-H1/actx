// Package model implements an OpenAI Responses API client with Server-Sent
// Events (SSE) streaming support. It is used by the ACTX CLI agent to send
// prompts to the model and consume streamed responses, tool calls, and token
// usage as a stream of ResponseEvent values.
// Package model 实现了具有 Server-Sent Events (SSE) 流支持功能的 OpenAI Responses API
// 客户端。ACTX CLI 代理使用它向模型发送提示，并以 ResponseEvent 值流的形式消费流式
// 响应、工具调用和令牌用量。
package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/H-H1/actx/internal/protocol"
)

// DefaultBaseURL is the default OpenAI API base URL used when no baseURL is
// provided to NewModelClient.
// DefaultBaseURL 是未向 NewModelClient 提供 baseURL 时使用的默认 OpenAI API 基础 URL。
const DefaultBaseURL = "https://api.deepseek.com"

// SSE event types emitted by the Responses API. These are exposed as the Type
// field of ResponseEvent so callers can switch on them without hardcoding
// strings.
// Responses API 发出的 SSE 事件类型。这些作为 ResponseEvent 的 Type 字段暴露，
// 以便调用者可以在不硬编码字符串的情况下对它们进行 switch 判断。
const (
	// EventResponseCreated 响应已创建：流式响应的首个事件，状态为 in_progress。
	// EventResponseCreated Response created: the first event of a streaming response, with status "in_progress".
	EventResponseCreated = "response.created"

	// EventResponseInProgress 响应进行中：响应正在生成。
	// EventResponseInProgress Response in progress: the response is currently being generated.
	EventResponseInProgress = "response.in_progress"

	// EventOutputItemAdded 输出项已添加：一个输出 item（如 message、reasoning、function_call）开始。
	// EventOutputItemAdded Output item added: an output item (e.g. message, reasoning, function_call) has started.
	EventOutputItemAdded = "response.output_item.added"

	// EventOutputItemDone 输出项已完成：一个输出 item 完成。
	// EventOutputItemDone Output item done: an output item has completed.
	EventOutputItemDone = "response.output_item.done"

	// EventContentPartAdded 内容块已添加：输出 item 中的一个内容块开始。
	// EventContentPartAdded Content part added: a content part within an output item has started.
	EventContentPartAdded = "response.content_part.added"

	// EventContentPartDone 内容块已完成：输出 item 中的一个内容块完成。
	// EventContentPartDone Content part done: a content part within an output item has completed.
	EventContentPartDone = "response.content_part.done"

	// EventOutputTextDelta 输出文本增量：每生成一小段输出文本就推送一次，用于实现打字机效果。
	// EventOutputTextDelta Output text delta: emitted each time a small chunk of output text is generated, used for typewriter-style streaming.
	EventOutputTextDelta = "response.output_text.delta"

	// EventOutputTextDone 输出文本已完成：输出文本完成，携带完整文本内容。
	// EventOutputTextDone Output text done: the output text is complete, carrying the full text content.
	EventOutputTextDone = "response.output_text.done"

	// EventReasoningDelta 推理摘要增量：推理摘要的文本增量，用于流式展示模型的思考过程摘要。
	// EventReasoningDelta Reasoning summary delta: a text delta of the reasoning summary, used to stream the model's summarized thought process.
	EventReasoningDelta = "response.reasoning_summary_text.delta"

	// EventFunctionCallArgsDelta 函数调用参数增量：函数调用参数的增量，逐步推送参数内容。
	// EventFunctionCallArgsDelta Function call arguments delta: an increment of function call arguments, streaming the arguments piece by piece.
	EventFunctionCallArgsDelta = "response.function_call_arguments.delta"

	// EventFunctionCallArgsDone 函数调用参数已完成：函数调用参数完成，携带完整参数。
	// EventFunctionCallArgsDone Function call arguments done: the function call arguments are complete, carrying the full arguments.
	EventFunctionCallArgsDone = "response.function_call_arguments.done"

	// EventRefusalDelta 拒绝文本增量：模型拒绝回答时，逐步推送拒绝内容。
	// EventRefusalDelta Refusal delta: emitted incrementally when the model refuses to answer, streaming the refusal content.
	EventRefusalDelta = "response.refusal.delta"

	// EventCompleted 响应已完成：响应正常完成，是最后一个事件，携带完整 response 对象和 usage 信息。
	// EventCompleted Response completed: the response finished successfully; the final event, carrying the full response object and usage info.
	EventCompleted = "response.completed"

	// EventIncomplete 标记被截断的响应（如达到 max_output_tokens）；Err 携带截断原因。
	// EventIncomplete marks a truncated response (e.g. reached max_output_tokens); Err carries the incomplete reason.
	EventIncomplete = "response.incomplete"

	// EventFailed 响应失败：响应失败，携带含 error 详情的 response 对象。
	// EventFailed Response failed: the response failed, carrying a response object with error details.
	EventFailed = "response.failed"

	// EventError 通用错误事件。
	// EventError Generic error event.
	EventError = "error"
)

// ModelClient is an HTTP client for the OpenAI Responses API. It is safe for
// concurrent use once constructed; the zero value is not usable, so callers
// should use NewModelClient.
// ModelClient 是 OpenAI Responses API 的 HTTP 客户端。构造后可以安全地并发使用；
// 零值不可用，因此调用者应使用 NewModelClient。
type ModelClient struct {
	// apiKey is the bearer token used to authenticate API requests.
	// apiKey 是用于认证 API 请求的 bearer 令牌。
	apiKey string
	// baseURL is the API root URL (scheme + host, no trailing slash required).
	// baseURL 是 API 根 URL（协议 + 主机，无需尾部斜杠）。
	baseURL string
	// model is the default model identifier used when a prompt does not override it.
	// model 是提示未指定模型时使用的默认模型标识符。
	model string
	// apiStyle selects the wire format: APIStyleChat (/chat/completions,
	// the default) or APIStyleResponses (/responses). Set via SetAPIStyle.
	// apiStyle 选择传输格式：APIStyleChat（/chat/completions，默认）或
	// APIStyleResponses（/responses）。通过 SetAPIStyle 设置。
	apiStyle string
	// httpClient performs the underlying HTTP requests with an overall timeout.
	// httpClient 执行带总体超时的底层 HTTP 请求。
	httpClient *http.Client
}

// NewModelClient creates a ModelClient configured to talk to the given model.
// If baseURL is empty, DefaultBaseURL is used. The returned client uses an
// *http.Client with an overall timeout so requests to invalid URLs or models
// do not hang indefinitely; rely on the context passed to Stream for
// cancellation of long-lived streams.
// NewModelClient 创建一个配置为与指定模型通信的 ModelClient。如果 baseURL 为空，
// 则使用 DefaultBaseURL。返回的客户端使用带总体超时的 *http.Client，
// 以便对无效 URL 或模型的请求不会无限挂起；请依赖传递给 Stream 的 context 来取消长时间运行的流。
func NewModelClient(apiKey, model, baseURL string) *ModelClient {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &ModelClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		// Default to the Chat Completions wire format for backwards
		// compatibility; call SetAPIStyle(APIStyleResponses) to switch.
		// 默认使用 Chat Completions 传输格式以保持向后兼容；
		// 调用 SetAPIStyle(APIStyleResponses) 可切换。
		apiStyle: APIStyleChat,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // 2 minute timeout for the entire request
		},
	}
}

// Prompt describes a single request to the Responses API. Input carries the
// conversation history, Tools advertises callable tools, and Instructions is
// the optional system/developer instructions. When Model is non-empty it
// overrides the client's default model for this request.
// Prompt 描述对 Responses API 的单次请求。Input 携带对话历史，Tools 声明可调用的工具，
// Instructions 是可选的系统/开发者指令。当 Model 为非空时，它会为此请求覆盖客户端的默认模型。
type Prompt struct {
	Input             []protocol.ResponseItem
	Tools             []ToolSpec
	Instructions      string
	ParallelToolCalls bool
	Model             string
	ReasoningEffort   string
}

// Usage reports token usage for a response. Field names match the Responses
// API JSON keys.
// Usage 报告响应的令牌用量。字段名与 Responses API 的 JSON 键匹配。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ResponseEvent is a single event parsed from the SSE stream. Only the fields
// relevant to the event's Type are populated; for example, DeltaText is set
// for "*delta" events, Item is set for "response.output_item.*" events, and
// Usage is set for the terminal "response.completed" event.
// ResponseEvent 是从 SSE 流中解析出的单个事件。仅填充与事件 Type 相关的字段；
// 例如，DeltaText 为 "*delta" 事件设置，Item 为 "response.output_item.*" 事件设置，
// Usage 为终止事件 "response.completed" 设置。
type ResponseEvent struct {
	// Type is the SSE event type (one of the Event* constants).
	// Type 是 SSE 事件类型（Event* 常量之一）。
	Type string
	// Item is the response item associated with item-level events.
	// Item 是与条目级事件关联的响应条目。
	Item *protocol.ResponseItem
	// DeltaText is the incremental text for "*delta" events.
	// DeltaText 是 "*delta" 事件的增量文本。
	DeltaText string
	// Usage is the token usage reported by the terminal event.
	// Usage 是终止事件报告的令牌用量。
	Usage *Usage
	// DeltaReasoning carries incremental reasoning content from models
	// that expose a reasoning_content stream (e.g. DeepSeek-R1, o1-style).
	// DeltaReasoning 携带来自暴露 reasoning_content 流的模型（如 DeepSeek-R1、o1 系列）的增量思考内容。
	DeltaReasoning string
	// Err carries the typed error for EventFailed / EventIncomplete /
	// EventError events (classifyResponsesError output or a plain error).
	// Err 为 EventFailed / EventIncomplete / EventError 事件携带类型化
	// 错误（classifyResponsesError 的输出或普通错误）。
	Err error
	// ItemID / CallID attribute delta events to their output item, so
	// concurrent streams (e.g. multiple tool calls) can be told apart.
	// ItemID / CallID 把增量事件归属到其输出项，使并发流（如多个工具
	// 调用）的增量可以相互区分。
	ItemID string
	CallID string
	// Trail carries a self-contained SSE event-trail fragment (complete
	// lines prefixed with [type]) for debugging. It rides on the event
	// itself so the consumer writes it single-threaded, in exact event
	// order, instead of racing with item logging from another goroutine.
	// Trail 携带自包含的 SSE 事件轨迹片段（带 [type] 前缀的完整行），
	// 用于调试。它随事件本身传递，由消费方单线程按真实事件顺序写入，
	// 避免与另一个 goroutine 的条目日志写入相互竞争乱序。
	Trail string
}

// chatRequest is the JSON body sent to the /chat/completions endpoint.
// chatRequest 是发送到 /chat/completions 端点的 JSON 请求体。
type chatRequest struct {
	// Model is the model identifier for the request.
	// Model 是请求的模型标识符。
	Model string `json:"model"`
	// Messages is the conversation messages.
	// Messages 是对话消息。
	Messages []chatMessage `json:"messages"`
	// Tools is the tool definitions available to the model.
	// Tools 是模型可用的工具定义。
	Tools []chatToolDef `json:"tools,omitempty"`
	// Stream indicates whether the response should be streamed.
	// Stream 指示响应是否应流式返回。
	Stream bool `json:"stream"`
}

// chatMessage represents a single message in the Chat Completions API.
// chatMessage 表示 Chat Completions API 中的单条消息。
type chatMessage struct {
	// Role is the message author role (system/user/assistant/tool).
	// Role 是消息作者角色（system/user/assistant/tool）。
	Role string `json:"role"`
	// Content is the message content as raw JSON (string or null).
	// Content 是以原始 JSON 表示的消息内容（字符串或 null）。
	Content json.RawMessage `json:"content,omitempty"`
	// ToolCalls is the tool calls issued by an assistant message.
	// ToolCalls 是 assistant 消息发起的工具调用。
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a tool message to the call it answers.
	// ToolCallID 将 tool 消息关联到它所应答的调用。
	ToolCallID string `json:"tool_call_id,omitempty"`
	// ReasoningContent carries the model's reasoning_content back to the
	// API in a subsequent request. DeepSeek-R1 (and o1-style) "thinking
	// mode" requires the prior reasoning_content to be echoed back so the
	// model can continue its reasoning context; omitting it triggers
	// "The reasoning_content in the thinking mode must be passed back to
	// the API." (HTTP 400).
	// ReasoningContent 把模型的 reasoning_content 在后续请求中回传给 API。
	// DeepSeek-R1（及 o1 系列）的"思考模式"要求把上一轮的 reasoning_content
	// 回传，模型才能延续其推理上下文；省略它会触发
	// "The reasoning_content in the thinking mode must be passed back to the API."（HTTP 400）。
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// chatToolCall represents a tool call in an assistant message.
// chatToolCall 表示助手消息中的工具调用。
type chatToolCall struct {
	// Index is the position of the call in the streamed tool-call list.
	// Index 是流式工具调用列表中该调用的位置。
	Index int `json:"index"`
	// ID is the unique identifier of the tool call.
	// ID 是工具调用的唯一标识符。
	ID string `json:"id"`
	// Type is the call type (always "function").
	// Type 是调用类型（始终为 "function"）。
	Type string `json:"type"`
	// Function holds the function name and serialized arguments.
	// Function 保存函数名称和序列化的参数。
	Function chatFunction `json:"function"`
}

// chatFunction holds the function name and arguments for a tool call.
// chatFunction 保存工具调用的函数名称和参数。
type chatFunction struct {
	// Name is the function name to call.
	// Name 是要调用的函数名称。
	Name string `json:"name"`
	// Arguments is the JSON-encoded function arguments.
	// Arguments 是 JSON 编码的函数参数。
	Arguments string `json:"arguments"`
}

// chatToolDef is the Chat Completions tool definition format.
// chatToolDef 是 Chat Completions 的工具定义格式。
type chatToolDef struct {
	// Type is the tool type (always "function").
	// Type 是工具类型（始终为 "function"）。
	Type string `json:"type"`
	// Function holds the function schema for the tool.
	// Function 保存工具的函数模式。
	Function chatToolFunc `json:"function"`
}

// chatToolFunc holds the function schema for tool definitions.
// chatToolFunc 保存工具定义的函数模式。
type chatToolFunc struct {
	// Name is the function name.
	// Name 是函数名称。
	Name string `json:"name"`
	// Description is the human-readable description of the function.
	// Description 是函数的人类可读描述。
	Description string `json:"description"`
	// Parameters is the JSON Schema describing the function arguments.
	// Parameters 是描述函数参数的 JSON Schema。
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

// chatChunk is a single SSE chunk from the Chat Completions stream.
// chatChunk 是 Chat Completions 流中的单个 SSE 块。
type chatChunk struct {
	// ID is the unique identifier of the completion.
	// ID 是补全的唯一标识符。
	ID string `json:"id"`
	// Object is the object type (e.g. "chat.completion.chunk").
	// Object 是对象类型（例如 "chat.completion.chunk"）。
	Object string `json:"object"`
	// Model is the model that generated the chunk.
	// Model 是生成该块的模型。
	Model string `json:"model"`
	// Choices is the choice list (usually a single element when streaming).
	// Choices 是选择列表（流式时通常为单个元素）。
	Choices []chatChoice `json:"choices"`
	// Usage is the token usage, typically present in the final chunk.
	// Usage 是令牌用量，通常出现在最后一个块中。
	Usage *chatUsage `json:"usage,omitempty"`
}

// chatChoice holds a single choice delta in a streaming chunk.
// chatChoice 保存流式块中的单个选择增量。
type chatChoice struct {
	// Index is the index of this choice.
	// Index 是该选择的索引。
	Index int `json:"index"`
	// Delta holds the incremental content of this choice.
	// Delta 保存该选择的增量内容。
	Delta chatDelta `json:"delta"`
	// FinishReason is the stop reason, set on the final chunk.
	// FinishReason 是停止原因，在最后一个块中设置。
	FinishReason string `json:"finish_reason"`
}

// chatDelta holds the delta content for a streaming chunk.
// chatDelta 保存流式块的增量内容。
type chatDelta struct {
	// Role is the message role, set in the first delta.
	// Role 是消息角色，在首个增量中设置。
	Role string `json:"role,omitempty"`
	// Content is the incremental assistant text.
	// Content 是 assistant 的增量文本。
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"` // DeepSeek/o1 系列等返回的思考增量内容
	// ToolCalls is the incremental tool-call fragments for this chunk.
	// ToolCalls 是该块的工具调用增量片段。
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

// chatUsage reports token usage in a Chat Completions response.
// chatUsage 报告 Chat Completions 响应中的令牌用量。
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Stream sends a prompt to the Chat Completions API and returns a channel of
// ResponseEvent values parsed from the SSE stream. The channel is closed when
// the stream completes, the [DONE] sentinel is received, or the context is
// cancelled.
//
// A non-nil error is returned only for request setup failures and non-2xx HTTP
// responses (including rate-limit and authentication errors). Errors that occur
// mid-stream are surfaced as a ResponseEvent with Type EventError before the
// channel is closed, unless the context was cancelled.
// Stream 向 Chat Completions API 发送提示，并返回一个从 SSE 流中解析出的 ResponseEvent
// 值通道。当流完成、收到 [DONE] 哨兵值或 context 被取消时，通道关闭。
//
// 仅在请求设置失败和非 2xx HTTP 响应（包括速率限制和认证错误）时返回非 nil 错误。
// 流中间发生的错误会在通道关闭前以 Type 为 EventError 的 ResponseEvent 形式呈现，
// 除非 context 已被取消。
func (c *ModelClient) Stream(ctx context.Context, prompt Prompt) (<-chan ResponseEvent, error) {
	// Resolve the model: the prompt's Model field takes precedence, falling
	// back to the client's default model when empty.
	// 解析模型：提示的 Model 字段优先，为空时回退到客户端的默认模型。
	model := prompt.Model
	if model == "" {
		model = c.model
	}

	// Dispatch to the /responses implementation when the client is
	// configured for the Responses API style; otherwise fall through to
	// Chat Completions below.
	// 当客户端配置为 Responses API 风格时分派到 /responses 实现；
	// 否则继续走下方的 Chat Completions。
	if c.apiStyle == APIStyleResponses {
		return c.streamResponses(ctx, prompt, model)
	}

	// Convert the conversation history (plus system instructions) and the
	// tool specs into Chat Completions wire format.
	// 将对话历史（含系统指令）和工具规范转换为 Chat Completions 传输格式。
	messages := buildChatMessages(prompt.Input, prompt.Instructions)
	tools := buildChatTools(prompt.Tools)

	// Assemble the request body; Stream=true enables SSE streaming.
	// 组装请求体；Stream=true 启用 SSE 流式返回。
	reqBody := chatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	}

	// Serialize the request body to JSON.
	// 将请求体序列化为 JSON。
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	// Build the endpoint URL (trim any trailing slash to avoid "//") and
	// create a POST request bound to ctx so cancellation propagates to the
	// in-flight HTTP call.
	// 构建端点 URL（去除尾部斜杠避免 "//"）并创建绑定到 ctx 的 POST 请求，
	// 使取消信号能传播到进行中的 HTTP 调用。
	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	// Set standard headers: JSON body, SSE response expected, bearer auth.
	// 设置标准请求头：JSON 请求体、期望 SSE 响应、bearer 认证。
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Send the HTTP request synchronously; only setup/transport errors fail
	// here, HTTP status errors are handled below.
	// 同步发送 HTTP 请求；此处仅设置/传输错误会失败，
	// HTTP 状态码错误在下方处理。
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Non-2xx responses are converted to typed errors (RateLimitError for
	// 429, AuthError for 401/403, APIError otherwise) before returning.
	// 非 2xx 响应在返回前被转换为类型化错误（429 为 RateLimitError，
	// 401/403 为 AuthError，其他为 APIError）。
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, parseAPIError(resp)
	}

	// Create the event channel consumed by the caller and an SSE scanner
	// over the response body. The channel is unbuffered: each event is
	// handed off directly to the consumer (backpressure).
	// 创建供调用者消费的事件通道以及基于响应体的 SSE 扫描器。
	// 通道无缓冲：每个事件直接交接给消费者（背压机制）。
	events := make(chan ResponseEvent)
	scanner := NewSSEScanner(resp.Body)

	// Consume the SSE stream in a background goroutine so Stream can return
	// the channel immediately; the goroutine closes events when done.
	// 在后台 goroutine 中消费 SSE 流，使 Stream 能立即返回通道；
	// goroutine 结束时关闭 events。
	go func() {
		// Ensure the HTTP body is closed and the events channel is closed
		// on every exit path (error, [DONE], context cancellation).
		// 确保在每条退出路径（错误、[DONE]、context 取消）上都关闭
		// HTTP 响应体和 events 通道。
		defer resp.Body.Close()
		defer close(events)

		// contentBuf accumulates text deltas until finish_reason "stop"
		// (or stream end) finalizes them into an assistant message item.
		// contentBuf 累积文本增量，直到 finish_reason 为 "stop"
		// （或流结束）时最终确定为 assistant 消息项。
		var contentBuf strings.Builder
		// reasoningBuf accumulates reasoning_content deltas so a complete
		// reasoning item can be emitted on stream completion.
		// reasoningBuf 累积 reasoning_content 增量，以便在流结束时发出完整的 reasoning 项。
		var reasoningBuf strings.Builder
		// toolCallAcc accumulates tool-call fragments keyed by their index
		// because the API streams name/arguments in separate chunks.
		// toolCallAcc 按索引累加工具调用片段，因为 API 会把
		// 名称/参数拆分到不同的块中流式传输。
		toolCallAcc := make(map[int]*toolCallAccumulator)
		// hasContent tracks whether any text delta was seen in this
		// response, so finish_reason "stop" only emits an item if there
		// is text to emit.
		// hasContent 跟踪本响应中是否出现过文本增量，
		// 使 finish_reason "stop" 仅在有文本可发时才发出条目。
		hasContent := false

		// Main SSE loop: each iteration processes one "data:" event.
		// 主 SSE 循环：每次迭代处理一个 "data:" 事件。
		for scanner.Next() {
			// Skip empty data payloads (e.g. keep-alive comments).
			// 跳过空数据负载（例如保活注释）。
			data := scanner.Data()
			if data == "" {
				continue
			}

			// Decode the JSON chunk; malformed chunks are skipped rather
			// than aborting the stream.
			// 解码 JSON 块；格式错误的块直接跳过而不是中止流。
			var chunk chatChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			// Chunks without choices (e.g. usage-only final chunk) carry
			// no delta; the usage check below still handles them.
			// 不含 choices 的块（例如仅含用量的最后一个块）没有增量；
			// 下方的用量检查仍会处理它们。
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			// Reasoning delta (DeepSeek-R1 / o1-style models emit reasoning_content).
			// 推理增量（DeepSeek-R1 / o1 系列模型会发出 reasoning_content）。
			if choice.Delta.ReasoningContent != "" {
				reasoningBuf.WriteString(choice.Delta.ReasoningContent)
				select {
				case <-ctx.Done():
					return
				case events <- ResponseEvent{Type: EventReasoningDelta, DeltaReasoning: choice.Delta.ReasoningContent}:
				}
			}

			// Content delta.
			// 内容增量。
			if choice.Delta.Content != "" {
				hasContent = true
				contentBuf.WriteString(choice.Delta.Content)
				select {
				case <-ctx.Done():
					return
				case events <- ResponseEvent{Type: EventOutputTextDelta, DeltaText: choice.Delta.Content}:
				}
			}

			// Tool call deltas (accumulate by index).
			// 工具调用增量（按索引累加）。
			for _, tc := range choice.Delta.ToolCalls {
				acc, ok := toolCallAcc[tc.Index]
				if !ok {
					acc = &toolCallAccumulator{}
					toolCallAcc[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.args.WriteString(tc.Function.Arguments)
			}

			// Handle finish_reason.
			// 处理 finish_reason。
			if choice.FinishReason != "" {
				switch choice.FinishReason {
				case "stop":
					// Model finished its reply: wrap the accumulated text in
					// an assistant message item and emit it as a completed
					// output item, then reset the buffer for safety.
					// 模型完成回复：把累积文本包装为 assistant 消息项并作为
					// 完成的输出条目发出，然后重置缓冲以防万一。
					if hasContent {
						text := contentBuf.String()
						item := protocol.NewAssistantMessage(text)
						select {
						case <-ctx.Done():
							return
						case events <- ResponseEvent{Type: EventOutputItemDone, Item: &item}:
						}
						contentBuf.Reset()
						hasContent = false
					}
				case "tool_calls":
					// Emit one EventOutputItemDone per tool call.
					// 每个工具调用发出一个 EventOutputItemDone。
					// The model requested tool execution: finalize each
					// accumulated tool call (id + name + concatenated
					// arguments) into a function_call item.
					// 模型请求执行工具：把每个累加完成的工具调用
					// （id + 名称 + 拼接的参数）最终确定为 function_call 条目。
					for _, acc := range toolCallAcc {
						item := protocol.NewFunctionCall(
							protocol.CallId(acc.id),
							acc.name,
							acc.args.String(),
						)
						select {
						case <-ctx.Done():
							return
						case events <- ResponseEvent{Type: EventOutputItemDone, Item: &item}:
						}
					}
					toolCallAcc = make(map[int]*toolCallAccumulator)
				}
			}

			// Usage (usually in final chunk).
			// 用量（通常在最后一个块中）。
			if chunk.Usage != nil {
				select {
				case <-ctx.Done():
					return
				case events <- ResponseEvent{
					Type: EventCompleted,
					Usage: &Usage{
						InputTokens:  chunk.Usage.PromptTokens,
						OutputTokens: chunk.Usage.CompletionTokens,
					},
				}:
				}
			}
		}

		// Flush any remaining accumulated content that wasn't finalized.
		// 刷新所有尚未完成的剩余累加内容。
		if contentBuf.Len() > 0 {
			text := contentBuf.String()
			item := protocol.NewAssistantMessage(text)
			select {
			case <-ctx.Done():
			case events <- ResponseEvent{Type: EventOutputItemDone, Item: &item}:
			}
		}

		// Flush accumulated reasoning content as a complete reasoning item.
		// 将累积的推理内容作为完整的 reasoning 项刷新。
		if reasoningBuf.Len() > 0 {
			item := protocol.NewReasoning(reasoningBuf.String())
			select {
			case <-ctx.Done():
			case events <- ResponseEvent{Type: EventOutputItemDone, Item: &item}:
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

// toolCallAccumulator accumulates tool call fragments across SSE chunks.
// toolCallAccumulator 跨 SSE 块累加工具调用片段。
type toolCallAccumulator struct {
	// id is the tool call identifier assigned by the model; it may arrive
	// in any chunk (usually the first) and is only set when non-empty.
	// id 是模型分配的工具调用标识符；可能出现在任意块中（通常是第一个），
	// 仅在非空时写入。
	id string
	// name is the function name to invoke, streamed incrementally.
	// name 是要调用的函数名称，以增量方式流式传输。
	name string
	// args concatenates the JSON argument fragments in arrival order.
	// args 按到达顺序拼接 JSON 参数片段。
	args strings.Builder
}

// buildChatMessages converts ResponseItems and instructions into Chat
// Completions message format. System messages from the input are replaced
// by the instructions parameter when non-empty.
// buildChatMessages 将 ResponseItem 和指令转换为 Chat Completions 消息格式。
// 当 instructions 参数非空时，输入中的系统消息会被替换。
func buildChatMessages(items []protocol.ResponseItem, instructions string) []chatMessage {
	var msgs []chatMessage

	// Prepend system message from instructions.
	// 从指令中前置 system 消息。
	if instructions != "" {
		b, _ := json.Marshal(instructions)
		msgs = append(msgs, chatMessage{Role: "system", Content: b})
	}

	// pendingReasoning carries reasoning_content that must be merged into
	// the next assistant message (either a content-bearing assistant
	// message or a function call). DeepSeek-R1 thinking mode requires
	// reasoning_content to live on the SAME assistant message as the
	// content/tool_calls it accompanied, not as a standalone message.
	// Splitting them triggers HTTP 400:
	// "The reasoning_content in the thinking mode must be passed back to
	// the API." when the model performed a tool call.
	// pendingReasoning 携带必须合并到下一条 assistant 消息（带 content 的
	// assistant 消息或 function call）中的 reasoning_content。DeepSeek-R1
	// 思考模式要求 reasoning_content 与其同时产出的 content/tool_calls 位于
	// 同一条 assistant 消息上，不能拆为独立消息。拆开会在模型进行工具调用
	// 时触发 HTTP 400："The reasoning_content in the thinking mode must be
	// passed back to the API."
	pendingReasoning := ""

	flushPendingReasoning := func() {
		// Emit any leftover reasoning as a standalone assistant message
		// when no subsequent assistant message/function call absorbed it.
		// This handles reasoning-only turns (rare but possible).
		// 当没有后续的 assistant 消息/函数调用吸收时，把剩余的推理作为
		// 独立的 assistant 消息发出。这处理仅有推理的轮次（罕见但可能）。
		if pendingReasoning != "" {
			empty, _ := json.Marshal("")
			msgs = append(msgs, chatMessage{
				Role:             "assistant",
				Content:          empty,
				ReasoningContent: pendingReasoning,
			})
			pendingReasoning = ""
		}
	}

	for _, item := range items {
		// Skip system messages from input (instructions already handled).
		// 跳过输入中的 system 消息（instructions 已处理）。
		if item.IsMessage() && item.Role == "system" {
			continue
		}

		switch {
		case item.IsMessage():
			text := item.GetText()
			b, _ := json.Marshal(text)
			// Only assistant messages can carry reasoning_content back to
			// the API. User messages must never carry it (the API would
			// reject a user role with reasoning_content). If reasoning was
			// pending but the next message is from the user, flush it as a
			// standalone assistant message first.
			// 只有 assistant 消息能把 reasoning_content 回传给 API。user
			// 消息绝不能携带 reasoning_content（API 会拒绝带 reasoning_content
			// 的 user 角色）。如果推理待处理但下一条是 user 消息，先把它作为
			// 独立的 assistant 消息刷出。
			if item.Role == "user" {
				flushPendingReasoning()
				msgs = append(msgs, chatMessage{Role: item.Role, Content: b})
			} else {
				msgs = append(msgs, chatMessage{
					Role:             item.Role,
					Content:          b,
					ReasoningContent: pendingReasoning,
				})
				pendingReasoning = ""
			}

		case item.IsFunctionCall():
			msgs = append(msgs, chatMessage{
				Role: "assistant",
				ToolCalls: []chatToolCall{{
					ID:   string(item.CallID),
					Type: "function",
					Function: chatFunction{
						Name:      item.Name,
						Arguments: item.Arguments,
					},
				}},
				ReasoningContent: pendingReasoning,
			})
			pendingReasoning = ""

		case item.Type == protocol.RITypeFunctionCallOutput || item.Type == protocol.RITypeCustomToolCallOutput:
			// Tool outputs belong to "tool" role and cannot carry
			// reasoning_content. Flush any pending reasoning first so the
			// assistant message ordering stays valid.
			// 工具输出属于 "tool" 角色，不能携带 reasoning_content。
			// 先把待处理的推理刷出，保持 assistant 消息顺序合法。
			flushPendingReasoning()
			b, _ := json.Marshal(item.Output)
			msgs = append(msgs, chatMessage{
				Role:       "tool",
				ToolCallID: string(item.CallID),
				Content:    b,
			})

		case item.Type == protocol.RITypeReasoning:
			// Accumulate reasoning summary; it will be merged into the
			// next assistant message/function call produced in the same
			// turn. If multiple reasoning items appear back-to-back
			// (e.g. separate summaries), concatenate them so the full
			// reasoning chain is preserved when echoed back to the API.
			// 累积推理摘要；它会被合并到同一轮中下一条 assistant 消息/
			// 函数调用上。如果多个 reasoning 项背靠背出现（例如分段摘要），
			// 拼接它们，使整条推理链在回传 API 时被完整保留。
			if pendingReasoning != "" && item.Summary != "" {
				pendingReasoning += "\n"
			}
			pendingReasoning += item.Summary
		}
	}

	// Flush any trailing reasoning that wasn't absorbed.
	// 刷出未被吸收的尾部推理。
	flushPendingReasoning()

	return msgs
}

// buildChatTools converts ToolSpecs to Chat Completions tool definition format.
// buildChatTools 将 ToolSpec 转换为 Chat Completions 工具定义格式。
func buildChatTools(specs []ToolSpec) []chatToolDef {
	// Pre-allocate the output slice; an empty (non-nil) slice keeps the
	// "tools" JSON field omitted when no tools are advertised.
	// 预分配输出切片；空（非 nil）切片使得未声明工具时
	// JSON 的 "tools" 字段被省略。
	tools := make([]chatToolDef, 0, len(specs))
	// Map each ToolSpec to the Chat Completions wire format: the schema
	// fields (name/description/parameters) pass through unchanged.
	// 把每个 ToolSpec 映射为 Chat Completions 传输格式：
	// 模式字段（名称/描述/参数）原样透传。
	for _, s := range specs {
		tools = append(tools, chatToolDef{
			Type: "function",
			Function: chatToolFunc{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  s.Parameters,
			},
		})
	}
	return tools
}
