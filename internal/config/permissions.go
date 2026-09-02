package config

import (
	"strings"
)

// BuiltinProfileReadOnly is the reserved identifier for the built-in
// read-only permission profile. It mirrors the Rust constant
// BUILT_IN_PERMISSION_PROFILE_READ_ONLY (":read-only").
// BuiltinProfileReadOnly 是内置只读权限配置文件的保留标识符。
// 它对应于 Rust 常量 BUILT_IN_PERMISSION_PROFILE_READ_ONLY（":read-only"）。
const BuiltinProfileReadOnly = ":read-only"

// BuiltinProfileWorkspace is the reserved identifier for the built-in
// workspace-write permission profile. It mirrors the Rust constant
// BUILT_IN_PERMISSION_PROFILE_WORKSPACE (":workspace").
// BuiltinProfileWorkspace 是内置工作区写入权限配置文件的保留标识符。
// 它对应于 Rust 常量 BUILT_IN_PERMISSION_PROFILE_WORKSPACE（":workspace"）。
const BuiltinProfileWorkspace = ":workspace"

// BuiltinProfileDangerFullAccess is the reserved identifier for the built-in
// full-access permission profile. It mirrors the Rust constant
// BUILT_IN_PERMISSION_PROFILE_DANGER_FULL_ACCESS (":danger-full-access").
// BuiltinProfileDangerFullAccess 是内置完全访问权限配置文件的保留标识符。
// 它对应于 Rust 常量 BUILT_IN_PERMISSION_PROFILE_DANGER_FULL_ACCESS（":danger-full-access"）。
const BuiltinProfileDangerFullAccess = ":danger-full-access"

// PermissionProfile describes a named set of filesystem and execution
// permissions that the sandbox enforces for agent-spawned commands.
//
// This is the Go counterpart of the Rust PermissionProfile type, simplified to
// the fields required by the CLI agent. A profile controls whether the
// filesystem is read-only, which additional paths may be written to, and
// whether arbitrary command execution is permitted.
// PermissionProfile 描述沙箱为代理生成的命令强制执行的一组命名的文件系统和执行权限。
//
// 这是 Rust PermissionProfile 类型的 Go 对应版本，简化到 CLI 代理所需的字段。
// 配置文件控制文件系统是否为只读、哪些额外路径可写入以及是否允许任意命令执行。
type PermissionProfile struct {
	// Name is the profile identifier. Built-in profiles use the ":"
	// prefix (for example ":read-only"); user-defined profiles use the
	// name given in the [permissions] table.
	// Name 是配置文件标识符。内置配置文件使用 ":" 前缀（例如 ":read-only"）；
	// 用户自定义配置文件使用 [permissions] 表中给出的名称。
	Name string

	// ReadOnly, when true, restricts all filesystem writes. The agent
	// may only read files and run commands that do not mutate state.
	// ReadOnly 为 true 时，限制所有文件系统写入。代理只能读取文件并运行不修改状态的命令。
	ReadOnly bool

	// WritePaths lists additional absolute paths that the agent may
	// write to when ReadOnly is false. The workspace root is always
	// writable under the workspace-write policy; this field extends the
	// writable set.
	// WritePaths 列出当 ReadOnly 为 false 时代理可以写入的额外绝对路径。
	// 工作区根目录在工作区写入策略下始终可写；此字段扩展了可写集合。
	WritePaths []string

	// ExecuteCommands controls whether the agent may execute shell
	// commands. When false, only read-only inspection commands are
	// allowed and only under ReadOnly profiles.
	// ExecuteCommands 控制代理是否可以执行 shell 命令。
	// 为 false 时，仅允许只读检查命令，且仅在只读配置文件下。
	ExecuteCommands bool
}

// BuiltinReadOnlyProfile is the built-in read-only permission profile.
//
// Under this profile the agent may read files and run read-only commands but
// cannot write to the filesystem. Network access is restricted.
// BuiltinReadOnlyProfile 是内置只读权限配置文件。
//
// 在此配置文件下，代理可以读取文件并运行只读命令，但不能写入文件系统。网络访问受限。
var BuiltinReadOnlyProfile = PermissionProfile{
	Name:            BuiltinProfileReadOnly,
	ReadOnly:        true,
	WritePaths:      nil,
	ExecuteCommands: false,
}

