// Package appserver implements the ACTX App Server protocol over stdio.
// 包 appserver 通过标准输入输出实现 ACTX App Server 协议。
// It speaks newline-delimited JSON-RPC 2.0, matching the protocol used by
// 它使用换行分隔的 JSON-RPC 2.0，与 ACTXHarbor 及其他通过 SSH 连接的桌面客户端
// ACTXHarbor and other desktop clients that connect via SSH.
// 所使用的协议一致。
//
// The server reads JSON-RPC requests from stdin and writes responses and
// 服务器从标准输入读取 JSON-RPC 请求，并将响应和通知写入标准输出。
// notifications to stdout. It bridges the internal agent SQ/EQ protocol to
// 它将内部代理的 SQ/EQ 协议桥接到应用服务器 JSON-RPC 协议。
// the app-server JSON-RPC protocol.
package appserver

import "encoding/json"

// JSONRPCRequest is a JSON-RPC 2.0 request with an id.
// JSONRPCRequest 是带有 id 的 JSON-RPC 2.0 请求。
type JSONRPCRequest struct {
	// JSONRPC is the protocol version string, always "2.0".
	// JSONRPC 是协议版本字符串，始终为 "2.0"。
	JSONRPC string `json:"jsonrpc"`
	// ID is the request identifier used to match the response.
	// ID 是用于匹配响应的请求标识符。
	ID json.RawMessage `json:"id"`
	// Method is the name of the method to invoke.
	// Method 是要调用的方法名称。
	Method string `json:"method"`
	// Params holds the raw parameters for the method.
	// Params 保存方法的原始参数。
	Params json.RawMessage `json:"params,omitempty"`
}

