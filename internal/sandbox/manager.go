package sandbox

import (
	"runtime"
)

// SandboxManager selects an appropriate sandbox for the current platform and
// transforms commands so they execute inside the chosen sandbox.
//
// The zero value is not ready for use; construct one with NewSandboxManager so
// that the platform default is resolved once and cached.
// SandboxManager 为当前平台选择适当的沙箱，并
// 转换命令使其在选定的沙箱内执行。
//
// 零值不可直接使用；使用 NewSandboxManager 构造一个，
// 以便平台默认值一次性解析并缓存。
type SandboxManager struct {
	// platformSandbox is the default SandboxType for the host operating
	// system, captured at construction time.
	// platformSandbox 是主机操作系统的默认 SandboxType，在构造时捕获。
	platformSandbox SandboxType
}

// NewSandboxManager returns a SandboxManager configured for the current
// platform. The platform default sandbox is resolved eagerly via
// GetPlatformSandbox and cached on the manager.
// NewSandboxManager 返回一个为当前平台配置的 SandboxManager。
// 平台默认沙箱通过 GetPlatformSandbox 急切解析并缓存在管理器上。
func NewSandboxManager() *SandboxManager {
	return &SandboxManager{platformSandbox: GetPlatformSandbox()}
}

// SelectInitial determines the SandboxType to use for a new session given the
// requested filesystem and network policies and the caller's sandboxing
// preference.
//
// The selection rules are:
//   - PreferenceForbid always yields SandboxNone.
//   - A PolicyNone or PolicyDangerFullAccess policy (for either filesystem or
//     network) yields SandboxNone, since no restriction is requested.
//   - Otherwise the host platform's default sandbox is used. When the
//     preference is PreferenceRequire and the platform has no sandbox, the
//     returned value is SandboxNone; callers that need to enforce the
//     requirement should check ShouldRequirePlatformSandbox separately.
// SelectInitial 根据请求的文件系统和网络策略以及调用者的沙箱偏好，
// 确定新会话要使用的 SandboxType。
//
// 选择规则：
//   - PreferenceForbid 始终产生 SandboxNone。
//   - PolicyNone 或 PolicyDangerFullAccess 策略（针对文件系统或网络）产生 SandboxNone，
//     因为没有请求任何限制。
//   - 否则使用主机平台的默认沙箱。当偏好为 PreferenceRequire 且平台没有沙箱时，
//     返回值为 SandboxNone；需要强制执行该要求的调用者应单独检查
//     ShouldRequirePlatformSandbox。
func (m *SandboxManager) SelectInitial(fileSystemPolicy, networkPolicy SandboxPolicy, pref SandboxablePreference) SandboxType {
	if pref == PreferenceForbid {
		return SandboxNone
	}
	if isUnrestricted(fileSystemPolicy) || isUnrestricted(networkPolicy) {
		return SandboxNone
	}
	return m.platformSandbox
}

// isUnrestricted reports whether a policy grants full access and therefore
// does not benefit from sandboxing.
// isUnrestricted 报告策略是否授予完全访问权限，因此不需要沙箱保护。
func isUnrestricted(p SandboxPolicy) bool {
	return p == PolicyNone || p == PolicyDangerFullAccess
}

// Transform wraps a SandboxCommand with the argument vector of the selected
// sandbox, producing a SandboxExecRequest ready for exec.
//
// When req.Sandbox is SandboxNone the command is returned unwrapped. When the
// sandbox type is not supported on the current platform (for example a
// seatbelt request on Linux), Transform returns an error.
// Transform 使用选定沙箱的参数向量包装 SandboxCommand，生成一个准备好执行的 SandboxExecRequest。
//
// 当 req.Sandbox 为 SandboxNone 时，命令将不加包装地返回。
// 当沙箱类型在当前平台上不受支持时（例如在 Linux 上请求 seatbelt），Transform 返回错误。
func (m *SandboxManager) Transform(req SandboxTransformRequest) (SandboxExecRequest, error) {
	cmd := req.Command
	argv := make([]string, 0, 1+len(cmd.Args))
	argv = append(argv, cmd.Program)
	argv = append(argv, cmd.Args...)

	if req.Sandbox == SandboxNone {
		return SandboxExecRequest{
			Command:     argv,
			Cwd:         cmd.Cwd,
			Env:         cmd.Env,
			SandboxType: SandboxNone,
		}, nil
	}

	wrapper, err := platformWrapperArgs(req.Sandbox, req.Permissions)
	if err != nil {
		return SandboxExecRequest{}, err
	}

	full := make([]string, 0, len(wrapper)+len(argv))
	full = append(full, wrapper...)
	full = append(full, argv...)

	return SandboxExecRequest{
		Command:     full,
		Cwd:         cmd.Cwd,
		Env:         cmd.Env,
		SandboxType: req.Sandbox,
	}, nil
}

// SandboxTransformRequest is the input to SandboxManager.Transform. It pairs
// the command to run with the permissions the sandbox should enforce and the
// sandbox implementation to use.
// SandboxTransformRequest 是 SandboxManager.Transform 的输入。
// 它将要运行的命令与沙箱应强制执行的权限以及要使用的沙箱实现配对。
type SandboxTransformRequest struct {
	// Command is the unwrapped command to execute.
	// Command 是要执行的未包装命令。
	Command SandboxCommand

	// Permissions describes the access profile the sandbox should enforce.
	// Permissions 描述沙箱应强制执行的访问配置。
	Permissions PermissionProfile

	// Sandbox is the sandbox implementation selected by SelectInitial (or
	// chosen explicitly by the caller).
	// Sandbox 是由 SelectInitial 选择（或由调用者显式选择）的沙箱实现。
	Sandbox SandboxType
}

// GetPlatformSandbox returns the default SandboxType for the host operating
// system, selected via runtime.GOOS. Systems without a supported sandbox
// return SandboxNone.
// GetPlatformSandbox 返回主机操作系统的默认 SandboxType，
// 通过 runtime.GOOS 选择。没有支持的沙箱的系统返回 SandboxNone。
func GetPlatformSandbox() SandboxType {
	switch runtime.GOOS {
	case "darwin":
		return SandboxMacosSeatbelt
	case "linux":
		return SandboxLinuxSeccomp
	case "windows":
		return SandboxWindowsRestrictedToken
	default:
		return SandboxNone
	}
}

// ShouldRequirePlatformSandbox reports whether the host platform has a sandbox
// that should be used by default. It returns true whenever GetPlatformSandbox
// returns a value other than SandboxNone.
// ShouldRequirePlatformSandbox 报告主机平台是否有默认应使用的沙箱。
// 当 GetPlatformSandbox 返回 SandboxNone 以外的值时，它返回 true。
func ShouldRequirePlatformSandbox() bool {
	return GetPlatformSandbox() != SandboxNone
}
