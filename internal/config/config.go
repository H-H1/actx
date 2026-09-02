package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ----------------------------------------------------------------------------
// ApprovalPolicy（审批策略）
// ----------------------------------------------------------------------------

// ApprovalPolicy 控制代理何时要求用户批准命令执行。
// 它是 Rust AskForApproval 枚举的 Go 对应版本。
//
// 该策略通过 AskForApproval helper 在每个命令调用中应用。
type ApprovalPolicy int8

const (
	// ApprovalNever 从不要求用户批准命令。失败会立即返回给模型且不会升级。
	ApprovalNever ApprovalPolicy = iota

	// ApprovalOnFailure 自动批准在沙箱内执行的命令。
	// 如果沙箱命令失败，执行将升级给用户审批（不使用沙箱）。
	ApprovalOnFailure

	// ApprovalOnRequest 让模型决定何时要求用户批准。这是默认策略。
	ApprovalOnRequest

	// ApprovalOnUntrusted 仅自动批准"已知安全"的只读命令。
	// 其他所有命令都需要明确的用户审批。
	ApprovalOnUntrusted
)

// String 返回在 config.toml 和 CLI 中使用的 kebab-case 表示，
// 匹配 Rust serde 的 rename_all = "kebab-case"。
func (p ApprovalPolicy) String() string {
	switch p {
	case ApprovalNever:
		return "never"
	case ApprovalOnFailure:
		return "on-failure"
	case ApprovalOnRequest:
		return "on-request"
	case ApprovalOnUntrusted:
		return "on-untrusted"
	default:
		return fmt.Sprintf("unknown(%d)", int8(p))
	}
}

// ParseApprovalPolicy 将字符串解析为 ApprovalPolicy。它接受 config.toml 中使用的 kebab-case 形式以及常见别名。
func ParseApprovalPolicy(s string) (ApprovalPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "never":
		return ApprovalNever, nil
	case "on-failure", "onfailure":
		return ApprovalOnFailure, nil
	case "on-request", "onrequest":
		return ApprovalOnRequest, nil
	case "on-untrusted", "onuntrusted", "untrusted", "unless-trusted":
		return ApprovalOnUntrusted, nil
	default:
		return 0, fmt.Errorf("unknown approval policy %q (expected never, on-failure, on-request, or on-untrusted)", s)
	}
}

