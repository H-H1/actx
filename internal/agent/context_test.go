package agent

import (
	"testing"

	"github.com/H-H1/actx/internal/protocol"
)

func TestContextManagerAppend(t *testing.T) {
	cm := NewContextManager("system instructions")

	if cm.Len() != 1 {
		t.Errorf("expected 1 item (system), got %d", cm.Len())
	}

	cm.Append(protocol.NewUserMessage("hello"))
	if cm.Len() != 2 {
		t.Errorf("expected 2 items, got %d", cm.Len())
	}

	items := cm.Items()
	if items[0].Role != "system" {
		t.Errorf("expected system message first, got %s", items[0].Role)
	}
	if items[1].GetText() != "hello" {
		t.Errorf("expected 'hello', got %s", items[1].GetText())
	}
}

func TestContextManagerVersion(t *testing.T) {
	cm := NewContextManager("system")
	v1 := cm.Version()

	cm.Append(protocol.NewUserMessage("hello"))
	if cm.Version() != v1 {
		t.Error("version should not change on append")
	}

	cm.Compact("summary", 1)
	if cm.Version() <= v1 {
		t.Error("version should increment on compact")
	}
}

func TestContextManagerCompact(t *testing.T) {
	cm := NewContextManager("system")
	cm.Append(protocol.NewUserMessage("msg1"))
	cm.Append(protocol.NewAssistantMessage("resp1"))
	cm.Append(protocol.NewUserMessage("msg2"))
	cm.Append(protocol.NewAssistantMessage("resp2"))

	cm.Compact("conversation summary", 2)

	items := cm.Items()
	// Should have: system, summary, 2 recent items
	if len(items) != 4 {
		t.Errorf("expected 4 items after compact, got %d", len(items))
	}

	if items[0].Role != "system" {
		t.Errorf("expected system message first, got %s", items[0].Role)
	}
}

func TestContextManagerRollback(t *testing.T) {
	cm := NewContextManager("system")
	cm.Append(protocol.NewUserMessage("msg1"))
	cm.Append(protocol.NewAssistantMessage("resp1"))
	cm.Append(protocol.NewUserMessage("msg2"))

	// Rollback to after system + msg1 + resp1 (keep 3 items).
	cm.Rollback(3)

	if cm.Len() != 3 {
		t.Errorf("expected 3 items after rollback, got %d", cm.Len())
	}

	items := cm.Items()
	if items[2].GetText() != "resp1" {
		t.Errorf("expected last item to be 'resp1', got %s", items[2].GetText())
	}
}

func TestContextManagerClear(t *testing.T) {
	cm := NewContextManager("system")
	cm.Append(protocol.NewUserMessage("hello"))
	cm.Append(protocol.NewAssistantMessage("world"))

	cm.Clear()

	if cm.Len() != 1 {
		t.Errorf("expected 1 item (system) after clear, got %d", cm.Len())
	}

	items := cm.Items()
	if items[0].Role != "system" {
		t.Errorf("expected system message to survive clear, got %s", items[0].Role)
	}
}
