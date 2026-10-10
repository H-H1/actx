package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/H-H1/actx/internal/model"
	"github.com/H-H1/actx/internal/protocol"
)

// turnLogMaxRunes limits a single logged message to keep the file readable;
// longer content is truncated with a marker.
// turnLogMaxRunes 限制单条日志消息的最大长度，超长内容截断并附加标记。
const turnLogMaxRunes = 4000

// TurnLogger writes model input/output to a per-thread plain-text log file,
// TurnLogger 把模型的输入输出写入按线程划分的本地纯文本日志文件，
// one entry per item with a timestamp and role tag, preserving real line
// 每项一条记录，带时间戳与角色标签，保留真实换行：
// breaks. Each context item is logged exactly once when it is appended
// 每个上下文条目在追加时且仅在追加时记录一次（用户输入在提交时、
// (user input at submission, model output and tool results as they stream
// 模型输出与工具结果在流式到达时）；请求头仅记录 items/tools 计数，
// in); request headers log only the item/tool counts, never the history
// 不再重复历史（模型 API 无状态，每次请求都会重发完整对话历史）：
// (the model API is stateless, so every request re-sends the full history):
//
//	[10:23:45.123] ===== Turn xxx | submission 35682... | model=... =====
//	[10:23:45.456] [user]
//	[10:23:45.457] ----- Request #1 | items=2 | tools=3 -----
//	[10:23:45.458] [instructions]
//	(system prompt, logged once per thread)
//	[10:23:46.100] [assistant call shell]
//	[10:23:46.500] [tool call_xxx]
//	[10:23:47.200] [reasoning]
//	[10:23:48.100] ----- Request #2 | items=6 | tools=3 -----
//
// One log file per conversation thread (window/task), named by the thread
// ID: every submission of the thread appends to the same file, each with
// its own Turn banner carrying the submission ID, so the whole
// conversation reads as one chronological trace:
// 每段对话（窗口/任务）一个日志文件，以线程 ID 命名：线程内的每次
// 提交都追加到同一文件，各自带携带提交 ID 的 Turn 横幅，整段对话
// 读起来就是一条按时间排列的追踪记录：
//
//	.actx/logs/4f2a1c9e....log
//	[10:23:45.123] ===== Turn xxx | submission 35682... | model=... =====
//	...
//	[10:31:02.010] ===== Turn yyy | submission 35689... | model=... =====
//	...
//
// The file lives under .actx/logs/<threadID>.log (the nearest .actx
// 文件位于 .actx/logs/<threadID>.log（从当前目录向上查找最近的 .actx
// directory found walking up from cwd) and is never committed to git.
// 目录），且不会提交到 git。
// All methods are nil-safe: a nil *TurnLogger (e.g. when the log file
// 所有方法均为 nil 安全：当日志文件无法创建时 *TurnLogger 为 nil，
// cannot be created) silently drops the writes.
// 写入会被静默丢弃。
type TurnLogger struct {
	mu sync.Mutex
	// file is the append-mode log file; nil after Close.
	// file 是追加模式的日志文件；Close 后为 nil。
	file *os.File
	// reqSeq counts requests within the logger's lifetime.
	// reqSeq 统计日志器生命周期内的请求序号。
	reqSeq int
	// lastInstr remembers the instructions text already logged so the
	// (constant-per-thread) system prompt is written only once — and
	// re-written when it actually changes (e.g. after the tools mode
	// auto-switches from api to prompt, which appends the tool prompt).
	// lastInstr 记住已记录过的 instructions 文本，使（线程内恒定的）
	// 系统提示词只写一次；内容真正变化时（如工具模式从 api 自动切换
	// 到 prompt、追加了工具提示词）才重新记录。
	lastInstr string
}

// NewTurnLogger opens (or appends to) the per-thread plain-text log file
// named after the given thread ID. All submissions of the thread share
// this one file. Returns nil if the file cannot be opened; all methods
// handle nil safely.
// NewTurnLogger 打开（或追加）以给定线程 ID 命名的按线程划分的纯文本
// 日志文件。线程内的所有提交共享这一个文件。如果文件无法打开则返回
// nil；所有方法均安全处理 nil。
func NewTurnLogger(id protocol.ThreadId) *TurnLogger {
	dir := turnLogsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%s.log", id)),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return &TurnLogger{file: f}
}

