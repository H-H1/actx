package sseweb

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/H-H1/actx/internal/agent"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/protocol"
)

// Server 是基于 HTTP + SSE 的代理服务器。
// 它持有基础配置，为每个 POST /api/sessions 请求派生一个
// 独立的 agent.ACTX 实例（独立会话、独立对话上下文）。
type Server struct {
	cfg     *config.Config
	apiKey  string
	baseURL string
	logger  *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
}

// New 创建 SSE 服务器。cfg 作为所有新会话的基础配置模板。
func New(cfg *config.Config, apiKey, baseURL string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:      cfg,
		apiKey:   apiKey,
		baseURL:  baseURL,
		logger:   logger,
		sessions: make(map[string]*session),
	}
}

// Handler 返回配置好全部路由的 http.Handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.handleEvents)
	mux.HandleFunc("POST /api/sessions/{id}/message", s.handleMessage)
	mux.HandleFunc("POST /api/sessions/{id}/interrupt", s.handleInterrupt)
	mux.HandleFunc("POST /api/sessions/{id}/approval", s.handleApproval)
	mux.HandleFunc("POST /api/sessions/{id}/compact", s.handleCompact)
	mux.HandleFunc("GET /", s.handleIndex)
	return mux
}

// Close 关闭所有会话（服务器退出时调用）。
func (s *Server) Close() {
	s.mu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessions = make(map[string]*session)
	s.mu.Unlock()
	for _, sess := range sessions {
		sess.shutdown()
	}
}

// ----------------------------------------------------------------------------
// 请求/响应类型
// ----------------------------------------------------------------------------

// CreateSessionRequest 是创建会话时的可选覆盖项。
type CreateSessionRequest struct {
	Model          string `json:"model,omitempty"`
	Instructions   string `json:"instructions,omitempty"`
	Cwd            string `json:"cwd,omitempty"`
	ApprovalPolicy string `json:"approval_policy,omitempty"`
	SandboxPolicy  string `json:"sandbox_policy,omitempty"`
	ToolsMode      string `json:"tools_mode,omitempty"`
}

// SessionInfo 是会话的对外描述。
type SessionInfo struct {
	SessionID      string    `json:"session_id"`
	ThreadID       string    `json:"thread_id"`
	Model          string    `json:"model"`
	Provider       string    `json:"provider"`
	ApprovalPolicy string    `json:"approval_policy"`
	SandboxPolicy  string    `json:"sandbox_policy"`
	ToolsMode      string    `json:"tools_mode"`
	Cwd            string    `json:"cwd"`
	CreatedAt      time.Time `json:"created_at"`
	Closed         bool      `json:"closed"`
	ItemsInContext int       `json:"items_in_context"`
}

// MessageRequest 是提交用户消息的请求体。
type MessageRequest struct {
	// Message 是纯文本消息，将包装为用户消息项。
	Message string `json:"message,omitempty"`
	// Items 是完整的 ResponseItem 列表（高级用法，优先于 Message）。
	Items []protocol.ResponseItem `json:"items,omitempty"`
}

// ApprovalRequest 是审批响应的请求体。
type ApprovalRequest struct {
	CallID   string `json:"call_id"`
	Approved bool   `json:"approved"`
	Feedback string `json:"feedback"`
	// Kind 为 "patch" 时提交补丁审批，默认为命令执行审批。
	Kind string `json:"kind,omitempty"`
}

// CompactRequest 是上下文压缩的请求体。
type CompactRequest struct {
	Full bool `json:"full"`
}

// errorResponse 统一的 JSON 错误响应。
type errorResponse struct {
	Error string `json:"error"`
}

// ----------------------------------------------------------------------------
// 会话管理
// ----------------------------------------------------------------------------