// MarshalText implements encoding.TextMarshaler so ApprovalPolicy can be used
// in JSON/TOML serialization.
// MarshalText 实现 encoding.TextMarshaler 接口，使 ApprovalPolicy 可用于 JSON/TOML 序列化。
func (p ApprovalPolicy) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler so ApprovalPolicy can be
// parsed from JSON/TOML strings.
// UnmarshalText 实现 encoding.TextUnmarshaler 接口，使 ApprovalPolicy 可从 JSON/TOML 字符串解析。
func (p *ApprovalPolicy) UnmarshalText(text []byte) error {
	parsed, err := ParseApprovalPolicy(string(text))
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// ----------------------------------------------------------------------------
// SandboxPolicy
// ----------------------------------------------------------------------------

// SandboxPolicy controls the filesystem and network restrictions applied to
// agent-spawned commands. It is the Go counterpart of the Rust SandboxMode
// enum, extended with a None sentinel for "not yet configured".
// SandboxPolicy 控制对代理生成命令应用的文件系统和网络限制。
// 它是 Rust SandboxMode 枚举的 Go 对应版本，扩展了用于"尚未配置"的 None 哨兵值。
type SandboxPolicy int8

const (
	// SandboxNone indicates that no sandbox policy has been set. The builder
	// replaces this with DefaultSandboxPolicy during resolution.
	// SandboxNone 表示尚未设置沙箱策略。构建器在解析期间会将其替换为 DefaultSandboxPolicy。
	SandboxNone SandboxPolicy = iota

	// SandboxReadOnly restricts the agent to read-only filesystem access.
	// Network access is disabled.
	// SandboxReadOnly 将代理限制为只读文件系统访问。网络访问被禁用。
	SandboxReadOnly

	// SandboxWorkspaceWrite allows the agent to write to the current workspace
	// root and any explicitly configured writable paths. Network access is
	// disabled by default.
	// SandboxWorkspaceWrite 允许代理写入当前工作区根目录和任何明确配置的可写路径。
	// 网络访问默认禁用。
	SandboxWorkspaceWrite

	// SandboxDangerFullAccess disables the sandbox entirely. The agent may
	// write to any path and access the network without restriction.
	// SandboxDangerFullAccess 完全禁用沙箱。代理可以写入任何路径并无限制地访问网络。
	SandboxDangerFullAccess
)

// String returns the kebab-case representation used in config.toml and on the
// CLI, matching the Rust serde rename_all = "kebab-case".
// String 返回在 config.toml 和 CLI 中使用的 kebab-case 表示，匹配 Rust serde 的 rename_all = "kebab-case"。
func (s SandboxPolicy) String() string {
	switch s {
	case SandboxNone:
		return "none"
	case SandboxReadOnly:
		return "read-only"
	case SandboxWorkspaceWrite:
		return "workspace-write"
	case SandboxDangerFullAccess:
		return "danger-full-access"
	default:
		return fmt.Sprintf("unknown(%d)", int8(s))
	}
}

// ParseSandboxPolicy parses a string into a SandboxPolicy. It accepts the
// kebab-case forms used in config.toml as well as common aliases.
// ParseSandboxPolicy 将字符串解析为 SandboxPolicy。它接受 config.toml 中使用的 kebab-case 形式以及常见别名。
func ParseSandboxPolicy(s string) (SandboxPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return SandboxNone, nil
	case "read-only", "readonly":
		return SandboxReadOnly, nil
	case "workspace-write", "workspacewrite":
		return SandboxWorkspaceWrite, nil
	case "danger-full-access", "dangerfullaccess", "full-access":
		return SandboxDangerFullAccess, nil
	default:
		return 0, fmt.Errorf("unknown sandbox policy %q (expected read-only, workspace-write, or danger-full-access)", s)
	}
}

