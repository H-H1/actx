// Package tui implements an interactive terminal user interface for ACTX CLI.
// tui 包实现 ACTX CLI 的交互式终端用户界面。
// It uses Bubble Tea (bubbletea) for the Elm-architecture-based event loop
// 它使用 Bubble Tea (bubbletea) 实现基于 Elm 架构的事件循环
// and Lipgloss for styled rendering. The TUI provides a chat-like interface
// 以及 Lipgloss 进行样式化渲染。该 TUI 提供类似聊天的界面，
// where users can type prompts, see streaming agent responses, approve
// 用户可以在其中输入提示、查看流式代理响应、批准命令
// commands, and view execution output.
// 以及查看执行输出。
package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/H-H1/actx/internal/agent"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/login"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/skills"
	"github.com/H-H1/actx/internal/tools"
)

// Styles holds the Lipgloss styles used throughout the TUI.
// Styles 持有整个 TUI 中使用的 Lipgloss 样式。
type Styles struct {
	// Title styles the application title.
	// Title 为应用标题设置样式。
	Title lipgloss.Style
	// UserMsg styles user chat messages.
	// UserMsg 为用户聊天消息设置样式。
	UserMsg lipgloss.Style
	// AgentMsg styles agent chat messages.
	// AgentMsg 为代理聊天消息设置样式。
	AgentMsg lipgloss.Style
	// SystemMsg styles system notices.
	// SystemMsg 为系统提示消息设置样式。
	SystemMsg lipgloss.Style
	// ErrorMsg styles error messages.
	// ErrorMsg 为错误消息设置样式。
	ErrorMsg lipgloss.Style
	// CommandMsg styles executed command lines.
	// CommandMsg 为执行的命令行设置样式。
	CommandMsg lipgloss.Style
	// OutputMsg styles command output text.
	// OutputMsg 为命令输出文本设置样式。
	OutputMsg lipgloss.Style
	// ThinkingMsg styles the model's thinking stream.
	// ThinkingMsg 为模型思考流设置样式。
	ThinkingMsg lipgloss.Style
	// Prompt styles the input prompt prefix.
	// Prompt 为输入提示符前缀设置样式。
	Prompt lipgloss.Style
	// Border styles generic bordered boxes.
	// Border 为通用边框盒设置样式。
	Border lipgloss.Style
	// HelpHint styles the bottom help hints.
	// HelpHint 为底部帮助提示设置样式。
	HelpHint lipgloss.Style
	// ApprovalBox styles the pending-approval box.
	// ApprovalBox 为待批准框设置样式。
	ApprovalBox lipgloss.Style
	// Status styles the status line.
	// Status 为状态行设置样式。
	Status lipgloss.Style
}

// defaultStyles returns the default set of styles for the TUI.
// defaultStyles 返回 TUI 的默认样式集。
func defaultStyles() *Styles {
	return &Styles{
		Title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")),
		UserMsg:     lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		AgentMsg:    lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
		SystemMsg:   lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		ErrorMsg:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		CommandMsg:  lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
		OutputMsg:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		ThinkingMsg: lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Italic(true),
		Prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true),
		Border:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		HelpHint:    lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		ApprovalBox: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("220")).Padding(0, 1),
		Status:      lipgloss.NewStyle().Foreground(lipgloss.Color("36")),
	}
}

// chatMessage represents a single message in the chat history.
// chatMessage 表示聊天历史中的单条消息。
type chatMessage struct {
	role string // "user", "agent", "system", "error", "command", "output", "thinking"
	// content is the message text.
	// content 是消息文本。
	content string
	// time records when the message was created.
	// time 记录消息创建的时间。
	time time.Time
}

// approvalRequest tracks a pending approval that needs user input.
// approvalRequest 跟踪需要用户输入的待批准请求。
type approvalRequest struct {
	// callID is the ID of the tool call awaiting approval.
	// callID 是等待批准的工具调用 ID。
	callID protocol.CallId
	// command is the command pending approval.
	// command 是等待批准的命令。
	command []string
	// cwd is the working directory for the command.
	// cwd 是命令的工作目录。
	cwd string
}

