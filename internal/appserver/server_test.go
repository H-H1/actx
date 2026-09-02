package appserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/H-H1/actx/internal/config"
)

// newTestServer creates a Server with a pipe-based scanner for testing.
func newTestServer(cfg *config.Config, stdin io.Reader, stdout io.Writer) *Server {
	return &Server{
		scanner: bufio.NewScanner(stdin),
		writer:  stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}
}

// parseResponses parses newline-delimited JSON-RPC output into responses.
func parseResponses(t *testing.T, output string) []JSONRPCResponse {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	responses := make([]JSONRPCResponse, 0, len(lines))
	for i, line := range lines {
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("line %d: unmarshal response: %v (line: %s)", i, err, line)
		}
		responses = append(responses, resp)
	}
	return responses
}

// parseNotifications parses newline-delimited JSON-RPC output into notifications.
func parseNotifications(t *testing.T, output string) []JSONRPCNotification {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	notifications := make([]JSONRPCNotification, 0, len(lines))
	for i, line := range lines {
		// Try to parse as notification (no id field or id is null).
		var msg struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("line %d: unmarshal notification: %v", i, err)
		}
		// Only include messages that have a method and no id (notifications).
		if msg.Method != "" && (len(msg.ID) == 0 || string(msg.ID) == "null") {
			notifications = append(notifications, JSONRPCNotification{
				JSONRPC: msg.JSONRPC,
				Method:  msg.Method,
				Params:  msg.Params,
			})
		}
	}
	return notifications
}

// --- Protocol type tests ---

// TestProtocolTypes verifies that the JSON-RPC types marshal correctly.
func TestProtocolTypes(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"clientInfo":{"name":"test","version":"0.1"}}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["method"] != "initialize" {
		t.Errorf("expected method 'initialize', got %v", parsed["method"])
	}
	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got %v", parsed["jsonrpc"])
	}
}

// TestNewResponse verifies response creation.
func TestNewResponse(t *testing.T) {
	id := json.RawMessage(`1`)
	resp := newResponse(id, map[string]any{"status": "ok"})

	if string(resp.ID) != `1` {
		t.Errorf("expected id '1', got %s", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("expected no error, got %v", resp.Error)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", result["status"])
	}
}

// TestNewErrorResponse verifies error response creation.
func TestNewErrorResponse(t *testing.T) {
	id := json.RawMessage(`2`)
	resp := newErrorResponse(id, ErrCodeMethodNotFound, "method not found: foo")

	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("expected code %d, got %d", ErrCodeMethodNotFound, resp.Error.Code)
	}
	if resp.Error.Message != "method not found: foo" {
		t.Errorf("expected message, got %s", resp.Error.Message)
	}
}

