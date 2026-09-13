package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// ShellTool executes shell commands on the local system. It is the primary
// ShellTool 在本地系统上执行 shell 命令。它是代理与用户环境交互的
// tool used by the agent to interact with the user's environment.
type ShellTool struct {
	// spec is the tool specification sent to the model API.
	// spec 是发送给模型 API 的工具规范。
	spec model.ToolSpec
}

// NewShellTool creates a ShellTool with the standard tool specification.
// NewShellTool 使用标准工具规范创建一个 ShellTool。
func NewShellTool() *ShellTool {
	return &ShellTool{spec: ShellToolSpec()}
}

// Name returns the tool name.
// Name 返回工具名称。
func (t *ShellTool) Name() protocol.ToolName { return "shell" }

// Spec returns the tool specification for the model API.
// Spec 返回用于模型 API 的工具规范。
func (t *ShellTool) Spec() model.ToolSpec { return t.spec }

// shellArgs represents the arguments parsed from the model's function call.
// shellArgs 表示从模型的函数调用中解析的参数。
type shellArgs struct {
	// Command is the command to execute, given as an argv array or a single string.
	// Command 是要执行的命令，以 argv 数组或单个字符串形式给出。
	Command flexibleStringSlice `json:"command"`
	// Cwd is the optional working directory for the command.
	// Cwd 是命令的可选工作目录。
	Cwd string `json:"cwd,omitempty"`
	// TimeoutMs is the optional execution timeout in milliseconds.
	// TimeoutMs 是可选的执行超时时间（毫秒）。
	TimeoutMs int `json:"timeout_ms,omitempty"`
	// Env holds optional environment variables for the command process.
	// Env 保存命令进程的可选环境变量。
	Env map[string]string `json:"env,omitempty"`
}

// flexibleStringSlice accepts both a JSON string and a JSON array of strings,
// normalizing to []string. Some models (e.g. DeepSeek) return command as a
// single string instead of an array.
// flexibleStringSlice 同时接受 JSON 字符串和 JSON 字符串数组，
// 统一转换为 []string。某些模型（如 DeepSeek）会将命令作为单个字符串返回。
type flexibleStringSlice []string

func (f *flexibleStringSlice) UnmarshalJSON(data []byte) error {
	// Try array first.
	// 先尝试数组。
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*f = arr
		return nil
	}
	// Fall back to single string, split by whitespace.
	// 回退到单个字符串，按空白分割。
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*f = strings.Fields(s)
	return nil
}

// Handle executes the shell command and returns the output.
// Handle 执行 shell 命令并返回输出。
func (t *ShellTool) Handle(ctx context.Context, inv tools.ToolInvocation) (tools.ToolOutput, error) {
	var args shellArgs
	if err := json.Unmarshal([]byte(inv.Arguments), &args); err != nil {
		return nil, tools.NewRespondToModelError(fmt.Sprintf(
			"invalid JSON arguments: %v. "+
				"command must be a quoted string or array of strings, e.g. "+
				`{"command": ["dir"]} or {"command": "dir"}`+
				". Raw: %s", err, inv.Arguments))
	}

	if len(args.Command) == 0 {
		return nil, tools.NewRespondToModelError("command is required")
	}

	// Enforce sandbox policy.
	// 强制执行沙箱策略。
	if inv.SandboxPolicy == config.SandboxReadOnly {
		// In read-only mode, block write commands.
		// 在只读模式下，阻止写命令。
		if isWriteCommand(args.Command) {
			return nil, tools.NewFatalError("command blocked by read-only sandbox policy")
		}
	}

	cwd := args.Cwd
	if cwd == "" {
		cwd = inv.Cwd
	}

	// Check writable roots - if cwd is outside workspace and writable roots, block.
	// 检查可写根目录 - 如果 cwd 在工作区和可写根目录之外，则阻止。
	if len(inv.WritableRoots) > 0 {
		allowed := false
		for _, root := range inv.WritableRoots {
			if strings.HasPrefix(cwd, root) {
				allowed = true
				break
			}
		}
		if !allowed && inv.SandboxPolicy == config.SandboxWorkspaceWrite {
			return nil, tools.NewFatalError(fmt.Sprintf("command blocked: cwd %s is outside writable roots", cwd))
		}
	}

	// Determine the shell and command to run.
	// 确定要运行的 shell 和命令。
	shell, cmdArgs := prepareCommand(args.Command)

	cmd := exec.CommandContext(ctx, shell, cmdArgs...)
	cmd.Dir = cwd

	// Set up environment.
	// 设置环境变量。
	cmd.Env = os.Environ()
	for k, v := range args.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Capture output.
	// 捕获输出。
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, tools.NewFatalError(fmt.Sprintf("failed to execute command: %v", err))
		}
	}

	// Truncate output if too large.
	// 如果输出太大则截断。
	// On Windows cmd.exe emits GBK-encoded text; convert to UTF-8 so the
	// model and logs see proper Chinese characters instead of mojibake.
	// Windows cmd.exe 输出 GBK 编码文本；转换为 UTF-8，
	// 避免模型与日志看到乱码。
	stdoutStr := truncateOutput(decodeShellOutput(stdout.Bytes()), 30000)
	stderrStr := truncateOutput(decodeShellOutput(stderr.Bytes()), 10000)

	return &tools.ExecOutput{
		Stdout:     stdoutStr,
		Stderr:     stderrStr,
		Exit:       exitCode,
		DurationMs: duration,
	}, nil
}

