package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/H-H1/actx/internal/protocol"
)

// TestBuildResponsesInput verifies conversation items convert to the
// /responses input format: system messages are dropped, messages become
// plain-string content, reasoning items echo back as reasoning_text blocks
// (DeepSeek thinking models require them before function calls), and tool
// calls/outputs round-trip their IDs.
// TestBuildResponsesInput 验证对话项转换为 /responses 输入格式：system
// 消息被丢弃、消息转为纯字符串内容、reasoning 项以 reasoning_text 块
// 回传（DeepSeek 思维链模型要求其位于 function_call 之前）、工具调用/
// 输出往返保留其 ID。
func TestBuildResponsesInput(t *testing.T) {
	items := []protocol.ResponseItem{
		protocol.NewSystemMessage("sys prompt"),
		protocol.NewUserMessage("hi"),
		protocol.NewAssistantMessage("hello"),
		protocol.NewReasoning("thinking..."),
		protocol.NewFunctionCall("call_1", "shell", `{"command":["dir"]}`),
		protocol.NewFunctionCallOutput("call_1", "done"),
	}

	got := buildResponsesInput(items)
	if len(got) != 5 {
		t.Fatalf("expected 5 items (system skipped), got %d", len(got))
	}

	// User message.
	// 用户消息。
	if got[0].Type != "message" || got[0].Role != "user" || got[0].Content != "hi" {
		t.Errorf("user item = %+v", got[0])
	}
	// Assistant message.
	// 助手消息。
	if got[1].Role != "assistant" || got[1].Content != "hello" {
		t.Errorf("assistant item = %+v", got[1])
	}
	// Reasoning item echoes back as a reasoning_text block array.
	// reasoning 项以 reasoning_text 块数组回传。
	blocks, ok := got[2].Content.([]responsesContentBlock)
	if got[2].Type != "reasoning" || !ok || len(blocks) != 1 ||
		blocks[0].Type != "reasoning_text" || blocks[0].Text != "thinking..." {
		t.Errorf("reasoning item = %+v", got[2])
	}
	// Function call.
	// 函数调用。
	if got[3].Type != "function_call" || got[3].CallID != "call_1" || got[3].Name != "shell" || got[3].Arguments != `{"command":["dir"]}` {
		t.Errorf("function_call item = %+v", got[3])
	}
	// Function call output.
	// 函数调用输出。
	if got[4].Type != "function_call_output" || got[4].CallID != "call_1" || got[4].Output == nil || *got[4].Output != "done" {
		t.Errorf("function_call_output item = %+v", got[4])
	}
}

// TestBuildResponsesInputEmptyReasoning verifies empty reasoning items are
// skipped (they would serialize as an empty block array, which DeepSeek
// rejects).
// TestBuildResponsesInputEmptyReasoning 验证空的 reasoning 项被跳过
// （否则会序列化为空块数组，被 DeepSeek 拒绝）。
func TestBuildResponsesInputEmptyReasoning(t *testing.T) {
	got := buildResponsesInput([]protocol.ResponseItem{protocol.NewReasoning("")})
	if len(got) != 0 {
		t.Fatalf("expected empty reasoning to be skipped, got %+v", got)
	}
}

