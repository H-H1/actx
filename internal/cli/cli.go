// Package cli implements the command-line interface for ACTX CLI.
// It uses cobra for subcommand dispatch and wires together the agent,
// config, login, and exec packages.
// Package cli 实现了 ACTX CLI 的命令行界面。它使用 cobra 进行子命令分发，
// 并将 agent、config、login 和 exec 包连接在一起。
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/H-H1/actx/internal/appserver"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/login"
	"github.com/H-H1/actx/internal/rollout"
	"github.com/H-H1/actx/internal/tui"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root cobra command with all subcommands registered.
// NewRootCmd 创建注册了所有子命令的根 cobra 命令。
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "ACTX",
		Short: "ACTX CLI - a coding agent that runs locally",
		Long: `ACTX CLI is a coding agent from OpenAI that runs locally on your computer.
It can execute commands, modify files, and help with software development tasks.`,
		SilenceUsage: true,
		Version:      "0.1.0",
		RunE:         runInteractive,
	}

	// Persistent flags inherited by subcommands for config loading.
	// 子命令继承的持久标志，用于配置加载。
	rootCmd.PersistentFlags().StringP("model", "m", "", "Model to use (default: deepseek-v4-flash)")
	rootCmd.PersistentFlags().StringP("provider", "p", "", "Model provider")
	rootCmd.PersistentFlags().StringP("config", "c", "", "Path to config file")
	rootCmd.PersistentFlags().StringArrayP("override", "o", nil, "Config overrides (key=value)")
	rootCmd.PersistentFlags().String("approval-policy", "", "Approval policy: never, on-failure, on-request, on-untrusted")
	rootCmd.PersistentFlags().String("sandbox", "", "Sandbox policy: none, read-only, workspace-write, danger-full-access")
	rootCmd.PersistentFlags().String("tools-mode", "", "Tool calling mode: api (default) or prompt (for models without function calling)")

	// Local flags for interactive mode only.
	// 本地标志，仅用于交互模式。
	rootCmd.Flags().String("base-url", "", "Override the API base URL")
	rootCmd.Flags().Bool("json", false, "Output events as JSON lines")

	// Add subcommands.
	// 添加子命令。
	rootCmd.AddCommand(newExecCmd())
	rootCmd.AddCommand(newAppServerCmd())
	rootCmd.AddCommand(newLoginCmd())
	rootCmd.AddCommand(newLogoutCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newDeleteCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newResumeCmd())

	return rootCmd
}

// newExecCmd creates the 'exec' subcommand for non-interactive execution.
// newExecCmd 创建 'exec' 子命令用于非交互式执行。
func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec [prompt]",
		Short: "Run ACTX non-interactively with a prompt",
		Long:  `Execute a single turn with the given prompt and print the result to stdout.`,
		Args:  cobra.MinimumNArgs(1),
		RunE:  runExec,
	}
	cmd.Flags().Bool("json", false, "Output events as JSON lines")
	cmd.Flags().String("base-url", "", "Override the API base URL")
	return cmd
}

// newAppServerCmd creates the 'app-server' subcommand for serving the
// ACTX App Server protocol over stdio. This is the integration point for
// desktop clients like ACTXHarbor.
// newAppServerCmd 创建 'app-server' 子命令，用于通过 stdio 提供 ACTX App Server
// 协议服务。这是 ACTXHarbor 等桌面客户端的集成点。
func newAppServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app-server",
		Short: "Run the ACTX App Server over stdio",
		Long: `Start the ACTX App Server, speaking newline-delimited JSON-RPC 2.0
over stdin/stdout. This is the protocol used by desktop clients like
ACTXHarbor to manage threads, turns, and approvals.

Usage:
  ACTX app-server --listen stdio://

The server reads JSON-RPC requests from stdin and writes responses and
notifications to stdout. stderr is used for diagnostic logging.`,
		RunE: runAppServer,
	}
	cmd.Flags().String("listen", "stdio://", "Listen address (only stdio:// is supported)")
	cmd.Flags().String("base-url", "", "Override the API base URL")
	return cmd
}

// newLoginCmd creates the 'login' subcommand.
// newLoginCmd 创建 'login' 子命令。
func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in with ChatGPT or API key",
		RunE:  runLogin,
	}
	cmd.Flags().String("api-key", "", "API key to store")
	return cmd
}

// newLogoutCmd creates the 'logout' subcommand.
// newLogoutCmd 创建 'logout' 子命令。
func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and remove stored credentials",
		RunE:  runLogout,
	}
}

// newListCmd creates the 'list' subcommand for listing saved sessions.
// newListCmd 创建 'list' 子命令用于列出已保存的会话。
func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved sessions",
		RunE:  runList,
	}
}

