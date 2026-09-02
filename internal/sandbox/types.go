// Package sandbox implements the sandboxing system that constrains
// filesystem, network, and process access for commands spawned by the ACTX
// CLI agent.
//
// The package is intentionally self-contained: it depends only on the Go
// standard library so it can be reused by both the agent runtime and
// stand-alone tooling. A SandboxManager selects an appropriate SandboxType
// for the host platform and transforms a SandboxCommand into a
// SandboxExecRequest whose argv is prefixed with the platform's sandbox
// wrapper (for example macOS sandbox-exec(1) or the ACTX-linux-sandbox
// helper).
// sandbox 包实现沙箱系统，用于约束 ACTX CLI 代理生成的命令的
// 文件系统、网络和进程访问。
//
// 该包有意保持自包含：它仅依赖 Go 标准库，因此可以被代理运行时和
// 独立工具复用。SandboxManager 为主机平台选择适当的 SandboxType，
// 并将 SandboxCommand 转换为 SandboxExecRequest，
// 其 argv 前缀为平台的沙箱包装器（例如 macOS sandbox-exec(1) 或 ACTX-linux-sandbox 辅助程序）。
package sandbox

import (
	"encoding/json"
	"fmt"
)

// SandboxType identifies the sandboxing mechanism used to constrain command
// execution. The value is chosen by SandboxManager based on the host platform
// and the caller's preference.
// SandboxType 标识用于约束命令执行的沙箱机制。
// 该值由 SandboxManager 根据主机平台和调用者的偏好选择。
type SandboxType int

const (
	// SandboxNone disables sandboxing entirely. Commands run with the full
	// privileges of the parent process and are not wrapped.
	// SandboxNone 完全禁用沙箱。命令以父进程的完全权限运行，不进行包装。
	SandboxNone SandboxType = iota

	// SandboxMacosSeatbelt uses the macOS sandbox-exec(1) facility to
	// restrict filesystem and network access for spawned commands.
	// SandboxMacosSeatbelt 使用 macOS sandbox-exec(1) 工具来
	// 限制生成命令的文件系统和网络访问。
	SandboxMacosSeatbelt

	// SandboxLinuxSeccomp uses the ACTX-linux-sandbox helper (a seccomp and
	// mount-namespace based wrapper) to restrict syscalls and filesystem
	// access on Linux.
	// SandboxLinuxSeccomp 使用 ACTX-linux-sandbox 辅助程序（基于 seccomp 和
	// 挂载命名空间的包装器）来限制 Linux 上的系统调用和文件系统访问。
	SandboxLinuxSeccomp

	// SandboxWindowsRestrictedToken uses a Windows restricted token to limit
	// the privileges of spawned processes.
	// SandboxWindowsRestrictedToken 使用 Windows 受限令牌来限制
	// 生成进程的权限。
	SandboxWindowsRestrictedToken
)

// String returns the kebab-case identifier for the sandbox type, suitable for
// logging, configuration, and serialization.
// String 返回沙箱类型的 kebab-case 标识符，适用于日志、配置和序列化。
func (s SandboxType) String() string {
	switch s {
	case SandboxNone:
		return "none"
	case SandboxMacosSeatbelt:
		return "macos-seatbelt"
	case SandboxLinuxSeccomp:
		return "linux-seccomp"
	case SandboxWindowsRestrictedToken:
		return "windows-restricted-token"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// MarshalJSON encodes the SandboxType as its string identifier so that sandbox
// types remain readable in JSON payloads.
// MarshalJSON 将 SandboxType 编码为其字符串标识符，以便沙箱类型在 JSON 负载中保持可读。
func (s SandboxType) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON decodes a SandboxType from either its string identifier or its
// underlying integer value.
// UnmarshalJSON 从字符串标识符或其底层整数值解码 SandboxType。
func (s *SandboxType) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		switch name {
		case SandboxNone.String():
			*s = SandboxNone
		case SandboxMacosSeatbelt.String():
			*s = SandboxMacosSeatbelt
		case SandboxLinuxSeccomp.String():
			*s = SandboxLinuxSeccomp
		case SandboxWindowsRestrictedToken.String():
			*s = SandboxWindowsRestrictedToken
		default:
			return fmt.Errorf("unknown sandbox type %q", name)
		}
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("sandbox type must be a string or integer: %w", err)
	}
	*s = SandboxType(n)
	return nil
}

