package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/H-H1/actx/internal/applypatch"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// ApplyPatchTool handles the apply_patch tool call, which allows the model
// ApplyPatchTool 处理 apply_patch 工具调用，允许模型
// to modify files using the ACTX apply_patch format.
type ApplyPatchTool struct {
	spec model.ToolSpec
}

// NewApplyPatchTool creates an ApplyPatchTool with the standard tool spec.
// NewApplyPatchTool 创建一个 ApplyPatchTool 实例，使用标准的工具规范。
func NewApplyPatchTool() *ApplyPatchTool {
	return &ApplyPatchTool{spec: ApplyPatchToolSpec()}
}

// Name returns the tool name.
// Name 返回工具名称。
func (t *ApplyPatchTool) Name() protocol.ToolName { return "apply_patch" }

// Spec returns the tool specification for the model API.
// Spec 返回用于模型 API 的工具规范。
func (t *ApplyPatchTool) Spec() model.ToolSpec { return t.spec }

// applyPatchArgs represents the arguments for the apply_patch tool.
// applyPatchArgs 表示 apply_patch 工具的参数。
type applyPatchArgs struct {
	// Input is the raw patch text in the ACTX apply_patch format.
	// Input 是 ACTX apply_patch 格式的原始补丁文本。
	Input string `json:"input"`
}

// Handle parses and applies the patch.
// Handle 解析并应用补丁。
func (t *ApplyPatchTool) Handle(ctx context.Context, inv tools.ToolInvocation) (tools.ToolOutput, error) {
	var args applyPatchArgs
	if err := json.Unmarshal([]byte(inv.Arguments), &args); err != nil {
		return nil, tools.NewRespondToModelError(fmt.Sprintf("invalid arguments: %v", err))
	}

	if args.Input == "" {
		return nil, tools.NewRespondToModelError("input is required")
	}

	// Enforce sandbox policy - block patches in read-only mode.
	// 强制执行沙箱策略 - 在只读模式下阻止补丁。
	if inv.SandboxPolicy == config.SandboxReadOnly {
		return nil, tools.NewFatalError("file modifications blocked by read-only sandbox policy")
	}

	patch, err := applypatch.Parse(args.Input)
	if err != nil {
		return nil, tools.NewRespondToModelError(fmt.Sprintf("parse patch: %v", err))
	}

	result, err := applypatch.Apply(patch, inv.Cwd)
	if err != nil {
		return nil, tools.NewFatalError(fmt.Sprintf("apply patch: %v", err))
	}

	if result.HasErrors() {
		return &tools.JSONToolOutput{
			Data: map[string]interface{}{
				"status":  "partial",
				"errors":  result.Errors,
				"summary": result.Summary(),
			},
			Exit: 1,
		}, nil
	}

	return &tools.JSONToolOutput{
		Data: map[string]interface{}{
			"status":        "success",
			"files_added":   result.FilesAdded,
			"files_updated": result.FilesUpdated,
			"files_deleted": result.FilesDeleted,
			"summary":       result.Summary(),
		},
		Exit: 0,
	}, nil
}