// newDeleteCmd creates the 'delete' subcommand for deleting a session.
// newDeleteCmd 创建 'delete' 子命令用于删除会话。
func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <session-id>",
		Short: "Delete a saved session",
		Args:  cobra.ExactArgs(1),
		RunE:  runDelete,
	}
}

// newDoctorCmd creates the 'doctor' subcommand for diagnostics.
// newDoctorCmd 创建 'doctor' 子命令用于诊断。
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostics checks",
		RunE:  runDoctor,
	}
}

// newResumeCmd creates the 'resume' subcommand for resuming a saved session.
// newResumeCmd 创建 'resume' 子命令用于恢复已保存的会话。
func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <session-id>",
		Short: "Resume a saved session",
		Args:  cobra.ExactArgs(1),
		RunE:  runResume,
	}
}

// runResume handles the 'resume' subcommand.
// runResume 处理 'resume' 子命令。
func runResume(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigFromFlags(cmd.Root())
	if err != nil {
		return err
	}

	apiKey, err := login.GetAPIKey(cfg.ACTXHome)
	if err != nil {
		return err
	}

	baseURL, _ := cmd.Root().Flags().GetString("base-url")
	if baseURL == "" {
		baseURL = cfg.BaseURL
	}

	rolloutDir := filepath.Join(cfg.ACTXHome, "sessions")
	items, err := rollout.LoadSession(rolloutDir, args[0])
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	ctx, cancel := signalContext()
	defer cancel()

	// Run exec with the loaded context.
	// 使用加载的上下文运行 exec。
	_ = items // The items are loaded; in a full implementation they'd be passed to the agent
	fmt.Printf("Resuming session %s with %d items...\n", args[0], len(items))
	return runExecWithConfig(ctx, cfg, apiKey, baseURL, "Continue from where we left off.", false)
}

// loadConfigFromFlags builds a Config from the root command flags.
// loadConfigFromFlags 从根命令标志构建 Config。
func loadConfigFromFlags(cmd *cobra.Command) (*config.Config, error) {
	ACTXHome := config.ACTXHomeDir()

	overrides := make(map[string]string)

	if cmd.Flags().Changed("model") {
		model, _ := cmd.Flags().GetString("model")
		overrides["model"] = model
	}
	if cmd.Flags().Changed("provider") {
		provider, _ := cmd.Flags().GetString("provider")
		overrides["model_provider"] = provider
	}
	if cmd.Flags().Changed("approval-policy") {
		policy, _ := cmd.Flags().GetString("approval-policy")
		overrides["approval_policy"] = policy
	}
	if cmd.Flags().Changed("sandbox") {
		sandbox, _ := cmd.Flags().GetString("sandbox")
		overrides["sandbox_policy"] = sandbox
	}
	if cmd.Flags().Changed("tools-mode") {
		toolsMode, _ := cmd.Flags().GetString("tools-mode")
		overrides["tools_mode"] = toolsMode
	}

	// Add -o overrides.
	// 添加 -o 覆盖。
	cliOverrides, _ := cmd.Flags().GetStringArray("override")
	for _, o := range cliOverrides {
		parts := splitOverride(o)
		if len(parts) == 2 {
			overrides[parts[0]] = parts[1]
		}
	}

	cfg, err := config.LoadConfig(ACTXHome, overrides)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return cfg, nil
}

// splitOverride splits a "key=value" string.
// splitOverride 拆分 "key=value" 字符串。
func splitOverride(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// runInteractive launches the TUI when no subcommand is provided.
// runInteractive 在未提供子命令时启动 TUI。
func runInteractive(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigFromFlags(cmd)
	if err != nil {
		return err
	}

	apiKey, err := login.GetAPIKey(cfg.ACTXHome)
	if err != nil {
		return err
	}

	baseURL, _ := cmd.Flags().GetString("base-url")
	if baseURL == "" {
		baseURL = cfg.BaseURL
	}

	ctx, cancel := signalContext()
	defer cancel()

	return tui.Run(ctx, cfg, apiKey, baseURL)
}

// signalContext returns a context that is cancelled on SIGINT or SIGTERM.
// signalContext 返回一个在 SIGINT 或 SIGTERM 时被取消的 context。
func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel() // 信号触发取消
		case <-ctx.Done():
			// ctx 已被主动取消，安静退出
		}
	}()
	return ctx, cancel
}