// TestNewNotification verifies notification creation.
func TestNewNotification(t *testing.T) {
	notif := newNotification("turn/started", map[string]any{"threadId": "abc"})

	if notif.Method != "turn/started" {
		t.Errorf("expected method 'turn/started', got %s", notif.Method)
	}

	var params map[string]any
	if err := json.Unmarshal(notif.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["threadId"] != "abc" {
		t.Errorf("expected threadId 'abc', got %v", params["threadId"])
	}
}

// TestInitializeResponse verifies the InitializeResponse type marshals to
// the expected JSON shape with camelCase fields.
func TestInitializeResponse(t *testing.T) {
	resp := InitializeResponse{
		UserAgent:      "actx/0.1.0",
		ACTXHome:      "/home/user/.ACTX",
		PlatformFamily: "windows",
		PlatformOS:     "windows",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	required := []string{"userAgent", "ACTXHome", "platformFamily", "platformOs"}
	for _, key := range required {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing field %s in response", key)
		}
	}
}

// TestThreadListResponse verifies thread/list response shape.
func TestThreadListResponse(t *testing.T) {
	resp := ThreadListResponse{
		Data: []Thread{
			{ID: "thread-1", Cwd: "/project"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	dataArr, ok := parsed["data"].([]any)
	if !ok {
		t.Fatal("expected data to be an array")
	}
	if len(dataArr) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(dataArr))
	}

	thread := dataArr[0].(map[string]any)
	if thread["id"] != "thread-1" {
		t.Errorf("expected id 'thread-1', got %v", thread["id"])
	}
}

// TestModelListResponse verifies model/list response shape.
func TestModelListResponse(t *testing.T) {
	resp := ModelListResponse{
		Data: []ModelInfo{
			{
				ID:          "gpt-5.2",
				Model:       "gpt-5.2",
				DisplayName: "GPT-5.2",
				IsDefault:   true,
				SupportedReasoningEfforts: []ReasoningEffortOption{
					{ReasoningEffort: "low"},
					{ReasoningEffort: "medium"},
					{ReasoningEffort: "high"},
				},
				DefaultReasoningEffort: "medium",
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	dataArr, ok := parsed["data"].([]any)
	if !ok {
		t.Fatal("expected data to be an array")
	}
	if len(dataArr) != 1 {
		t.Fatalf("expected 1 model, got %d", len(dataArr))
	}

	model := dataArr[0].(map[string]any)
	if model["id"] != "gpt-5.2" {
		t.Errorf("expected id 'gpt-5.2', got %v", model["id"])
	}
	if model["isDefault"] != true {
		t.Errorf("expected isDefault true, got %v", model["isDefault"])
	}
}

// TestAccountReadResponse verifies account/read response shape.
func TestAccountReadResponse(t *testing.T) {
	resp := AccountReadResponse{
		Account: &AccountInfo{
			Type:  "api_key",
			Email: "user@example.com",
		},
		RequiresOpenaiAuth: false,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["requiresOpenaiAuth"] != false {
		t.Errorf("expected requiresOpenaiAuth false, got %v", parsed["requiresOpenaiAuth"])
	}

	account, ok := parsed["account"].(map[string]any)
	if !ok {
		t.Fatal("expected account to be an object")
	}
	if account["type"] != "api_key" {
		t.Errorf("expected type 'api_key', got %v", account["type"])
	}
}

// TestTurnStartParams verifies turn/start params parsing.
func TestTurnStartParams(t *testing.T) {
	jsonStr := `{
		"threadId": "thread-123",
		"input": [{"type": "text", "text": "Hello"}],
		"cwd": "/project",
		"model": "gpt-5.2",
		"effort": "medium",
		"approvalPolicy": "onRequest",
		"sandboxPolicy": {"type": "workspaceWrite", "writableRoots": ["/project"], "networkAccess": true}
	}`

	var p TurnStartParams
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if p.ThreadID != "thread-123" {
		t.Errorf("expected threadId 'thread-123', got %s", p.ThreadID)
	}
	if len(p.Input) != 1 {
		t.Fatalf("expected 1 input, got %d", len(p.Input))
	}
	if p.Input[0].Text != "Hello" {
		t.Errorf("expected text 'Hello', got %s", p.Input[0].Text)
	}
	if p.Cwd != "/project" {
		t.Errorf("expected cwd '/project', got %s", p.Cwd)
	}
	if p.Effort != "medium" {
		t.Errorf("expected effort 'medium', got %s", p.Effort)
	}
}

// TestThreadStartParams verifies thread/start params parsing.
func TestThreadStartParams(t *testing.T) {
	jsonStr := `{
		"cwd": "/srv/project",
		"model": "gpt-5.2",
		"approvalPolicy": "onRequest",
		"sandbox": "workspaceWrite",
		"serviceName": "ACTXharbor"
	}`

	var p ThreadStartParams
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if p.Cwd != "/srv/project" {
		t.Errorf("expected cwd '/srv/project', got %s", p.Cwd)
	}
	if p.Model != "gpt-5.2" {
		t.Errorf("expected model 'gpt-5.2', got %s", p.Model)
	}
	if p.ServiceName != "ACTXharbor" {
		t.Errorf("expected serviceName 'ACTXharbor', got %s", p.ServiceName)
	}
}

// --- Server handler tests ---

// TestServerHandleLine tests the JSON-RPC dispatch with a real pipe.
func TestServerHandleLine(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp/test-ACTX",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"test","version":"0.1"}}}`
	server.handleLine(context.Background(), []byte(initReq))

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var initResp InitializeResponse
	if err := json.Unmarshal(resp.Result, &initResp); err != nil {
		t.Fatalf("unmarshal init response: %v", err)
	}

	if initResp.UserAgent != "actx/0.1.0" {
		t.Errorf("expected userAgent 'actx/0.1.0', got %s", initResp.UserAgent)
	}
}

// TestServerModelList tests the model/list handler.
func TestServerModelList(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp/test-ACTX",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":2,"method":"model/list","params":{}}`
	server.handleLine(context.Background(), []byte(req))

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var modelResp ModelListResponse
	if err := json.Unmarshal(resp.Result, &modelResp); err != nil {
		t.Fatalf("unmarshal model response: %v", err)
	}

	if len(modelResp.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(modelResp.Data))
	}

	if modelResp.Data[0].ID != "gpt-5.2" {
		t.Errorf("expected model id 'gpt-5.2', got %s", modelResp.Data[0].ID)
	}

	if !modelResp.Data[0].IsDefault {
		t.Error("expected model to be default")
	}
}

// TestServerThreadList tests the thread/list handler with an empty server.
func TestServerThreadList(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp/test-ACTX",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":3,"method":"thread/list","params":{"limit":100,"sourceKinds":["cli","appServer"]}}`
	server.handleLine(context.Background(), []byte(req))

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var threadResp ThreadListResponse
	if err := json.Unmarshal(resp.Result, &threadResp); err != nil {
		t.Fatalf("unmarshal thread response: %v", err)
	}

	if len(threadResp.Data) != 0 {
		t.Fatalf("expected 0 threads, got %d", len(threadResp.Data))
	}
}

// TestServerMethodNotFound tests the error response for unknown methods.
func TestServerMethodNotFound(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp/test-ACTX",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":4,"method":"unknown/method","params":{}}`
	server.handleLine(context.Background(), []byte(req))

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("expected code %d, got %d", ErrCodeMethodNotFound, resp.Error.Code)
	}
}

// TestServerConfigRead tests the config/read handler.
func TestServerConfigRead(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp/test-ACTX",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":5,"method":"config/read","params":{}}`
	server.handleLine(context.Background(), []byte(req))

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if result["model"] != "gpt-5.2" {
		t.Errorf("expected model 'gpt-5.2', got %v", result["model"])
	}
}

// TestServerAccountRead tests the account/read handler.
func TestServerAccountRead(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/nonexistent-path",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":6,"method":"account/read","params":{}}`
	server.handleLine(context.Background(), []byte(req))

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d", len(lines))
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var result AccountReadResponse
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Since /nonexistent-path has no auth.json, we expect requiresOpenaiAuth=true.
	if !result.RequiresOpenaiAuth {
		t.Error("expected requiresOpenaiAuth to be true for nonexistent ACTX home")
	}
}

// --- Notification param tests ---

// TestItemDeltaParams verifies the streaming delta notification shape.
func TestItemDeltaParams(t *testing.T) {
	params := ItemDeltaParams{
		ThreadID: "thread-1",
		TurnID:   "turn-1",
		ItemID:   "msg-turn-1",
		Delta:    "Hello",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["threadId"] != "thread-1" {
		t.Errorf("expected threadId 'thread-1', got %v", parsed["threadId"])
	}
	if parsed["itemId"] != "msg-turn-1" {
		t.Errorf("expected itemId 'msg-turn-1', got %v", parsed["itemId"])
	}
	if parsed["delta"] != "Hello" {
		t.Errorf("expected delta 'Hello', got %v", parsed["delta"])
	}
}

// TestTurnStartedParams verifies the turn/started notification shape.
func TestTurnStartedParams(t *testing.T) {
	params := TurnStartedParams{
		ThreadID: "thread-1",
		Turn:     Turn{ID: "turn-1", Status: "inProgress"},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["threadId"] != "thread-1" {
		t.Errorf("expected threadId, got %v", parsed["threadId"])
	}

	turn, ok := parsed["turn"].(map[string]any)
	if !ok {
		t.Fatal("expected turn to be an object")
	}
	if turn["id"] != "turn-1" {
		t.Errorf("expected turn id 'turn-1', got %v", turn["id"])
	}
	if turn["status"] != "inProgress" {
		t.Errorf("expected status 'inProgress', got %v", turn["status"])
	}
}

// TestTurnCompletedParams verifies the turn/completed notification includes a Turn object.
func TestTurnCompletedParams(t *testing.T) {
	params := TurnCompletedParams{
		ThreadID: "thread-1",
		TurnID:   "turn-1",
		Turn:     Turn{ID: "turn-1", Status: "completed"},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["threadId"] != "thread-1" {
		t.Errorf("expected threadId, got %v", parsed["threadId"])
	}

	turn, ok := parsed["turn"].(map[string]any)
	if !ok {
		t.Fatal("expected turn to be an object")
	}
	if turn["id"] != "turn-1" {
		t.Errorf("expected turn id 'turn-1', got %v", turn["id"])
	}
	if turn["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", turn["status"])
	}
}

// TestThreadStartedParams verifies the thread/started notification shape.
func TestThreadStartedParams(t *testing.T) {
	params := ThreadStartedParams{
		ThreadID: "thread-1",
		Thread:   Thread{ID: "thread-1", Cwd: "/project"},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["threadId"] != "thread-1" {
		t.Errorf("expected threadId, got %v", parsed["threadId"])
	}

	thread, ok := parsed["thread"].(map[string]any)
	if !ok {
		t.Fatal("expected thread to be an object")
	}
	if thread["id"] != "thread-1" {
		t.Errorf("expected thread id 'thread-1', got %v", thread["id"])
	}
}

// TestExecApprovalParams verifies the exec approval request shape.
func TestExecApprovalParams(t *testing.T) {
	params := ExecApprovalParams{
		ThreadID: "thread-1",
		TurnID:   "turn-1",
		CallID:   "call-123",
		Command:  []string{"ls", "-la"},
		Cwd:      "/project",
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["threadId"] != "thread-1" {
		t.Errorf("expected threadId, got %v", parsed["threadId"])
	}
	if parsed["callId"] != "call-123" {
		t.Errorf("expected callId, got %v", parsed["callId"])
	}

	cmd, ok := parsed["command"].([]any)
	if !ok {
		t.Fatal("expected command to be an array")
	}
	if len(cmd) != 2 {
		t.Fatalf("expected 2 command parts, got %d", len(cmd))
	}
	if cmd[0] != "ls" {
		t.Errorf("expected 'ls', got %v", cmd[0])
	}
}

// --- Integration tests ---

// TestFullProtocolFlow simulates a full ACTXHarbor connection flow.
func TestFullProtocolFlow(t *testing.T) {
	cfg := &config.Config{
		Model:          "gpt-5.2",
		ModelProvider:  "openai",
		ACTXHome:      "/nonexistent-test-home",
		Cwd:            "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	ctx := context.Background()

	// Step 1: Initialize
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ACTXharbor","title":"ACTXHarbor","version":"0.1.0"},"capabilities":{"mcpServerOpenaiFormElicitation":true}}}`))

	// Step 2: initialized notification (no response expected)
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","method":"initialized","params":{}}`))

	// Step 3: model/list
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"model/list","params":{"limit":100,"includeHidden":false}}`))

	// Step 4: account/read
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":3,"method":"account/read","params":{"refreshToken":false}}`))

	// Step 5: thread/list
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":4,"method":"thread/list","params":{"limit":100,"sortKey":"recency_at","sortDirection":"desc","modelProviders":[],"sourceKinds":["cli","vscode","exec","appServer","subAgent","subAgentReview","subAgentCompact","subAgentThreadSpawn","subAgentOther","unknown"],"archived":false,"useStateDbOnly":false}}`))

	// Step 6: config/read
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":5,"method":"config/read","params":{}}`))

	// Verify all responses were sent (initialized notification should not produce a response).
	responses := parseResponses(t, stdout.String())
	if len(responses) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(responses))
	}

	// Verify each response has the right id.
	for i, resp := range responses {
		expectedID := json.RawMessage(string(rune('1' + i)))
		if string(resp.ID) != string(expectedID) {
			t.Errorf("response %d: expected id %s, got %s", i, string(expectedID), string(resp.ID))
		}

		if resp.Error != nil {
			t.Errorf("response %d: unexpected error: %v", i, resp.Error)
		}
	}
}

