// Package config implements the configuration system for the ACTX CLI agent.
//
// This is a Go rewrite of the Rust ACTX-rs configuration crate. It provides
// types for representing agent configuration, permission profiles, and sandbox
// policies, along with a builder that loads settings from ACTX_HOME/config.toml,
// project-local .ACTX/config.toml files, and CLI overrides.
//
// The configuration layering follows the same precedence as the Rust
// implementation:
//
//  1. Built-in defaults (lowest precedence)
//  2. ACTX_HOME/config.toml
//  3. <project>/.ACTX/config.toml
//  4. CLI overrides (highest precedence)
//
// Package config 实现了 ACTX CLI 代理的配置系统。
//
// 这是 Rust ACTX-rs 配置 crate 的 Go 重写版本。它提供了表示代理配置、
// 权限配置文件和沙箱策略的类型，以及一个从 ACTX_HOME/config.toml、
// 项目本地 .ACTX/config.toml 文件和 CLI 覆盖中加载设置的构建器。
//
// 配置分层遵循与 Rust 实现相同的优先级：
//
//  1. 内置默认值（最低优先级）
//  2. ACTX_HOME/config.toml
//  3. <project>/.ACTX/config.toml
//  4. CLI 覆盖（最高优先级）
package config

import (
	_ "embed"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// DefaultModel is the model used when none is specified in config or CLI.
// DefaultModel 是当配置或 CLI 中未指定时使用的默认模型。
const DefaultModel = "deepseek-v4-flash"

// DefaultProvider is the model provider used when none is specified.
// DefaultProvider 是当未指定时使用的默认模型提供商。
const DefaultProvider = "openai"

// DefaultApprovalPolicy is the approval policy applied when none is configured.
//
// OnRequest lets the model decide when to ask the user for approval, matching
// the Rust default of AskForApproval::OnRequest.
// DefaultApprovalPolicy 是当未配置时应用的默认审批策略。
//
// OnRequest 让模型决定何时向用户请求批准，与 Rust 默认的 AskForApproval::OnRequest 一致。
const DefaultApprovalPolicy = ApprovalOnRequest

// DefaultSandboxPolicy is the sandbox policy applied when none is configured.
//
// WorkspaceWrite confines writes to the current workspace root, matching the
// Rust default of SandboxMode::WorkspaceWrite for trusted projects.
// DefaultSandboxPolicy 是当未配置时应用的默认沙箱策略。
//
// WorkspaceWrite 将写入限制在当前工作区根目录，与 Rust 对于受信任项目的
// SandboxMode::WorkspaceWrite 默认值一致。
const DefaultSandboxPolicy = SandboxWorkspaceWrite

// DefaultContextWindow is the default token limit for the model context window.
// DefaultContextWindow 是模型上下文窗口的默认令牌限制。
const DefaultContextWindow = 200000

// DefaultMaxHistoryLines is the default maximum number of lines retained in
// the persistent history file.
// DefaultMaxHistoryLines 是持久化历史文件中保留的默认最大行数。
const DefaultMaxHistoryLines = 50000

// ACTXHomeEnvVar is the name of the environment variable that overrides the
// location of the ACTX home directory.
// ACTXHomeEnvVar 是用于覆盖 ACTX 主目录位置的环境变量名称。
const ACTXHomeEnvVar = "ACTX_HOME"

// ACTXHomeDir returns the path to the ACTX home directory.
//
// The directory is resolved in the following order:
//
//  1. The ACTX_HOME environment variable when set and non-empty.
//  2. The .ACTX subdirectory of the current user's home directory.
//
// If neither the environment variable nor the user home directory can be
// resolved, ACTXHomeDir returns the platform-specific default of ".ACTX"
// relative to the working directory.
// ACTXHomeDir 返回 ACTX 主目录的路径。
//
// 目录按以下顺序解析：
//
//  1. 当 ACTX_HOME 环境变量已设置且非空时使用该值。
//  2. 当前用户主目录下的 .ACTX 子目录。
//
// 如果环境变量和用户主目录都无法解析，ACTXHomeDir 返回相对于工作目录的
// 平台特定默认值 ".ACTX"。
func ACTXHomeDir() string {
	if home := os.Getenv(ACTXHomeEnvVar); home != "" {
		return home
	}

	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return filepath.Join(u.HomeDir, ".ACTX")
	}

	// Fall back to a relative path so callers always receive a usable value.
	// 回退到相对路径，以便调用者始终能获得可用值。
	return ".ACTX"
}

//go:embed prompt.md
var baseInstructionsMD string

// BaseInstructions is the default system prompt injected into every model
// request. It is loaded at compile time from prompt.md (same directory) via
// go:embed instead of being hardcoded as a string literal, so the prompt can
// be edited without touching Go source. The BOM, if present, and surrounding
// whitespace are stripped.
// BaseInstructions 是注入到每个模型请求中的默认系统提示。
// 编译期通过 go:embed 从同目录的 prompt.md 加载，而不再使用硬编码的
// 字符串常量，方便直接编辑提示词文件。若文件带 BOM 或首尾空白，会一并去除。
var BaseInstructions = strings.TrimSpace(strings.TrimPrefix(baseInstructionsMD, "\uFEFF"))

//go:embed promptCn.md
var baseInstructionsZh string

var BaseInstructionsZh = strings.TrimSpace(strings.TrimPrefix(baseInstructionsZh, "\uFEFF"))