// TestBuildResponsesInputEmptyOutput verifies the output field is always
// serialized for function_call_output items, even when empty: DeepSeek's
// Responses API rejects the request when the field is missing.
// TestBuildResponsesInputEmptyOutput 验证 function_call_output 项即使输出为空
// 也始终序列化 output 字段：字段缺失时 DeepSeek Responses API 会拒绝请求。
func TestBuildResponsesInputEmptyOutput(t *testing.T) {
	items := []protocol.ResponseItem{
		protocol.NewFunctionCallOutput("call_2", ""),
	}
	got := buildResponsesInput(items)
	if len(got) != 1 || got[0].Output == nil || *got[0].Output != "" {
		t.Fatalf("empty output item = %+v", got)
	}

	// Marshal and confirm the JSON contains "output": "".
	// 序列化并确认 JSON 含 "output": ""。
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"output":""`) {
		t.Errorf("marshaled JSON missing empty output field: %s", raw)
	}
}

// TestResponsesStreamE2E drives Stream against a fake /responses SSE server
// and verifies the full event mapping: text deltas, reasoning deltas,
// function_call output items, and terminal usage.
// TestResponsesStreamE2E 用伪造的 /responses SSE 服务器驱动 Stream，
// 验证完整事件映射：文本增量、推理增量、function_call 输出项与终止用量。
func TestResponsesStreamE2E(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `event: response.output_item.added`+"\n"+`data: {"type":"response.output_item.added"}`+"\n\n")
		io.WriteString(w, `event: response.reasoning_text.delta`+"\n"+`data: {"type":"response.reasoning_text.delta","delta":"思考"}`+"\n\n")
		io.WriteString(w, `event: response.output_text.delta`+"\n"+`data: {"type":"response.output_text.delta","delta":"你"}`+"\n\n")
		io.WriteString(w, `event: response.output_text.delta`+"\n"+`data: {"type":"response.output_text.delta","delta":"好"}`+"\n\n")
		io.WriteString(w, `event: response.output_item.done`+"\n"+`data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"你好"}]}}`+"\n\n")
		io.WriteString(w, `event: response.output_item.done`+"\n"+`data: {"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_9","name":"shell","arguments":"{\"command\":[\"dir\"]}"}}`+"\n\n")
		io.WriteString(w, `event: response.completed`+"\n"+`data: {"type":"response.completed","response":{"usage":{"input_tokens":11,"output_tokens":7}}}`+"\n\n")
	}))
	defer srv.Close()

	client := NewModelClient("test-key", "deepseek-v4-flash", srv.URL)
	client.SetAPIStyle(APIStyleResponses)

	events, err := client.Stream(context.Background(), Prompt{
		Input:        []protocol.ResponseItem{protocol.NewUserMessage("hi")},
		Instructions: "be brief",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Consume events and check them in order.
	// 消费事件并按序校验。
	var textDelta, reasoningDelta string
	var sawUsage bool
	var fnItem, msgItem *protocol.ResponseItem
	for ev := range events {
		switch ev.Type {
		case EventOutputTextDelta:
			textDelta += ev.DeltaText
		case EventReasoningDelta:
			reasoningDelta += ev.DeltaReasoning
		case EventOutputItemDone:
			if ev.Item.IsFunctionCall() {
				fnItem = ev.Item
			} else if ev.Item.IsMessage() {
				msgItem = ev.Item
			}
		case EventCompleted:
			sawUsage = ev.Usage != nil && ev.Usage.InputTokens == 11 && ev.Usage.OutputTokens == 7
		}
	}

	if gotPath != "/responses" {
		t.Errorf("request path = %s, want /responses", gotPath)
	}
	// Request body sanity: model, instructions, stream.
	// 请求体健全性检查：model、instructions、stream。
	if gotBody["model"] != "deepseek-v4-flash" || gotBody["instructions"] != "be brief" || gotBody["stream"] != true {
		t.Errorf("request body = %v", gotBody)
	}
	if textDelta != "你好" {
		t.Errorf("text delta = %q, want 你好", textDelta)
	}
	if reasoningDelta != "思考" {
		t.Errorf("reasoning delta = %q, want 思考", reasoningDelta)
	}
	if msgItem == nil || msgItem.GetText() != "你好" || msgItem.Role != "assistant" {
		t.Errorf("message item = %+v", msgItem)
	}
	if fnItem == nil || string(fnItem.CallID) != "call_9" || fnItem.Name != "shell" {
		t.Errorf("function call item = %+v", fnItem)
	}
	if !sawUsage {
		t.Errorf("usage event missing or wrong")
	}
}

// TestSetAPIStyleFallback verifies unknown styles fall back to chat.
// TestSetAPIStyleFallback 验证未知风格回退到 chat。
func TestSetAPIStyleFallback(t *testing.T) {
	c := NewModelClient("k", "m", "")
	c.SetAPIStyle("nonsense")
	if c.apiStyle != APIStyleChat {
		t.Errorf("apiStyle = %q, want chat fallback", c.apiStyle)
	}
	c.SetAPIStyle(APIStyleResponses)
	if c.apiStyle != APIStyleResponses {
		t.Errorf("apiStyle = %q, want responses", c.apiStyle)
	}
}
