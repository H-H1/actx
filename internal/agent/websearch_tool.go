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

// WebSearchTool provides web search capability to the agent.
// WebSearchTool 为代理提供网络搜索能力。
type WebSearchTool struct {
	spec model.ToolSpec
}

// NewWebSearchTool creates a WebSearchTool.
// NewWebSearchTool 创建一个 WebSearchTool。
func NewWebSearchTool() *WebSearchTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query.",
			},
			"num_results": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return (default 5).",
			},
		},
		"required": []string{"query"},
	}
	paramsJSON, _ := json.Marshal(params)
	return &WebSearchTool{
		spec: model.NewFunctionTool("web_search", "Search the web for information.", paramsJSON),
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
	// Query is the search query string.
	// Query 是搜索查询字符串。
	Query string `json:"query"`
	// NumResults is the maximum number of results to return; defaults to 5.
	// NumResults 是返回结果的最大数量；默认为 5。
	NumResults int `json:"num_results,omitempty"`
}

// Handle performs a web search using a public search API.
// Handle 使用公开搜索 API 执行网络搜索。
func (t *WebSearchTool) Handle(ctx context.Context, inv tools.ToolInvocation) (tools.ToolOutput, error) {
	var args webSearchArgs
	if err := json.Unmarshal([]byte(inv.Arguments), &args); err != nil {
		return nil, tools.NewRespondToModelError(fmt.Sprintf("invalid arguments: %v", err))
	}

	if args.Query == "" {
		return nil, tools.NewRespondToModelError("query is required")
	}

	if args.NumResults == 0 {
		args.NumResults = 5
	}

	// Use DuckDuckGo Instant Answer API as a free search backend.
	// 使用 DuckDuckGo Instant Answer API 作为免费搜索后端。
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1", url.QueryEscape(args.Query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("create request: %v", err))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("search request failed: %v", err))
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("parse response: %v", err))
	}

	var sb strings.Builder
	if abstract, ok := result["AbstractText"].(string); ok && abstract != "" {
		sb.WriteString(abstract)
		sb.WriteString("\n\n")
	}
	if heading, ok := result["Heading"].(string); ok && heading != "" {
		sb.WriteString("Topic: " + heading + "\n")
	}
	// Related topics.
	// 相关主题。
	if related, ok := result["RelatedTopics"].([]interface{}); ok {
		count := 0
		for _, item := range related {
			if count >= args.NumResults {
				break
			}
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["Text"].(string); ok && text != "" {
					count++
					sb.WriteString(fmt.Sprintf("%d. %s\n", count, text))
				}
			}
		}
	}

	if sb.Len() == 0 {
		sb.WriteString("No results found for: " + args.Query)
	}

	return &tools.ExecOutput{
		Stdout: sb.String(),
		Exit:   0,
	}, nil
}
