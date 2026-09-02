package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
)

// mockExecutor is a test ToolExecutor that returns a fixed output.
type mockExecutor struct {
	name   protocol.ToolName
	spec   model.ToolSpec
	output ToolOutput
}

func (m *mockExecutor) Name() protocol.ToolName { return m.name }
func (m *mockExecutor) Spec() model.ToolSpec    { return m.spec }
func (m *mockExecutor) Handle(_ context.Context, _ ToolInvocation) (ToolOutput, error) {
	return m.output, nil
}

func TestToolRegistryRegister(t *testing.T) {
	registry := NewToolRegistry()

	tool := &mockExecutor{
		name:   "test_tool",
		spec:   model.NewFunctionTool("test_tool", "A test tool", json.RawMessage(`{"type":"object"}`)),
		output: JSONToolOutput{Data: map[string]string{"result": "ok"}},
	}

	registry.Register(tool)

	if len(registry.List()) != 1 {
		t.Errorf("expected 1 tool, got %d", len(registry.List()))
	}

	specs := registry.Specs()
	if len(specs) != 1 {
		t.Errorf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Name != "test_tool" {
		t.Errorf("expected name test_tool, got %s", specs[0].Name)
	}
}

func TestToolRegistryGet(t *testing.T) {
	registry := NewToolRegistry()
	tool := &mockExecutor{name: "shell", spec: model.ToolSpec{}, output: nil}
	registry.Register(tool)

	found, ok := registry.Get("shell")
	if !ok {
		t.Fatal("expected to find 'shell' tool")
	}
	if found.Name() != "shell" {
		t.Errorf("expected name shell, got %s", found.Name())
	}

	_, ok = registry.Get("nonexistent")
	if ok {
		t.Error("expected not to find 'nonexistent' tool")
	}
}

func TestToolRouterDispatch(t *testing.T) {
	registry := NewToolRegistry()
	output := JSONToolOutput{Data: map[string]string{"result": "success"}}
	registry.Register(&mockExecutor{
		name:   "test",
		spec:   model.ToolSpec{},
		output: output,
	})

	router := NewToolRouter(registry)
	result, err := router.Dispatch(context.Background(), ToolInvocation{
		ToolName:  "test",
		CallID:    "call_1",
		Arguments: "{}",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}

	if result.ExitCode() != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode())
	}
}

func TestToolRouterDispatchUnknown(t *testing.T) {
	registry := NewToolRegistry()
	router := NewToolRouter(registry)

	_, err := router.Dispatch(context.Background(), ToolInvocation{
		ToolName: "unknown",
	})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}

	fcErr, ok := err.(*FunctionCallError)
	if !ok {
		t.Fatalf("expected *FunctionCallError, got %T", err)
	}
	if fcErr.Kind != ErrorKindFatal {
		t.Errorf("expected ErrorKindFatal, got %v", fcErr.Kind)
	}
}

func TestJSONToolOutput(t *testing.T) {
	out := JSONToolOutput{
		Data: map[string]string{"key": "value"},
		Exit: 0,
	}

	if out.ExitCode() != 0 {
		t.Errorf("expected exit code 0, got %d", out.ExitCode())
	}

	item := out.ToResponseItem("call_123")
	if item.Type != protocol.RITypeFunctionCallOutput {
		t.Errorf("expected type %s, got %s", protocol.RITypeFunctionCallOutput, item.Type)
	}
	if item.CallID != "call_123" {
		t.Errorf("expected call_id call_123, got %s", item.CallID)
	}
}

func TestExecOutput(t *testing.T) {
	out := ExecOutput{
		Stdout: "hello",
		Stderr: "",
		Exit:   0,
	}

	if out.ExitCode() != 0 {
		t.Errorf("expected exit code 0, got %d", out.ExitCode())
	}

	item := out.ToResponseItem("call_456")
	if item.Output != "hello" {
		t.Errorf("expected output 'hello', got %s", item.Output)
	}
}

func TestExecOutputWithExitCode(t *testing.T) {
	out := ExecOutput{
		Stdout: "output",
		Stderr: "error",
		Exit:   1,
	}

	if out.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", out.ExitCode())
	}

	item := out.ToResponseItem("call_789")
	if item.Output == "" {
		t.Error("expected non-empty output")
	}
}
