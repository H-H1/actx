package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
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
	// Fall back to a single string: keep it verbatim as one element so that
	// quoting and $variables survive untouched. Splitting by whitespace would
	// destroy embedded quotes and mangle PowerShell syntax like foreach($i ...).
	// 回退到单个字符串：整体保留为一个元素，确保引号和 $ 变量不被破坏。
	// 按空白分割会破坏内嵌引号并损坏 foreach($i ...) 等 PowerShell 语法。
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*f = []string{s}
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

	// Drop a redundant nested powershell wrapper before anything else so the
	// sandbox check sees the real command and $variables are not double-parsed.
	// 先剥离多余的嵌套 powershell 包装，让沙箱检查看到真实命令，
	// 同时避免 $ 变量被外层 shell 双重解析。
	args.Command = stripNestedPowerShell(args.Command)

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
	// On Windows the legacy shell emits GBK-encoded text; convert to UTF-8
	// so the model and logs see proper Chinese characters instead of
	// mojibake. PowerShell output is forced to UTF-8 via a command prefix,
	// so this is a safety net for cmd.exe fallback and misbehaving tools.
	// Windows 上旧 shell 会输出 GBK 编码文本；转换为 UTF-8，避免模型与
	// 日志看到乱码。PowerShell 的输出已通过命令前缀强制为 UTF-8，
	// 此处是针对 cmd.exe 兜底路径与不规范工具的安全网。
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
// On Windows, cmd.exe and Windows PowerShell 5.1 emit GBK (CP936) for the
// system locale when stdout is redirected; a plain string(b) cast produces
// mojibake for Chinese text. If the bytes are not valid UTF-8, try GBK
// decoding and fall back to the raw bytes on failure.
// 在 Windows 上，stdout 被重定向时 cmd.exe 与 Windows PowerShell 5.1 按
// 系统区域设置输出 GBK（CP936）；直接 string(b) 转换会让中文变乱码。
// 如果字节不是有效的 UTF-8，先尝试 GBK 解码，失败则回退为原始字节。
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
// fail on Windows). Windows guidance mirrors codex: the model is steered to
// native PowerShell syntax instead of POSIX commands, because the execution
// shell on Windows is resolved to PowerShell first (cmd.exe only as last
// resort).
// platformInstructions 返回追加到系统提示词的操作系统专属 shell 指引，
// 让模型生成宿主 shell 真正能执行的命令（否则模型常发出 pwd/ls 等在
// Windows 上会失败的 Unix 命令）。Windows 指引与 codex 一致：引导模型
// 使用原生 PowerShell 语法而非 POSIX 命令，因为 Windows 上的执行 shell
// 优先解析为 PowerShell（cmd.exe 仅作最后兜底）。
func platformInstructions() string {
	if runtime.GOOS == "windows" {
		return `# Platform

You are running on Windows. The shell tool executes commands via PowerShell.
- Use PowerShell cmdlets and syntax: Get-ChildItem (ls/dir), Get-Content (cat/type), Remove-Item (rm/del), Copy-Item (cp/copy), Move-Item (mv/move), New-Item (mkdir/touch).
- Prefer -LiteralPath on file cmdlets when paths may contain special characters.
- The shell tool already runs inside PowerShell. Never wrap commands in powershell -Command, pwsh, or cmd /c — the nested invocation mangles quoting and $variables. Call cmdlets directly.
- $() subexpressions, pipes and backtick line continuation work; use double quotes for string interpolation, single quotes for literal strings.
- Start background/GUI processes with Start-Process -WindowStyle Hidden.
- Path separator is backslash (\).`
	}
	return `# Platform

You are running on a Unix-like system. The shell tool executes commands via sh.`
}

// powershellUTF8Prefix forces UTF-8 output decoding for the PowerShell
// process. Windows PowerShell 5.1 emits native/cmdlet output in the system
// ANSI codepage (GBK on Chinese Windows) when stdout is redirected, producing
// mojibake; setting [Console]::OutputEncoding makes both cmdlet output and
// native command output UTF-8. Wrapped in try/catch because the setter
// throws "handle is invalid" in console-less hosts.
// powershellUTF8Prefix 强制 PowerShell 进程以 UTF-8 输出。stdout 被重定向时
// Windows PowerShell 5.1 按系统 ANSI 代码页（中文 Windows 为 GBK）输出
// 原生命令与 cmdlet 结果，导致中文乱码；设置 [Console]::OutputEncoding 后
// 二者均为 UTF-8。用 try/catch 包裹是因为在无控制台宿主中该属性 setter
// 会抛出 "handle is invalid"。
const powershellUTF8Prefix = `try{[Console]::OutputEncoding=[System.Text.Encoding]::UTF8}catch{}; `