// getSession 按 ID 查找会话。
func (s *Server) getSession(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

// sessionInfo 构建会话的对外描述。
func (s *Server) sessionInfo(sess *session) SessionInfo {
	items := 0
	if sess.agent.Session() != nil {
		if cm := sess.agent.Session().Context(); cm != nil {
			items = cm.Len()
		}
	}
	return SessionInfo{
		SessionID:      sess.id,
		ThreadID:       sess.threadID,
		Model:          sess.cfg.Model,
		Provider:       sess.cfg.ModelProvider,
		ApprovalPolicy: sess.cfg.ApprovalPolicy.String(),
		SandboxPolicy:  sess.cfg.SandboxPolicy.String(),
		ToolsMode:      sess.cfg.ToolsMode,
		Cwd:            sess.cfg.Cwd,
		CreatedAt:      sess.createdAt,
		Closed:         sess.isClosed(),
		ItemsInContext: items,
	}
}

// handleHealth 处理 GET /health。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListSessions 处理 GET /api/sessions。
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sessions := make([]*session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()

	infos := make([]SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		infos = append(infos, s.sessionInfo(sess))
	}
	writeJSON(w, http.StatusOK, infos)
}

// handleCreateSession 处理 POST /api/sessions：
// 克隆基础配置、应用覆盖项，然后派生一个独立的代理实例。
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	// 请求体可为空；解析失败返回 400。
	if r.Body != nil {
		data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("read body: %v", err))
			return
		}
		if len(data) > 0 {
			if err := json.Unmarshal(data, &req); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
				return
			}
		}
	}

	cfg, err := s.deriveConfig(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 派生代理：Registry 传 nil，Spawn 会注册内置工具
	// （shell、apply_patch、web_search）。
	ag, err := agent.Spawn(agent.ACTXArgs{
		Config:  cfg,
		APIKey:  s.apiKey,
		BaseURL: s.baseURL,
		Logger:  s.logger,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("spawn agent: %v", err))
		return
	}

	sess := newSession(cfg, ag, s.logger)

	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()

	s.logger.Info("session created", "session_id", sess.id, "model", cfg.Model)
	writeJSON(w, http.StatusCreated, s.sessionInfo(sess))
}

// deriveConfig 克隆基础配置并应用会话级覆盖项。
func (s *Server) deriveConfig(req *CreateSessionRequest) (*config.Config, error) {
	cfg := *s.cfg
	if req.Model != "" {
		cfg.Model = req.Model
	}
	if req.Instructions != "" {
		cfg.UserInstructions = req.Instructions
	}
	if req.Cwd != "" {
		cfg.Cwd = req.Cwd
	}
	if req.ApprovalPolicy != "" {
		p, err := config.ParseApprovalPolicy(req.ApprovalPolicy)
		if err != nil {
			return nil, err
		}
		cfg.ApprovalPolicy = p
	}
	if req.SandboxPolicy != "" {
		p, err := config.ParseSandboxPolicy(req.SandboxPolicy)
		if err != nil {
			return nil, err
		}
		cfg.SandboxPolicy = p
	}
	if req.ToolsMode != "" {
		if req.ToolsMode != "api" && req.ToolsMode != "prompt" {
			return nil, fmt.Errorf("invalid tools_mode %q (expected api or prompt)", req.ToolsMode)
		}
		cfg.ToolsMode = req.ToolsMode
	}
	return &cfg, nil
}

// handleGetSession 处理 GET /api/sessions/{id}。
func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r.PathValue("id"))
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionInfo(sess))
}

// handleDeleteSession 处理 DELETE /api/sessions/{id}：
// 立即从路由表移除并在后台优雅关闭代理。
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	sess := s.sessions[id]
	delete(s.sessions, id)
	s.mu.Unlock()
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	sess.shutdown()
	s.logger.Info("session deleted", "session_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// ----------------------------------------------------------------------------
// 操作提交端点
// ----------------------------------------------------------------------------

// handleMessage 处理 POST /api/sessions/{id}/message：提交用户输入。
func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r.PathValue("id"))
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req MessageRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	items := req.Items
	if len(items) == 0 {
		if req.Message == "" {
			writeError(w, http.StatusBadRequest, "message or items is required")
			return
		}
		items = []protocol.ResponseItem{protocol.NewUserMessage(req.Message)}
	}

	subID := sess.agent.Submit(protocol.NewUserInputOp(items))
	writeJSON(w, http.StatusAccepted, map[string]string{"submission_id": subID})
}

