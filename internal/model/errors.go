package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIError represents an error returned by the OpenAI API. It captures the
// HTTP status code, the human-readable message, and the API-supplied error type
// (for example "rate_limit_exceeded" or "invalid_api_key").
// APIError 表示 OpenAI API 返回的错误。它捕获 HTTP 状态码、人类可读的消息以及 API
// 提供的错误类型（例如 "rate_limit_exceeded" 或 "invalid_api_key"）。
type APIError struct {
	StatusCode int
	Message    string
	Type       string
}

// Error implements the error interface.
// Error 实现 error 接口。
func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("openai api error (status %d, type %s): %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("openai api error (status %d): %s", e.StatusCode, e.Message)
}

// RateLimitError is returned when the API responds with HTTP 429. It wraps the
// underlying APIError so callers can use errors.As to detect rate limiting
// while still accessing the full error details.
// RateLimitError 在 API 响应 HTTP 429 时返回。它包装了底层的 APIError，以便调用者
// 可以使用 errors.As 检测速率限制，同时仍能访问完整的错误详情。
type RateLimitError struct {
	// APIError is the wrapped underlying API error.
	// APIError 是被包装的底层 API 错误。
	*APIError
}

// AuthError is returned when the API responds with HTTP 401 or 403, indicating
// an authentication or authorization failure. It wraps the underlying APIError.
// AuthError 在 API 响应 HTTP 401 或 403 时返回，表示认证或授权失败。它包装了底层的 APIError。
type AuthError struct {
	*APIError
}

// Unwrap allows errors.As to recover the wrapped *APIError from a
// RateLimitError.
// Unwrap 允许 errors.As 从 RateLimitError 中恢复包装的 *APIError。
func (e *RateLimitError) Unwrap() error { return e.APIError }

// Sentinel error kinds for mid-stream failures delivered via the
// response.failed SSE event. Unlike HTTP-status errors there is no status
// code to branch on, so the message text is classified into these kinds;
// callers can use errors.Is to react (e.g. abort on context window
// exceeded, retry on server overloaded).
// 通过 response.failed SSE 事件传递的流内失败错误的哨兵类别。
// 与 HTTP 状态码错误不同，这里没有状态码可供分支，因此将消息文本
// 归类为这些类别；调用者可用 errors.Is 做出反应（如上下文超限时中止、
// 服务器超载时重试）。
var (
	// ErrContextWindowExceeded marks prompts larger than the model context.
	// ErrContextWindowExceeded 标记提示超过模型上下文窗口。
	ErrContextWindowExceeded = errors.New("context window exceeded")
	// ErrQuotaExceeded marks exhausted balance or quota.
	// ErrQuotaExceeded 标记余额或配额耗尽。
	ErrQuotaExceeded = errors.New("quota exceeded")
	// ErrServerOverloaded marks transient server capacity errors.
	// ErrServerOverloaded 标记瞬时的服务器容量错误。
	ErrServerOverloaded = errors.New("server overloaded")
)

// classifyResponsesError maps a response.failed message to a typed error:
// one of the sentinel kinds above, a *RateLimitError for rate limiting, or
// a plain *APIError carrying the original message. Mirrors the Rust
// implementation's failure taxonomy.
// classifyResponsesError 把 response.failed 的消息映射为类型化错误：
// 上述哨兵类别之一、速率限制的 *RateLimitError，或携带原始消息的纯
// *APIError。与 Rust 实现的失败分类保持一致。
func classifyResponsesError(msg string) error {
	apiErr := &APIError{Message: msg, Type: "responses_stream_error"}
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "context length") || strings.Contains(m, "context window") ||
		strings.Contains(m, "maximum context") || strings.Contains(m, "too many tokens"):
		return fmt.Errorf("%w: %s", ErrContextWindowExceeded, msg)
	case strings.Contains(m, "quota") || strings.Contains(m, "balance") ||
		strings.Contains(m, "insufficient"):
		return fmt.Errorf("%w: %s", ErrQuotaExceeded, msg)
	case strings.Contains(m, "overloaded") || strings.Contains(m, "at capacity"):
		return fmt.Errorf("%w: %s", ErrServerOverloaded, msg)
	case strings.Contains(m, "rate limit"):
		return &RateLimitError{APIError: apiErr}
	default:
		return apiErr
	}
}

// Unwrap allows errors.As to recover the wrapped *APIError from an AuthError.
// Unwrap 允许 errors.As 从 AuthError 中恢复包装的 *APIError。
func (e *AuthError) Unwrap() error { return e.APIError }

// parseAPIError reads a non-2xx HTTP response body and converts it into a
// typed API error. It attempts to parse the standard OpenAI error envelope
// ({"error":{"message":...,"type":...}}); if that fails it falls back to the
// raw body, and ultimately to the HTTP status text. The returned error is a
// *RateLimitError for 429, an *AuthError for 401/403, or a plain *APIError
// otherwise.
// parseAPIError 读取非 2xx HTTP 响应体并将其转换为类型化的 API 错误。它尝试解析标准
// OpenAI 错误封装（{"error":{"message":...,"type":...}}）；如果失败则回退到原始响应体，
// 最终回退到 HTTP 状态文本。返回的错误：429 时为 *RateLimitError，401/403 时为
// *AuthError，其他情况为纯 *APIError。
func parseAPIError(resp *http.Response) error {
	// Read the entire error body (best effort; read errors are ignored so
	// we still produce an error from the status code).
	// 读取完整错误响应体（尽力而为；读取错误被忽略，
	// 仍可基于状态码生成错误）。
	body, _ := io.ReadAll(resp.Body)

	// Default the message to the raw body and try to decode the structured
	// OpenAI error envelope below.
	// 默认消息为原始响应体，随后尝试解码结构化的 OpenAI 错误封装。
	message := strings.TrimSpace(string(body))
	errType := ""
	if message != "" {
		// errResp is the standard OpenAI error envelope used to parse error bodies.
		// errResp 是用于解析错误响应体的标准 OpenAI 错误封装。
		var errResp struct {
			// Error is the nested error object returned by the API.
			// Error 是 API 返回的嵌套错误对象。
			Error struct {
				// Message is the human-readable error message.
				// Message 是人类可读的错误消息。
				Message string `json:"message"`
				// Type is the machine-readable error type identifier.
				// Type 是机器可读的错误类型标识符。
				Type string `json:"type"`
			} `json:"error"`
		}
		// If the body matches the standard envelope, prefer its structured
		// message and type over the raw text.
		// 如果响应体匹配标准封装，优先使用其结构化消息和类型而非原始文本。
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			message = errResp.Error.Message
			errType = errResp.Error.Type
		}
	}
	// Last resort for empty bodies: use the standard status text.
	// 响应体为空时的最后手段：使用标准状态文本。
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}

	// Build the base error with status, message, and type.
	// 构建包含状态码、消息和类型的基础错误。
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Type:       errType,
	}
	// Wrap into a typed error so callers can branch on the concrete type
	// (e.g. retry on rate limit, re-auth on auth failure).
	// 包装为类型化错误，使调用者可以按具体类型分支处理
	// （例如速率限制时重试、认证失败时重新登录）。
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return &RateLimitError{APIError: apiErr}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &AuthError{APIError: apiErr}
	default:
		return apiErr
	}
}