// Model is the Bubble Tea model for the ACTX TUI.
// Model 是 ACTX TUI 的 Bubble Tea 模型。
type Model struct {
	// styles holds the Lipgloss styles used for rendering.
	// styles 持有用于渲染的 Lipgloss 样式。
	styles *Styles
	// input is the text input box for user prompts.
	// input 是用于用户输入提示的文本输入框。
	input textinput.Model
	// messages is the chat message history.
	// messages 是聊天消息历史。
	messages []chatMessage
	// width is the current terminal width.
	// width 是当前终端宽度。
	width int
	// height is the current terminal height.
	// height 是当前终端高度。
	height int
	// ACTX is the agent instance driven by the TUI.
	// ACTX 是由 TUI 驱动的代理实例。
	ACTX *agent.ACTX
	// config is the runtime configuration.
	// config 是运行时配置。
	config *config.Config
	// logger is the structured logger for diagnostics.
	// logger 是用于诊断的结构化日志记录器。
	logger *slog.Logger

	// State
	// 状态
	// agentRunning indicates whether the agent is processing a turn.
	// agentRunning 指示代理是否正在处理一个轮次。
	agentRunning bool
	// pendingApproval holds the approval awaiting user input, if any.
	// pendingApproval 持有等待用户输入的批准请求（若有）。
	pendingApproval *approvalRequest
	// scrollOffset is the vertical scroll offset of the message view.
	// scrollOffset 是消息视图的垂直滚动偏移量。
	scrollOffset int

	// Streaming state
	// 流式状态
	// currentAgentText accumulates agent message deltas for live rendering.
	// currentAgentText 累积代理消息增量以进行实时渲染。
	currentAgentText strings.Builder
	// currentReasoningText accumulates reasoning_content deltas so the TUI
	// can render the model's live thinking stream before the final answer.
	// currentReasoningText 累积 reasoning_content 增量，
	// 让 TUI 在最终答案产出前渲染模型实时的思考流。
	currentReasoningText strings.Builder

	// Skills loaded from ~/.ACTX/skills/
	// 从 ~/.ACTX/skills/ 加载的技能列表
	skills []skills.Skill

	// Context for shutdown
	// 用于关闭的上下文
	ctx context.Context
	// cancel cancels ctx to release resources on shutdown.
	// cancel 在关闭时取消 ctx 以释放资源。
	cancel context.CancelFunc
}

// tickMsg is sent periodically to check for new events.
// tickMsg 定期发送以检查新事件。
type tickMsg time.Time

// eventMsg wraps a protocol Event for the Bubble Tea update loop.
// eventMsg 包装协议事件，用于 Bubble Tea 更新循环。
type eventMsg struct {
	// event is the wrapped protocol event delivered to the update loop.
	// event 是传递给更新循环的被包装协议事件。
	event protocol.Event
}

// NewModel creates a new TUI Model with the given ACTX instance.
// NewModel 使用给定的 ACTX 实例创建一个新的 TUI 模型。
func NewModel(cfg *config.Config, ACTX *agent.ACTX, logger *slog.Logger) *Model {
	ti := textinput.New()
	ti.Placeholder = "Type your message... (Enter to send, Ctrl+C to quit)"
	ti.Focus()
	ti.CharLimit = 0
	ti.Width = 80

	ctx, cancel := context.WithCancel(context.Background())

	m := &Model{
		styles:   defaultStyles(),
		input:    ti,
		messages: []chatMessage{},
		ACTX:     ACTX,
		config:   cfg,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
	}

	// 加载技能
	loadedSkills, err := skills.Load(cfg.ACTXHome)
	if err != nil {
		logger.Warn("加载技能失败", "error", err)
	}
	m.skills = loadedSkills

	m.addMessage("system", fmt.Sprintf("ACTX CLI - Model: %s | Provider: %s", cfg.Model, cfg.ModelProvider))
	m.addMessage("system", "Type your message and press Enter to send. Ctrl+C to quit.")
	if len(m.skills) > 0 {
		m.addMessage("system", fmt.Sprintf("已加载 %d 个技能，使用 $技能名 激活，/skills 查看。", len(m.skills)))
	}

	return m
}