// TestACTXHarborFullConnectionFlow simulates the exact sequence of requests
// that ACTXHarbor sends when connecting to a ACTX app-server.
func TestACTXHarborFullConnectionFlow(t *testing.T) {
	cfg := &config.Config{
		Model:          "gpt-5.2",
		ModelProvider:  "openai",
		ACTXHome:      "/nonexistent-test-home",
		Cwd:            "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	ctx := context.Background()

	// 1. Initialize handshake (exactly what ACTXHarbor sends)
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ACTXharbor","title":"ACTXHarbor","version":"0.1.0"},"capabilities":{"mcpServerOpenaiFormElicitation":true}}}`))

	// 2. initialized notification
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","method":"initialized","params":{}}`))

	// 3. model/list
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"model/list","params":{"limit":100,"includeHidden":false}}`))

	// 4. account/read
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":3,"method":"account/read","params":{"refreshToken":false}}`))

	// 5. thread/list (with full sourceKinds array that ACTXHarbor sends)
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":4,"method":"thread/list","params":{"limit":100,"sortKey":"recency_at","sortDirection":"desc","modelProviders":[],"sourceKinds":["cli","vscode","exec","appServer","subAgent","subAgentReview","subAgentCompact","subAgentThreadSpawn","subAgentOther","unknown"],"archived":false,"useStateDbOnly":false}}`))

	// 6. config/read
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":5,"method":"config/read","params":{}}`))

	// 7. account/rateLimits/read
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":6,"method":"account/rateLimits/read","params":{}}`))

	// 8. app/list
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":7,"method":"app/list","params":{}}`))

	// Verify responses
	responses := parseResponses(t, stdout.String())
	if len(responses) != 7 {
		t.Fatalf("expected 7 responses (initialized notification has no response), got %d", len(responses))
	}

	// Verify initialize response
	var initResp InitializeResponse
	if err := json.Unmarshal(responses[0].Result, &initResp); err != nil {
		t.Fatalf("unmarshal init response: %v", err)
	}
	if initResp.UserAgent != "actx/0.1.0" {
		t.Errorf("expected userAgent 'actx/0.1.0', got %s", initResp.UserAgent)
	}

	// Verify model/list response
	var modelResp ModelListResponse
	if err := json.Unmarshal(responses[1].Result, &modelResp); err != nil {
		t.Fatalf("unmarshal model response: %v", err)
	}
	if len(modelResp.Data) == 0 {
		t.Error("expected at least 1 model")
	}

	// Verify account/read response
	var accountResp AccountReadResponse
	if err := json.Unmarshal(responses[2].Result, &accountResp); err != nil {
		t.Fatalf("unmarshal account response: %v", err)
	}
	if !accountResp.RequiresOpenaiAuth {
		t.Error("expected requiresOpenaiAuth=true for nonexistent ACTX home")
	}

	// Verify thread/list response
	var threadResp ThreadListResponse
	if err := json.Unmarshal(responses[3].Result, &threadResp); err != nil {
		t.Fatalf("unmarshal thread response: %v", err)
	}
	if len(threadResp.Data) != 0 {
		t.Errorf("expected 0 threads, got %d", len(threadResp.Data))
	}

	// Verify config/read response
	var configResult map[string]any
	if err := json.Unmarshal(responses[4].Result, &configResult); err != nil {
		t.Fatalf("unmarshal config response: %v", err)
	}
	if configResult["model"] != "gpt-5.2" {
		t.Errorf("expected model 'gpt-5.2', got %v", configResult["model"])
	}

	// Verify account/rateLimits/read response
	var rateLimitsResp AccountRateLimitsResponse
	if err := json.Unmarshal(responses[5].Result, &rateLimitsResp); err != nil {
		t.Fatalf("unmarshal rate limits response: %v", err)
	}

	// Verify app/list response
	var appResult map[string]any
	if err := json.Unmarshal(responses[6].Result, &appResult); err != nil {
		t.Fatalf("unmarshal app response: %v", err)
	}
	data, ok := appResult["data"].([]any)
	if !ok {
		t.Fatal("expected app/list to return data array")
	}
	if len(data) != 0 {
		t.Errorf("expected empty app list, got %d items", len(data))
	}
}

