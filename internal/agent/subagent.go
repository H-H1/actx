package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// SubAgent is a lightweight agent that runs independently with its own context.
// SubAgent 是一个轻量级代理，拥有独立的上下文独立运行。
type SubAgent struct {
	// id is the sub-agent's session identifier.
	// id 是子代理的会话标识符。
	id string
	// session is the underlying Session that runs the sub-agent's turns.
	// session 是运行子代理轮次的底层 Session。
	session *Session
	// cancel cancels the sub-agent's context to stop it.
	// cancel 取消子代理的上下文以停止其运行。
	cancel context.CancelFunc
}

// SubAgentArgs holds parameters for creating a sub-agent.
// SubAgentArgs 保存创建子代理的参数。
type SubAgentArgs struct {
	// Config is the resolved configuration for the sub-agent.
	// Config 是子代理的已解析配置。
	Config *config.Config
	// APIKey is the provider API key used to create the model client.
	// APIKey 是用于创建模型客户端的提供商 API 密钥。
	APIKey string
	// BaseURL overrides the provider's default API base URL.
	// BaseURL 覆盖提供商的默认 API 基础 URL。
	BaseURL string
	// Prompt is the task description injected as the initial user message.
	// Prompt 是作为初始用户消息注入的任务描述。
	Prompt string
	// Registry is the tool registry; defaults to shell and apply_patch tools.
	// Registry 是工具注册表；默认注册 shell 和 apply_patch 工具。
	Registry *tools.ToolRegistry
	// Logger is the structured logger; defaults to slog.Default().
	// Logger 是结构化日志记录器；默认为 slog.Default()。
	Logger *slog.Logger
	// ParentCtx is the parent context; defaults to context.Background().
	// ParentCtx 是父上下文；默认为 context.Background()。
	ParentCtx context.Context
}

// SpawnSubAgent creates a sub-agent that runs a single-turn task independently.
// SpawnSubAgent 创建一个独立运行单轮任务的子代理。
func SpawnSubAgent(args SubAgentArgs) (*SubAgent, error) {
	if args.Config == nil {
		return nil, fmt.Errorf("config is required")
	}

	client := model.NewModelClient(args.APIKey, args.Config.Model, args.BaseURL)

	if args.Registry == nil {
		args.Registry = tools.NewToolRegistry()
		args.Registry.Register(NewShellTool())
		args.Registry.Register(NewApplyPatchTool())
	}

	if args.Logger == nil {
		args.Logger = slog.Default()
	}

	parentCtx := args.ParentCtx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)

	session := &Session{
		id:         protocol.NewSessionId(),
		threadID:   protocol.NewThreadId(),
		config:     args.Config,
		client:     client,
		registry:   args.Registry,
		router:     tools.NewToolRouter(args.Registry),
		logger:     args.Logger,
		ctx:        ctx,
		txSub:      make(chan protocol.Submission, 16),
		rxEvent:    make(chan protocol.Event, 64),
		approvalCh: make(chan approvalResponse, 1),
	}
	session.context = NewContextManager(args.Config.BaseInstructions)

	// Add the prompt as user input.
	// 将提示作为用户输入添加。
	session.context.Append(protocol.NewUserMessage(args.Prompt))

	return &SubAgent{
		id:      session.id,
		session: session,
		cancel:  cancel,
	}, nil
}

// Run executes the sub-agent's turn and returns when complete.
// Run 执行子代理的轮次并在完成时返回。
func (sa *SubAgent) Run(parentCtx context.Context) {
	turnID := protocol.NewTurnId()
	sa.session.runTurn(parentCtx, turnID, protocol.NewSubmissionId())
}

// Cancel stops the sub-agent.
// Cancel 停止子代理。
func (sa *SubAgent) Cancel() {
	sa.cancel()
}

// ID returns the sub-agent's session ID.
// ID 返回子代理的会话 ID。
func (sa *SubAgent) ID() string {
	return sa.id
}