// BuiltinWorkspaceProfile is the built-in workspace-write permission profile.
//
// Under this profile the agent may write to the current workspace root and any
// explicitly listed WritePaths. Network access remains restricted by default.
// BuiltinWorkspaceProfile 是内置工作区写入权限配置文件。
//
// 在此配置文件下，代理可以写入当前工作区根目录和任何明确列出的 WritePaths。默认情况下网络访问受限。
var BuiltinWorkspaceProfile = PermissionProfile{
	Name:            BuiltinProfileWorkspace,
	ReadOnly:        false,
	WritePaths:      nil,
	ExecuteCommands: true,
}

// BuiltinDangerFullAccessProfile is the built-in full-access permission
// profile.
//
// Under this profile the sandbox is disabled entirely. The agent may write to
// any path and access the network without restriction. This profile should
// only be used in fully trusted environments.
// BuiltinDangerFullAccessProfile 是内置完全访问权限配置文件。
//
// 在此配置文件下，沙箱完全禁用。代理可以写入任何路径并无限制地访问网络。
// 此配置文件仅应在完全受信任的环境中使用。
var BuiltinDangerFullAccessProfile = PermissionProfile{
	Name:            BuiltinProfileDangerFullAccess,
	ReadOnly:        false,
	WritePaths:      nil,
	ExecuteCommands: true,
}

// SandboxPolicyFromProfile derives the coarse-grained SandboxPolicy that
// corresponds to a PermissionProfile.
//
// The mapping is:
//
//   - ReadOnly profiles  -> SandboxReadOnly
//   - Full-access profiles (named ":danger-full-access") -> SandboxDangerFullAccess
//   - All other profiles -> SandboxWorkspaceWrite
//
// A profile whose Name is empty is treated as None (no sandboxing configured).
// SandboxPolicyFromProfile 根据 PermissionProfile 推导出对应的粗粒度 SandboxPolicy。
//
// 映射关系如下：
//
//   - 只读配置文件 -> SandboxReadOnly
//   - 完全访问配置文件（名称为 ":danger-full-access"） -> SandboxDangerFullAccess
//   - 所有其他配置文件 -> SandboxWorkspaceWrite
//
// Name 为空的配置文件被视为 None（未配置沙箱）。
func SandboxPolicyFromProfile(profile PermissionProfile) SandboxPolicy {
	switch profile.Name {
	case BuiltinProfileReadOnly:
		return SandboxReadOnly
	case BuiltinProfileDangerFullAccess:
		return SandboxDangerFullAccess
	case BuiltinProfileWorkspace:
		return SandboxWorkspaceWrite
	case "":
		return SandboxNone
	default:
		if profile.ReadOnly {
			return SandboxReadOnly
		}
		return SandboxWorkspaceWrite
	}
}

// readOnlyCommands is the set of command names that are considered safe
// (non-mutating) for the purposes of the OnUntrusted approval policy. These
// commands only read files or state and do not modify the filesystem.
// readOnlyCommands 是被认为在 OnUntrusted 审批策略下安全（不修改状态）的命令名称集合。
// 这些命令只读取文件或状态，不修改文件系统。
var readOnlyCommands = map[string]bool{
	"cat":      true,
	"ls":       true,
	"ll":       true,
	"dir":      true,
	"head":     true,
	"tail":     true,
	"less":     true,
	"more":     true,
	"grep":     true,
	"rg":       true,
	"find":     true,
	"fd":       true,
	"wc":       true,
	"file":     true,
	"stat":     true,
	"pwd":      true,
	"echo":     true,
	"which":    true,
	"where":    true,
	"whoami":   true,
	"uname":    true,
	"date":     true,
	"env":      true,
	"git":      true, // safe sub-commands are further checked
	"diff":     true,
	"tree":     true,
	"du":       true,
	"df":       true,
	"free":     true,
	"top":      true,
	"ps":       true,
	"lsof":     true,
	"printenv": true,
}

