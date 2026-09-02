package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/H-H1/actx/internal/model"
)

// toolBlock 构造一个 ```tool ... ``` 代码块字符串。
func toolBlock(jsonContent string) string {
	return "```tool\n" + jsonContent + "\n```"
}

func TestParseToolCallsFromText(t *testing.T) {
	text := "我来查看当前目录的文件。\n\n" +
		toolBlock(`{"name": "shell", "arguments": {"command": ["ls", "-la"]}}`) +
		"\n\n请等待执行结果。"

	calls, clean := ParseToolCallsFromText(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "shell" {
		t.Errorf("expected name 'shell', got '%s'", calls[0].Name)
	}

	var args struct {
		Command []string `json:"command"`
	}
	if err := json.Unmarshal([]byte(calls[0].Arguments), &args); err != nil {
		t.Fatalf("failed to parse arguments: %v", err)
	}
	if len(args.Command) != 2 || args.Command[0] != "ls" || args.Command[1] != "-la" {
		t.Errorf("unexpected command: %v", args.Command)
	}

	if clean == "" {
		t.Error("clean text should not be empty")
	}
	if !strings.Contains(clean, "请等待执行结果") {
		t.Error("clean text should contain non-tool text")
	}
}

func TestParseToolCallsMultiple(t *testing.T) {
	text := "我来执行两个操作。\n\n" +
		toolBlock(`{"name": "shell", "arguments": {"command": ["echo", "hello"]}}`) +
		"\n\n然后：\n\n" +
		toolBlock(`{"name": "shell", "arguments": {"command": ["echo", "world"]}}`) +
		"\n\n完成。"

	calls, clean := ParseToolCallsFromText(text)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "shell" || calls[1].Name != "shell" {
		t.Errorf("unexpected names: %s, %s", calls[0].Name, calls[1].Name)
	}
	if !strings.Contains(clean, "完成") {
		t.Error("clean text should contain trailing text")
	}
}

func TestParseToolCallsNone(t *testing.T) {
	text := "这是一条普通消息，没有工具调用。"
	calls, clean := ParseToolCallsFromText(text)
	if len(calls) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(calls))
	}
	if clean != text {
		t.Error("clean text should equal original when no tool calls")
	}
}

func TestParseToolCallsInvalidJSON(t *testing.T) {
	text := "测试无效JSON。\n\n" + toolBlock("{invalid json}") + "\n"
	calls, _ := ParseToolCallsFromText(text)
	if len(calls) != 0 {
		t.Fatalf("expected 0 calls for invalid JSON, got %d", len(calls))
	}
}

func TestBuildToolPrompt(t *testing.T) {
	specs := []model.ToolSpec{
		{
			Type:        "function",
			Name:        "shell",
			Description: "Execute a shell command",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"command":{"type":"array","items":{"type":"string"}}}}`),
		},
		{
			Type:        "function",
			Name:        "apply_patch",
			Description: "Apply a file patch",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"patch":{"type":"string"}}}`),
		},
	}

	prompt := BuildToolPrompt(specs)
	if prompt == "" {
		t.Fatal("prompt should not be empty")
	}
	if !strings.Contains(prompt, "shell") {
		t.Error("prompt should mention 'shell'")
	}
	if !strings.Contains(prompt, "apply_patch") {
		t.Error("prompt should mention 'apply_patch'")
	}
	if !strings.Contains(prompt, "```tool") {
		t.Error("prompt should contain tool call format example")
	}
}

func TestShouldFallbackToPrompt(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "400 with tools keyword",
			err:      &model.APIError{StatusCode: 400, Message: "tools parameter is not supported", Type: "invalid_request_error"},
			expected: true,
		},
		{
			name:     "400 with reasoning_content keyword",
			err:      &model.APIError{StatusCode: 400, Message: "The reasoning_content is not supported", Type: "invalid_request_error"},
			expected: true,
		},
		{
			name:     "400 with function keyword",
			err:      &model.APIError{StatusCode: 400, Message: "function calling not available", Type: "invalid_request_error"},
			expected: true,
		},
		{
			name:     "400 without relevant keywords",
			err:      &model.APIError{StatusCode: 400, Message: "invalid model name", Type: "invalid_request_error"},
			expected: false,
		},
		{
			name:     "429 rate limit should not fallback",
			err:      &model.APIError{StatusCode: 429, Message: "rate limit exceeded", Type: "rate_limit_exceeded"},
			expected: false,
		},
		{
			name:     "401 auth error should not fallback",
			err:      &model.APIError{StatusCode: 401, Message: "invalid api key", Type: "invalid_api_key"},
			expected: false,
		},
		{
			name:     "non-APIError should not fallback",
			err:      errors.New("network timeout"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldFallbackToPrompt(tt.err)
			if result != tt.expected {
				t.Errorf("shouldFallbackToPrompt() = %v, want %v", result, tt.expected)
			}
		})
	}
}
