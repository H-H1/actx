package protocol

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestNewUserMessage(t *testing.T) {
	msg := NewUserMessage("hello")
	if msg.Type != RITypeMessage {
		t.Errorf("expected type %s, got %s", RITypeMessage, msg.Type)
	}
	if msg.Role != "user" {
		t.Errorf("expected role user, got %s", msg.Role)
	}
	if msg.GetText() != "hello" {
		t.Errorf("expected text 'hello', got %s", msg.GetText())
	}
}

func TestNewFunctionCall(t *testing.T) {
	call := NewFunctionCall("call_123", "shell", `{"command":["ls"]}`)
	if call.Type != RITypeFunctionCall {
		t.Errorf("expected type %s, got %s", RITypeFunctionCall, call.Type)
	}
	if call.CallID != "call_123" {
		t.Errorf("expected call_id call_123, got %s", call.CallID)
	}
	if call.Name != "shell" {
		t.Errorf("expected name shell, got %s", call.Name)
	}
}

func TestResponseItemJSON(t *testing.T) {
	item := NewUserMessage("test message")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed ResponseItem
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed.GetText() != "test message" {
		t.Errorf("expected 'test message', got %s", parsed.GetText())
	}
}

func TestOpTypes(t *testing.T) {
	tests := []struct {
		op       Op
		wantType OpType
	}{
		{NewUserInputOp(nil), OpUserInput},
		{NewInterruptOp(), OpInterrupt},
		{NewShutdownOp(), OpShutdown},
		{NewExecApprovalOp("call_1", true, "ok"), OpExecApproval},
		{NewPatchApprovalOp("call_1", false), OpPatchApproval},
		{NewCompactOp(true), OpCompact},
	}

	for _, tt := range tests {
		if tt.op.Type != tt.wantType {
			t.Errorf("expected %s, got %s", tt.wantType, tt.op.Type)
		}
	}
}

func TestEventHelpers(t *testing.T) {
	turnID := NewTurnId()

	ev := AgentMessageEvent(turnID, "hello")
	if ev.Type != EvAgentMessage || ev.Message != "hello" {
		t.Errorf("unexpected event: %+v", ev)
	}

	ev = TurnStartedEvent(turnID)
	if ev.Type != EvTurnStarted || ev.TurnID != turnID {
		t.Errorf("unexpected event: %+v", ev)
	}
}

func TestNewSubmissionId(t *testing.T) {
	seen := make(map[SubmissionId]struct{}, 1000)
	prev := int64(0)
	for i := 0; i < 1000; i++ {
		id := NewSubmissionId()
		// 十进制字符串：int64 全范围可安全往返 JSON。
		// Decimal string: the full int64 range survives JSON round trips.
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			t.Fatalf("ID %q is not a decimal int64: %v", id, err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate submission ID %q at iteration %d", id, i)
		}
		seen[id] = struct{}{}
		// 雪花 ID 内嵌时间戳，同进程内单调不减。
		// Snowflake IDs embed a timestamp: monotonic within a process.
		if n < prev {
			t.Fatalf("ID went backwards: prev=%d cur=%d", prev, n)
		}
		prev = n
	}
}

func TestIsFunctionCall(t *testing.T) {
	tests := []struct {
		item ResponseItem
		want bool
	}{
		{NewFunctionCall("c1", "shell", "{}"), true},
		{NewUserMessage("hi"), false},
		{NewAssistantMessage("hi"), false},
		{NewReasoning("thinking"), false},
	}

	for _, tt := range tests {
		if got := tt.item.IsFunctionCall(); got != tt.want {
			t.Errorf("IsFunctionCall() = %v, want %v", got, tt.want)
		}
	}
}