// MarshalText implements encoding.TextMarshaler so SandboxPolicy can be used
// in JSON/TOML serialization.
// MarshalText 实现 encoding.TextMarshaler 接口，使 SandboxPolicy 可用于 JSON/TOML 序列化。
func (s SandboxPolicy) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler so SandboxPolicy can be
// parsed from JSON/TOML strings.
// UnmarshalText 实现 encoding.TextUnmarshaler 接口，使 SandboxPolicy 可从 JSON/TOML 字符串解析。
func (s *SandboxPolicy) UnmarshalText(text []byte) error {
	parsed, err := ParseSandboxPolicy(string(text))
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// ----------------------------------------------------------------------------
// McpServerConfig
// ----------------------------------------------------------------------------

// McpServerConfig describes a single MCP (Model Context Protocol) server that
// the agent can reach out to for tool calls. This is a simplified version of
// the Rust McpServerConfig type.
// McpServerConfig 描述代理可以连接以进行工具调用的单个 MCP（模型上下文协议）服务器。
// 这是 Rust McpServerConfig 类型的简化版本。
type McpServerConfig struct {
	// Command is the executable to launch for stdio-based MCP servers.
	// Command 是用于启动基于 stdio 的 MCP 服务器的可执行文件。
	Command string

	// Args are the command-line arguments passed to Command.
	// Args 是传递给 Command 的命令行参数。
	Args []string

	// Env holds environment variables to set for the MCP server process.
	// Env 包含为 MCP 服务器进程设置的环境变量。
	Env map[string]string

	// URL is the endpoint for SSE/streamable-HTTP MCP servers. When set,
	// Command is ignored.
	// URL 是 SSE/streamable-HTTP MCP 服务器的端点。设置后，Command 将被忽略。
	URL string
}

// ----------------------------------------------------------------------------
// Config
// ----------------------------------------------------------------------------

// Config holds the effective configuration for a ACTX agent session after
// all layers (defaults, home config, project config, CLI overrides) have been
// merged.
// Config 保存 ACTX 代理会话经过所有层级（默认值、主目录配置、项目配置、CLI 覆盖）
// 合并后的有效配置。
type Config struct {
	// Model is the model identifier sent to the provider (for example
	// "o4-mini").
	// Model 是发送给提供商的模型标识符（例如 "o4-mini"）。
	Model string

	// ModelProvider is the provider key from the model_providers map
	// (for example "openai").
	// ModelProvider 是来自 model_providers 映射的提供商键（例如 "openai"）。
	ModelProvider string

	// Models is a list of model identifiers to try in order. When non-empty,
	// Model is set to the first entry. Populated from .actx/config.json.
	// Models 是按顺序尝试的模型标识符列表。非空时，Model 设置为第一个条目。从 .actx/config.json 填充。
	Models []string

	// BaseURL is the API base URL, overriding the model client default.
	// Populated from .actx/config.json.
	// BaseURL 是 API 基础 URL，覆盖模型客户端的默认值。从 .actx/config.json 填充。
	BaseURL string

	// ApprovalPolicy controls when the user is asked to approve commands.
	// ApprovalPolicy 控制用户何时被要求批准命令。
	ApprovalPolicy ApprovalPolicy

	// SandboxPolicy controls filesystem and network restrictions.
	// SandboxPolicy 控制文件系统和网络限制。
	SandboxPolicy SandboxPolicy

	// WritableRoots lists additional directories the agent may write to
	// beyond the workspace root. Only effective when SandboxPolicy is
	// SandboxWorkspaceWrite.
	// WritableRoots 列出代理可以写入的工作区根目录之外的额外目录。
	// 仅在 SandboxPolicy 为 SandboxWorkspaceWrite 时生效。
	WritableRoots []string

	// BaseInstructions is the system prompt injected into every request.
	// BaseInstructions 是注入到每个请求中的系统提示。
	BaseInstructions string

	// UserInstructions is additional instructions from config.toml
	// "instructions" field, appended after BaseInstructions.
	// UserInstructions 是来自 config.toml "instructions" 字段的额外指令，追加在 BaseInstructions 之后。
	UserInstructions string

	// Cwd is the working directory the agent operates in.
	// Cwd 是代理操作的工作目录。
	Cwd string

	// ACTXHome is the path to the ACTX home directory (~/.ACTX or
	// ACTX_HOME).
	// ACTXHome 是 ACTX 主目录的路径（~/.ACTX 或 ACTX_HOME）。
	ACTXHome string

	// McpServers maps server names to their connection configuration.
	// McpServers 将服务器名称映射到其连接配置。
	McpServers map[string]McpServerConfig

	// ToolsWebSearch enables the web search tool when true.
	// ToolsWebSearch 为 true 时启用网络搜索工具。
	ToolsWebSearch bool

	// ToolsMode controls how tools are presented to the model.
	// "api" (default) sends tools via the API tools parameter (requires
	// function-calling support). "prompt" injects tool descriptions into
	// the system prompt and parses tool calls from model text output.
	// ToolsMode 控制工具如何呈现给模型。
	// "api"（默认）通过 API tools 参数发送工具（需要函数调用支持）。
	// "prompt" 将工具描述注入系统提示词，并从模型文本输出中解析工具调用。
	ToolsMode string

	// History is the maximum number of lines retained in the persistent
	// history file.
	// History 是持久化历史文件中保留的最大行数。
	History int

	// ModelContextWindow is the token limit for the model's context window.
	// ModelContextWindow 是模型上下文窗口的令牌限制。
	ModelContextWindow int
}

// ----------------------------------------------------------------------------
// Minimal TOML parser
// ----------------------------------------------------------------------------

// tomlTable is a minimal representation of a TOML table with support for
// nested sub-tables and string values. It is intentionally simple and does
// not implement the full TOML specification—only the subset used by ACTX
// config files.
// tomlTable 是 TOML 表的最小表示，支持嵌套子表和字符串值。
// 它有意保持简单，不实现完整的 TOML 规范——仅支持 ACTX 配置文件所使用的子集。
type tomlTable struct {
	// values holds key-value pairs at this table level. Values are stored as
	// raw strings with surrounding quotes stripped. Array values retain their
	// bracket syntax for later parsing by getStringSlice.
	// values 保存此表级别的键值对。值以去除引号的原始字符串形式存储。
	// 数组值保留其括号语法，供后续 getStringSlice 解析。
	values map[string]string

	// tables holds nested sub-tables keyed by their table name.
	// tables 保存按表名索引的嵌套子表。
	tables map[string]*tomlTable
}

func newTomlTable() *tomlTable {
	return &tomlTable{
		values: make(map[string]string),
		tables: make(map[string]*tomlTable),
	}
}

// getString returns the string value for key and whether it was present.
// getString 返回 key 对应的字符串值以及是否存在的标志。
func (t *tomlTable) getString(key string) (string, bool) {
	v, ok := t.values[key]
	return v, ok
}

// getBool returns the boolean value for key and whether it was present.
// getBool 返回 key 对应的布尔值以及是否存在的标志。
func (t *tomlTable) getBool(key string) (bool, bool) {
	v, ok := t.values[key]
	if !ok {
		return false, false
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, false
	}
	return b, true
}

// getInt returns the integer value for key and whether it was present.
// getInt 返回 key 对应的整数值以及是否存在的标志。
func (t *tomlTable) getInt(key string) (int, bool) {
	v, ok := t.values[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, false
	}
	return n, true
}

// getStringSlice parses a TOML string array value (["a", "b"]) into a Go
// string slice. It returns the slice and whether the key was present.
// getStringSlice 将 TOML 字符串数组值（如 ["a", "b"]）解析为 Go 字符串切片。
// 返回切片以及 key 是否存在的标志。
func (t *tomlTable) getStringSlice(key string) ([]string, bool) {
	v, ok := t.values[key]
	if !ok {
		return nil, false
	}
	return parseTomlArray(v), true
}

// getTable returns the nested sub-table with the given name, or nil.
// getTable 返回指定名称的嵌套子表，如果不存在则返回 nil。
func (t *tomlTable) getTable(name string) *tomlTable {
	return t.tables[name]
}

// parseTomlArray parses a TOML inline array of strings like
// `["-y", "@some/server"]` into a Go []string. It is intentionally tolerant
// of whitespace and single/double quoting.
// parseTomlArray 将 TOML 内联字符串数组（如 `["-y", "@some/server"]`）解析为 Go []string。
// 它有意容忍空格和单/双引号。
func parseTomlArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}

	var result []string
	for _, part := range splitArrayItems(s) {
		part = strings.TrimSpace(part)
		part = unquoteTomlString(part)
		result = append(result, part)
	}
	return result
}

