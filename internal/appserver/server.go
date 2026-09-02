package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/H-H1/actx/internal/agent"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/login"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// LineScanner is the minimal interface for reading newline-delimited input.
// LineScanner 是用于读取换行分隔输入的最小接口。
// *bufio.Scanner satisfies this interface.
// *bufio.Scanner 满足此接口。
type LineScanner interface {
	Scan() bool
	Bytes() []byte
	Err() error
}

// Server is the App Server that reads JSON-RPC from stdin and writes to stdout.
// Server 是从标准输入读取 JSON-RPC 并写入标准输出的应用服务器。
type Server struct {
	// scanner reads newline-delimited JSON-RPC lines from stdin.
	// scanner 从标准输入读取换行分隔的 JSON-RPC 行。
	scanner LineScanner
	// writer writes JSON-RPC messages to stdout.
	// writer 将 JSON-RPC 消息写入标准输出。
	writer io.Writer
	// writerMu protects concurrent writes to writer.
	// writerMu 保护对 writer 的并发写入。
	writerMu sync.Mutex
	// logger logs server diagnostics.
	// logger 记录服务器诊断日志。
	logger *slog.Logger
	// cfg is the loaded application configuration.
	// cfg 是已加载的应用程序配置。
	cfg *config.Config
	// apiKey is the API key used for model requests.
	// apiKey 是用于模型请求的 API 密钥。
	apiKey string
	// baseURL is the base URL of the model API.
	// baseURL 是模型 API 的基础 URL。
	baseURL string

	// mu guards the mutable state below.
	// mu 保护以下可变状态。
	mu sync.Mutex
	// threads maps thread IDs to their state.
	// threads 将线程 ID 映射到其状态。
	threads map[string]*threadState
	// initialized indicates whether initialize has been handled.
	// initialized 指示是否已处理 initialize。
	initialized bool
}

// threadState tracks a single ACTX thread and its agent instance.
// threadState 跟踪单个 ACTX 线程及其代理实例。
type threadState struct {
	// id is the unique thread identifier.
	// id 是唯一的线程标识符。
	id string
	// ACTX is the agent instance backing the thread.
	// ACTX 是支撑线程的代理实例。
	ACTX *agent.ACTX
	// cwd is the working directory of the thread.
	// cwd 是线程的工作目录。
	cwd string
	// model is the model used by the thread.
	// model 是线程使用的模型。
	model string
	// provider is the model provider for the thread.
	// provider 是线程的模型提供者。
	provider string
	// approval is the approval policy for the thread.
	// approval 是线程的审批策略。
	approval string
	// sandbox is the sandbox policy for the thread.
	// sandbox 是线程的沙箱策略。
	sandbox string
	// createdAt is the Unix timestamp of thread creation.
	// createdAt 是线程创建的 Unix 时间戳。
	createdAt int64
	// updatedAt is the Unix timestamp of the last update.
	// updatedAt 是最后更新的 Unix 时间戳。
	updatedAt int64
	// turns is the list of turns in the thread.
	// turns 是线程中的轮次列表。
	turns []Turn
	// activeTurn is the id of the currently active turn, if any.
	// activeTurn 是当前活动轮次的 id（如果有）。
	activeTurn string
	// archived indicates whether the thread is archived.
	// archived 指示线程是否已归档。
	archived bool
	items    map[string]*TurnItem // item ID -> item (accumulated)
	// item ID -> item（累积的）
}

// New creates a new App Server reading from stdin and writing to stdout.
// New 创建一个从标准输入读取并写入标准输出的新应用服务器。
func New(cfg *config.Config, apiKey, baseURL string) *Server {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024*16)
	return &Server{
		scanner: scanner,
		writer:  os.Stdout,
		logger:  slog.Default(),
		cfg:     cfg,
		apiKey:  apiKey,
		baseURL: baseURL,
		threads: make(map[string]*threadState),
	}
}