// windowsShell resolves the Windows execution shell once per process:
// PowerShell 7 (pwsh) first, then Windows PowerShell 5.1, then cmd.exe as
// the last resort. This mirrors codex's shell resolution — Windows has no
// POSIX login shell, so the modern shell is preferred and cmd.exe is only
// a fallback. An empty result means "use cmd.exe".
// windowsShell 每进程解析一次 Windows 执行 shell：优先 PowerShell 7（pwsh），
// 其次 Windows PowerShell 5.1，最后以 cmd.exe 兜底。这与 codex 的 shell
// 解析一致——Windows 没有 POSIX 登录 shell，因此优先现代 shell，
// cmd.exe 仅作回退。结果为空表示"使用 cmd.exe"。
var windowsShell = sync.OnceValue(func() string {
	if path, err := exec.LookPath("pwsh.exe"); err == nil {
		return path
	}
	// pwsh not on PATH: check the standard PowerShell 7 install locations.
	// PATH 上没有 pwsh：检查标准 PowerShell 7 安装位置。
	for _, p := range []string{
		`C:\Program Files\PowerShell\7\pwsh.exe`,
		`C:\Program Files\PowerShell\6\pwsh.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("powershell.exe"); err == nil {
		return path
	}
	const systemPowerShell = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	if _, err := os.Stat(systemPowerShell); err == nil {
		return systemPowerShell
	}
	return ""
})

// stripNestedPowerShell removes a redundant "powershell ... -Command <cmd>"
// wrapper emitted by the model. The shell tool already executes via
// PowerShell, so keeping the wrapper would make the outer PowerShell parse the
// inner command line as a double-quoted string and expand its $variables
// against the outer (empty) scope before the nested powershell.exe even
// starts — e.g. `Where-Object { $_.FullName }` degrades to `{ .FullName }`
// and `foreach($i ...)` to `foreach( in )`.
// stripNestedPowerShell 移除模型多余的 "powershell ... -Command <cmd>" 包装。
// shell 工具本身已在 PowerShell 中执行，保留包装会让外层 PowerShell 把内层
// 命令行当双引号字符串解析，并在内层 powershell.exe 启动前就按外层（空）
// 作用域展开其 $ 变量——例如 `Where-Object { $_.FullName }` 会退化为
// `{ .FullName }`，`foreach($i ...)` 会退化为 `foreach( in )`。
func stripNestedPowerShell(command []string) []string {
	if len(command) < 2 {
		return command
	}
	prog := strings.ToLower(command[0])
	if idx := strings.LastIndexAny(prog, `/\`); idx >= 0 {
		prog = prog[idx+1:]
	}
	prog = strings.TrimSuffix(prog, ".exe")
	if prog != "powershell" && prog != "pwsh" {
		return command
	}
	for i := 1; i < len(command); i++ {
		flag := strings.ToLower(command[i])
		if flag == "-command" || flag == "-c" {
			if i+1 < len(command) {
				return command[i+1:]
			}
			return command
		}
		if !strings.HasPrefix(flag, "-") {
			// Unexpected positional argument; leave the command untouched.
			// 出现非预期的位置参数；保持原样不动。
			return command
		}
	}
	return command
}

// encodePowerShellCommand encodes a script for -EncodedCommand: Base64 of
// UTF-16LE, which is what PowerShell expects. Encoding bypasses the native
// command line parser entirely, so quotes, $variables and non-ASCII text
// arrive at PowerShell verbatim with a single parse.
// encodePowerShellCommand 将脚本编码为 -EncodedCommand 形式：PowerShell 要求
// 的 UTF-16LE Base64。编码方式完全绕过原生命令行解析器，引号、$ 变量和
// 非 ASCII 文本原样到达 PowerShell，只经一次解析。
func encodePowerShellCommand(script string) string {
	units := utf16.Encode([]rune(script))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		b[i*2] = byte(u)
		b[i*2+1] = byte(u >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// prepareCommand converts the command array into a shell invocation.
// prepareCommand 将命令数组转换为 shell 调用。
// On Windows it uses PowerShell -EncodedCommand (resolved by windowsShell,
// cmd.exe /c only as last resort); on Unix it uses sh -c.
// Windows 上使用 PowerShell -EncodedCommand（由 windowsShell 解析，仅作最后
// 兜底时用 cmd.exe /c）；Unix 上使用 sh -c。
func prepareCommand(command []string) (string, []string) {
	if runtime.GOOS == "windows" {
		if ps := windowsShell(); ps != "" {
			script := powershellUTF8Prefix + joinPowerShellCommand(command)
			return ps, []string{"-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShellCommand(script)}
		}
		return "cmd.exe", []string{"/c", joinCommand(command)}
	}
	return "sh", []string{"-c", joinCommand(command)}
}

// joinPowerShellCommand joins a command array into a single script string.
// Parts containing spaces or quotes are wrapped in single quotes — PowerShell
// literal strings — with embedded single quotes doubled. Unlike Go's %q this
// survives PowerShell's own parsing (no \" escapes) and keeps $variables literal.
// joinPowerShellCommand 将命令数组拼接为单个脚本字符串。包含空格或引号的
// 部分用单引号包裹（PowerShell 字面量字符串），内嵌单引号翻倍。与 Go 的 %q
// 不同，这种形式能经受 PowerShell 自身的解析（无 \" 转义）且保持 $ 变量字面量。
func joinPowerShellCommand(command []string) string {
	if len(command) == 1 {
		return command[0]
	}
	parts := make([]string, len(command))
	for i, part := range command {
		if containsSpace(part) || strings.ContainsAny(part, `"'`) {
			parts[i] = "'" + strings.ReplaceAll(part, "'", "''") + "'"
		} else {
			parts[i] = part
		}
	}
	return strings.Join(parts, " ")
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

	// Extract the program name from the first token (command may be a single
	// verbatim string): lowercase, strip path prefix and .exe suffix.
	// 从第一个 token 提取程序名称（command 可能是单个原样字符串）：
	// 转为小写，去除路径前缀和 .exe 后缀。
	prog := strings.ToLower(command[0])
	if idx := strings.IndexAny(prog, " \t"); idx >= 0 {
		prog = prog[:idx]
	}
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