// splitArrayItems splits a comma-separated list of TOML string values,
// respecting double-quoted strings that may contain commas.
// splitArrayItems 分割逗号分隔的 TOML 字符串值列表，正确处理可能包含逗号的双引号字符串。
func splitArrayItems(s string) []string {
	var items []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, ch := range s {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' && inQuotes {
			current.WriteRune(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			current.WriteRune(ch)
			inQuotes = !inQuotes
			continue
		}
		if ch == ',' && !inQuotes {
			items = append(items, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(ch)
	}
	if current.Len() > 0 {
		items = append(items, current.String())
	}
	return items
}

// unquoteTomlString removes surrounding quotes from a TOML string value and
// unescapes basic escape sequences.
// unquoteTomlString 去除 TOML 字符串值周围的引号，并反转义基本转义序列。
func unquoteTomlString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			inner := s[1 : len(s)-1]
			if s[0] == '"' {
				// Process escape sequences for double-quoted strings.
				// 处理双引号字符串的转义序列。
				inner = strings.ReplaceAll(inner, "\\\"", "\"")
				inner = strings.ReplaceAll(inner, "\\n", "\n")
				inner = strings.ReplaceAll(inner, "\\t", "\t")
				inner = strings.ReplaceAll(inner, "\\\\", "\\")
			}
			return inner
		}
	}
	return s
}