// turnLogsDir finds the nearest .actx directory walking up from cwd and
// turnLogsDir 从当前目录向上查找最近的 .actx 目录，
// returns its logs subdirectory; falls back to ./.actx/logs.
// 返回其 logs 子目录；找不到则回退到 ./.actx/logs。
func turnLogsDir() string {
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidate := filepath.Join(dir, ".actx")
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				return filepath.Join(candidate, "logs")
			}
			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
	}
	return filepath.Join(".actx", "logs")
}

// Close flushes and closes the log file. Safe to call on nil or twice.
// Close 刷新并关闭日志文件。nil 或重复调用均安全。
func (l *TurnLogger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// logTimestamp returns the wall-clock timestamp prefix used on every log
// entry so the file reads like a chronological trace.
// logTimestamp 返回每条日志记录使用的时间戳前缀，使日志文件读起来
// 像按时间排序的追踪记录。
func logTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

// writeSection appends a "[ts] [role]" header line followed by the
// multi-line content with real line breaks preserved.
// writeSection 追加 "[时间戳] [角色]" 头行，随后是保留真实换行的多行内容。
func (l *TurnLogger) writeSection(role, content string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	if r := []rune(content); len(r) > turnLogMaxRunes {
		content = string(r[:turnLogMaxRunes]) + fmt.Sprintf("\n…(truncated +%d chars)", len(r)-turnLogMaxRunes)
	}
	fmt.Fprintf(l.file, "[%s] [%s]\n%s\n\n", logTimestamp(), role, content)
}

// LogError records a model/stream error into the log so failures leave a
// trace right where they happened (after the request header line), instead
// of vanishing between "Request #N" and EOF.
// LogError 把模型/流错误记入日志，使失败在发生处（请求头行之后）留下
// 痕迹，而不是消失在 "Request #N" 与 EOF 之间无迹可寻。
func (l *TurnLogger) LogError(errMsg string) {
	l.writeSection("error", errMsg)
}

// LogSSE appends a raw SSE event-trail fragment as-is (no timestamp, no
// role prefix): the model layer aggregates the trail itself — one line per
// event type prefixed with [type], consecutive same-type deltas appended
// inline. Fragments may or may not end with a newline; the caller controls
// line breaks.
// LogSSE 原样追加原始 SSE 事件轨迹片段（无时间戳、无角色前缀）：
// 轨迹的聚合由模型层完成——每种事件类型一行、以 [type] 开头，
// 同 type 的连续增量行内追加。片段末尾是否换行由调用方控制。
func (l *TurnLogger) LogSSE(fragment string) {
	if l == nil || fragment == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	_, _ = l.file.WriteString(fragment)
}

// LogTurnStart writes a separator block marking the beginning of a turn.
// The banner carries the submission ID alongside the turn ID: the log file
// is shared by the whole thread, so the submission ID is what links a turn
// back to the API response of POST /message.
// LogTurnStart 写入标记代理轮次开始的分隔块。横幅在轮次 ID 之外携带
// 提交 ID：日志文件由整个线程共享，提交 ID 是把轮次关联回
// POST /message 接口响应的纽带。
func (l *TurnLogger) LogTurnStart(turnID protocol.TurnId, subID protocol.SubmissionId, mdl string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "%s\n[%s] ===== Turn %s | submission %s | model=%s =====\n%s\n\n",
		turnLogSep, logTimestamp(), turnID, subID, mdl, turnLogSep)
}

