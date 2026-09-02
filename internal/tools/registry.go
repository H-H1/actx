package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
)

// ToolRegistry maps tool names to their executors and maintains registration
// ToolRegistry 将工具名称映射到其执行器，并维护注册顺序，
// order so that tool specs are presented to the model deterministically.
// 以便工具规范以确定性的顺序呈现给模型。
// All methods are safe for concurrent use.
// 所有方法均可安全并发使用。
type ToolRegistry struct {
	mu        sync.RWMutex
	executors map[protocol.ToolName]ToolExecutor
	order     []protocol.ToolName
}

// NewToolRegistry creates an empty ToolRegistry ready for registration.
// NewToolRegistry 创建一个空的 ToolRegistry，准备注册。
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		executors: make(map[protocol.ToolName]ToolExecutor),
	}
}

// Register adds a tool executor to the registry. If a tool with the same name
// Register 将一个工具执行器添加到注册表中。如果同名工具已存在则替换，
// already exists it is replaced, but the original registration order is preserved.
// 但保留原始注册顺序。
func (r *ToolRegistry) Register(executor ToolExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := executor.Name()
	if _, exists := r.executors[name]; !exists {
		r.order = append(r.order, name)
	}
	r.executors[name] = executor
}

// Get retrieves the executor registered under the given name.
// Get 检索在给定名称下注册的执行器。
// The boolean return value is false if no executor is registered for that name.
// 如果没有为该名称注册执行器，则布尔返回值为 false。
func (r *ToolRegistry) Get(name protocol.ToolName) (ToolExecutor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[name]
	return executor, ok
}

// List returns all registered executors in registration order.
// List 按注册顺序返回所有已注册的执行器。
func (r *ToolRegistry) List() []ToolExecutor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ToolExecutor, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.executors[name])
	}
	return result
}

// Specs returns the tool specifications for all registered tools in
// Specs 按注册顺序返回所有已注册工具的工具规范。
// registration order. This is suitable for sending to the model API.
// 适用于发送给模型 API。
func (r *ToolRegistry) Specs() []model.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.executors[name].Spec())
	}
	return result
}

// ToolRouter wraps a ToolRegistry and provides dispatch logic to route
// ToolRouter 包装 ToolRegistry，并提供将传入工具调用
// incoming tool calls to the correct executor.
// 路由到正确执行器的分派逻辑。
type ToolRouter struct {
	// registry is the underlying registry that holds the tool executors.
	// registry 是持有工具执行器的底层注册表。
	registry *ToolRegistry
}

// NewToolRouter creates a ToolRouter backed by the given registry.
// NewToolRouter 创建一个由给定注册表支持的工具路由器。
func NewToolRouter(registry *ToolRegistry) *ToolRouter {
	return &ToolRouter{registry: registry}
}

// Dispatch routes a tool call to the matching executor and returns its output.
// Dispatch 将工具调用路由到匹配的执行器并返回其输出。
//
// If no executor is registered for the given name a fatal FunctionCallError is
// 如果没有为给定名称注册执行器，则返回致命 FunctionCallError。
// returned. The invocation is passed through as-is, so callers should set the
// 调用按原样传递，因此调用者应在调用前在 Invocation 上设置
// ApprovalCb on the invocation before calling if approval handling is needed.
// ApprovalCb（如果需要审批处理）。
func (rt *ToolRouter) Dispatch(
	ctx context.Context,
	invocation ToolInvocation,
) (ToolOutput, error) {
	executor, ok := rt.registry.Get(invocation.ToolName)
	if !ok {
		return nil, NewFatalError(fmt.Sprintf("unknown tool: %s", invocation.ToolName))
	}
	return executor.Handle(ctx, invocation)
}