// runExec handles the 'exec' subcommand.
// runExec 处理 'exec' 子命令。
func runExec(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigFromFlags(cmd.Root())
	if err != nil {
		return err
	}

	apiKey, err := login.GetAPIKey(cfg.ACTXHome)
	if err != nil {
		return err
	}

	baseURL, _ := cmd.Flags().GetString("base-url")
	if baseURL == "" {
		baseURL = cfg.BaseURL
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	prompt := joinArgs(args)

	ctx, cancel := signalContext()
	defer cancel()

	return runExecWithConfig(ctx, cfg, apiKey, baseURL, prompt, jsonOutput)
}

// runAppServer handles the 'app-server' subcommand.
// runAppServer 处理 'app-server' 子命令。
func runAppServer(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfigFromFlags(cmd.Root())
	if err != nil {
		return err
	}

	// API key is optional at startup; the server can handle account/read
	// with requiresOpenaiAuth=true when no key is present. Thread creation
	// will fail if no key is available, which ACTXHarbor handles by
	// prompting the user to sign in.
	apiKey, _ := login.GetAPIKey(cfg.ACTXHome)

	baseURL, _ := cmd.Flags().GetString("base-url")
	if baseURL == "" {
		baseURL = cfg.BaseURL
	}

	ctx, cancel := signalContext()
	defer cancel()

	server := appserver.New(cfg, apiKey, baseURL)
	return server.Run(ctx)
}

// runLogin handles the 'login' subcommand.
// runLogin 处理 'login' 子命令。
func runLogin(cmd *cobra.Command, args []string) error {
	ACTXHome := config.ACTXHomeDir()

	apiKey, _ := cmd.Flags().GetString("api-key")
	if apiKey != "" {
		if err := login.StoreAPIKey(ACTXHome, apiKey); err != nil {
			return err
		}
		fmt.Println("API key stored successfully.")
		return nil
	}

	// Interactive login: choose between API key and OAuth.
	// 交互式登录：在 API 密钥和 OAuth 之间选择。
	fmt.Println("1. Enter API key manually")
	fmt.Println("2. Login with ChatGPT (OAuth)")
	fmt.Print("Choose option (1 or 2): ")
	var choice string
	fmt.Scanln(&choice)

	if choice == "2" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		token, err := login.StartOAuthFlow(ctx)
		if err != nil {
			return fmt.Errorf("oauth login failed: %w", err)
		}
		if err := login.SaveToken(ACTXHome, token); err != nil {
			return err
		}
		fmt.Println("OAuth login successful!")
		return nil
	}

	// Default: API key.
	// 默认：API 密钥。
	fmt.Print("Enter your API key: ")
	var input string
	fmt.Scanln(&input)
	if input == "" {
		return fmt.Errorf("no API key provided")
	}

	if err := login.StoreAPIKey(ACTXHome, input); err != nil {
		return err
	}
	fmt.Println("API key stored successfully.")
	return nil
}

// runLogout handles the 'logout' subcommand.
// runLogout 处理 'logout' 子命令。
func runLogout(cmd *cobra.Command, args []string) error {
	ACTXHome := config.ACTXHomeDir()
	if err := login.DeleteToken(ACTXHome); err != nil {
		return err
	}
	fmt.Println("Signed out successfully.")
	return nil
}

// runList handles the 'list' subcommand.
// runList 处理 'list' 子命令。
func runList(cmd *cobra.Command, args []string) error {
	ACTXHome := config.ACTXHomeDir()
	rolloutDir := ACTXHome + "/sessions"

	sessions, err := rollout.ListSessions(rolloutDir)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	fmt.Print(rollout.FormatSessionList(sessions))
	return nil
}

// runDelete handles the 'delete' subcommand.
// runDelete 处理 'delete' 子命令。
func runDelete(cmd *cobra.Command, args []string) error {
	ACTXHome := config.ACTXHomeDir()
	rolloutDir := ACTXHome + "/sessions"

	if err := rollout.DeleteSession(rolloutDir, args[0]); err != nil {
		return err
	}
	fmt.Printf("Session %s deleted.\n", args[0])
	return nil
}

// runDoctor handles the 'doctor' subcommand.
// runDoctor 处理 'doctor' 子命令。
func runDoctor(cmd *cobra.Command, args []string) error {
	ACTXHome := config.ACTXHomeDir()
	fmt.Printf("ACTX home: %s\n", ACTXHome)

	// Check if auth exists.
	token, err := login.LoadToken(ACTXHome)
	if err != nil {
		fmt.Printf("Auth: error - %v\n", err)
	} else if token == nil {
		fmt.Println("Auth: not configured (run 'ACTX login')")
	} else if token.IsValid() {
		fmt.Println("Auth: valid")
	} else {
		fmt.Println("Auth: expired (run 'ACTX login')")
	}

	// Check config.
	cfg, err := config.LoadConfig(ACTXHome, nil)
	if err != nil {
		fmt.Printf("Config: error - %v\n", err)
	} else {
		fmt.Printf("Model: %s\n", cfg.Model)
		fmt.Printf("Provider: %s\n", cfg.ModelProvider)
		fmt.Printf("Approval policy: %s\n", cfg.ApprovalPolicy)
		fmt.Printf("Sandbox: %s\n", cfg.SandboxPolicy)
	}

	return nil
}

// joinArgs joins command-line arguments into a single prompt string.
// joinArgs 将命令行参数连接为单个提示字符串。
func joinArgs(args []string) string {
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		result += arg
	}
	return result
}
