package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// deepSeekToolsBaseURL is the default base URL for the DeepSeek Search/Fetch
// tool APIs. It can be overridden per tool (e.g. for tests).
// deepSeekToolsBaseURL 是 DeepSeek Search/Fetch 工具 API 的默认基础 URL，
// 可按工具覆盖（例如测试时）。
const deepSeekToolsBaseURL = "https://api.deepseek.com"

// webToolsClient is a small HTTP client shared by the web_search and
// web_fetch tools. Both DeepSeek tool APIs use the same Bearer auth as the
// model API.
// webToolsClient 是 web_search 与 web_fetch 工具共享的小型 HTTP 客户端。
// DeepSeek 的两个工具 API 与模型 API 使用相同的 Bearer 鉴权。
type webToolsClient struct {
	// APIKey is the DeepSeek API key used for Bearer auth.
	// APIKey 是用于 Bearer 鉴权的 DeepSeek API 密钥。
	APIKey string
	// BaseURL is the API base URL, e.g. https://api.deepseek.com.
	// BaseURL 是 API 基础 URL，例如 https://api.deepseek.com。
	BaseURL string
	// HTTP is the underlying HTTP client.
	// HTTP 是底层 HTTP 客户端。
	HTTP *http.Client
}

func newWebToolsClient(apiKey, baseURL string) *webToolsClient {
	if baseURL == "" {
		baseURL = deepSeekToolsBaseURL
	}
	return &webToolsClient{
		APIKey:  apiKey,
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// postJSON sends a POST request with a JSON body to the given API path and
// returns the status code and response body. Non-2xx responses are returned
// with their status so callers can craft model-friendly error messages.
// postJSON 向给定 API 路径发送携带 JSON 请求体的 POST 请求，返回状态码与
// 响应体。非 2xx 响应连同状态码一并返回，由调用方构造对模型友好的错误信息。
func (c *webToolsClient) postJSON(ctx context.Context, path string, payload interface{}) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, data, nil
}

// apiErrorMessage extracts the OpenAI-compatible error message from a
// DeepSeek tool API error response, falling back to the status code.
// apiErrorMessage 从 DeepSeek 工具 API 的错误响应中提取 OpenAI 兼容的
// 错误信息，失败时回退为状态码描述。
func apiErrorMessage(status int, body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Error.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", status, e.Error.Message)
	}
	return fmt.Sprintf("HTTP %d: %s", status, truncateOutput(string(body), 500))
}

// WebSearchTool searches the web via the DeepSeek Search API.
// WebSearchTool 通过 DeepSeek Search API 进行网络搜索。
type WebSearchTool struct {
	spec   model.ToolSpec
	client *webToolsClient
}

// NewWebSearchTool creates a WebSearchTool. apiKey and baseURL come from the
// agent configuration; they default to the standard DeepSeek endpoint.
// NewWebSearchTool 创建一个 WebSearchTool。apiKey 与 baseURL 来自代理配置，
// 为空时使用标准 DeepSeek 端点。
func NewWebSearchTool(apiKey, baseURL string) *WebSearchTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query, 1-128 Unicode characters. Supports operators: site:, -site:, -term, \"exact phrase\", lang:zh,en, after:YYYY-MM-DD, before:YYYY-MM-DD.",
			},
			"count": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return, 1-50 (default 5).",
			},
			"need_content": map[string]interface{}{
				"type":        "boolean",
				"description": "Return full page content in each result (default false).",
			},
			"ori_question": map[string]interface{}{
				"type":        "string",
				"description": "Optional original user question that triggered the search, to improve intent understanding.",
			},
		},
		"required": []string{"query"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &WebSearchTool{
		spec:   model.NewFunctionTool("web_search", "Search the web for real-time information. Returns structured results with title, url, summary and optionally page content.", paramsJSON),
		client: newWebToolsClient(apiKey, baseURL),
	}
}

// Name returns the tool name.
// Name 返回工具名称。
func (t *WebSearchTool) Name() protocol.ToolName { return "web_search" }

