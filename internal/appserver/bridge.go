package appserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/H-H1/actx/internal/protocol"
)

// nextServerRequestID is the auto-incrementing id for server-initiated requests.
// nextServerRequestID 是服务器发起请求的自增 ID。
var nextServerRequestID int64 = 1

// bridgeEvent converts an internal protocol.Event to the appropriate
// bridgeEvent 将内部 protocol.Event 转换为适当的
// JSON-RPC notification(s) and sends them to the client.
// JSON-RPC 通知并发送给客户端。
func (s *Server) bridgeEvent(threadID string, ts *threadState, ev protocol.Event) {
	msg := ev.Msg
	turnID := ts.activeTurn

	switch msg.Type {

	case protocol.EvSessionConfigured:
		// Already handled during thread/start response.
		// 已在 thread/start 响应期间处理。
		return

	case protocol.EvTurnStarted:
		// Already notified in turn/start handler.
		// 已在 turn/start 处理程序中通知。
		return

	case protocol.EvAgentMessageDelta:
		itemID := "msg-" + turnID
		s.upsertItem(ts, itemID, "agentMessage", turnID, "inProgress")
		s.sendNotification("item/agentMessage/delta", ItemDeltaParams{
			ThreadID: threadID,
			TurnID:   turnID,
			ItemID:   itemID,
			Delta:    msg.Delta,
		})
		return

	case protocol.EvAgentMessage:
		itemID := "msg-" + turnID
		text := msg.Message
		if text == "" {
			return
		}
		textJSON, _ := json.Marshal(text)
		s.upsertItemComplete(ts, itemID, "agentMessage", turnID, textJSON)
		s.sendNotification("item/completed", ItemNotificationParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Item:     *ts.items[itemID],
		})
		return

	case protocol.EvAgentReasoning:
		if msg.Message == "" {
			return
		}
		itemID := "reasoning-" + turnID
		summaryJSON, _ := json.Marshal(msg.Message)
		s.upsertItemComplete(ts, itemID, "reasoning", turnID, summaryJSON)
		s.sendNotification("item/completed", ItemNotificationParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Item:     *ts.items[itemID],
		})
		return

	case protocol.EvReasoningDelta:
		// Stream reasoning deltas to the client so IDE/JSON-RPC clients
		// can render the model's live thinking stream.
		// 向客户端流式推送推理增量，以便 IDE/JSON-RPC 客户端能渲染模型实时的思考流。
		if msg.Delta == "" {
			return
		}
		itemID := "reasoning-" + turnID
		s.upsertItem(ts, itemID, "reasoning", turnID, "inProgress")
		s.sendNotification("item/reasoning/delta", ItemDeltaParams{
			ThreadID: threadID,
			TurnID:   turnID,
			ItemID:   itemID,
			Delta:    msg.Delta,
		})
		return

	case protocol.EvExecCommandBegin:
		itemID := "cmd-" + msg.CallID
		cmdJSON, _ := json.Marshal(msg.Command)
		cwdJSON, _ := json.Marshal(msg.Cwd)
		s.upsertItemStarted(ts, itemID, "commandExecution", turnID, cmdJSON, cwdJSON)
		s.sendNotification("item/started", ItemNotificationParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Item:     *ts.items[itemID],
		})

		// If approval is needed, send a server request.
		// 如果需要审批，发送服务器请求。
		if s.needsApproval(ts, msg.Command) {
			s.sendExecApprovalRequest(threadID, turnID, msg.CallID, msg.Command, msg.Cwd)
		}
		return

	case protocol.EvExecCommandOutputDelta:
		itemID := "cmd-" + msg.CallID
		s.sendNotification("item/commandExecution/outputDelta", ItemDeltaParams{
			ThreadID: threadID,
			TurnID:   turnID,
			ItemID:   itemID,
			Delta:    msg.OutputDelta,
		})
		return

	case protocol.EvExecCommandEnd:
		itemID := "cmd-" + msg.CallID
		exitCode := 0
		if msg.ExitCode != nil {
			exitCode = *msg.ExitCode
		}
		outputJSON, _ := json.Marshal(msg.Output)
		s.updateCommandItem(ts, itemID, turnID, exitCode, msg.DurationMs, outputJSON)
		s.sendNotification("item/completed", ItemNotificationParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Item:     *ts.items[itemID],
		})
		return

	case protocol.EvExecApprovalRequest:
		// Send approval request as a server-initiated JSON-RPC request.
		// 作为服务器发起的 JSON-RPC 请求发送审批请求。
		s.sendExecApprovalRequest(threadID, turnID, msg.CallID, msg.Command, msg.Cwd)
		return

	case protocol.EvPatchApplyBegin:
		itemID := "patch-" + msg.CallID
		s.upsertItemStarted(ts, itemID, "fileChange", turnID, nil, nil)
		s.sendNotification("item/started", ItemNotificationParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Item:     *ts.items[itemID],
		})
		return

	case protocol.EvPatchApplyUpdated:
		itemID := "patch-" + msg.CallID
		changesJSON, _ := json.Marshal(msg.Message)
		s.updatePatchItem(ts, itemID, turnID, changesJSON)
		s.sendNotification("item/completed", ItemNotificationParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Item:     *ts.items[itemID],
		})
		return

	case protocol.EvPatchApplyEnd:
		return

	case protocol.EvPatchApprovalRequest:
		s.sendPatchApprovalRequest(threadID, turnID, msg.CallID, msg.PatchContent)
		return

	case protocol.EvTokenCount:
		// Could emit token count notification if needed.
		// 如果需要，可以发出令牌计数通知。
		return

	case protocol.EvTurnComplete:
		// Mark the active turn as completed.
		// 将活动轮次标记为已完成。
		var completedTurn Turn
		s.mu.Lock()
		for i := range ts.turns {
			if ts.turns[i].ID == turnID {
				ts.turns[i].Status = "completed"
				completedTurn = ts.turns[i]
			}
		}
		ts.activeTurn = ""
		ts.updatedAt = time.Now().Unix()
		s.mu.Unlock()

		s.sendNotification("turn/completed", TurnCompletedParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Turn:     completedTurn,
		})
		s.sendNotification("thread/status/changed", ThreadStatusChangedParams{
			ThreadID: threadID,
			Status:   ThreadStatus{Type: "idle"},
		})
		return

	case protocol.EvTurnAborted:
		var abortedTurn Turn
		s.mu.Lock()
		for i := range ts.turns {
			if ts.turns[i].ID == turnID {
				ts.turns[i].Status = "interrupted"
				abortedTurn = ts.turns[i]
			}
		}
		ts.activeTurn = ""
		s.mu.Unlock()

		s.sendNotification("turn/completed", TurnCompletedParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Turn:     abortedTurn,
		})
		s.sendNotification("thread/status/changed", ThreadStatusChangedParams{
			ThreadID: threadID,
			Status:   ThreadStatus{Type: "idle"},
		})
		return

	case protocol.EvStreamError:
		errParams := ErrorNotificationParams{
			ThreadID: threadID,
			TurnID:   turnID,
		}
		errParams.Error.Message = msg.ErrorMsg

		s.sendNotification("error", errParams)

		// Also complete the turn.
		// 同时完成轮次。
		var failedTurn Turn
		s.mu.Lock()
		for i := range ts.turns {
			if ts.turns[i].ID == turnID {
				ts.turns[i].Status = "failed"
				failedTurn = ts.turns[i]
			}
		}
		ts.activeTurn = ""
		s.mu.Unlock()

		s.sendNotification("turn/completed", TurnCompletedParams{
			ThreadID: threadID,
			TurnID:   turnID,
			Turn:     failedTurn,
		})
		s.sendNotification("thread/status/changed", ThreadStatusChangedParams{
			ThreadID: threadID,
			Status:   ThreadStatus{Type: "idle"},
		})
		return

	case protocol.EvShutdownComplete:
		return

	default:
		s.logger.Debug("unhandled event in bridge", "type", msg.Type)
		return
	}
}