// addMessage appends a message to the chat history.
// addMessage 向聊天历史追加一条消息。
func (m *Model) addMessage(role, content string) {
	m.messages = append(m.messages, chatMessage{
		role:    role,
		content: content,
		time:    time.Now(),
	})
}

// Init implements tea.Model.
// Init 实现 tea.Model 接口。
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.pollEvents())
}

// pollEvents returns a command that polls the ACTX event channel.
// pollEvents 返回一个轮询 ACTX 事件通道的命令。
func (m *Model) pollEvents() tea.Cmd {
	return func() tea.Msg {
		ev, ok := m.ACTX.NextEvent()
		if !ok {
			return tea.Quit()
		}
		return eventMsg{event: ev}
	}
}

// Update implements tea.Model.
// Update 实现 tea.Model 接口。
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = msg.Width - 4
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tickMsg:
		return m, m.pollEvents()

	case eventMsg:
		return m.handleEvent(msg.event)

	default:
		var cmds []tea.Cmd
		newInput, cmd := m.input.Update(msg)
		m.input = newInput
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}
}

// handleKeyPress processes keyboard input.
// handleKeyPress 处理键盘输入。
func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {

	case tea.KeyCtrlC:
		m.cancel()
		m.ACTX.Shutdown()
		return m, tea.Quit

	case tea.KeyEsc:
		if m.pendingApproval != nil {
			// Deny approval on Escape.
			// 按 Esc 拒绝批准。
			m.ACTX.Submit(protocol.NewExecApprovalOp(m.pendingApproval.callID, false, "denied by user"))
			m.pendingApproval = nil
			m.addMessage("system", "Approval denied.")
		}
		return m, nil

	case tea.KeyEnter:
		// If there's a pending approval, Enter approves it.
		// 如果有待处理的批准，按 Enter 批准它。
		if m.pendingApproval != nil {
			m.ACTX.Submit(protocol.NewExecApprovalOp(m.pendingApproval.callID, true, ""))
			m.pendingApproval = nil
			m.addMessage("system", "Approved.")
			return m, nil
		}

		// Otherwise, submit the user's input.
		// 否则，提交用户的输入。
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}

		// Handle slash commands.
		// 处理斜杠命令。
		if strings.HasPrefix(text, "/") {
			return m.handleSlashCommand(text)
		}

		// Submit to the agent.
		// 提交给代理。
		m.addMessage("user", text)
		m.input.Reset()
		m.agentRunning = true
		m.currentAgentText.Reset()
		m.currentReasoningText.Reset()

		// 检查并注入技能提及
		injectedText := text
		if len(m.skills) > 0 {
			mentioned := skills.ExtractMentions(text, m.skills)
			if len(mentioned) > 0 {
				injectedText = skills.InjectSkills(text, m.skills, mentioned)
				m.addMessage("system", fmt.Sprintf("已激活技能: %s", strings.Join(mentioned, ", ")))
			}
		}

		items := []protocol.ResponseItem{
			protocol.NewUserMessage(injectedText),
		}
		m.ACTX.Submit(protocol.NewUserInputOp(items))

		return m, m.pollEvents()
	}

	// Default: pass to text input.
	// 默认：传递给文本输入。显示tui文字内容
	newInput, cmd := m.input.Update(msg)
	m.input = newInput
	return m, cmd
}