// JSONRPCNotification is a JSON-RPC 2.0 notification (no id).
// JSONRPCNotification 是 JSON-RPC 2.0 通知（无 id）。
type JSONRPCNotification struct {
	// JSONRPC is the protocol version string, always "2.0".
	// JSONRPC 是协议版本字符串，始终为 "2.0"。
	JSONRPC string `json:"jsonrpc"`
	// Method is the name of the notification method.
	// Method 是通知方法的名称。
	Method string `json:"method"`
	// Params holds the raw parameters for the notification.
	// Params 保存通知的原始参数。
	Params json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response to a request.
// JSONRPCResponse 是针对请求的 JSON-RPC 2.0 响应。
type JSONRPCResponse struct {
	// JSONRPC is the protocol version string, always "2.0".
	// JSONRPC 是协议版本字符串，始终为 "2.0"。
	JSONRPC string `json:"jsonrpc"`
	// ID is the identifier of the request this response matches.
	// ID 是此响应所匹配请求的标识符。
	ID json.RawMessage `json:"id"`
	// Result holds the raw result payload on success.
	// Result 保存成功时的原始结果负载。
	Result json.RawMessage `json:"result,omitempty"`
	// Error holds the error object on failure.
	// Error 保存失败时的错误对象。
	Error *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
// JSONRPCError 是 JSON-RPC 2.0 错误对象。
type JSONRPCError struct {
	// Code is the numeric error code.
	// Code 是数字错误码。
	Code int `json:"code"`
	// Message is a short description of the error.
	// Message 是错误的简短描述。
	Message string `json:"message"`
	// Data is optional additional error data.
	// Data 是可选的附加错误数据。
	Data json.RawMessage `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
// 标准 JSON-RPC 错误码。
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// newResponse creates a successful JSON-RPC response.
// newResponse 创建一个成功的 JSON-RPC 响应。
func newResponse(id json.RawMessage, result any) JSONRPCResponse {
	data, _ := json.Marshal(result)
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  data,
	}
}

// newErrorResponse creates an error JSON-RPC response.
// newErrorResponse 创建一个错误 JSON-RPC 响应。
func newErrorResponse(id json.RawMessage, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
}

// newNotification creates a JSON-RPC notification.
// newNotification 创建一个 JSON-RPC 通知。
func newNotification(method string, params any) JSONRPCNotification {
	data, _ := json.Marshal(params)
	return JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  data,
	}
}

// --- Initialize types ---
// --- 初始化类型 ---

// InitializeParams is the client->server initialize request.
// InitializeParams 是客户端到服务器的初始化请求。
type InitializeParams struct {
	// ClientInfo identifies the connecting client.
	// ClientInfo 标识连接的客户端。
	ClientInfo ClientInfo `json:"clientInfo"`
	// Capabilities declares optional client capabilities.
	// Capabilities 声明可选的客户端能力。
	Capabilities *InitializeCapabilities `json:"capabilities,omitempty"`
}

// ClientInfo identifies the connecting client.
// ClientInfo 标识连接的客户端。
type ClientInfo struct {
	// Name is the client application name.
	// Name 是客户端应用程序名称。
	Name string `json:"name"`
	// Title is an optional human-readable client title.
	// Title 是可选的人类可读的客户端标题。
	Title string `json:"title,omitempty"`
	// Version is the client application version.
	// Version 是客户端应用程序版本。
	Version string `json:"version"`
}

// InitializeCapabilities declares client capabilities.
// InitializeCapabilities 声明客户端能力。
type InitializeCapabilities struct {
	// ExperimentalAPI indicates whether the client accepts experimental APIs.
	// ExperimentalAPI 指示客户端是否接受实验性 API。
	ExperimentalAPI bool `json:"experimentalApi,omitempty"`
	// RequestAttestation indicates whether the client requests attestation.
	// RequestAttestation 指示客户端是否请求证明。
	RequestAttestation bool `json:"requestAttestation,omitempty"`
	// OptOutNotificationMethods lists notification methods the client opts out of.
	// OptOutNotificationMethods 列出客户端选择退出的通知方法。
	OptOutNotificationMethods []string `json:"optOutNotificationMethods,omitempty"`
	// McpServerOpenaiFormElicitation indicates support for OpenAI-form elicitation in MCP servers.
	// McpServerOpenaiFormElicitation 指示对 MCP 服务器中 OpenAI 表单引导的支持。
	McpServerOpenaiFormElicitation bool `json:"mcpServerOpenaiFormElicitation,omitempty"`
}

// InitializeResponse is the server's response to initialize.
// InitializeResponse 是服务器对初始化的响应。
type InitializeResponse struct {
	// UserAgent is the server's user agent string.
	// UserAgent 是服务器的用户代理字符串。
	UserAgent string `json:"userAgent"`
	// ACTXHome is the ACTX home directory path.
	// ACTXHome 是 ACTX 主目录路径。
	ACTXHome string `json:"ACTXHome"`
	// PlatformFamily is the platform family, e.g. "unix" or "windows".
	// PlatformFamily 是平台族，例如 "unix" 或 "windows"。
	PlatformFamily string `json:"platformFamily"`
	// PlatformOS is the operating system name.
	// PlatformOS 是操作系统名称。
	PlatformOS string `json:"platformOs"`
}

// --- Thread types ---
// --- 线程类型 ---

// Thread represents a ACTX conversation thread.
// Thread 表示一个 ACTX 对话线程。
type Thread struct {
	// ID is the unique thread identifier.
	// ID 是唯一的线程标识符。
	ID string `json:"id"`
	// SessionID is an optional associated session identifier.
	// SessionID 是可选的关联会话标识符。
	SessionID string `json:"sessionId,omitempty"`
	// Name is the optional thread name; nil means unset.
	// Name 是可选的线程名称；nil 表示未设置。
	Name *string `json:"name"`
	// Preview is a short preview of the thread content.
	// Preview 是线程内容的简短预览。
	Preview string `json:"preview,omitempty"`
	// Cwd is the working directory of the thread.
	// Cwd 是线程的工作目录。
	Cwd string `json:"cwd,omitempty"`
	// CreatedAt is the Unix timestamp of thread creation.
	// CreatedAt 是线程创建的 Unix 时间戳。
	CreatedAt int64 `json:"createdAt,omitempty"`
	// UpdatedAt is the Unix timestamp of the last update.
	// UpdatedAt 是最后更新的 Unix 时间戳。
	UpdatedAt int64 `json:"updatedAt,omitempty"`
	// RecencyAt is the Unix timestamp used for recency sorting.
	// RecencyAt 是用于近期排序的 Unix 时间戳。
	RecencyAt int64 `json:"recencyAt,omitempty"`
	// ModelProvider is the provider of the model used by the thread.
	// ModelProvider 是线程所用模型的提供者。
	ModelProvider string `json:"modelProvider,omitempty"`
	// Status is the current status of the thread.
	// Status 是线程的当前状态。
	Status ThreadStatus `json:"status,omitempty"`
	// Turns is the list of turns in the thread.
	// Turns 是线程中的轮次列表。
	Turns []Turn `json:"turns,omitempty"`
}

// ThreadStatus describes the current state of a thread.
// ThreadStatus 描述线程的当前状态。
type ThreadStatus struct {
	// Type is the status type, e.g. "idle", "active" or "archived".
	// Type 是状态类型，例如 "idle"、"active" 或 "archived"。
	Type string `json:"type,omitempty"`
	// ActiveFlags lists additional active-state flags.
	// ActiveFlags 列出附加的活动状态标志。
	ActiveFlags []string `json:"activeFlags,omitempty"`
}

// Turn represents a single turn within a thread.
// Turn 表示线程中的一个单轮对话。
type Turn struct {
	// ID is the unique turn identifier.
	// ID 是唯一的轮次标识符。
	ID string `json:"id"`
	// Status is the current status of the turn.
	// Status 是轮次的当前状态。
	Status string `json:"status,omitempty"`
	// Items is the list of items produced in the turn.
	// Items 是轮次中生成的项目列表。
	Items []TurnItem `json:"items,omitempty"`
}

// TurnItem represents an item within a turn (message, command, file change, etc.).
// TurnItem 表示轮次中的一个项目（消息、命令、文件变更等）。
type TurnItem struct {
	// ID is the unique item identifier.
	// ID 是唯一的项目标识符。
	ID string `json:"id"`
	// Type is the item type (message, command, file change, etc.).
	// Type 是项目类型（消息、命令、文件变更等）。
	Type string `json:"type"`
	// Status is the current status of the item.
	// Status 是项目的当前状态。
	Status string `json:"status,omitempty"`
	// Text holds the raw text payload of the item.
	// Text 保存项目的原始文本负载。
	Text json.RawMessage `json:"text,omitempty"`
	// Content holds the raw content payload of the item.
	// Content 保存项目的原始内容负载。
	Content json.RawMessage `json:"content,omitempty"`
	// Summary holds the raw summary of the item.
	// Summary 保存项目的原始摘要。
	Summary json.RawMessage `json:"summary,omitempty"`
	// Command holds the raw command of a command item.
	// Command 保存命令项的原始命令。
	Command json.RawMessage `json:"command,omitempty"`
	// Cwd holds the raw working directory of a command item.
	// Cwd 保存命令项的原始工作目录。
	Cwd json.RawMessage `json:"cwd,omitempty"`
	// AggregatedOutput holds the raw aggregated output of the item.
	// AggregatedOutput 保存项目的原始聚合输出。
	AggregatedOutput json.RawMessage `json:"aggregatedOutput,omitempty"`
	// Output holds the raw output of the item.
	// Output 保存项目的原始输出。
	Output json.RawMessage `json:"output,omitempty"`
	// StreamedOutput holds the raw streamed output chunks.
	// StreamedOutput 保存原始的流式输出块。
	StreamedOutput json.RawMessage `json:"streamedOutput,omitempty"`
	// ExitCode is the exit code of a command item.
	// ExitCode 是命令项的退出码。
	ExitCode *int `json:"exitCode,omitempty"`
	// DurationMs is the duration of the item in milliseconds.
	// DurationMs 是项目的持续时间（毫秒）。
	DurationMs *int64 `json:"durationMs,omitempty"`
	// Changes holds the raw file changes of the item.
	// Changes 保存项目的原始文件变更。
	Changes json.RawMessage `json:"changes,omitempty"`
	// TurnID links the item to its containing turn.
	// TurnID 将项目链接到其所属轮次。
	TurnID string `json:"turnId,omitempty"`
}

// ThreadListParams filters thread listing.
// ThreadListParams 过滤线程列表。
type ThreadListParams struct {
	// Limit is the maximum number of threads to return.
	// Limit 是返回线程的最大数量。
	Limit int `json:"limit,omitempty"`
	// SortKey is the key to sort threads by.
	// SortKey 是线程排序所依据的键。
	SortKey string `json:"sortKey,omitempty"`
	// SortDirection is the sort direction, "asc" or "desc".
	// SortDirection 是排序方向，"asc" 或 "desc"。
	SortDirection string `json:"sortDirection,omitempty"`
	// ModelProviders filters threads by model providers.
	// ModelProviders 按模型提供者过滤线程。
	ModelProviders []string `json:"modelProviders,omitempty"`
	// SourceKinds filters threads by source kinds.
	// SourceKinds 按来源类型过滤线程。
	SourceKinds []string `json:"sourceKinds,omitempty"`
	// Archived filters for archived threads.
	// Archived 过滤已归档的线程。
	Archived bool `json:"archived,omitempty"`
	// UseStateDbOnly restricts listing to the state database only.
	// UseStateDbOnly 限制仅从状态数据库列出。
	UseStateDbOnly bool `json:"useStateDbOnly,omitempty"`
	// Cursor is the pagination cursor.
	// Cursor 是分页游标。
	Cursor string `json:"cursor,omitempty"`
}

// ThreadListResponse is the response to thread/list.
// ThreadListResponse 是 thread/list 的响应。
type ThreadListResponse struct {
	// Data is the list of threads.
	// Data 是线程列表。
	Data []Thread `json:"data"`
	// NextCursor is the cursor for the next page, if any.
	// NextCursor 是下一页的游标（如果有）。
	NextCursor *string `json:"nextCursor,omitempty"`
}

// ThreadReadParams requests a thread by id.
// ThreadReadParams 通过 id 请求一个线程。
type ThreadReadParams struct {
	// ThreadID is the id of the thread to read.
	// ThreadID 是要读取的线程 id。
	ThreadID string `json:"threadId"`
	// IncludeTurns indicates whether to include turns in the response.
	// IncludeTurns 指示响应中是否包含轮次。
	IncludeTurns bool `json:"includeTurns,omitempty"`
}

// ThreadReadResponse is the response to thread/read.
// ThreadReadResponse 是 thread/read 的响应。
type ThreadReadResponse struct {
	// Thread is the requested thread.
	// Thread 是所请求的线程。
	Thread Thread `json:"thread"`
}

// ThreadStartParams creates a new thread.
// ThreadStartParams 创建一个新线程。
type ThreadStartParams struct {
	// Model is the model to use for the new thread.
	// Model 是新线程使用的模型。
	Model string `json:"model,omitempty"`
	// ModelProvider is the provider of the model.
	// ModelProvider 是模型的提供者。
	ModelProvider string `json:"modelProvider,omitempty"`
	// Cwd is the working directory for the new thread.
	// Cwd 是新线程的工作目录。
	Cwd string `json:"cwd,omitempty"`
	// ApprovalPolicy is the approval policy for the new thread.
	// ApprovalPolicy 是新线程的审批策略。
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	// Sandbox is the sandbox policy for the new thread.
	// Sandbox 是新线程的沙箱策略。
	Sandbox string `json:"sandbox,omitempty"`
	// ServiceName is the service name to bind the thread to.
	// ServiceName 是线程绑定的服务名称。
	ServiceName string `json:"serviceName,omitempty"`
}

// ThreadStartResponse is the response to thread/start.
// ThreadStartResponse 是 thread/start 的响应。
type ThreadStartResponse struct {
	// Thread is the newly created thread.
	// Thread 是新创建的线程。
	Thread Thread `json:"thread"`
	// Model is the model used by the thread.
	// Model 是线程使用的模型。
	Model string `json:"model"`
	// ModelProvider is the provider of the model.
	// ModelProvider 是模型的提供者。
	ModelProvider string `json:"modelProvider"`
	// Cwd is the working directory of the thread.
	// Cwd 是线程的工作目录。
	Cwd string `json:"cwd"`
	// ApprovalPolicy is the approval policy of the thread.
	// ApprovalPolicy 是线程的审批策略。
	ApprovalPolicy string `json:"approvalPolicy"`
	// Sandbox is the sandbox policy of the thread.
	// Sandbox 是线程的沙箱策略。
	Sandbox string `json:"sandbox"`
}

// ThreadResumeParams resumes an existing thread.
// ThreadResumeParams 恢复一个现有线程。
type ThreadResumeParams struct {
	// ThreadID is the id of the thread to resume.
	// ThreadID 是要恢复的线程 id。
	ThreadID string `json:"threadId"`
}

// ThreadResumeResponse is the response to thread/resume.
// ThreadResumeResponse 是 thread/resume 的响应。
type ThreadResumeResponse struct {
	// Thread is the resumed thread.
	// Thread 是恢复的线程。
	Thread Thread `json:"thread"`
}

// ThreadArchiveParams archives a thread.
// ThreadArchiveParams 归档一个线程。
type ThreadArchiveParams struct {
	// ThreadID is the id of the thread to archive.
	// ThreadID 是要归档的线程 id。
	ThreadID string `json:"threadId"`
}

// --- Turn types ---
// --- 轮次类型 ---

// TurnStartParams starts a new turn in a thread.
// TurnStartParams 在线程中启动一个新轮次。
type TurnStartParams struct {
	// ThreadID is the id of the thread that owns the turn.
	// ThreadID 是轮次所属线程的 id。
	ThreadID string `json:"threadId"`
	// Input is the list of user input items for the turn.
	// Input 是轮次的用户输入项列表。
	Input []UserInput `json:"input"`
	// Cwd is an optional working directory override.
	// Cwd 是可选的工作目录覆盖。
	Cwd string `json:"cwd,omitempty"`
	// Model is an optional model override.
	// Model 是可选的模型覆盖。
	Model string `json:"model,omitempty"`
	// Effort is an optional reasoning effort override.
	// Effort 是可选的推理努力覆盖。
	Effort string `json:"effort,omitempty"`
	// ApprovalPolicy is an optional approval policy override.
	// ApprovalPolicy 是可选的审批策略覆盖。
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	// SandboxPolicy is an optional raw sandbox policy override.
	// SandboxPolicy 是可选的原始沙箱策略覆盖。
	SandboxPolicy json.RawMessage `json:"sandboxPolicy,omitempty"`
}

// UserInput is a single input item for a turn.
// UserInput 是轮次的单个输入项。
type UserInput struct {
	// Type is the input item type, e.g. "text".
	// Type 是输入项类型，例如 "text"。
	Type string `json:"type"`
	// Text is the text content of the input item.
	// Text 是输入项的文本内容。
	Text string `json:"text,omitempty"`
}

// TurnStartResponse is the response to turn/start.
// TurnStartResponse 是 turn/start 的响应。
type TurnStartResponse struct {
	// Turn is the started turn.
	// Turn 是已启动的轮次。
	Turn Turn `json:"turn"`
}

// TurnSteerParams sends steering input to an active turn.
// TurnSteerParams 向活动轮次发送引导输入。
type TurnSteerParams struct {
	// ThreadID is the id of the thread that owns the turn.
	// ThreadID 是轮次所属线程的 id。
	ThreadID string `json:"threadId"`
	// ExpectedTurnID optionally specifies the turn expected to be active.
	// ExpectedTurnID 可选地指定预期处于活动状态的轮次。
	ExpectedTurnID string `json:"expectedTurnId,omitempty"`
	// Input is the steering input items to send.
	// Input 是要发送的引导输入项。
	Input []UserInput `json:"input"`
}

// TurnInterruptParams interrupts an active turn.
// TurnInterruptParams 中断一个活动轮次。
type TurnInterruptParams struct {
	// ThreadID is the id of the thread that owns the turn.
	// ThreadID 是轮次所属线程的 id。
	ThreadID string `json:"threadId"`
	// TurnID optionally targets a specific turn to interrupt.
	// TurnID 可选地指定要中断的特定轮次。
	TurnID string `json:"turnId,omitempty"`
}

// --- Model types ---
// --- 模型类型 ---

// ModelListParams filters model listing.
// ModelListParams 过滤模型列表。
type ModelListParams struct {
	// Cursor is the pagination cursor.
	// Cursor 是分页游标。
	Cursor *string `json:"cursor,omitempty"`
	// Limit is the maximum number of models to return.
	// Limit 是返回模型的最大数量。
	Limit *uint32 `json:"limit,omitempty"`
	// IncludeHidden indicates whether hidden models are included.
	// IncludeHidden 指示是否包含隐藏模型。
	IncludeHidden *bool `json:"includeHidden,omitempty"`
}

// ModelInfo describes a single model.
// ModelInfo 描述单个模型。
type ModelInfo struct {
	// ID is the unique model identifier.
	// ID 是唯一的模型标识符。
	ID string `json:"id"`
	// Model is the model name used in requests.
	// Model 是请求中使用的模型名称。
	Model string `json:"model"`
	// DisplayName is the human-readable model name.
	// DisplayName 是人类可读的模型名称。
	DisplayName string `json:"displayName"`
	// Description is an optional model description.
	// Description 是可选的模型描述。
	Description string `json:"description,omitempty"`
	// Hidden indicates whether the model is hidden from listings.
	// Hidden 指示模型是否在列表中隐藏。
	Hidden bool `json:"hidden"`
	// SupportedReasoningEfforts lists the supported reasoning effort levels.
	// SupportedReasoningEfforts 列出支持的推理努力级别。
	SupportedReasoningEfforts []ReasoningEffortOption `json:"supportedReasoningEfforts"`
	// DefaultReasoningEffort is the default reasoning effort level.
	// DefaultReasoningEffort 是默认的推理努力级别。
	DefaultReasoningEffort string `json:"defaultReasoningEffort"`
	// IsDefault indicates whether this is the default model.
	// IsDefault 指示这是否为默认模型。
	IsDefault bool `json:"isDefault"`
}

// ReasoningEffortOption describes a reasoning effort level.
// ReasoningEffortOption 描述推理努力级别。
type ReasoningEffortOption struct {
	// ReasoningEffort is the reasoning effort level name.
	// ReasoningEffort 是推理努力级别名称。
	ReasoningEffort string `json:"reasoningEffort"`
	// Description is an optional description of the level.
	// Description 是该级别的可选描述。
	Description string `json:"description,omitempty"`
}

// ModelListResponse is the response to model/list.
// ModelListResponse 是 model/list 的响应。
type ModelListResponse struct {
	// Data is the list of models.
	// Data 是模型列表。
	Data []ModelInfo `json:"data"`
	// NextCursor is the cursor for the next page, if any.
	// NextCursor 是下一页的游标（如果有）。
	NextCursor *string `json:"nextCursor,omitempty"`
}

// --- Account types ---
// --- 账户类型 ---

// AccountReadParams reads account information.
// AccountReadParams 读取账户信息。
type AccountReadParams struct {
	// RefreshToken indicates whether to refresh the auth token.
	// RefreshToken 指示是否刷新认证令牌。
	RefreshToken bool `json:"refreshToken,omitempty"`
}

// AccountReadResponse is the response to account/read.
// AccountReadResponse 是 account/read 的响应。
type AccountReadResponse struct {
	// Account is the authenticated account, if any.
	// Account 是已认证的账户（如果有）。
	Account *AccountInfo `json:"account"`
	// RequiresOpenaiAuth indicates whether OpenAI authentication is required.
	// RequiresOpenaiAuth 指示是否需要 OpenAI 认证。
	RequiresOpenaiAuth bool `json:"requiresOpenaiAuth"`
}

// AccountInfo describes the authenticated account.
// AccountInfo 描述已认证的账户。
type AccountInfo struct {
	// Type is the account type, e.g. "api_key" or "chatgpt".
	// Type 是账户类型，例如 "api_key" 或 "chatgpt"。
	Type string `json:"type,omitempty"`
	// Email is the account email address.
	// Email 是账户的电子邮件地址。
	Email string `json:"email,omitempty"`
	// PlanType is the account plan type.
	// PlanType 是账户的计划类型。
	PlanType string `json:"planType,omitempty"`
	// CredentialSource is the source of the credential.
	// CredentialSource 是凭据来源。
	CredentialSource string `json:"credentialSource,omitempty"`
}

// AccountLoginStartParams starts a device-code login flow.
// AccountLoginStartParams 启动设备码登录流程。
type AccountLoginStartParams struct {
	// Type is the login flow type.
	// Type 是登录流程类型。
	Type string `json:"type"`
}

// AccountLoginStartResponse is the response to account/login/start.
// AccountLoginStartResponse 是 account/login/start 的响应。
type AccountLoginStartResponse struct {
	// VerificationURL is the URL the user visits to verify.
	// VerificationURL 是用户访问以进行验证的 URL。
	VerificationURL string `json:"verificationUrl"`
	// UserCode is the device-code login user code.
	// UserCode 是设备码登录的用户代码。
	UserCode string `json:"userCode"`
}

// AccountRateLimitsResponse is the response to account/rateLimits/read.
// AccountRateLimitsResponse 是 account/rateLimits/read 的响应。
type AccountRateLimitsResponse struct {
	// Used is the amount of the limit already used.
	// Used 是已使用的配额量。
	Used int `json:"used,omitempty"`
	// Limit is the total rate limit.
	// Limit 是总速率限制。
	Limit int `json:"limit,omitempty"`
	// ResetAt is the Unix timestamp when the limit resets.
	// ResetAt 是限制重置的 Unix 时间戳。
	ResetAt int64 `json:"resetAt,omitempty"`
}

// --- Notification param types ---
// --- 通知参数类型 ---

// ItemNotificationParams is used for item/started, item/completed notifications.
// ItemNotificationParams 用于 item/started、item/completed 通知。
type ItemNotificationParams struct {
	// ThreadID is the id of the thread that owns the item.
	// ThreadID 是项目所属线程的 id。
	ThreadID string `json:"threadId"`
	// TurnID is the id of the turn that owns the item.
	// TurnID 是项目所属轮次的 id。
	TurnID string `json:"turnId,omitempty"`
	// Item is the item the notification is about.
	// Item 是通知所涉及的项目。
	Item TurnItem `json:"item"`
}

// ItemDeltaParams is used for streaming delta notifications.
// ItemDeltaParams 用于流式增量通知。
type ItemDeltaParams struct {
	// ThreadID is the id of the thread that owns the item.
	// ThreadID 是项目所属线程的 id。
	ThreadID string `json:"threadId"`
	// TurnID is the id of the turn that owns the item.
	// TurnID 是项目所属轮次的 id。
	TurnID string `json:"turnId,omitempty"`
	// ItemID is the id of the item being streamed.
	// ItemID 是正在流式传输的项目 id。
	ItemID string `json:"itemId"`
	// Delta is the streamed text chunk.
	// Delta 是流式传输的文本块。
	Delta string `json:"delta"`
}

// TurnStartedParams is used for turn/started notifications.
// TurnStartedParams 用于 turn/started 通知。
type TurnStartedParams struct {
	// ThreadID is the id of the thread that owns the turn.
	// ThreadID 是轮次所属线程的 id。
	ThreadID string `json:"threadId"`
	// Turn is the turn that started.
	// Turn 是已启动的轮次。
	Turn Turn `json:"turn"`
}

// TurnCompletedParams is used for turn/completed notifications.
// TurnCompletedParams 用于 turn/completed 通知。
type TurnCompletedParams struct {
	// ThreadID is the id of the thread that owns the turn.
	// ThreadID 是轮次所属线程的 id。
	ThreadID string `json:"threadId"`
	// TurnID is the id of the completed turn.
	// TurnID 是已完成轮次的 id。
	TurnID string `json:"turnId,omitempty"`
	// Turn is the completed turn.
	// Turn 是已完成的轮次。
	Turn Turn `json:"turn"`
}

// ThreadStartedParams is used for thread/started notifications.
// ThreadStartedParams 用于 thread/started 通知。
type ThreadStartedParams struct {
	// ThreadID is the id of the started thread.
	// ThreadID 是已启动线程的 id。
	ThreadID string `json:"threadId"`
	// Thread is the started thread.
	// Thread 是已启动的线程。
	Thread Thread `json:"thread"`
}

// ThreadArchivedParams is used for thread/archived notifications.
// ThreadArchivedParams 用于 thread/archived 通知。
type ThreadArchivedParams struct {
	// ThreadID is the id of the archived thread.
	// ThreadID 是已归档线程的 id。
	ThreadID string `json:"threadId"`
}

// ThreadStatusChangedParams is used for thread/status/changed notifications.
// ThreadStatusChangedParams 用于 thread/status/changed 通知。
type ThreadStatusChangedParams struct {
	// ThreadID is the id of the thread whose status changed.
	// ThreadID 是状态已更改线程的 id。
	ThreadID string `json:"threadId"`
	// Status is the new thread status.
	// Status 是新的线程状态。
	Status ThreadStatus `json:"status"`
}

// ErrorNotificationParams is used for error notifications.
// ErrorNotificationParams 用于错误通知。
type ErrorNotificationParams struct {
	// ThreadID is the optional id of the related thread.
	// ThreadID 是相关线程的可选 id。
	ThreadID string `json:"threadId,omitempty"`
	// TurnID is the optional id of the related turn.
	// TurnID 是相关轮次的可选 id。
	TurnID string `json:"turnId,omitempty"`
	// Error holds the error details.
	// Error 保存错误详情。
	Error struct {
		// Message is the error message.
		// Message 是错误消息。
		Message string `json:"message"`
	} `json:"error"`
}

// --- Server request types (approval flows) ---
// --- 服务器请求类型（审批流程） ---

// ExecApprovalParams is sent by the server to request command approval.
// ExecApprovalParams 由服务器发送以请求命令审批。
type ExecApprovalParams struct {
	// ThreadID is the id of the thread requesting approval.
	// ThreadID 是请求审批的线程 id。
	ThreadID string `json:"threadId"`
	// TurnID is the optional id of the related turn.
	// TurnID 是相关轮次的可选 id。
	TurnID string `json:"turnId,omitempty"`
	// CallID is the id of the tool call awaiting approval.
	// CallID 是等待审批的工具调用 id。
	CallID string `json:"callId"`
	// Command is the command argv to be approved.
	// Command 是待审批的命令参数列表。
	Command []string `json:"command"`
	// Cwd is the working directory of the command.
	// Cwd 是命令的工作目录。
	Cwd string `json:"cwd,omitempty"`
}

// ExecApprovalResult is the client's response to an exec approval.
// ExecApprovalResult 是客户端对执行审批的响应。
type ExecApprovalResult struct {
	// Approved indicates whether the command was approved.
	// Approved 指示命令是否被批准。
	Approved bool `json:"approved"`
	// Feedback is optional feedback accompanying the decision.
	// Feedback 是随决定附带的可选反馈。
	Feedback string `json:"feedback,omitempty"`
}

// PatchApprovalParams is sent by the server to request patch approval.
// PatchApprovalParams 由服务器发送以请求补丁审批。
type PatchApprovalParams struct {
	// ThreadID is the id of the thread requesting approval.
	// ThreadID 是请求审批的线程 id。
	ThreadID string `json:"threadId"`
	// TurnID is the optional id of the related turn.
	// TurnID 是相关轮次的可选 id。
	TurnID string `json:"turnId,omitempty"`
	// CallID is the id of the tool call awaiting approval.
	// CallID 是等待审批的工具调用 id。
	CallID string `json:"callId"`
	// PatchContent is the patch text to be approved.
	// PatchContent 是待审批的补丁文本。
	PatchContent string `json:"patchContent,omitempty"`
}

// PatchApprovalResult is the client's response to a patch approval.
// PatchApprovalResult 是客户端对补丁审批的响应。
type PatchApprovalResult struct {
	// Approved indicates whether the patch was approved.
	// Approved 指示补丁是否被批准。
	Approved bool `json:"approved"`
}
