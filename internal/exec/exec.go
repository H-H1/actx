// Package exec implements the non-interactive execution mode for ACTX.
// exec 包实现 ACTX 的非交互式执行模式。
// It drives the agent loop programmatically, consuming user input and
// 它通过程序化方式驱动代理循环，消费用户输入并
// producing output to stdout/stderr without a TUI.
// 在没有 TUI 的情况下将输出生成到 stdout/stderr。
package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/H-H1/actx/internal/agent"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// ExecOptions configures the non-interactive execution.
// ExecOptions 配置非交互式执行。
type ExecOptions struct {
	// Config is the runtime configuration for the run.
	// Config 是本次运行的运行时配置。
	Config *config.Config
	// APIKey is the API key for the model provider.
	// APIKey 是模型服务商的 API 密钥。
	APIKey string
	// BaseURL is the base URL of the OpenAI-compatible endpoint.
	// BaseURL 是 OpenAI 兼容端点的基础 URL。
	BaseURL string
	// Prompt is the user prompt to execute.
	// Prompt 是要执行的用户提示。
	Prompt string
	// JSONOutput enables JSON-line output mode.
	// JSONOutput 启用 JSON 行输出模式。
	JSONOutput bool
	// Stdout is the writer for standard output (defaults to os.Stdout).
	// Stdout 是标准输出的写入器（默认为 os.Stdout）。
	Stdout io.Writer
	// Stderr is the writer for standard error (defaults to os.Stderr).
	// Stderr 是标准错误的写入器（默认为 os.Stderr）。
	Stderr io.Writer
	// Registry is the tool registry; default tools are used when nil.
	// Registry 是工具注册表；为 nil 时使用默认工具。
	Registry *tools.ToolRegistry
}

// Run executes a single non-interactive turn and prints results.
// Run 执行单次非交互式任务轮次并打印结果。
// It creates a ACTX instance, submits the user prompt, consumes all events,
// 它创建 ACTX 实例，提交用户提示，消费所有事件，
// and prints the output to the configured writers.
// 并将输出打印到已配置的写入器。
func Run(ctx context.Context, opts ExecOptions) error {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Build the tool registry with all built-in tools.
	// 使用全部内置工具构建工具注册表。
	registry := opts.Registry
	if registry == nil {
		registry = tools.NewToolRegistry()
		agent.RegisterBuiltinTools(registry, opts.APIKey, opts.BaseURL)
	}

	// Spawn the ACTX agent.
	// 启动 ACTX 代理。
	ACTX, err := agent.Spawn(agent.ACTXArgs{
		Config:   opts.Config,
		APIKey:   opts.APIKey,
		BaseURL:  opts.BaseURL,
		Registry: registry,
	})
	if err != nil {
		return fmt.Errorf("spawn ACTX: %w", err)
	}
	defer ACTX.Shutdown()

	// Submit the user prompt.
	// 提交用户提示。
	items := []protocol.ResponseItem{
		protocol.NewUserMessage(opts.Prompt),
	}
	subID := ACTX.Submit(protocol.NewUserInputOp(items))

	// Consume events until the turn completes.
	// 消费事件直到任务轮次完成。
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ACTX.Events():
			if !ok {
				return nil
			}
			if err := handleEvent(ev, subID, stdout, stderr, opts.JSONOutput); err != nil {
				return err
			}
		}
	}
}

// handleEvent processes a single event and writes output.
// handleEvent 处理单个事件并写入输出。
func handleEvent(ev protocol.Event, subID protocol.SubmissionId, stdout, stderr io.Writer, jsonOutput bool) error {
	// Only process events for our submission.
	// 仅处理我们提交的事件。
	if ev.ID != "" && ev.ID != subID && ev.Msg.Type != protocol.EvShutdownComplete {
		// Still process some global events.
		// 仍然处理一些全局事件。
	}

	msg := ev.Msg

	switch msg.Type {
	case protocol.EvAgentMessage:
		if jsonOutput {
			return writeJSON(stdout, map[string]string{
				"type":    "message",
				"message": msg.Message,
			})
		}
		fmt.Fprintln(stdout, msg.Message)

	case protocol.EvAgentMessageDelta:
		if !jsonOutput {
			fmt.Fprint(stdout, msg.Delta)
		}

	case protocol.EvExecCommandBegin:
		if jsonOutput {
			return writeJSON(stdout, map[string]interface{}{
				"type":    "exec_begin",
				"command": msg.Command,
				"cwd":     msg.Cwd,
			})
		}
		fmt.Fprintf(stderr, "$ %s\n", strings.Join(msg.Command, " "))

	case protocol.EvExecCommandEnd:
		if jsonOutput {
			data := map[string]interface{}{
				"type":   "exec_end",
				"output": msg.Output,
			}
			if msg.ExitCode != nil {
				data["exit_code"] = *msg.ExitCode
			}
			return writeJSON(stdout, data)
		}
		if msg.Output != "" {
			fmt.Fprint(stderr, msg.Output)
		}

	case protocol.EvStreamError:
		if jsonOutput {
			return writeJSON(stdout, map[string]string{
				"type":  "error",
				"error": msg.ErrorMsg,
			})
		}
		fmt.Fprintf(stderr, "Error: %s\n", msg.ErrorMsg)

	case protocol.EvTurnComplete:
		if jsonOutput {
			return writeJSON(stdout, map[string]string{
				"type": "turn_complete",
			})
		}

	case protocol.EvShutdownComplete:
		return nil
	}

	return nil
}

// writeJSON writes a JSON object to the writer followed by a newline.
// writeJSON 将 JSON 对象写入写入器，后跟一个换行符。
func writeJSON(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}