// handleSlashCommand processes slash commands like /help, /clear, /compact.
// handleSlashCommand 处理斜杠命令，如 /help、/clear、/compact。
func (m *Model) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(text)
	cmd := parts[0]

	switch cmd {
	case "/help", "/h":
		m.addMessage("system", "Available commands:")
		m.addMessage("system", "  /help       - Show this help message")
		m.addMessage("system", "  /clear      - Clear the conversation history")
		m.addMessage("system", "  /compact    - Compact the conversation context")
		m.addMessage("system", "  /model [n]  - Show or switch model")
		m.addMessage("system", "  /init       - Create AGENTS.md project file")
		m.addMessage("system", "  /diff       - Show git changes")
		m.addMessage("system", "  /review     - AI code review of changes")
		m.addMessage("system", "  /status     - Show session status and token usage")
		m.addMessage("system", "  /approvals  - Show/set approval policy")
		m.addMessage("system", "  /new        - Start a new session")
		m.addMessage("system", "  /undo       - Undo last turn")
		m.addMessage("system", "  /logout     - Sign out and remove credentials")
		m.addMessage("system", "  /mcp        - Show MCP server status")
		m.addMessage("system", "  /mention    - Add file to context: /mention <path>")
		m.addMessage("system", "  /skills     - 查看可用技能，使用 $技能名 激活")
		m.addMessage("system", "  /quit       - Exit ACTX CLI")

	case "/clear", "/c":
		m.messages = nil
		m.addMessage("system", "Conversation cleared.")

	case "/compact":
		m.ACTX.Submit(protocol.NewCompactOp(false))
		m.addMessage("system", "Compacting context...")

	case "/model", "/m":
		if len(parts) >= 2 {
			// Switch model
			// 切换模型
			m.config.Model = parts[1]
			m.addMessage("system", fmt.Sprintf("Model switched to: %s", parts[1]))
		} else {
			m.addMessage("system", fmt.Sprintf("Model: %s (Provider: %s)", m.config.Model, m.config.ModelProvider))
			if len(m.config.Models) > 0 {
				m.addMessage("system", fmt.Sprintf("Available models: %s", strings.Join(m.config.Models, ", ")))
			}
		}

	case "/init":
		// Generate AGENTS.md in the current working directory
		// 在当前工作目录生成 AGENTS.md
		agentsContent := `# AGENTS.md - Project Instructions for ACTX

## Project Overview
Describe your project here.

## Tech Stack
- Language: 
- Framework: 
- Build Tool: 

## Code Style
- Follow standard formatting conventions
- Add comments for complex logic

## Testing
- Run tests with: go test ./...

## Build
- Build with: go build -o ACTX.exe ./cmd/ACTX/
`
		err := os.WriteFile(filepath.Join(m.config.Cwd, "AGENTS.md"), []byte(agentsContent), 0o644)
		if err != nil {
			m.addMessage("error", fmt.Sprintf("Failed to create AGENTS.md: %v", err))
		} else {
			m.addMessage("system", "Created AGENTS.md with project template. Edit it to customize.")
		}

	case "/diff":
		// Run git diff to show changes
		// 运行 git diff 显示变更
		cmd := exec.Command("git", "diff", "--stat")
		cmd.Dir = m.config.Cwd
		output, err := cmd.CombinedOutput()
		if err != nil {
			m.addMessage("error", fmt.Sprintf("git diff failed: %v", err))
		} else {
			m.addMessage("output", string(output))
		}
		// Also show untracked files
		// 同时显示未追踪文件
		cmd2 := exec.Command("git", "status", "--short")
		cmd2.Dir = m.config.Cwd
		output2, _ := cmd2.CombinedOutput()
		if len(output2) > 0 {
			m.addMessage("system", "Untracked/modified files:")
			m.addMessage("output", string(output2))
		}

	case "/review":
		// Submit a code review request to the agent
		// 向代理提交代码审查请求
		reviewPrompt := "Please review the current working directory changes. Run git diff to see what changed, then provide a code review highlighting any issues, bugs, or improvements."
		m.addMessage("user", "/review")
		m.agentRunning = true
		m.currentAgentText.Reset()
		items := []protocol.ResponseItem{
			protocol.NewUserMessage(reviewPrompt),
		}
		m.ACTX.Submit(protocol.NewUserInputOp(items))

	case "/status":
		ctx := m.ACTX.Session()
		itemCount := 0
		if ctx != nil {
			itemCount = ctx.Context().Len()
		}
		m.addMessage("system", fmt.Sprintf("Session: %s\nItems in context: %d\nModel: %s\nProvider: %s\nApproval: %s\nSandbox: %s",
			m.ACTX.SessionID()[:8],
			itemCount,
			m.config.Model,
			m.config.ModelProvider,
			m.config.ApprovalPolicy,
			m.config.SandboxPolicy))

	case "/approvals":
		if len(parts) < 2 {
			m.addMessage("system", fmt.Sprintf("Current approval policy: %s", m.config.ApprovalPolicy))
			m.addMessage("system", "Usage: /approvals <never|on-failure|on-request|on-untrusted>")
		} else {
			policy, err := config.ParseApprovalPolicy(parts[1])
			if err != nil {
				m.addMessage("error", fmt.Sprintf("Invalid policy: %v", err))
			} else {
				m.config.ApprovalPolicy = policy
				m.addMessage("system", fmt.Sprintf("Approval policy set to: %s", policy))
			}
		}

	case "/new":
		m.messages = nil
		m.currentAgentText.Reset()
		// Compact the context to start fresh
		// 压缩上下文以重新开始
		m.ACTX.Submit(protocol.NewCompactOp(true))
		m.addMessage("system", "New session started. Previous context compacted.")

	case "/undo":
		// Submit a rollback operation
		// 提交回滚操作
		m.ACTX.Submit(protocol.Op{Type: protocol.OpThreadRollback, RollbackTo: -1})
		m.addMessage("system", "Undoing last turn...")

	case "/logout":
		ACTXHome := config.ACTXHomeDir()
		err := login.DeleteToken(ACTXHome)
		if err != nil {
			m.addMessage("error", fmt.Sprintf("Logout failed: %v", err))
		} else {
			m.addMessage("system", "Signed out successfully. Run 'ACTX login' to sign in again.")
		}

	case "/mcp":
		if len(m.config.McpServers) == 0 {
			m.addMessage("system", "No MCP servers configured.")
			m.addMessage("system", "Configure MCP servers in config.toml under [mcp_servers] section.")
		} else {
			m.addMessage("system", "Configured MCP servers:")
			for name, srv := range m.config.McpServers {
				m.addMessage("system", fmt.Sprintf("  %s: %s", name, srv.Command))
			}
		}

	case "/mention":
		if len(parts) < 2 {
			m.addMessage("error", "Usage: /mention <file_path>")
		} else {
			filePath := parts[1]
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(m.config.Cwd, filePath)
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				m.addMessage("error", fmt.Sprintf("Failed to read file: %v", err))
			} else {
				// Add file content as user context
				// 将文件内容作为用户上下文添加
				mentionText := fmt.Sprintf("File: %s\n```\n%s\n```", parts[1], string(content))
				items := []protocol.ResponseItem{
					protocol.NewUserMessage(mentionText),
				}
				m.ACTX.Submit(protocol.NewUserInputOp(items))
				m.addMessage("system", fmt.Sprintf("Added %s to context.", parts[1]))
			}
		}

	case "/skills":
		if len(m.skills) == 0 {
			m.addMessage("system", "可用技能: (无)")
			m.addMessage("system", "技能是 ~/.ACTX/skills/ 下的文件夹，每个文件夹需包含 SKILL.md 文件。")
			m.addMessage("system", "SKILL.md 需包含 YAML frontmatter（name 和 description 字段）。")
			m.addMessage("system", "示例：~/.ACTX/skills/my-skill/SKILL.md")
		} else {
			m.addMessage("system", fmt.Sprintf("可用技能 (%d):", len(m.skills)))
			for _, s := range m.skills {
				desc := s.Description
				if len(desc) > 60 {
					desc = desc[:60] + "..."
				}
				m.addMessage("system", fmt.Sprintf("  $%s - %s", s.Name, desc))
			}
			m.addMessage("system", "在消息中输入 $技能名 即可激活对应技能。")
		}

	case "/quit", "/q":
		m.cancel()
		m.ACTX.Shutdown()
		return m, tea.Quit

	default:
		m.addMessage("error", fmt.Sprintf("Unknown command: %s (try /help)", cmd))
	}

	m.input.Reset()
	return m, nil
}