// safeGitSubcommands lists git sub-commands that are read-only and therefore
// safe under the OnUntrusted policy without asking for approval.
// safeGitSubcommands 列出只读的 git 子命令，因此在 OnUntrusted 策略下无需审批即可安全执行。
var safeGitSubcommands = map[string]bool{
	"status":    true,
	"log":       true,
	"diff":      true,
	"show":      true,
	"branch":    true,
	"ls-files":  true,
	"ls-tree":   true,
	"remote":    true,
	"rev-parse": true,
	"describe":  true,
	"blame":     true,
	"shortlog":  true,
	"reflog":    true,
	"tag":       true, // listing tags is read-only; creation is blocked elsewhere
	// 列出标签是只读的；创建操作在其他地方被阻止
}

// isReadOnlyCommand returns true when the given command is considered safe
// (non-mutating) for the OnUntrusted approval policy. It inspects only the
// program name and, for git, the first sub-command argument.
// isReadOnlyCommand 在给定命令被认为在 OnUntrusted 审批策略下安全（不修改状态）时返回 true。
// 它仅检查程序名称，对于 git 则检查第一个子命令参数。
func isReadOnlyCommand(command []string) bool {
	if len(command) == 0 {
		return false
	}

	// Strip any path prefix from the program name and lowercase it.
	// 从程序名称中去除路径前缀并转为小写。
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

	if !readOnlyCommands[prog] {
		return false
	}

	// For git, verify the sub-command is read-only.
	// 对于 git，验证子命令是否为只读。
	if prog == "git" {
		if len(command) < 2 {
			return false
		}
		// Handle "git --git-dir=... status" style invocations by skipping
		// flags to find the real sub-command.
		// 处理 "git --git-dir=... status" 这类调用，跳过标志位来找到真正的子命令。
		for _, arg := range command[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			return safeGitSubcommands[arg]
		}
		return false
	}

	return true
}

// AskForApproval determines whether the user should be prompted to approve a
// command before execution, based on the active approval policy.
//
// The return value is true when the caller must ask the user for explicit
// approval, and false when the command may proceed without prompting:
//
//   - Never: never ask. Commands run (or fail) without user intervention.
//   - OnFailure: do not ask up front. The command runs inside the sandbox;
//     only failures are escalated. AskForApproval returns false here so the
//     initial execution is not blocked.
//   - OnRequest: the model decides. AskForApproval returns true, deferring
//     the decision to the model's explicit approval request flow.
//   - OnUntrusted: ask unless the command is classified as read-only by
//     isReadOnlyCommand. Safe commands proceed without prompting.
//
// The command slice is the argv of the proposed command (program followed by
// arguments).
// AskForApproval 根据当前审批策略确定是否应在执行前提示用户批准命令。
//
// 当调用者必须请求用户明确批准时返回 true，当命令可以直接执行无需提示时返回 false：
//
//   - Never：从不询问。命令运行（或失败），无需用户干预。
//   - OnFailure：不提前询问。命令在沙箱内运行；仅在失败时升级。
//     AskForApproval 在这里返回 false，因此初始执行不会被阻止。
//   - OnRequest：由模型决定。AskForApproval 返回 true，将决定权交给模型的明确审批请求流程。
//   - OnUntrusted：除非命令被 isReadOnlyCommand 归类为只读，否则都需要询问。
//     安全命令无需提示即可执行。
//
// command 切片是提议命令的 argv（程序名后跟参数）。
func AskForApproval(policy ApprovalPolicy, command []string) bool {
	switch policy {
	case ApprovalNever:
		return false
	case ApprovalOnFailure:
		// Auto-approve for initial execution; escalation happens only on
		// sandbox failures.
		// 初始执行自动批准；仅在沙箱失败时升级。
		return false
	case ApprovalOnRequest:
		// The model decides when to ask; surface the approval prompt.
		// 模型决定何时询问；显示审批提示。
		return true
	case ApprovalOnUntrusted:
		// Ask for everything except known-safe read-only commands.
		// 对所有命令都需要审批，除了已知安全的只读命令。
		return !isReadOnlyCommand(command)
	default:
		// Unknown policies default to asking for safety.
		// 未知策略默认要求审批以确保安全。
		return true
	}
}