// SandboxablePreference expresses how strongly a caller wants sandboxing to be
// applied. It is the Go counterpart of the Rust Sandboxability enum.
// SandboxablePreference 表示调用者希望应用沙箱的强度。
// 它是 Rust Sandboxability 枚举的 Go 对应物。
type SandboxablePreference int

const (
	// PreferenceAuto lets the SandboxManager pick a sensible default based on
	// the host platform and the requested policies. This is the value used by
	// most callers.
	// PreferenceAuto 让 SandboxManager 根据主机平台和请求的策略选择合理的默认值。
	// 这是大多数调用者使用的值。
	PreferenceAuto SandboxablePreference = iota

	// PreferenceRequire demands that a platform sandbox be used. When the host
	// platform has no supported sandbox, SelectInitial falls back to
	// SandboxNone and the caller is responsible for surfacing the mismatch.
	// PreferenceRequire 要求使用平台沙箱。当主机平台没有支持的沙箱时，
	// SelectInitial 回退到 SandboxNone，调用者负责提示不匹配。
	PreferenceRequire

	// PreferenceForbid disables sandboxing regardless of platform support or
	// requested policies.
	// PreferenceForbid 禁用沙箱，无论平台支持或请求的策略如何。
	PreferenceForbid
)

// String returns the kebab-case identifier for the preference.
// String 返回偏好的 kebab-case 标识符。
func (p SandboxablePreference) String() string {
	switch p {
	case PreferenceAuto:
		return "auto"
	case PreferenceRequire:
		return "require"
	case PreferenceForbid:
		return "forbid"
	default:
		return fmt.Sprintf("unknown(%d)", int(p))
	}
}

// SandboxPolicy describes the coarse-grained filesystem and network access
// policy a sandbox should enforce.
//
// This type mirrors config.SandboxPolicy but is defined locally so the
// sandbox package remains free of non-standard-library dependencies. Callers
// that already hold a config.SandboxPolicy should map it to the equivalent
// value here before invoking SandboxManager.
// SandboxPolicy 描述沙箱应强制执行的粗粒度文件系统和网络访问策略。
//
// 此类型镜像了 config.SandboxPolicy，但在本地定义，以便
// sandbox 包保持不依赖非标准库。已经持有 config.SandboxPolicy 的调用者
// 应在调用 SandboxManager 之前将其映射为此处的等效值。
type SandboxPolicy int

const (
	// PolicyNone indicates that no sandbox policy has been configured.
	// SelectInitial treats this the same as PolicyDangerFullAccess and
	// returns SandboxNone.
	// PolicyNone 表示尚未配置沙箱策略。
	// SelectInitial 将其与 PolicyDangerFullAccess 同等对待，并返回 SandboxNone。
	PolicyNone SandboxPolicy = iota

	// PolicyReadOnly restricts the sandboxed command to read-only filesystem
	// access. Network access is denied.
	// PolicyReadOnly 将沙箱命令限制为只读文件系统访问。拒绝网络访问。
	PolicyReadOnly

	// PolicyWorkspaceWrite allows the sandboxed command to write to the
	// workspace root and any explicitly granted WritePaths. Network access is
	// denied by default.
	// PolicyWorkspaceWrite 允许沙箱命令写入工作区根目录和任何显式授予的 WritePaths。
	// 默认拒绝网络访问。
	PolicyWorkspaceWrite

	// PolicyDangerFullAccess disables the sandbox entirely. The command may
	// write anywhere and access the network without restriction. This policy
	// should only be used in fully trusted environments.
	// PolicyDangerFullAccess 完全禁用沙箱。命令可以写入任何位置并无限制地访问网络。
	// 此策略应仅在完全受信任的环境中使用。
	PolicyDangerFullAccess
)

// String returns the kebab-case identifier for the policy, matching the
// spelling used in configuration files.
// String 返回策略的 kebab-case 标识符，与配置文件中使用的拼写匹配。
func (p SandboxPolicy) String() string {
	switch p {
	case PolicyNone:
		return "none"
	case PolicyReadOnly:
		return "read-only"
	case PolicyWorkspaceWrite:
		return "workspace-write"
	case PolicyDangerFullAccess:
		return "danger-full-access"
	default:
		return fmt.Sprintf("unknown(%d)", int(p))
	}
}