// TestACTXHarborThreadListWithArchived tests that archived threads are filtered.
func TestACTXHarborThreadListWithArchived(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	// Add one active and one archived thread.
	server.threads["thread-1"] = &threadState{
		id:        "thread-1",
		cwd:       "/project1",
		createdAt: time.Now().Unix(),
		updatedAt: time.Now().Unix(),
		items:     make(map[string]*TurnItem),
	}
	server.threads["thread-2"] = &threadState{
		id:        "thread-2",
		cwd:       "/project2",
		archived:  true,
		createdAt: time.Now().Unix(),
		updatedAt: time.Now().Unix(),
		items:     make(map[string]*TurnItem),
	}

	// Query non-archived threads.
	req := `{"jsonrpc":"2.0","id":1,"method":"thread/list","params":{"archived":false}}`
	server.handleLine(context.Background(), []byte(req))

	responses := parseResponses(t, stdout.String())
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	var threadResp ThreadListResponse
	if err := json.Unmarshal(responses[0].Result, &threadResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(threadResp.Data) != 1 {
		t.Fatalf("expected 1 non-archived thread, got %d", len(threadResp.Data))
	}
	if threadResp.Data[0].ID != "thread-1" {
		t.Errorf("expected thread-1, got %s", threadResp.Data[0].ID)
	}
}

// TestACTXHarborThreadReadNotFound tests thread/read with a nonexistent thread.
func TestACTXHarborThreadReadNotFound(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":1,"method":"thread/read","params":{"threadId":"nonexistent"}}`
	server.handleLine(context.Background(), []byte(req))

	responses := parseResponses(t, stdout.String())
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	if responses[0].Error == nil {
		t.Fatal("expected error response for nonexistent thread")
	}
	if responses[0].Error.Code != ErrCodeInternal {
		t.Errorf("expected code %d, got %d", ErrCodeInternal, responses[0].Error.Code)
	}
}

// TestACTXHarborThreadArchive tests thread archive flow.
func TestACTXHarborThreadArchive(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	// Add a thread.
	server.threads["thread-1"] = &threadState{
		id:        "thread-1",
		cwd:       "/project",
		createdAt: time.Now().Unix(),
		updatedAt: time.Now().Unix(),
		items:     make(map[string]*TurnItem),
	}

	// Archive it.
	req := `{"jsonrpc":"2.0","id":1,"method":"thread/archive","params":{"threadId":"thread-1"}}`
	server.handleLine(context.Background(), []byte(req))

	// Should get a response and a notification.
	allLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(allLines) < 2 {
		t.Fatalf("expected at least 2 lines (response + notification), got %d", len(allLines))
	}

	// First line should be the response.
	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(allLines[0]), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Second line should be the thread/archived notification.
	var notif JSONRPCNotification
	if err := json.Unmarshal([]byte(allLines[1]), &notif); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if notif.Method != "thread/archived" {
		t.Errorf("expected method 'thread/archived', got %s", notif.Method)
	}

	var params ThreadArchivedParams
	if err := json.Unmarshal(notif.Params, &params); err != nil {
		t.Fatalf("unmarshal notification params: %v", err)
	}
	if params.ThreadID != "thread-1" {
		t.Errorf("expected threadId 'thread-1', got %s", params.ThreadID)
	}

	// Verify the thread is now archived.
	if !server.threads["thread-1"].archived {
		t.Error("expected thread to be archived")
	}
}

// TestACTXHarborShutdown tests the shutdown method.
func TestACTXHarborShutdown(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":1,"method":"shutdown","params":{}}`
	server.handleLine(context.Background(), []byte(req))

	responses := parseResponses(t, stdout.String())
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Errorf("unexpected error: %v", responses[0].Error)
	}
}