// parseTOML parses raw TOML data into a tomlTable tree. It supports:
//   - Comments (lines starting with #)
//   - Top-level key = value pairs
//   - Section headers [section] and nested [section.subsection]
//   - String, integer, and boolean values
//   - Inline string arrays ["a", "b"]
//
// It does not support inline tables { }, multiline strings, or datetimes.
// parseTOML 将原始 TOML 数据解析为 tomlTable 树。它支持：
//   - 注释（以 # 开头的行）
//   - 顶层 key = value 键值对
//   - 节头 [section] 和嵌套 [section.subsection]
//   - 字符串、整数和布尔值
//   - 内联字符串数组 ["a", "b"]
//
// 它不支持内联表 { }、多行字符串或日期时间。
func parseTOML(data []byte) (*tomlTable, error) {
	root := newTomlTable()
	current := root

	lines := strings.Split(string(data), "\n")
	for lineNum, raw := range lines {
		line := raw

		// Strip trailing carriage return (Windows line endings).
		// 去除末尾的回车符（Windows 行尾符）。
		line = strings.TrimSuffix(line, "\r")

		// Strip inline comments that are outside of quotes.
		// 去除引号外的行内注释。
		line = stripComment(line)

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Section header: [section] or [section.subsection]
		// 节头：[section] 或 [section.subsection]
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			sectionName := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			if sectionName == "" {
				return nil, fmt.Errorf("toml line %d: empty section header", lineNum+1)
			}
			current = navigateToTable(root, sectionName)
			continue
		}

		// Key = value pair
		// 键值对
		key, value, ok := splitKeyValue(trimmed)
		if !ok {
			return nil, fmt.Errorf("toml line %d: expected key = value, got %q", lineNum+1, trimmed)
		}
		current.values[key] = value
	}

	return root, nil
}

// stripComment removes a trailing # comment from a line, respecting double
// quotes. Single-quoted (literal) strings are also respected.
// stripComment 从行中去除末尾的 # 注释，正确处理双引号。单引号（字面量）字符串也同样正确处理。
func stripComment(line string) string {
	inDouble := false
	inSingle := false
	for i, ch := range line {
		if ch == '"' && !inSingle {
			inDouble = !inDouble
		} else if ch == '\'' && !inDouble {
			inSingle = !inSingle
		} else if ch == '#' && !inDouble && !inSingle {
			return line[:i]
		}
	}
	return line
}

// splitKeyValue splits a "key = value" line into its components. The value is
// stored as a raw string (quotes are preserved for later type inference).
// splitKeyValue 将 "key = value" 行分割为其组成部分。
// 值以原始字符串形式存储（引号保留以供后续类型推断）。
func splitKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, "=")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])

	// Strip quotes from string values.
	// 去除字符串值的引号。
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = unquoteTomlString(value)
		}
	}

	return key, value, true
}

// navigateToTable walks the section path (e.g. "mcp_servers.my-server") from
// the root, creating intermediate tables as needed, and returns the leaf.
// navigateToTable 从根开始遍历节路径（例如 "mcp_servers.my-server"），
// 根据需要创建中间表，并返回叶子节点。
func navigateToTable(root *tomlTable, section string) *tomlTable {
	parts := strings.Split(section, ".")
	current := root
	for _, part := range parts {
		part = strings.TrimSpace(part)
		next, ok := current.tables[part]
		if !ok {
			next = newTomlTable()
			current.tables[part] = next
		}
		current = next
	}
	return current
}