// upsertItem creates or updates an item in the thread state with "inProgress" status.
// upsertItem 在线程状态中创建或更新一个状态为 "inProgress" 的项目。
func (s *Server) upsertItem(ts *threadState, itemID, itemType, turnID, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ts.items == nil {
		ts.items = make(map[string]*TurnItem)
	}

	if _, ok := ts.items[itemID]; !ok {
		ts.items[itemID] = &TurnItem{
			ID:     itemID,
			Type:   itemType,
			Status: status,
			TurnID: turnID,
		}
	} else {
		ts.items[itemID].Status = status
		ts.items[itemID].TurnID = turnID
	}
}

// upsertItemStarted creates an item with initial data.
// upsertItemStarted 创建一个带有初始数据的项目。
func (s *Server) upsertItemStarted(ts *threadState, itemID, itemType, turnID string, cmdJSON, cwdJSON json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ts.items == nil {
		ts.items = make(map[string]*TurnItem)
	}

	ts.items[itemID] = &TurnItem{
		ID:      itemID,
		Type:    itemType,
		Status:  "inProgress",
		Command: cmdJSON,
		Cwd:     cwdJSON,
		TurnID:  turnID,
	}
}

// upsertItemComplete creates or updates an item with completed status and text.
// upsertItemComplete 创建或更新一个状态为已完成并带有文本的项目。
func (s *Server) upsertItemComplete(ts *threadState, itemID, itemType, turnID string, textJSON json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ts.items == nil {
		ts.items = make(map[string]*TurnItem)
	}

	item, ok := ts.items[itemID]
	if !ok {
		item = &TurnItem{
			ID:     itemID,
			Type:   itemType,
			TurnID: turnID,
		}
		ts.items[itemID] = item
	}

	item.Status = "completed"
	switch itemType {
	case "agentMessage":
		item.Text = textJSON
	case "reasoning":
		item.Summary = textJSON
	}
}