// handleEvent processes a protocol Event from the agent.
// handleEvent 处理来自代理的协议事件。
func (m *Model) handleEvent(ev protocol.Event) (tea.Model, tea.Cmd) {
	msg := ev.Msg

	switch msg.Type {

	case protocol.EvAgentMessageDelta:
		m.currentAgentText.WriteString(msg.Delta)
		return m, m.pollEvents()

	case protocol.EvAgentMessage:
		text := msg.Message
		if text == "" {
			text = m.currentAgentText.String()
		}
		if text != "" {
			m.addMessage("agent", text)
		}
		m.currentAgentText.Reset()
		return m, m.pollEvents()

	case protocol.EvAgentReasoning:
		// A complete reasoning item arrived. Flush the in-flight thinking
		// stream (if any) into history, then add the finalized summary as
		// a thinking message so users still see non-streamed reasoning.
		// 收到完整的推理项。先把进行中的思考流（若有）刷入历史，
		// 再把最终摘要作为思考消息加入，让用户仍能看到未流式输出的推理。
		if m.currentReasoningText.Len() > 0 {
			m.addMessage("thinking", "✻ Thinking: "+m.currentReasoningText.String())
			m.currentReasoningText.Reset()
		}
		if msg.Message != "" {
			m.addMessage("thinking", "✻ Thinking: "+msg.Message)
		}
		return m, m.pollEvents()

	case protocol.EvReasoningDelta:
		// Accumulate the reasoning delta for live display in View().
		// 累积推理增量以便在 View() 中实时显示。
		m.currentReasoningText.WriteString(msg.Delta)
		return m, m.pollEvents()

	case protocol.EvExecCommandBegin:
		cmdStr := strings.Join(msg.Command, " ")
		m.addMessage("command", fmt.Sprintf("$ %s", cmdStr))
		return m, m.pollEvents()

	case protocol.EvExecCommandEnd:
		if msg.Output != "" {
			m.addMessage("output", strings.TrimRight(msg.Output, "\n"))
		}
		if msg.ExitCode != nil && *msg.ExitCode != 0 {
			m.addMessage("error", fmt.Sprintf("Exit code: %d", *msg.ExitCode))
		}
		return m, m.pollEvents()

	case protocol.EvExecApprovalRequest:
		cmdStr := strings.Join(msg.Command, " ")
		m.pendingApproval = &approvalRequest{
			callID:  msg.CallID,
			command: msg.Command,
			cwd:     msg.Cwd,
		}
		m.addMessage("system", fmt.Sprintf("Approval required for: %s", cmdStr))
		m.addMessage("system", "Press Enter to approve, Esc to deny.")
		return m, m.pollEvents()

	case protocol.EvTokenCount:
		m.addMessage("system", fmt.Sprintf("Tokens - Input: %d, Output: %d", msg.InputTokens, msg.OutputTokens))
		return m, m.pollEvents()

	case protocol.EvTurnComplete:
		m.agentRunning = false
		// Flush any remaining streaming text.
		// 刷新任何剩余的流式文本。
		if m.currentReasoningText.Len() > 0 {
			m.addMessage("thinking", "✻ Thinking: "+m.currentReasoningText.String())
			m.currentReasoningText.Reset()
		}
		if m.currentAgentText.Len() > 0 {
			m.addMessage("agent", m.currentAgentText.String())
			m.currentAgentText.Reset()
		}
		return m, m.pollEvents()

	case protocol.EvTurnStarted:
		m.agentRunning = true
		return m, m.pollEvents()

	case protocol.EvStreamError:
		m.addMessage("error", msg.ErrorMsg)
		m.agentRunning = false
		return m, m.pollEvents()

	case protocol.EvSessionConfigured:
		return m, m.pollEvents()

	case protocol.EvShutdownComplete:
		return m, tea.Quit

	default:
		return m, m.pollEvents()
	}
}