// TestACTXHarborLoginCancel tests account/login/cancel.
func TestACTXHarborLoginCancel(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":1,"method":"account/login/cancel","params":{}}`
	server.handleLine(context.Background(), []byte(req))

	responses := parseResponses(t, stdout.String())
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Errorf("unexpected error: %v", responses[0].Error)
	}
}

// TestACTXHarborConfigList tests config/list.
func TestACTXHarborConfigList(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":1,"method":"config/list","params":{}}`
	server.handleLine(context.Background(), []byte(req))

	responses := parseResponses(t, stdout.String())
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	var result map[string]any
	if err := json.Unmarshal(responses[0].Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("expected data to be an array")
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 config entry, got %d", len(data))
	}
}

// TestACTXHarborPipeBasedFlow tests the full protocol using actual pipes,
// simulating how ACTXHarbor communicates over SSH stdio.
func TestACTXHarborPipeBasedFlow(t *testing.T) {
	cfg := &config.Config{
		Model:          "gpt-5.2",
		ModelProvider:  "openai",
		ACTXHome:      "/nonexistent-test-home",
		Cwd:            "/tmp",
	}

	// Create a pipe for stdin.
	stdinR, stdinW := io.Pipe()
	var stdout bytes.Buffer

	server := newTestServer(cfg, stdinR, &stdout)

	// Start the server in a goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()

	// Send requests through the pipe (simulating ACTXHarbor over SSH).
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ACTXharbor","title":"ACTXHarbor","version":"0.1.0"},"capabilities":{"mcpServerOpenaiFormElicitation":true}}}`,
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"model/list","params":{"limit":100,"includeHidden":false}}`,
		`{"jsonrpc":"2.0","id":3,"method":"account/read","params":{"refreshToken":false}}`,
		`{"jsonrpc":"2.0","id":4,"method":"thread/list","params":{"limit":100,"sourceKinds":["cli","appServer"],"archived":false}}`,
		`{"jsonrpc":"2.0","id":5,"method":"config/read","params":{}}`,
		`{"jsonrpc":"2.0","id":6,"method":"shutdown","params":{}}`,
	}

	for _, req := range requests {
		stdinW.Write([]byte(req + "\n"))
	}

	// Close stdin to end the server loop.
	stdinW.Close()

	// Wait for server to finish.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish within 5 seconds")
	}

	// Verify responses.
	responses := parseResponses(t, stdout.String())
	// Should have 6 responses (initialized notification has no response).
	if len(responses) != 6 {
		t.Fatalf("expected 6 responses, got %d", len(responses))
	}

	// Verify each response id.
	for i, resp := range responses {
		expectedID := json.RawMessage(string(rune('1' + i)))
		if string(resp.ID) != string(expectedID) {
			t.Errorf("response %d: expected id %s, got %s", i, string(expectedID), string(resp.ID))
		}
		if resp.Error != nil {
			t.Errorf("response %d: unexpected error: %v", i, resp.Error)
		}
	}
}

// TestACTXHarborNotificationMethodNames verifies that all notification
// method names match what ACTXHarbor expects.
func TestACTXHarborNotificationMethodNames(t *testing.T) {
	expectedMethods := []string{
		"turn/started",
		"turn/completed",
		"thread/status/changed",
		"thread/started",
		"thread/archived",
		"item/started",
		"item/completed",
		"item/agentMessage/delta",
		"item/commandExecution/outputDelta",
		"error",
	}

	for _, method := range expectedMethods {
		notif := newNotification(method, map[string]any{})
		if notif.Method != method {
			t.Errorf("expected method %s, got %s", method, notif.Method)
		}
	}
}

// TestThreadStateToThread verifies thread state conversion.
func TestThreadStateToThread(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	server := &Server{
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	now := time.Now().Unix()
	ts := &threadState{
		id:        "thread-1",
		cwd:       "/project",
		model:     "gpt-5.2",
		provider:  "openai",
		approval:  "on-request",
		sandbox:   "workspace-write",
		createdAt: now,
		updatedAt: now,
		items:     make(map[string]*TurnItem),
	}

	thread := server.threadStateToThread(ts)

	if thread.ID != "thread-1" {
		t.Errorf("expected id 'thread-1', got %s", thread.ID)
	}
	if thread.Cwd != "/project" {
		t.Errorf("expected cwd '/project', got %s", thread.Cwd)
	}
	if thread.Status.Type != "idle" {
		t.Errorf("expected status 'idle', got %s", thread.Status.Type)
	}

	// Test active status.
	ts.activeTurn = "turn-1"
	thread = server.threadStateToThread(ts)
	if thread.Status.Type != "active" {
		t.Errorf("expected status 'active', got %s", thread.Status.Type)
	}

	// Test archived status.
	ts.archived = true
	thread = server.threadStateToThread(ts)
	if thread.Status.Type != "archived" {
		t.Errorf("expected status 'archived', got %s", thread.Status.Type)
	}
}

// TestACTXHarborServerErrorHandling tests various error conditions.
func TestACTXHarborServerErrorHandling(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	ctx := context.Background()

	// Test 1: thread/read with missing threadId
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"thread/read","params":{}}`))

	// Test 2: thread/archive with missing threadId
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"thread/archive","params":{}}`))

	// Test 3: turn/start with missing threadId
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":3,"method":"turn/start","params":{"input":[{"type":"text","text":"hello"}]}}`))

	// Test 4: turn/steer with missing threadId
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":4,"method":"turn/steer","params":{"input":[]}}`))

	// Test 5: turn/interrupt with missing threadId
	server.handleLine(ctx, []byte(`{"jsonrpc":"2.0","id":5,"method":"turn/interrupt","params":{}}`))

	responses := parseResponses(t, stdout.String())
	if len(responses) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(responses))
	}

	// All should have errors (thread not found or invalid params).
	for i, resp := range responses {
		if resp.Error == nil {
			t.Errorf("response %d: expected error, got nil", i)
		}
	}
}