// ----------------------------------------------------------------------------
// ConfigBuilder
// ----------------------------------------------------------------------------

// ConfigBuilder constructs a Config by layering defaults, config files, and
// CLI overrides. The zero value is not usable; use NewConfigBuilder or set
// the fields explicitly before calling Build.
// ConfigBuilder 通过分层叠加默认值、配置文件和 CLI 覆盖来构建 Config。
// 零值不可用；在调用 Build 之前使用 NewConfigBuilder 或显式设置字段。
type ConfigBuilder struct {
	// ACTXHome is the path to the ACTX home directory. When empty, Build
	// resolves it via ACTXHomeDir().
	// ACTXHome 是 ACTX 主目录的路径。为空时，Build 通过 ACTXHomeDir() 解析。
	ACTXHome string

	// Cwd is the working directory. When empty, Build uses os.Getwd().
	// Cwd 是工作目录。为空时，Build 使用 os.Getwd()。
	Cwd string

	// CLIOverrides holds key-value overrides from the command line. Keys use
	// the same names as config.toml (for example "model", "approval_policy").
	// CLI overrides take the highest precedence.
	// CLIOverrides 保存来自命令行的键值覆盖。键使用与 config.toml 相同的名称（例如 "model"、"approval_policy"）。
	// CLI 覆盖具有最高优先级。
	CLIOverrides map[string]string
}

// NewConfigBuilder returns a ConfigBuilder with the given ACTX home and CLI
// overrides. The working directory is left empty and resolved during Build.
// NewConfigBuilder 返回一个带有给定 ACTX 主目录和 CLI 覆盖的 ConfigBuilder。
// 工作目录留空，在 Build 期间解析。
func NewConfigBuilder(ACTXHome string, cliOverrides map[string]string) *ConfigBuilder {
	return &ConfigBuilder{
		ACTXHome:     ACTXHome,
		CLIOverrides: cliOverrides,
	}
}

// Build resolves the effective Config by applying all configuration layers.
//
// The resolution order is (later layers override earlier ones):
//  1. Built-in defaults
//  2. ACTX_HOME/config.toml
//  3. <cwd>/.ACTX/config.toml
//  4. <project>/.actx/config.json
//  5. ~/.actx/config.json (user cache, takes precedence over project)
//  6. CLI overrides
//
// Build 通过应用所有配置层级来解析有效 Config。
//
// 解析顺序为（后面的层级覆盖前面的层级）：
//  1. 内置默认值
//  2. ACTX_HOME/config.toml
//  3. <cwd>/.ACTX/config.toml
//  4. <project>/.actx/config.json
//  5. ~/.actx/config.json（用户缓存，优先于项目配置）
//  6. CLI 覆盖
func (b *ConfigBuilder) Build() (*Config, error) {
	ACTXHome := b.ACTXHome
	if ACTXHome == "" {
		ACTXHome = ACTXHomeDir()
	}

	cwd := b.Cwd
	if cwd == "" {
		if dir, err := os.Getwd(); err == nil {
			cwd = dir
		}
	}

	cfg := &Config{
		Model:              DefaultModel,
		ModelProvider:      DefaultProvider,
		ApprovalPolicy:     DefaultApprovalPolicy,
		SandboxPolicy:      DefaultSandboxPolicy,
		BaseInstructions:   BaseInstructions,
		UserInstructions:   "",
		Cwd:                cwd,
		ACTXHome:           ACTXHome,
		McpServers:         make(map[string]McpServerConfig),
		ToolsWebSearch:     false,
		ToolsMode:          "api",
		History:            DefaultMaxHistoryLines,
		ModelContextWindow: DefaultContextWindow,
	}

	// Layer 2: ACTX_HOME/config.toml
	// 第2层：ACTX_HOME/config.toml
	homeConfigPath := filepath.Join(ACTXHome, "config.toml")
	if err := applyConfigFile(cfg, homeConfigPath); err != nil {
		return nil, fmt.Errorf("reading %s: %w", homeConfigPath, err)
	}

	// Layer 3: <cwd>/.ACTX/config.toml
	// 第3层：<cwd>/.ACTX/config.toml
	projectConfigPath := filepath.Join(cwd, ".ACTX", "config.toml")
	if err := applyConfigFile(cfg, projectConfigPath); err != nil {
		return nil, fmt.Errorf("reading %s: %w", projectConfigPath, err)
	}

	// Layer 4: <project>/.actx/config.json
	// 第4层：<project>/.actx/config.json
	if projectDir := findProjectConfigDir(cwd); projectDir != "" {
		projectCfgPath := filepath.Join(projectDir, ACTXgoConfigFileName)
		if cc, err := loadACTXGoConfig(projectCfgPath); err != nil {
			return nil, fmt.Errorf("reading %s: %w", projectCfgPath, err)
		} else {
			applyACTXGoConfig(cfg, cc)
		}
	}

	// Layer 5: ~/.actx/config.json (user cache)
	// 第5层：~/.actx/config.json（用户缓存）
	if cacheDir := userCacheDir(); cacheDir != "" {
		cacheCfgPath := filepath.Join(cacheDir, ACTXgoConfigFileName)
		if cc, err := loadACTXGoConfig(cacheCfgPath); err != nil {
			return nil, fmt.Errorf("reading %s: %w", cacheCfgPath, err)
		} else {
			applyACTXGoConfig(cfg, cc)
		}
	}

	// Layer 6: CLI overrides
	// 第6层：CLI 覆盖
	if err := applyCLIOverrides(cfg, b.CLIOverrides); err != nil {
		return nil, fmt.Errorf("applying CLI overrides: %w", err)
	}

	return cfg, nil
}