// View implements tea.Model.
// View 实现 tea.Model 接口。
func (m *Model) View() string {
	var sb strings.Builder

	// Render messages.
	// 渲染消息。
	visibleHeight := m.height - 4 // Reserve space for input and status
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	start := 0
	if len(m.messages) > visibleHeight {
		start = len(m.messages) - visibleHeight
	}

	for i := start; i < len(m.messages); i++ {
		msg := m.messages[i]
		sb.WriteString(m.renderMessage(msg))
		sb.WriteString("\n")
	}

	// Render streaming reasoning (live thinking stream).
	// 渲染流式推理（实时思考流）。
	if m.currentReasoningText.Len() > 0 {
		sb.WriteString(m.styles.ThinkingMsg.Render("✻ Thinking: " + m.currentReasoningText.String()))
		sb.WriteString("\n")
	}

	// Render streaming text.
	// 渲染流式文本。
	if m.currentAgentText.Len() > 0 {
		sb.WriteString(m.styles.AgentMsg.Render(m.currentAgentText.String()))
		sb.WriteString("\n")
	}

	// Render pending approval box.
	// 渲染待批准框。
	if m.pendingApproval != nil {
		cmdStr := strings.Join(m.pendingApproval.command, " ")
		approvalText := fmt.Sprintf("⚠ Approve command: %s\n  [Enter] Approve  [Esc] Deny", cmdStr)
		sb.WriteString("\n")
		sb.WriteString(m.styles.ApprovalBox.Render(approvalText))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Render status line.
	// 渲染状态行。
	status := m.renderStatus()
	sb.WriteString(status)
	sb.WriteString("\n")

	// Render input.
	// 渲染输入。
	sb.WriteString(m.styles.Prompt.Render("> "))
	sb.WriteString(m.input.View())

	return sb.String()
}

// renderMessage renders a single chat message with appropriate styling.
// renderMessage 使用适当的样式渲染单条聊天消息。
func (m *Model) renderMessage(msg chatMessage) string {
	switch msg.role {
	case "user":
		return m.styles.UserMsg.Render("You: ") + msg.content
	case "agent":
		return m.styles.AgentMsg.Render("ACTX: ") + msg.content
	case "system":
		return m.styles.SystemMsg.Render(msg.content)
	case "error":
		return m.styles.ErrorMsg.Render("Error: " + msg.content)
	case "command":
		return m.styles.CommandMsg.Render(msg.content)
	case "output":
		return m.styles.OutputMsg.Render(msg.content)
	case "thinking":
		return m.styles.ThinkingMsg.Render(msg.content)
	default:
		return msg.content
	}
}

// renderStatus renders the bottom status line.
// renderStatus 渲染底部状态行。
func (m *Model) renderStatus() string {
	var status string
	if m.agentRunning {
		status = m.styles.Status.Render("● Working...")
	} else if m.pendingApproval != nil {
		status = m.styles.Status.Render("⚠ Approval needed")
	} else {
		status = m.styles.Status.Render("○ Ready")
	}

	hint := m.styles.HelpHint.Render("  Enter: Send | Ctrl+C: Quit | /help: Commands")
	return status + hint
}

// Run starts the TUI program. It blocks until the user quits.
// Run 启动 TUI 程序。它会阻塞直到用户退出。
func Run(ctx context.Context, cfg *config.Config, apiKey, baseURL string) error {
	// Build the tool registry. It maps tool names to executors and
	// guarantees a deterministic order when specs are exposed to the model.
	// 构建工具注册表。它把工具名映射到执行器，并保证向模型暴露
	// 工具描述时的顺序是确定的。
	registry := tools.NewToolRegistry()

	// Register the shell tool: executes commands under sandbox and
	// approval-policy constraints.
	// 注册 shell 工具：在沙箱与审批策略约束下执行命令。
	registry.Register(agent.NewShellTool())

	// Register the apply_patch tool: applies structured file edits.
	// 注册 apply_patch 工具：应用结构化的文件补丁修改。
	registry.Register(agent.NewApplyPatchTool())

	// Spawn the ACTX agent. This starts the background submission-loop
	// goroutine and wires up the txSub/rxEvent channels.
	// 启动 ACTX 代理。这会启动后台提交循环 goroutine，
	// 并接通 txSub/rxEvent 两条通道。
	ACTX, err := agent.Spawn(agent.ACTXArgs{
		// Runtime configuration: model, policies, instructions, cwd, etc.
		// 运行时配置：模型、策略、系统提示词、工作目录等。
		Config: cfg,
		// API key for the model provider (from env or auth file).
		// 模型服务商的 API 密钥（来自环境变量或认证文件）。
		APIKey: apiKey,
		// Base URL of the OpenAI-compatible endpoint.
		// OpenAI 兼容端点的基础 URL。
		BaseURL: baseURL,
		// Tool registry built above; the agent dispatches calls into it.
		// 上面构建的工具注册表；代理把工具调用分发到这里。
		Registry: registry,
	})
	// Propagate spawn failure to the caller (CLI main).
	// 把启动失败向上传递给调用方（CLI main）。
	if err != nil {
		return fmt.Errorf("spawn ACTX: %w", err)
	}

	// Use the default structured logger for TUI-side diagnostics.
	// 使用默认的结构化日志记录器输出 TUI 侧诊断信息。
	logger := slog.Default()

	// Create the Bubble Tea Model: holds UI state, the text input box,
	// the message history, and a reference to the agent used for
	// Submit/NextEvent calls.
	// 创建 Bubble Tea Model：持有 UI 状态、文本输入框、消息历史，
	// 以及指向代理的引用（用于 Submit/NextEvent 调用）。
	model := NewModel(cfg, ACTX, logger)

	// Create the Bubble Tea program. AltScreen renders on a dedicated
	// terminal buffer so quitting restores the previous screen; mouse
	// cell motion enables wheel scrolling over the message area.
	// 创建 Bubble Tea 程序。AltScreen 在专用终端缓冲区渲染，
	// 退出时恢复之前的屏幕；鼠标单元格运动启用消息区滚轮滚动。
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Watchdog goroutine: when the parent context is cancelled (Ctrl+C at
	// the CLI level or a process signal), shut the agent down so the
	// submission loop drains and blocked event sends are released.
	// 看门狗 goroutine：当父 context 被取消（CLI 层面的 Ctrl+C 或进程
	// 信号）时关闭代理，使提交循环退出并解除阻塞中的事件发送。
	go func() {
		// Block until the context is cancelled.
		// 阻塞直到 context 被取消。
		<-ctx.Done()
		// Stop the agent: closes channels and cancels the session context.
		// 停止代理：关闭通道并取消会话 context。
		ACTX.Shutdown()
	}()

	// Run the TUI event loop. This call blocks the current goroutine until
	// the user quits (Ctrl+C, /quit, or EvShutdownComplete triggers Quit).
	// 运行 TUI 事件循环。此调用阻塞当前 goroutine，直到用户退出
	// （Ctrl+C、/quit 或 EvShutdownComplete 触发 Quit）。
	_, err = p.Run()
	// On TUI failure, still shut the agent down to avoid leaking the
	// background goroutine, then bubble the error up.
	// TUI 出错时仍要关闭代理以避免泄漏后台 goroutine，然后把错误向上冒泡。
	if err != nil {
		ACTX.Shutdown()
		return fmt.Errorf("tui error: %w", err)
	}

	// Normal exit: the TUI finished cleanly. Agent shutdown is handled by
	// the watchdog goroutine when the caller cancels the context.
	// 正常退出：TUI 干净地结束。当调用方取消 context 时，
	// 由看门狗 goroutine 负责关闭代理。
	return nil
}