// TestACTXHarborInvalidJSON tests handling of malformed JSON input.
func TestACTXHarborInvalidJSON(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	// Send invalid JSON - should be silently ignored (no response).
	server.handleLine(context.Background(), []byte(`{invalid json`))

	// Verify no output was produced.
	if stdout.Len() > 0 {
		t.Errorf("expected no output for invalid JSON, got: %s", stdout.String())
	}
}

// TestACTXHarborServerRequestResponse tests handling of client responses
// to server-initiated requests (e.g., approval responses).
func TestACTXHarborServerRequestResponse(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	// Simulate a client response to a server-initiated approval request.
	// This should not crash and should be silently handled.
	server.handleLine(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"result":{"approved":true}}`))

	// No response should be sent for a client response.
	if stdout.Len() > 0 {
		t.Errorf("expected no output for client response, got: %s", stdout.String())
	}
}

// TestACTXHarborAccountLoginStart tests that account/login/start returns
// an appropriate error since device-code login is not supported.
func TestACTXHarborAccountLoginStart(t *testing.T) {
	cfg := &config.Config{
		Model:         "gpt-5.2",
		ModelProvider: "openai",
		ACTXHome:     "/tmp",
	}

	var stdout bytes.Buffer
	server := &Server{
		writer:  &stdout,
		cfg:     cfg,
		threads: make(map[string]*threadState),
	}

	req := `{"jsonrpc":"2.0","id":1,"method":"account/login/start","params":{"type":"chatgpt"}}`
	server.handleLine(context.Background(), []byte(req))

	responses := parseResponses(t, stdout.String())
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	if responses[0].Error == nil {
		t.Fatal("expected error response for account/login/start")
	}
}
