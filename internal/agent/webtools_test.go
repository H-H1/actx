package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/H-H1/actx/internal/tools"
)

// newTestInvocation 构造最小可用的 ToolInvocation。
func newTestInvocation(args string) tools.ToolInvocation {
	return tools.ToolInvocation{Arguments: args}
}

// TestWebSearchToolSuccess 验证 Search API 的正常解析与格式化。
func TestWebSearchToolSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected auth %q", got)
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req["query"] != "DeepSeek V4" {
			t.Errorf("unexpected query %v", req["query"])
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"rerank_score": 2.4,
					"title":        "DeepSeek V4 模型发布",
					"url":          "https://example.com/v4",
					"snippet":      "V4 已上线",
					"summary":      "DeepSeek 发布了 V4 模型。",
					"date":         "2026-06-01T00:00:00+00:00",
					"authority":    3,
				},
			},
		})
	}))
	defer srv.Close()

	tool := NewWebSearchTool("test-key", srv.URL)
	out, err := tool.Handle(context.Background(), newTestInvocation(`{"query": "DeepSeek V4", "count": 5}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	exec, ok := out.(*tools.ExecOutput)
	if !ok {
		t.Fatalf("unexpected output type %T", out)
	}
	if exec.Exit != 0 {
		t.Fatalf("exit %d, stderr %q", exec.Exit, exec.Stderr)
	}
	for _, want := range []string{"DeepSeek V4 模型发布", "https://example.com/v4", "DeepSeek 发布了 V4 模型。", "Authority: 3"} {
		if !strings.Contains(exec.Stdout, want) {
			t.Errorf("output missing %q:\n%s", want, exec.Stdout)
		}
	}
}

// TestWebSearchToolEmptyAndErrors 验证空结果与非 200 错误的处理。
func TestWebSearchToolEmptyAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "empty") {
			json.NewEncoder(w).Encode(map[string]interface{}{"results": []interface{}{}})
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"message": "账号余额不足", "type": "invalid_request_error"},
		})
	}))
	defer srv.Close()

	tool := NewWebSearchTool("test-key", srv.URL)

	out, err := tool.Handle(context.Background(), newTestInvocation(`{"query": "empty case"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if exec := out.(*tools.ExecOutput); !strings.Contains(exec.Stdout, "No results found") {
		t.Errorf("expected empty-results message, got: %s", exec.Stdout)
	}

	out, err = tool.Handle(context.Background(), newTestInvocation(`{"query": "error case"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	exec := out.(*tools.ExecOutput)
	if exec.Exit != http.StatusPaymentRequired {
		t.Errorf("exit %d, want 402", exec.Exit)
	}
	if !strings.Contains(exec.Stderr, "账号余额不足") {
		t.Errorf("stderr missing API message: %s", exec.Stderr)
	}
}

// TestWebSearchToolValidation 验证参数校验。
func TestWebSearchToolValidation(t *testing.T) {
	tool := NewWebSearchTool("test-key", "")

	if _, err := tool.Handle(context.Background(), newTestInvocation(`{"query": "  "}`)); err == nil {
		t.Error("empty query should be rejected")
	}
	if _, err := tool.Handle(context.Background(), newTestInvocation(`{"query": "ok", "count": 99}`)); err == nil {
		t.Error("count > 50 should be rejected")
	}
}

// TestWebFetchToolSuccess 验证 Fetch API 的正常解析。
func TestWebFetchToolSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/fetch" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var req struct {
			URL string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.URL != "https://example.com/article" {
			t.Errorf("unexpected url %q", req.URL)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"title":   "Example article",
			"content": "# Example article\n\nArticle content.",
		})
	}))
	defer srv.Close()

	tool := NewWebFetchTool("test-key", srv.URL)
	out, err := tool.Handle(context.Background(), newTestInvocation(`{"url": "https://example.com/article"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	exec := out.(*tools.ExecOutput)
	if exec.Exit != 0 {
		t.Fatalf("exit %d, stderr %q", exec.Exit, exec.Stderr)
	}
	for _, want := range []string{"Title: Example article", "# Example article", "Article content."} {
		if !strings.Contains(exec.Stdout, want) {
			t.Errorf("output missing %q:\n%s", want, exec.Stdout)
		}
	}
}

// TestWebFetchToolValidation 验证 URL 校验规则。
func TestWebFetchToolValidation(t *testing.T) {
	tool := NewWebFetchTool("test-key", "")
	cases := []string{
		`{"url": ""}`,
		`{"url": "ftp://example.com/x"}`,
		`{"url": "https://"` + strings.Repeat("a", 8300) + `"}`,
		`{"url": "https://user:pass@example.com/x"}`,
	}
	for _, args := range cases {
		if _, err := tool.Handle(context.Background(), newTestInvocation(args)); err == nil {
			t.Errorf("args %s should be rejected", args)
		}
	}
}

// TestWebFetchToolNotFound 验证 Fetch 404 时错误信息返回给模型。
func TestWebFetchToolNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"message": "未能获取可用的网页内容", "type": "invalid_request_error"},
		})
	}))
	defer srv.Close()

	tool := NewWebFetchTool("test-key", srv.URL)
	out, err := tool.Handle(context.Background(), newTestInvocation(`{"url": "https://blocked.example.com/x"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	exec := out.(*tools.ExecOutput)
	if exec.Exit != http.StatusNotFound {
		t.Errorf("exit %d, want 404", exec.Exit)
	}
	if !strings.Contains(exec.Stderr, "未能获取可用的网页内容") {
		t.Errorf("stderr missing API message: %s", exec.Stderr)
	}
}