// updateCommandItem updates a command execution item with output and exit code.
// updateCommandItem 使用输出和退出码更新命令执行项目。
func (s *Server) updateCommandItem(ts *threadState, itemID, turnID string, exitCode int, durationMs *int64, outputJSON json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := ts.items[itemID]
	if !ok {
		item = &TurnItem{
			ID:     itemID,
			Type:   "commandExecution",
			TurnID: turnID,
		}
		ts.items[itemID] = item
	}

	item.Status = "completed"
	item.ExitCode = &exitCode
	item.Output = outputJSON
	item.AggregatedOutput = outputJSON
	if durationMs != nil {
		d := *durationMs
		item.DurationMs = &d
	}
}

// updatePatchItem updates a file change item with changes.
// updatePatchItem 使用变更内容更新文件变更项目。
func (s *Server) updatePatchItem(ts *threadState, itemID, turnID string, changesJSON json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := ts.items[itemID]
	if !ok {
		item = &TurnItem{
			ID:     itemID,
			Type:   "fileChange",
			TurnID: turnID,
		}
		ts.items[itemID] = item
	}

	item.Status = "completed"
	item.Changes = changesJSON
}

// needsApproval checks if a command requires approval based on the thread's policy.
// needsApproval 根据线程策略检查命令是否需要审批。
func (s *Server) needsApproval(ts *threadState, command []string) bool {
	// The agent session already handles approval logic internally.
	// 代理会话已在内部处理审批逻辑。
	// This is a fallback for the bridge layer.
	// 这是桥接层的回退方案。
	return false
}

// sendExecApprovalRequest sends a server-initiated JSON-RPC request for
// sendExecApprovalRequest 发送服务器发起的 JSON-RPC 请求，
// command execution approval.
// 用于命令执行审批。
func (s *Server) sendExecApprovalRequest(threadID, turnID, callID string, command []string, cwd string) {
	reqID := nextServerRequestID
	nextServerRequestID++

	params := ExecApprovalParams{
		ThreadID: threadID,
		TurnID:   turnID,
		CallID:   callID,
		Command:  command,
		Cwd:      cwd,
	}

	paramsJSON, _ := json.Marshal(params)

	s.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  "shellExecApproval",
		"params":  json.RawMessage(paramsJSON),
	})
}

// sendPatchApprovalRequest sends a server-initiated JSON-RPC request for
// sendPatchApprovalRequest 发送服务器发起的 JSON-RPC 请求，
// patch application approval.
// 用于补丁应用审批。
func (s *Server) sendPatchApprovalRequest(threadID, turnID, callID string, patchContent string) {
	reqID := nextServerRequestID
	nextServerRequestID++

	params := PatchApprovalParams{
		ThreadID:     threadID,
		TurnID:       turnID,
		CallID:       callID,
		PatchContent: patchContent,
	}

	paramsJSON, _ := json.Marshal(params)

	s.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      reqID,
		"method":  "applyPatchApproval",
		"params":  json.RawMessage(paramsJSON),
	})
}

// newItemID generates a unique item ID.
// newItemID 生成唯一的项目 ID。
func newItemID(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, uuid.NewString()[:8])
}

// jsonString safely marshals a string to JSON RawMessage.
// jsonString 安全地将字符串序列化为 JSON RawMessage。
func jsonString(s string) json.RawMessage {
	data, _ := json.Marshal(s)
	return json.RawMessage(data)
}

// jsonStringArray safely marshals a string slice to JSON RawMessage.
// jsonStringArray 安全地将字符串切片序列化为 JSON RawMessage。
func jsonStringArray(ss []string) json.RawMessage {
	data, _ := json.Marshal(ss)
	return json.RawMessage(data)
}

// ensureFields returns a non-nil empty slice if the input is nil, for
// ensureFields 如果输入为 nil 则返回非 nil 的空切片，
// JSON serialization consistency.
// 以保证 JSON 序列化的一致性。
func ensureFields[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// splitCommand splits a command string into argv for display purposes.
// splitCommand 将命令字符串拆分为 argv 用于显示目的。
func splitCommand(cmd string) []string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return []string{cmd}
	}
	return parts
}