// LogRequest logs one model request as a header line plus, on first sight,
// the instructions (system prompt): the item count shows how the context
// grows across the tool-call loop. Because the model API is stateless,
// every request re-sends the full conversation history; items themselves
// are logged exactly once when they are appended to the context (user
// input at submission, model output and tool results as they stream in),
// so re-logging the input here would duplicate every entry in every
// request section. The instructions are likewise constant per thread and
// logged only when their content changes (e.g. after a tools-mode switch).
// LogRequest 以头行记录一次模型请求，并在首次出现时记录 instructions
// （系统提示词）：条目数显示上下文在工具调用循环中如何增长。因为模型
// API 无状态，每次请求都会重发完整对话历史；条目本身在其追加进上下文
// 时记录且仅记录一次（用户输入在提交时、模型输出与工具结果在流式到达
// 时），此处再记输入会让每个请求章节全部重复。instructions 在线程内
// 同样恒定，仅在内容变化时（如工具模式切换后）才重新记录。
func (l *TurnLogger) LogRequest(p model.Prompt) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.reqSeq++
	seq := l.reqSeq
	instr, instrChanged := p.Instructions, p.Instructions != l.lastInstr
	if instr != "" {
		l.lastInstr = instr
	}
	l.mu.Unlock()
	l.writef("------------------------------------------------------------\n[%s] Request #%d | items=%d | tools=%d\n\n",
		logTimestamp(), seq, len(p.Input), len(p.Tools))
	if instrChanged && instr != "" {
		l.writeSection("instructions", instr)
	}
}

// LogRequestBody logs the exact JSON body sent to the model API, pretty
// printed for readability. It is wired via ModelClient.SetRequestTrace and
// fires on the session goroutine right after LogRequest writes the header,
// so the body always lands between its "Request #N" header and the first
// SSE trail line of that request. Unlike writeSection, no truncation is
// applied: the point is to see precisely what was sent.
// LogRequestBody 记录发送给模型 API 的确切 JSON 请求体，并格式化输出便于
// 阅读。它经由 ModelClient.SetRequestTrace 接入，在会话 goroutine 上、
// LogRequest 写完头行之后立即触发，因此请求体总是位于其 "Request #N"
// 头行与该请求的首条 SSE 轨迹行之间。与 writeSection 不同，此处不做
// 截断：目的正是查看实际发送的完整内容。
func (l *TurnLogger) LogRequestBody(raw []byte) {
	if l == nil {
		return
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(raw)
	}
	l.writeSection("request body", pretty.String())
}

// turnLogSep is the heavy separator line between turns (60 chars).
// turnLogSep 是轮次之间的重分隔线（60 字符）。
const turnLogSep = "============================================================"

// writef appends a raw formatted line (separators, headers).
// writef 追加原始格式化行（分隔线、头行）。
func (l *TurnLogger) writef(format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, format, args...)
}

// LogItem logs a single conversation item as a role-tagged entry:
// LogItem 按角色标签记录单个对话项：
//   - message        -> its role ("user" / "assistant" / "sys")
//   - message        -> 其角色（"user" / "assistant" / "sys"）
//   - function call  -> assistant call 工具名
//   - call output    -> tool callID
//   - 调用输出       -> tool 调用ID
//   - reasoning      -> reasoning
//   - 推理           -> reasoning
func (l *TurnLogger) LogItem(item protocol.ResponseItem) {
	if l == nil {
		return
	}
	switch {
	case item.IsMessage():
		// Skip system messages: static boilerplate (environment description,
		// operating principles) repeated in every request.
		// 跳过 system 消息：每次请求都重复的固定模板
		// （环境描述、操作原则等）。
		if item.Role == "system" || item.Role == "sys" || item.Role == "developer" {
			return
		}
		role := item.Role
		if role == "" {
			role = "unknown"
		}
		if text := item.GetText(); text != "" {
			l.writeSection(role, text)
		}
	case item.IsFunctionCall():
		l.writeSection("assistant call "+item.Name, item.Arguments)
	case item.Type == protocol.RITypeFunctionCallOutput || item.Type == protocol.RITypeCustomToolCallOutput:
		l.writeSection("tool "+string(item.CallID), item.Output)
	case item.Type == protocol.RITypeReasoning:
		if item.Summary != "" {
			l.writeSection("reasoning", item.Summary)
		}
	}
}
