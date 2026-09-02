package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// platformWrapperArgs selects the platform-specific sandbox wrapper argument
// vector for the given SandboxType. It uses runtime.GOOS to validate that the
// requested sandbox matches the host operating system and delegates to the
// corresponding builder. SandboxNone yields a nil slice (no wrapping).
// platformWrapperArgs 为给定的 SandboxType 选择平台特定的沙箱包装参数向量。
// 它使用 runtime.GOOS 来验证请求的沙箱是否与主机操作系统匹配，并委托给相应的构建器。
// SandboxNone 产生 nil 切片（无包装）。
func platformWrapperArgs(st SandboxType, profile PermissionProfile) ([]string, error) {
	switch st {
	case SandboxMacosSeatbelt:
		if runtime.GOOS != "darwin" {
			return nil, fmt.Errorf("seatbelt sandbox is only available on macOS, got %s", runtime.GOOS)
		}
		return createSeatbeltCommandArgs(profile), nil
	case SandboxLinuxSeccomp:
		if runtime.GOOS != "linux" {
			return nil, fmt.Errorf("linux sandbox is only available on Linux, got %s", runtime.GOOS)
		}
		return createLinuxSandboxArgs(profile), nil
	case SandboxWindowsRestrictedToken:
		if runtime.GOOS != "windows" {
			return nil, fmt.Errorf("windows restricted-token sandbox is only available on Windows, got %s", runtime.GOOS)
		}
		return createWindowsSandboxArgs(profile), nil
	case SandboxNone:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported sandbox type: %s", st)
	}
}

// createSeatbeltCommandArgs builds the argument vector passed to macOS
// sandbox-exec(1). The wrapped command is appended after these arguments by
// SandboxManager.Transform, yielding:
//
//	sandbox-exec -p <profile> <program> [args...]
//
// The generated profile is intentionally minimal: it allows default
// operations, denies all filesystem writes when the profile is read-only, and
// re-grants writes to each path listed in profile.WritePaths. Full Seatbelt
// policy generation (network, process, temp directories) is handled elsewhere
// in the Rust implementation; this Go counterpart covers the subset needed to
// launch a sandboxed command.
// createSeatbeltCommandArgs 构建传递给 macOS sandbox-exec(1) 的参数向量。
// 被包装的命令由 SandboxManager.Transform 在这些参数之后追加，产生：
//
//	sandbox-exec -p <profile> <program> [args...]
//
// 生成的配置有意保持最小化：它允许默认操作，
// 当配置为只读时拒绝所有文件系统写入，
// 并重新授予对 profile.WritePaths 中列出的每个路径的写入权限。
// 完整的 Seatbelt 策略生成（网络、进程、临时目录）在 Rust 实现中的其他地方处理；
// 此 Go 对应部分涵盖启动沙箱命令所需的子集。
func createSeatbeltCommandArgs(profile PermissionProfile) []string {
	args := []string{"sandbox-exec"}
	args = append(args, "-p", seatbeltProfile(profile))
	return args
}

// seatbeltProfile renders a minimal Seatbelt policy string for the given
// permission profile.
// seatbeltProfile 为给定的权限配置渲染最小化的 Seatbelt 策略字符串。
func seatbeltProfile(profile PermissionProfile) string {
	p := "(version 1)\n(allow default)\n"
	if profile.ReadOnly {
		p += "(deny file-write*)\n"
	}
	for _, wp := range profile.WritePaths {
		p += fmt.Sprintf("(allow file-write* (subpath \"%s\"))\n", absPath(wp))
	}
	return p
}

// createLinuxSandboxArgs builds the argument vector for the
// ACTX-linux-sandbox helper. The wrapped command is appended after these
// arguments by SandboxManager.Transform, yielding:
//
//	ACTX-linux-sandbox [--read-only] [--write <p> ...] [--allow-exec] <program> [args...]
// createLinuxSandboxArgs 为 ACTX-linux-sandbox 辅助程序构建参数向量。
// 被包装的命令由 SandboxManager.Transform 在这些参数之后追加，产生：
//
//	ACTX-linux-sandbox [--read-only] [--write <p> ...] [--allow-exec] <program> [args...]
func createLinuxSandboxArgs(profile PermissionProfile) []string {
	args := []string{"ACTX-linux-sandbox"}
	if profile.ReadOnly {
		args = append(args, "--read-only")
	}
	for _, wp := range profile.WritePaths {
		args = append(args, "--write", absPath(wp))
	}
	if profile.ExecuteCommands {
		args = append(args, "--allow-exec")
	}
	return args
}

// createWindowsSandboxArgs returns the wrapper argument vector for the Windows
// restricted-token sandbox. Restricted-token setup is performed programmatically
// when spawning the process rather than via an argv prefix, so this returns nil
// and the command runs as-is. Future work will add a helper-based wrapper here.
// createWindowsSandboxArgs 返回 Windows 受限令牌沙箱的包装参数向量。
// 受限令牌设置是在生成进程时以编程方式执行的，而不是通过 argv 前缀，
// 因此这里返回 nil，命令按原样运行。未来工作将在此处添加基于辅助程序的包装器。
func createWindowsSandboxArgs(_ PermissionProfile) []string {
	return nil
}

// absPath resolves a possibly-relative path to an absolute path for inclusion
// in sandbox argument vectors. When the path cannot be resolved the original
// value is returned unchanged.
// absPath 将可能相对的路径解析为绝对路径，以便包含在沙箱参数向量中。
// 当路径无法解析时，原始值将原样返回。
func absPath(p string) string {
	if p == "" {
		return p
	}
	if filepath.IsAbs(p) {
		return p
	}
	abs, err := filepath.Abs(p)
	if err == nil {
		return abs
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, p)
	}
	return p
}