// applyConfigFile reads and applies a single config.toml file. If the file
// does not exist, it is silently skipped (the file is optional).
// applyConfigFile 读取并应用单个 config.toml 文件。如果文件不存在，则静默跳过（该文件是可选的）。
func applyConfigFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	toml, err := parseTOML(data)
	if err != nil {
		return err
	}

	applyTomlToConfig(cfg, toml)
	return nil
}

// applyTomlToConfig merges a parsed tomlTable into a Config, overwriting
// existing values for any keys that are present.
// applyTomlToConfig 将解析后的 tomlTable 合并到 Config 中，覆盖任何已存在键的现有值。
func applyTomlToConfig(cfg *Config, t *tomlTable) {
	if v, ok := t.getString("model"); ok {
		cfg.Model = v
	}
	if v, ok := t.getString("model_provider"); ok {
		cfg.ModelProvider = v
	}
	if v, ok := t.getString("approval_policy"); ok {
		if p, err := ParseApprovalPolicy(v); err == nil {
			cfg.ApprovalPolicy = p
		}
	}
	// Accept both "sandbox_mode" (matching Rust's config key) and
	// "sandbox_policy" (a common alias) for the sandbox policy.
	// 同时接受 "sandbox_mode"（匹配 Rust 的配置键）和 "sandbox_policy"（常用别名）作为沙箱策略键。
	for _, key := range []string{"sandbox_mode", "sandbox_policy"} {
		if v, ok := t.getString(key); ok {
			if s, err := ParseSandboxPolicy(v); err == nil {
				cfg.SandboxPolicy = s
			}
		}
	}
	if v, ok := t.getString("instructions"); ok {
		cfg.UserInstructions = v
	}
	if v, ok := t.getBool("tools.web_search"); ok {
		cfg.ToolsWebSearch = v
	}
	if v, ok := t.getString("tools_mode"); ok {
		cfg.ToolsMode = v
	}
	// Also accept top-level web_search for compatibility.
	// 同时也接受顶层 web_search 以保持兼容性。
	if v, ok := t.getBool("web_search"); ok {
		cfg.ToolsWebSearch = v
	}
	if v, ok := t.getInt("model_context_window"); ok {
		cfg.ModelContextWindow = v
	}

	// History: accept history.max_bytes (converted to lines via rough estimate)
	// or a top-level history.max_lines.
	// 历史记录：接受 history.max_bytes（通过粗略估计转换为行数）或顶层 history.max_lines。
	historyTable := t.getTable("history")
	if historyTable != nil {
		if v, ok := historyTable.getInt("max_bytes"); ok {
			// Estimate lines from bytes: assume ~80 bytes per line on average.
			// 从字节估算行数：假设平均每行约80字节。
			if v > 0 {
				cfg.History = v / 80
			}
		}
		if v, ok := historyTable.getInt("max_lines"); ok {
			cfg.History = v
		}
	}

	// MCP servers: [mcp_servers.<name>] sections.
	// MCP 服务器：[mcp_servers.<name>] 节。
	mcpTable := t.getTable("mcp_servers")
	if mcpTable != nil {
		for name, serverTable := range mcpTable.tables {
			server := McpServerConfig{}
			if cmd, ok := serverTable.getString("command"); ok {
				server.Command = cmd
			}
			if args, ok := serverTable.getStringSlice("args"); ok {
				server.Args = args
			}
			if url, ok := serverTable.getString("url"); ok {
				server.URL = url
			}
			envTable := serverTable.getTable("env")
			if envTable != nil {
				server.Env = make(map[string]string)
				for k, v := range envTable.values {
					server.Env[k] = v
				}
			}
			cfg.McpServers[name] = server
		}
	}
}