// handleInterrupt 处理 POST /api/sessions/{id}/interrupt：中止当前轮次。
func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r.PathValue("id"))
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	subID := sess.agent.Submit(protocol.NewInterruptOp())
	writeJSON(w, http.StatusAccepted, map[string]string{"submission_id": subID})
}

// handleApproval 处理 POST /api/sessions/{id}/approval：响应审批请求。
func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r.PathValue("id"))
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req ApprovalRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}
	if req.CallID == "" {
		writeError(w, http.StatusBadRequest, "call_id is required")
		return
	}

	var op protocol.Op
	if req.Kind == "patch" {
		op = protocol.NewPatchApprovalOp(req.CallID, req.Approved)
	} else {
		op = protocol.NewExecApprovalOp(req.CallID, req.Approved, req.Feedback)
	}
	subID := sess.agent.Submit(op)
	writeJSON(w, http.StatusAccepted, map[string]string{"submission_id": subID})
}

// handleCompact 处理 POST /api/sessions/{id}/compact：触发上下文压缩。
func (s *Server) handleCompact(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r.PathValue("id"))
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req CompactRequest
	// 请求体可为空。
	if r.Body != nil {
		data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &req)
		}
	}

	subID := sess.agent.Submit(protocol.NewCompactOp(req.Full))
	writeJSON(w, http.StatusAccepted, map[string]string{"submission_id": subID})
}

// ----------------------------------------------------------------------------
// SSE 事件流
// ----------------------------------------------------------------------------

// handleEvents 处理 GET /api/sessions/{id}/events：SSE 事件流。
// 支持 Last-Event-ID 头或 ?since=N 查询参数重放缺失事件；
// 每 15 秒发送一次注释心跳保持连接。
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r.PathValue("id"))
	if sess == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	since := parseSince(r)
	// 先订阅再重放：订阅时刻之后的事件进入通道，
	// 之前的从环形历史读取，两者无缝衔接。
	sub, upTo, closed := sess.subscribe()
	if sub != nil {
		defer sess.unsubscribe(sub)
	}
	for _, e := range sess.history(since, upTo) {
		if !writeSSE(w, e) {
			return
		}
	}
	io.WriteString(w, ": stream open\n\n")
	flusher.Flush()

	// 会话已关闭：重放完历史后以 session_closed 结束。
	if closed || sub == nil {
		io.WriteString(w, "event: session_closed\ndata: {}\n\n")
		flusher.Flush()
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			// 客户端断开：退订并结束。
			return
		case <-ticker.C:
			// 注释行心跳，防止中间层空闲超时。
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case e, ok := <-sub.ch:
			if !ok {
				// 代理已关闭：发送终止事件。
				io.WriteString(w, "event: session_closed\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			if !writeSSE(w, e) {
				return
			}
			flusher.Flush()
		}
	}
}

// parseSince 解析重放起点：优先 Last-Event-ID 头，其次 ?since= 查询参数。
func parseSince(r *http.Request) uint64 {
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return n
		}
	}
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// writeSSE 写入一条 SSE 事件并返回是否成功。
func writeSSE(w io.Writer, e sseEntry) bool {
	data, err := json.Marshal(e)
	if err != nil {
		return true
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Event.Msg.Type, data)
	return err == nil
}

// ----------------------------------------------------------------------------
// 工具函数
// ----------------------------------------------------------------------------

// writeJSON 写入 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return
	}
}

// writeError 写入 JSON 错误响应。
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