// MarshalJSON encodes the SandboxPolicy as its string identifier.
// MarshalJSON 将 SandboxPolicy 编码为其字符串标识符。
func (p SandboxPolicy) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

// UnmarshalJSON decodes a SandboxPolicy from either its string identifier or
// its underlying integer value.
// UnmarshalJSON 从字符串标识符或其底层整数值解码 SandboxPolicy。
func (p *SandboxPolicy) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		switch name {
		case PolicyNone.String():
			*p = PolicyNone
		case PolicyReadOnly.String():
			*p = PolicyReadOnly
		case PolicyWorkspaceWrite.String():
			*p = PolicyWorkspaceWrite
		case PolicyDangerFullAccess.String():
			*p = PolicyDangerFullAccess
		default:
			return fmt.Errorf("unknown sandbox policy %q", name)
		}
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("sandbox policy must be a string or integer: %w", err)
	}
	*p = SandboxPolicy(n)
	return nil
}

// SandboxCommand describes a command to be executed, together with its working
// directory and environment, before any sandbox wrapping is applied.
// SandboxCommand 描述要执行的命令，包括其工作目录和环境，在任何沙箱包装应用之前。
type SandboxCommand struct {
	// Program is the executable to run.
	// Program 是要运行的可执行文件。
	Program string `json:"program"`

	// Args are the arguments to pass to Program.
	// Args 是要传递给 Program 的参数。
	Args []string `json:"args,omitempty"`

	// Cwd is the working directory in which to run the command. When empty
	// the caller's current directory is used.
	// Cwd 是运行命令的工作目录。为空时使用调用者的当前目录。
	Cwd string `json:"cwd,omitempty"`

	// Env is the environment for the process, as a list of "KEY=VALUE"
	// strings. When empty the parent environment is inherited.
	// Env 是进程的环境变量，以 "KEY=VALUE" 字符串列表形式提供。为空时继承父环境。
	Env []string `json:"env,omitempty"`
}

// SandboxExecRequest is the fully prepared command ready for execution. The
// Command slice is the complete argv, including any sandbox wrapper prefix,
// with Command[0] being the program to exec.
// SandboxExecRequest 是准备完全就绪、可以执行的命令。
// Command 切片是完整的 argv，包括任何沙箱包装前缀，其中 Command[0] 是要执行的程序。
type SandboxExecRequest struct {
	// Command is the full argv to execute, including the sandbox wrapper when
	// one is in use.
	// Command 是要执行的完整 argv，包括使用沙箱包装器时的包装器。
	Command []string `json:"command"`

	// Cwd is the working directory for the spawned process.
	// Cwd 是生成进程的工作目录。
	Cwd string `json:"cwd,omitempty"`

	// Env is the environment for the spawned process.
	// Env 是生成进程的环境变量。
	Env []string `json:"env,omitempty"`

	// SandboxType records the sandbox mechanism that wraps the command, or
	// SandboxNone when the command runs unwrapped.
	// SandboxType 记录包装命令的沙箱机制，当命令不加包装运行时为 SandboxNone。
	SandboxType SandboxType `json:"sandboxType"`
}

// PermissionProfile describes the access a sandboxed command is granted. It is
// the input the platform-specific wrappers consume when building their
// argument vectors.
// PermissionProfile 描述沙箱命令被授予的访问权限。
// 它是平台特定包装器在构建其参数向量时消耗的输入。
type PermissionProfile struct {
	// Name is a human-readable label for the profile.
	// Name 是配置的人类可读标签。
	Name string `json:"name"`

	// ReadOnly, when true, denies all filesystem writes inside the sandbox.
	// ReadOnly 为 true 时，拒绝沙箱内的所有文件系统写入。
	ReadOnly bool `json:"readOnly"`

	// WritePaths lists additional filesystem paths the sandbox permits writing
	// to, on top of any workspace-default writable set. Paths may be relative;
	// they are resolved against the command's working directory before being
	// passed to the sandbox.
	// WritePaths 列出沙箱允许写入的额外文件系统路径，
	// 在工作区默认可写集之外。路径可以是相对的；
	// 它们在传递给沙箱之前会相对于命令的工作目录进行解析。
	WritePaths []string `json:"writePaths,omitempty"`

	// ExecuteCommands, when true, allows the sandboxed command to spawn
	// further executables.
	// ExecuteCommands 为 true 时，允许沙箱命令生成新的可执行文件。
	ExecuteCommands bool `json:"executeCommands"`
}
