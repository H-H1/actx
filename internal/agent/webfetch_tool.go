package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// WebFetchTool fetches a single web page via the DeepSeek Fetch API and
// returns its content as Markdown.
// WebFetchTool 通过 DeepSeek Fetch API 抓取单个网页，并以 Markdown 返回内容。
type WebFetchTool struct {
	spec   model.ToolSpec
	client *webToolsClient
}

// NewWebFetchTool creates a WebFetchTool. apiKey and baseURL come from the
// agent configuration; they default to the standard DeepSeek endpoint.
// NewWebFetchTool 创建一个 WebFetchTool。apiKey 与 baseURL 来自代理配置，
// 为空时使用标准 DeepSeek 端点。
func NewWebFetchTool(apiKey, baseURL string) *WebFetchTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The HTTP/HTTPS URL of the page to fetch (max 8192 chars, must include a host, no username/password).",
			},
		},
		"required": []string{"url"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &WebFetchTool{
		spec:   model.NewFunctionTool("web_fetch", "Fetch a single web page and return its full content as Markdown. Use after web_search to read a specific result in depth.", paramsJSON),
		client: newWebToolsClient(apiKey, baseURL),
	}
}

// Name returns the tool name.
// Name 返回工具名称。
func (t *WebFetchTool) Name() protocol.ToolName { return "web_fetch" }

// Spec returns the tool specification for the model API.
// Spec 返回用于模型 API 的工具规范。
func (t *WebFetchTool) Spec() model.ToolSpec { return t.spec }

// webFetchArgs represents the arguments for the web_fetch tool.
// webFetchArgs 表示 web_fetch 工具的参数。
type webFetchArgs struct {
	// URL is the page address to fetch.
	// URL 是要抓取的网页地址。
	URL string `json:"url"`
}

// Handle fetches a single web page via the DeepSeek Fetch API.
// Handle 通过 DeepSeek Fetch API 抓取单个网页。
func (t *WebFetchTool) Handle(ctx context.Context, inv tools.ToolInvocation) (tools.ToolOutput, error) {
	var args webFetchArgs
	if err := json.Unmarshal([]byte(inv.Arguments), &args); err != nil {
		return nil, tools.NewRespondToModelError(fmt.Sprintf("invalid arguments: %v", err))
	}

	rawURL := strings.TrimSpace(args.URL)
	if rawURL == "" {
		return nil, tools.NewRespondToModelError("url is required")
	}
	if len(rawURL) > 8192 {
		return nil, tools.NewRespondToModelError("url must not exceed 8192 characters")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, tools.NewRespondToModelError("url must be an HTTP/HTTPS URL with a host")
	}
	if u.User != nil {
		return nil, tools.NewRespondToModelError("url must not contain a username or password")
	}

	status, body, err := t.client.postJSON(ctx, "/v1/tools/fetch", map[string]string{"url": rawURL})
	if err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("fetch request failed: %v", err))
	}
	if status != http.StatusOK {
		return &tools.ExecOutput{
			Stderr: apiErrorMessage(status, body),
			Exit:   status,
		}, nil
	}

	var page struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("parse fetch response: %v", err))
	}

	if strings.TrimSpace(page.Content) == "" {
		return &tools.ExecOutput{
			Stdout: fmt.Sprintf("Fetched %s but the page has no extractable content.", rawURL),
			Exit:   0,
		}, nil
	}

	var sb strings.Builder
	if page.Title != "" {
		sb.WriteString("Title: " + page.Title + "\n\n")
	}
	sb.WriteString(page.Content)

	return &tools.ExecOutput{
		Stdout: truncateOutput(sb.String(), 30000),
		Exit:   0,
	}, nil
}
