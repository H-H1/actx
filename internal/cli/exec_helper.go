package cli

import (
	"context"
	"os"

	"github.com/H-H1/actx/internal/agent"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/exec"
	"github.com/H-H1/actx/internal/tools"
)

// runExecWithConfig runs the non-interactive exec with a loaded config.
// runExecWithConfig 使用已加载的配置运行非交互式 exec。
func runExecWithConfig(ctx context.Context, cfg *config.Config, apiKey, baseURL, prompt string, jsonOutput bool) error {
	// Build the tool registry with all built-in tools.
	// 使用全部内置工具构建工具注册表。
	registry := tools.NewToolRegistry()
	agent.RegisterBuiltinTools(registry, apiKey, baseURL)

	return exec.Run(ctx, exec.ExecOptions{
		Config:     cfg,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Prompt:     prompt,
		JSONOutput: jsonOutput,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Registry:   registry,
	})
}