// Run starts the read-dispatch loop. It blocks until stdin is closed or
// Run 启动读取分派循环。它会阻塞直到标准输入关闭或请求关闭。
// a shutdown is requested.
func (s *Server) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !s.scanner.Scan() {
			if err := s.scanner.Err(); err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			return nil // stdin closed
			// stdin 已关闭
		}

		line := s.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		s.handleLine(ctx, line)
	}
}

// handleLine parses a single JSON-RPC line and dispatches it.
// handleLine 解析单行 JSON-RPC 并将其分发。
func (s *Server) handleLine(ctx context.Context, line []byte) {
	// Ensure logger is initialized (for tests that construct Server directly).
	// 确保日志记录器已初始化（用于直接构造 Server 的测试）。
	if s.logger == nil {
		s.logger = slog.Default()
	}

	// Try to parse as a request or notification.
	// 尝试解析为请求或通知。
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		s.logger.Warn("failed to parse JSON-RPC line", "error", err, "line", string(line))
		return
	}

	if msg.Method == "" {
		// This is a response to a server-initiated request (e.g., approval).
		// 这是对服务器发起请求的响应（例如审批）。
		s.handleClientResponse(line)
		return
	}

	if len(msg.ID) == 0 || string(msg.ID) == "null" {
		// Notification (no id).
		// 通知（无 id）。
		s.handleNotification(ctx, msg.Method, msg.Params)
		return
	}

	// Request (has id).
	// 请求（有 id）。
	s.handleRequest(ctx, msg.ID, msg.Method, msg.Params)
}

// send writes a JSON-RPC message to stdout as a single line.
// send 将 JSON-RPC 消息作为单行写入标准输出。
func (s *Server) send(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error("failed to marshal JSON-RPC message", "error", err)
		return
	}
	s.writerMu.Lock()
	defer s.writerMu.Unlock()
	s.writer.Write(data)
	s.writer.Write([]byte("\n"))
}

// sendResponse sends a successful response.
// sendResponse 发送成功响应。
func (s *Server) sendResponse(id json.RawMessage, result any) {
	s.send(newResponse(id, result))
}

// sendError sends an error response.
// sendError 发送错误响应。
func (s *Server) sendError(id json.RawMessage, code int, message string) {
	s.send(newErrorResponse(id, code, message))
}

// sendNotification sends a notification.
// sendNotification 发送通知。
func (s *Server) sendNotification(method string, params any) {
	s.send(newNotification(method, params))
}

// handleNotification processes a JSON-RPC notification.
// handleNotification 处理 JSON-RPC 通知。
func (s *Server) handleNotification(_ context.Context, method string, params json.RawMessage) {
	switch method {
	case "initialized":
		s.logger.Info("client initialized")
	case "exit":
		s.logger.Info("client requested exit")
	default:
		s.logger.Debug("unhandled notification", "method", method)
	}
}