// applyCLIOverrides merges CLI override key-value pairs into the Config.
// CLI overrides use the same key names as config.toml.
// applyCLIOverrides 将 CLI 覆盖键值对合并到 Config 中。
// CLI 覆盖使用与 config.toml 相同的键名。
func applyCLIOverrides(cfg *Config, overrides map[string]string) error {
	for key, value := range overrides {
		switch key {
		case "model":
			cfg.Model = value
		case "model_provider":
			cfg.ModelProvider = value
		case "approval_policy":
			p, err := ParseApprovalPolicy(value)
			if err != nil {
				return err
			}
			cfg.ApprovalPolicy = p
		case "sandbox_mode", "sandbox_policy":
			s, err := ParseSandboxPolicy(value)
			if err != nil {
				return err
			}
			cfg.SandboxPolicy = s
		case "instructions":
			cfg.UserInstructions = value
		case "tools_mode":
			cfg.ToolsMode = value
		case "web_search", "tools.web_search":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid boolean for %s: %q", key, value)
			}
			cfg.ToolsWebSearch = b
		case "model_context_window":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid integer for %s: %q", key, value)
			}
			cfg.ModelContextWindow = n
		case "history", "history.max_lines":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid integer for %s: %q", key, value)
			}
			cfg.History = n
		case "cwd":
			cfg.Cwd = value
		default:
			// Unknown keys are silently ignored to allow forward compatibility.
			// 未知键被静默忽略，以保持向前兼容性。
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// LoadConfig convenience function
// ----------------------------------------------------------------------------

// LoadConfig is a convenience function that creates a ConfigBuilder with the
// given ACTX home and CLI overrides, then calls Build. It is the simplest
// entry point for loading a fully resolved configuration.
//
// Pass an empty ACTXHome to use the default (ACTX_HOME env or ~/.ACTX).
// Pass nil or an empty map for cliOverrides when no CLI overrides are needed.
// LoadConfig 是一个便捷函数，创建具有给定 ACTX 主目录和 CLI 覆盖的 ConfigBuilder，
// 然后调用 Build。它是加载完整解析配置的最简单入口点。
//
// 传入空的 ACTXHome 以使用默认值（ACTX_HOME 环境变量或 ~/.ACTX）。
// 当不需要 CLI 覆盖时，为 cliOverrides 传入 nil 或空 map。
func LoadConfig(ACTXHome string, cliOverrides map[string]string) (*Config, error) {
	builder := NewConfigBuilder(ACTXHome, cliOverrides)
	return builder.Build()
}