// decodeShellOutput converts shell output bytes to a UTF-8 string.
// decodeShellOutput 将 shell 输出字节转换为 UTF-8 字符串。
// On Windows, cmd.exe emits GBK (CP936) for the system locale; a plain
// string(b) cast produces mojibake for Chinese text. If the bytes are not
// valid UTF-8, try GBK decoding and fall back to the raw bytes on failure.
// 在 Windows 上 cmd.exe 按系统区域设置输出 GBK（CP936）；直接 string(b)
// 转换会让中文变乱码。如果字节不是有效的 UTF-8，先尝试 GBK 解码，
// 失败则回退为原始字节。
func decodeShellOutput(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	if decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(b); err == nil {
		return string(decoded)
	}
	return string(b)
}

// platformInstructions returns OS-specific shell guidance appended to the
// system prompt so the model emits commands the host shell can actually run
// (e.g. without it, models often send Unix commands like "pwd" or "ls" that
// fail on Windows cmd.exe).
// platformInstructions 返回追加到系统提示词的操作系统专属 shell 指引，
// 让模型生成宿主 shell 真正能执行的命令（否则模型常发出 pwd/ls 等
// 在 Windows cmd.exe 中会失败的 Unix 命令）。
func platformInstructions() string {
	if runtime.GOOS == "windows" {
		return `# Platform

You are running on Windows. The shell tool executes commands via cmd.exe.
- Use Windows commands: dir (not ls), type (not cat), cd (not pwd), copy/move/del (not cp/mv/rm), findstr (not grep).
- Use double quotes only; single quotes are literal characters in cmd.exe.
- No Unix shell features: no $() substitution, no backticks, no 2>/dev/null (use 2>nul).
- Path separator is backslash (\).`
	}
	return `# Platform

You are running on a Unix-like system. The shell tool executes commands via sh.`
}

// prepareCommand converts the command array into a shell invocation.
// prepareCommand 将命令数组转换为 shell 调用。
// On Windows it uses cmd.exe /c, on Unix it uses sh -c.
func prepareCommand(command []string) (string, []string) {
	// Join the command array into a single string for shell execution.
	// 将命令数组连接为单个字符串以供 shell 执行。
	cmdStr := joinCommand(command)
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", cmdStr}
	}
	return "sh", []string{"-c", cmdStr}
}

// joinCommand joins a command array into a single shell command string.
// joinCommand 将命令数组连接为单个 shell 命令字符串。
func joinCommand(command []string) string {
	if len(command) == 1 {
		return command[0]
	}
	result := ""
	for i, part := range command {
		if i > 0 {
			result += " "
		}
		// Quote parts that contain spaces.
		// 引用包含空格的部分。
		if containsSpace(part) {
			result += fmt.Sprintf("%q", part)
		} else {
			result += part
		}
	}
	return result
}

func containsSpace(s string) bool {
	for _, c := range s {
		if c == ' ' || c == '\t' {
			return true
		}
	}
	return false
}

// truncateOutput truncates the output string to maxLen characters,
// truncateOutput 将输出字符串截断为 maxLen 个字符，
// keeping the beginning and end.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	half := maxLen / 2
	return s[:half] + "\n... [truncated] ...\n" + s[len(s)-half:]
}

// writeCommands is the set of command names that are known to write to or
// modify the filesystem. It is used by isWriteCommand to enforce the
// read-only sandbox policy.
// writeCommands 是已知会写入或修改文件系统的命令名称集合。
// 它被 isWriteCommand 用于强制执行只读沙箱策略。
var writeCommands = map[string]bool{
	// POSIX write/mutate commands.
	"rm":       true,
	"rmdir":    true,
	"mkdir":    true,
	"touch":    true,
	"cp":       true,
	"mv":       true,
	"ln":       true,
	"chmod":    true,
	"chown":    true,
	"chattr":   true,
	"truncate": true,
	"dd":       true,
	"tee":      true,
	"install":  true,
	"mkfifo":   true,
	"mknod":    true,
	// Windows write/mutate commands.
	"del":      true,
	"rd":       true,
	"md":       true,
	"copy":     true,
	"move":     true,
	"replace":  true,
	"xcopy":    true,
	"robocopy": true,
	"attrib":   true,
	"icacls":   true,
	"fsutil":   true,
	"format":   true,
	"diskpart": true,
	// PowerShell cmdlets that write.
	"set-content":   true,
	"out-file":      true,
	"add-content":   true,
	"clear-content": true,
	"remove-item":   true,
	"new-item":      true,
	"copy-item":     true,
	"move-item":     true,
	"rename-item":   true,
	"write-host":    true,
	"write-output":  true,
}

// isWriteCommand returns true when the given command is likely to write to or
// modify the filesystem. It inspects the program name and the joined command
// string for shell redirection operators. This is a best-effort heuristic used
// to enforce the read-only sandbox policy.
// isWriteCommand 在给定命令可能写入或修改文件系统时返回 true。
// 它检查程序名称以及连接后的命令字符串中是否包含 shell 重定向操作符。
// 这是用于强制执行只读沙箱策略的尽力而为的启发式判断。
func isWriteCommand(command []string) bool {
	if len(command) == 0 {
		return false
	}

	// Check for shell redirection operators in the joined command string.
	// 检查连接后的命令字符串中是否包含 shell 重定向操作符。
	joined := strings.Join(command, " ")
	if strings.Contains(joined, ">") || strings.Contains(joined, ">>") {
		return true
	}

	// Extract the program name: lowercase, strip path prefix and .exe suffix.
	// 提取程序名称：转为小写，去除路径前缀和 .exe 后缀。
	prog := strings.ToLower(command[0])
	if idx := strings.LastIndex(prog, "/"); idx >= 0 {
		prog = prog[idx+1:]
	}
	if idx := strings.LastIndex(prog, "\\"); idx >= 0 {
		prog = prog[idx+1:]
	}
	if strings.HasSuffix(prog, ".exe") {
		prog = prog[:len(prog)-4]
	}

	return writeCommands[prog]
}