// handleClientResponse processes a response from the client to a
// handleClientResponse 处理客户端对服务器发起请求的响应
// server-initiated request (e.g., approval response).
// （例如审批响应）。
func (s *Server) handleClientResponse(line []byte) {
	var resp struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *JSONRPCError   `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		s.logger.Warn("failed to parse client response", "error", err)
		return
	}

	// The response id is the server request id, which encodes the thread id
	// 响应 id 是服务器请求 id，它编码了线程 id 和调用 id。
	// and call id. For now, we use a simple mapping.
	// 目前我们使用简单的映射。
	s.logger.Info("received client response", "id", string(resp.ID))
}

// handleRequest dispatches a JSON-RPC request to the appropriate handler.
// handleRequest 将 JSON-RPC 请求分派到适当的处理程序。
func (s *Server) handleRequest(ctx context.Context, id json.RawMessage, method string, params json.RawMessage) {
	s.logger.Info("handling request", "method", method, "id", string(id))

	switch method {
	// Lifecycle
	// 生命周期
	case "initialize":
		s.handleInitialize(id, params)
	case "shutdown":
		s.sendResponse(id, map[string]any{})

	// Thread methods
	// 线程方法
	case "thread/list":
		s.handleThreadList(id, params)
	case "thread/read":
		s.handleThreadRead(id, params)
	case "thread/start":
		s.handleThreadStart(ctx, id, params)
	case "thread/resume":
		s.handleThreadResume(id, params)
	case "thread/archive":
		s.handleThreadArchive(id, params)

	// Turn methods
	// 轮次方法
	case "turn/start":
		s.handleTurnStart(ctx, id, params)
	case "turn/steer":
		s.handleTurnSteer(id, params)
	case "turn/interrupt":
		s.handleTurnInterrupt(id, params)

	// Model methods
	// 模型方法
	case "model/list":
		s.handleModelList(id, params)

	// Account methods
	// 账户方法
	case "account/read":
		s.handleAccountRead(id, params)
	case "account/rateLimits/read":
		s.handleAccountRateLimits(id, params)
	case "account/login/start":
		s.handleAccountLoginStart(id, params)
	case "account/login/cancel":
		s.sendResponse(id, map[string]any{})

	// Config methods
	// 配置方法
	case "config/read":
		s.handleConfigRead(id, params)
	case "config/list":
		s.handleConfigList(id, params)

	// App methods
	// 应用方法
	case "app/list":
		s.sendResponse(id, map[string]any{"data": []any{}})

	default:
		s.sendError(id, ErrCodeMethodNotFound, fmt.Sprintf("method not found: %s", method))
	}
}

// --- Lifecycle handlers ---
// --- 生命周期处理程序 ---

func (s *Server) handleInitialize(id json.RawMessage, params json.RawMessage) {
	var p InitializeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			s.sendError(id, ErrCodeInvalidParams, "invalid initialize params")
			return
		}
	}

	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()

	platformOS := runtime.GOOS
	platformFamily := "unix"
	if platformOS == "windows" {
		platformFamily = "windows"
	}

	resp := InitializeResponse{
		UserAgent:      "actx/0.1.0",
		ACTXHome:       s.cfg.ACTXHome,
		PlatformFamily: platformFamily,
		PlatformOS:     platformOS,
	}
	s.sendResponse(id, resp)
}

// --- Thread handlers ---
// --- 线程处理程序 ---

func (s *Server) handleThreadList(id json.RawMessage, params json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	threads := make([]Thread, 0, len(s.threads))
	for _, ts := range s.threads {
		if ts.archived {
			continue
		}
		threads = append(threads, s.threadStateToThread(ts))
	}

	resp := ThreadListResponse{
		Data: threads,
	}
	s.sendResponse(id, resp)
}

func (s *Server) handleThreadRead(id json.RawMessage, params json.RawMessage) {
	var p ThreadReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, ErrCodeInvalidParams, "invalid thread/read params")
		return
	}

	s.mu.Lock()
	ts, ok := s.threads[p.ThreadID]
	s.mu.Unlock()

	if !ok {
		s.sendError(id, ErrCodeInternal, fmt.Sprintf("thread not found: %s", p.ThreadID))
		return
	}

	resp := ThreadReadResponse{
		Thread: s.threadStateToThread(ts),
	}
	s.sendResponse(id, resp)
}

func (s *Server) handleThreadStart(ctx context.Context, id json.RawMessage, params json.RawMessage) {
	var p ThreadStartParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, ErrCodeInvalidParams, "invalid thread/start params")
		return
	}

	// Build config overrides for this thread.
	// 为此线程构建配置覆盖。
	threadCfg := s.cfg
	if p.Cwd != "" {
		copyCfg := *threadCfg
		copyCfg.Cwd = p.Cwd
		threadCfg = &copyCfg
	}
	if p.Model != "" {
		copyCfg := *threadCfg
		copyCfg.Model = p.Model
		threadCfg = &copyCfg
	}

	// Build the tool registry.
	// 构建工具注册表。
	registry := tools.NewToolRegistry()
	registry.Register(agent.NewShellTool())
	registry.Register(agent.NewApplyPatchTool())

	// Spawn the ACTX agent.
	// 启动 ACTX 代理。
	ACTX, err := agent.Spawn(agent.ACTXArgs{
		Config:   threadCfg,
		APIKey:   s.apiKey,
		BaseURL:  s.baseURL,
		Registry: registry,
	})
	if err != nil {
		s.sendError(id, ErrCodeInternal, fmt.Sprintf("failed to spawn agent: %v", err))
		return
	}

	threadID := uuid.NewString()
	now := time.Now().Unix()
	cwd := p.Cwd
	if cwd == "" {
		cwd = s.cfg.Cwd
	}

	ts := &threadState{
		id:        threadID,
		ACTX:      ACTX,
		cwd:       cwd,
		model:     threadCfg.Model,
		provider:  threadCfg.ModelProvider,
		approval:  threadCfg.ApprovalPolicy.String(),
		sandbox:   threadCfg.SandboxPolicy.String(),
		createdAt: now,
		updatedAt: now,
		items:     make(map[string]*TurnItem),
	}

	s.mu.Lock()
	s.threads[threadID] = ts
	s.mu.Unlock()

	// Start consuming events from this agent.
	// 开始消费此代理的事件。
	go s.consumeEvents(ctx, threadID)

	resp := ThreadStartResponse{
		Thread:         s.threadStateToThread(ts),
		Model:          ts.model,
		ModelProvider:  ts.provider,
		Cwd:            ts.cwd,
		ApprovalPolicy: ts.approval,
		Sandbox:        ts.sandbox,
	}
	s.sendResponse(id, resp)

	// Notify thread/started.
	// 通知 thread/started。
	s.sendNotification("thread/started", ThreadStartedParams{
		ThreadID: threadID,
		Thread:   s.threadStateToThread(ts),
	})
}

func (s *Server) handleThreadResume(id json.RawMessage, params json.RawMessage) {
	var p ThreadResumeParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, ErrCodeInvalidParams, "invalid thread/resume params")
		return
	}

	s.mu.Lock()
	ts, ok := s.threads[p.ThreadID]
	s.mu.Unlock()

	if !ok {
		s.sendError(id, ErrCodeInternal, fmt.Sprintf("thread not found: %s", p.ThreadID))
		return
	}

	resp := ThreadResumeResponse{
		Thread: s.threadStateToThread(ts),
	}
	s.sendResponse(id, resp)
}

func (s *Server) handleThreadArchive(id json.RawMessage, params json.RawMessage) {
	var p ThreadArchiveParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, ErrCodeInvalidParams, "invalid thread/archive params")
		return
	}

	s.mu.Lock()
	ts, ok := s.threads[p.ThreadID]
	if ok {
		ts.archived = true
	}
	s.mu.Unlock()

	if !ok {
		s.sendError(id, ErrCodeInternal, fmt.Sprintf("thread not found: %s", p.ThreadID))
		return
	}

	s.sendResponse(id, map[string]any{})

	s.sendNotification("thread/archived", ThreadArchivedParams{
		ThreadID: p.ThreadID,
	})
}

// --- Turn handlers ---
// --- 轮次处理程序 ---

func (s *Server) handleTurnStart(ctx context.Context, id json.RawMessage, params json.RawMessage) {
	var p TurnStartParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, ErrCodeInvalidParams, "invalid turn/start params")
		return
	}

	s.mu.Lock()
	ts, ok := s.threads[p.ThreadID]
	s.mu.Unlock()

	if !ok {
		s.sendError(id, ErrCodeInternal, fmt.Sprintf("thread not found: %s", p.ThreadID))
		return
	}

	// Build the user input items.
	// 构建用户输入项。
	items := make([]protocol.ResponseItem, 0, len(p.Input))
	for _, input := range p.Input {
		if input.Type == "text" && input.Text != "" {
			items = append(items, protocol.NewUserMessage(input.Text))
		}
	}

	turnID := uuid.NewString()
	now := time.Now().Unix()

	s.mu.Lock()
	ts.activeTurn = turnID
	ts.updatedAt = now
	ts.turns = append(ts.turns, Turn{
		ID:     turnID,
		Status: "inProgress",
	})
	s.mu.Unlock()

	// Submit the user input to the agent.
	// 将用户输入提交给代理。
	ts.ACTX.Submit(protocol.NewUserInputOp(items))

	// Respond with the turn.
	// 用轮次响应。
	resp := TurnStartResponse{
		Turn: Turn{
			ID:     turnID,
			Status: "inProgress",
		},
	}
	s.sendResponse(id, resp)

	// Notify turn/started.
	// 通知 turn/started。
	s.sendNotification("turn/started", TurnStartedParams{
		ThreadID: p.ThreadID,
		Turn: Turn{
			ID:     turnID,
			Status: "inProgress",
		},
	})

	// Notify thread/status/changed.
	// 通知 thread/status/changed。
	s.sendNotification("thread/status/changed", ThreadStatusChangedParams{
		ThreadID: p.ThreadID,
		Status:   ThreadStatus{Type: "active"},
	})
}

func (s *Server) handleTurnSteer(id json.RawMessage, params json.RawMessage) {
	var p TurnSteerParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, ErrCodeInvalidParams, "invalid turn/steer params")
		return
	}

	s.mu.Lock()
	ts, ok := s.threads[p.ThreadID]
	s.mu.Unlock()

	if !ok {
		s.sendError(id, ErrCodeInternal, fmt.Sprintf("thread not found: %s", p.ThreadID))
		return
	}

	// Build steering input items.
	// 构建引导输入项。
	items := make([]protocol.ResponseItem, 0, len(p.Input))
	for _, input := range p.Input {
		if input.Type == "text" && input.Text != "" {
			items = append(items, protocol.NewUserMessage(input.Text))
		}
	}

	// Submit as additional context.
	// 作为附加上下文提交。
	ts.ACTX.Submit(protocol.NewUserInputOp(items))

	s.sendResponse(id, map[string]any{})
}

func (s *Server) handleTurnInterrupt(id json.RawMessage, params json.RawMessage) {
	var p TurnInterruptParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendError(id, ErrCodeInvalidParams, "invalid turn/interrupt params")
		return
	}

	s.mu.Lock()
	ts, ok := s.threads[p.ThreadID]
	if ok {
		ts.ACTX.Submit(protocol.NewInterruptOp())
	}
	s.mu.Unlock()

	if !ok {
		s.sendError(id, ErrCodeInternal, fmt.Sprintf("thread not found: %s", p.ThreadID))
		return
	}

	s.sendResponse(id, map[string]any{})
}

// --- Model handlers ---
// --- 模型处理程序 ---

func (s *Server) handleModelList(id json.RawMessage, _ json.RawMessage) {
	models := s.getDefaultModels()
	resp := ModelListResponse{
		Data: models,
	}
	s.sendResponse(id, resp)
}

// --- Account handlers ---
// --- 账户处理程序 ---

func (s *Server) handleAccountRead(id json.RawMessage, params json.RawMessage) {
	token, _ := login.LoadToken(s.cfg.ACTXHome)

	resp := AccountReadResponse{
		Account:            nil,
		RequiresOpenaiAuth: false,
	}

	if token != nil && token.IsValid() {
		accountType := "api_key"
		if token.APIKey == "" {
			accountType = "chatgpt"
		}
		resp.Account = &AccountInfo{
			Type:             accountType,
			Email:            token.Email,
			PlanType:         token.AccountID,
			CredentialSource: "stored",
		}
	} else {
		resp.RequiresOpenaiAuth = true
	}

	s.sendResponse(id, resp)
}

func (s *Server) handleAccountRateLimits(id json.RawMessage, _ json.RawMessage) {
	resp := AccountRateLimitsResponse{
		Used:  0,
		Limit: 0,
	}
	s.sendResponse(id, resp)
}

func (s *Server) handleAccountLoginStart(id json.RawMessage, _ json.RawMessage) {
	// Device-code login is not supported in the Go implementation.
	// 设备码登录在 Go 实现中不受支持。
	// Return an error indicating the user should use `ACTX login` on the CLI.
	// 返回错误，提示用户应在 CLI 上使用 `ACTX login`。
	s.sendError(id, ErrCodeInternal, "device-code login is not supported; run 'ACTX login' on the CLI instead")
}

// --- Config handlers ---
// --- 配置处理程序 ---

func (s *Server) handleConfigRead(id json.RawMessage, _ json.RawMessage) {
	s.sendResponse(id, map[string]any{
		"model":          s.cfg.Model,
		"modelProvider":  s.cfg.ModelProvider,
		"approvalPolicy": s.cfg.ApprovalPolicy.String(),
		"sandbox":        s.cfg.SandboxPolicy.String(),
	})
}

func (s *Server) handleConfigList(id json.RawMessage, _ json.RawMessage) {
	s.sendResponse(id, map[string]any{
		"data": []any{
			map[string]any{
				"model":          s.cfg.Model,
				"modelProvider":  s.cfg.ModelProvider,
				"approvalPolicy": s.cfg.ApprovalPolicy.String(),
				"sandbox":        s.cfg.SandboxPolicy.String(),
			},
		},
	})
}

// --- Helpers ---
// --- 辅助函数 ---

// threadStateToThread converts internal threadState to a Thread.
// threadStateToThread 将内部 threadState 转换为 Thread。
func (s *Server) threadStateToThread(ts *threadState) Thread {
	status := ThreadStatus{Type: "idle"}
	if ts.activeTurn != "" {
		status = ThreadStatus{Type: "active"}
	}
	if ts.archived {
		status = ThreadStatus{Type: "archived"}
	}

	turns := make([]Turn, len(ts.turns))
	copy(turns, ts.turns)

	// Attach items to turns.
	// 将项目附加到轮次。
	for i := range turns {
		if turns[i].Items == nil {
			turns[i].Items = []TurnItem{}
		}
	}

	return Thread{
		ID:            ts.id,
		Cwd:           ts.cwd,
		CreatedAt:     ts.createdAt,
		UpdatedAt:     ts.updatedAt,
		RecencyAt:     ts.updatedAt,
		ModelProvider: ts.provider,
		Status:        status,
		Turns:         turns,
	}
}

// getDefaultModels returns the default model list.
// getDefaultModels 返回默认模型列表。
func (s *Server) getDefaultModels() []ModelInfo {
	return []ModelInfo{
		{
			ID:          s.cfg.Model,
			Model:       s.cfg.Model,
			DisplayName: s.cfg.Model,
			Hidden:      false,
			SupportedReasoningEfforts: []ReasoningEffortOption{
				{ReasoningEffort: "low"},
				{ReasoningEffort: "medium"},
				{ReasoningEffort: "high"},
			},
			DefaultReasoningEffort: "medium",
			IsDefault:              true,
		},
	}
}

// consumeEvents reads events from a thread's ACTX agent and converts them
// consumeEvents 从线程的 ACTX 代理读取事件，并将其转换为
// to JSON-RPC notifications.
// JSON-RPC 通知。
func (s *Server) consumeEvents(ctx context.Context, threadID string) {
	s.mu.Lock()
	ts, ok := s.threads[threadID]
	s.mu.Unlock()
	if !ok {
		return
	}

	for {
		ev, ok := ts.ACTX.NextEvent()
		if !ok {
			break
		}
		s.bridgeEvent(threadID, ts, ev)
	}
}
