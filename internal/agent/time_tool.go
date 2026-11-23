package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
	"github.com/H-H1/actx/internal/tools"
)

// GetTimeTool returns the current date and time.
// GetTimeTool 返回当前日期和时间。
type GetTimeTool struct {
	spec model.ToolSpec
}

// NewGetTimeTool creates a GetTimeTool.
// NewGetTimeTool 创建一个 GetTimeTool。
func NewGetTimeTool() *GetTimeTool {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"timezone": map[string]interface{}{
				"type":        "string",
				"description": "Optional IANA timezone name (e.g. Asia/Shanghai, UTC). Defaults to the local timezone.",
			},
		},
	}
	paramsJSON, _ := json.Marshal(params)
	return &GetTimeTool{
		spec: model.NewFunctionTool("get_time", "Get the current date and time. Use this whenever you need to know the current time, e.g. for timestamps, deadlines, or date math.", paramsJSON),
	}
}

// Name returns the tool name.
// Name 返回工具名称。
func (t *GetTimeTool) Name() protocol.ToolName { return "get_time" }

// Spec returns the tool specification for the model API.
// Spec 返回用于模型 API 的工具规范。
func (t *GetTimeTool) Spec() model.ToolSpec { return t.spec }

// getTimeArgs represents the arguments for the get_time tool.
// getTimeArgs 表示 get_time 工具的参数。
type getTimeArgs struct {
	// Timezone is an optional IANA timezone name.
	// Timezone 是可选的 IANA 时区名称。
	Timezone string `json:"timezone,omitempty"`
}

// Handle returns the current time, optionally converted to the requested
// timezone.
// Handle 返回当前时间，可按需转换到指定时区。
func (t *GetTimeTool) Handle(ctx context.Context, inv tools.ToolInvocation) (tools.ToolOutput, error) {
	var args getTimeArgs
	if err := json.Unmarshal([]byte(inv.Arguments), &args); err != nil {
		return nil, tools.NewRespondToModelError(fmt.Sprintf("invalid arguments: %v", err))
	}

	loc := time.Local
	name := "Local"
	if tz := args.Timezone; tz != "" {
		var err error
		loc, err = time.LoadLocation(tz)
		if err != nil {
			return nil, tools.NewRespondToModelError(fmt.Sprintf("unknown timezone %q: %v", tz, err))
		}
		name = tz
	}

	now := time.Now().In(loc)
	out := fmt.Sprintf(
		"timezone: %s\nlocal:   %s\nutc:     %s\nunix:    %d\nweekday: %s",
		name,
		now.Format("2006-01-02 15:04:05.000 -07:00 MST"),
		now.UTC().Format("2006-01-02 15:04:05 -07:00"),
		now.Unix(),
		now.Weekday(),
	)

	return &tools.ExecOutput{
		Stdout: out,
		Exit:   0,
	}, nil
}