// Spec returns the tool specification for the model API.
// Spec 返回用于模型 API 的工具规范。
func (t *WebSearchTool) Spec() model.ToolSpec { return t.spec }

// webSearchArgs represents the arguments for the web_search tool.
// webSearchArgs 表示 web_search 工具的参数。
type webSearchArgs struct {
	// Query is the search query string (1-128 chars).
	// Query 是搜索查询字符串（1-128 字符）。
	Query string `json:"query"`
	// Count is the number of results, 1-50.
	// Count 是返回条数，1-50。
	Count int `json:"count,omitempty"`
	// NeedContent requests full page content per result.
	// NeedContent 请求在每个结果中返回网页正文。
	NeedContent bool `json:"need_content,omitempty"`
	// OriQuestion is the optional original user question.
	// OriQuestion 是可选的触发搜索的用户原始问题。
	OriQuestion string `json:"ori_question,omitempty"`
}

// searchResult is one entry of the Search API results array.
// searchResult 是 Search API results 数组中的一项。
type searchResult struct {
	RerankScore float64 `json:"rerank_score"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Summary     string  `json:"summary"`
	Content     string  `json:"content"`
	SiteName    *string `json:"site_name"`
	Date        *string `json:"date"`
	Authority   int     `json:"authority"`
}

// Handle performs a web search via the DeepSeek Search API.
// Handle 通过 DeepSeek Search API 执行网络搜索。
func (t *WebSearchTool) Handle(ctx context.Context, inv tools.ToolInvocation) (tools.ToolOutput, error) {
	var args webSearchArgs
	if err := json.Unmarshal([]byte(inv.Arguments), &args); err != nil {
		return nil, tools.NewRespondToModelError(fmt.Sprintf("invalid arguments: %v", err))
	}

	query := strings.TrimSpace(args.Query)
	if query == "" {
		return nil, tools.NewRespondToModelError("query is required")
	}
	if len([]rune(query)) > 128 {
		return nil, tools.NewRespondToModelError("query must be 1-128 characters")
	}
	if args.Count == 0 {
		args.Count = 5
	}
	if args.Count < 1 || args.Count > 50 {
		return nil, tools.NewRespondToModelError("count must be between 1 and 50")
	}

	payload := map[string]interface{}{
		"query":        query,
		"count":        args.Count,
		"need_content": args.NeedContent,
	}
	if args.OriQuestion != "" {
		payload["ori_question"] = args.OriQuestion
	}

	status, body, err := t.client.postJSON(ctx, "/v1/tools/search", payload)
	if err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("search request failed: %v", err))
	}
	if status != http.StatusOK {
		return &tools.ExecOutput{
			Stderr: apiErrorMessage(status, body),
			Exit:   status,
		}, nil
	}

	var result struct {
		Results []searchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("parse search response: %v", err))
	}

	if len(result.Results) == 0 {
		return &tools.ExecOutput{
			Stdout: "No results found for: " + query,
			Exit:   0,
		}, nil
	}

	var sb strings.Builder
	for i, r := range result.Results {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, r.Title)
		fmt.Fprintf(&sb, "   URL: %s\n", r.URL)
		if r.SiteName != nil && *r.SiteName != "" {
			fmt.Fprintf(&sb, "   Site: %s\n", *r.SiteName)
		}
		if r.Date != nil && *r.Date != "" {
			fmt.Fprintf(&sb, "   Date: %s\n", *r.Date)
		}
		fmt.Fprintf(&sb, "   Authority: %d\n", r.Authority)
		// summary is the model-facing long abstract; fall back to snippet.
		// summary 是面向模型的长摘要；缺失时回退为 snippet。
		summary := r.Summary
		if summary == "" {
			summary = r.Snippet
		}
		if summary != "" {
			fmt.Fprintf(&sb, "   Summary: %s\n", summary)
		}
		if r.Content != "" {
			fmt.Fprintf(&sb, "   Content: %s\n", r.Content)
		}
		sb.WriteString("\n")
	}

	return &tools.ExecOutput{
		Stdout: truncateOutput(sb.String(), 30000),
		Exit:   0,
	}, nil
}
