package model

import (
	"bufio"
	"io"
	"strings"
)

const (
	// sseDataPrefix marks a data line in the text/event-stream format. The
	// field value is the remainder of the line after the prefix and a single
	// optional leading space (per the SSE specification).
	// sseDataPrefix 标记 text/event-stream 格式中的数据行。字段值是该行去除前缀
	// 和一个可选的前导空格后的剩余部分（根据 SSE 规范）。
	sseDataPrefix = "data:"
	// sseEventPrefix marks an event-type line.
	// sseEventPrefix 标记一个事件类型行。
	sseEventPrefix = "event:"
	// sseDoneSentinel is the data value OpenAI sends to signal the end of a
	// stream.
	// sseDoneSentinel 是 OpenAI 发送用于表示流结束的数据值。
	sseDoneSentinel = "[DONE]"
)

// SSEScanner reads Server-Sent Events from an io.Reader. It parses the
// text/event-stream format used by the OpenAI Responses API: "data:" and
// "event:" lines are accumulated per event, empty lines delimit events, and the
// "[DONE]" sentinel terminates the stream.
//
// Unlike bufio.Scanner, SSEScanner does not impose a maximum line length, so
// large JSON payloads emitted on a single data line are handled correctly. The
// scanner is intended for single-goroutine use, which matches the typical
// streaming-consumer pattern.
// SSEScanner 从 io.Reader 读取 Server-Sent Events。它解析 OpenAI Responses API
// 使用的 text/event-stream 格式："data:" 和 "event:" 行按事件累加，空行分隔事件，
// "[DONE]" 哨兵终止流。
//
// 与 bufio.Scanner 不同，SSEScanner 不限制最大行长度，因此能正确处理在单个 data
// 行上发出的大型 JSON 有效负载。扫描器设计为单 goroutine 使用，这符合典型的
// 流消费者模式。
type SSEScanner struct {
	// reader is the underlying buffered reader for the stream.
	// reader 是流的底层缓冲读取器。
	reader *bufio.Reader
	// event is the event type of the event being accumulated.
	// event 是正在累加的事件的事件类型。
	event string
	// data accumulates the data lines of the current event.
	// data 累加当前事件的数据行。
	data strings.Builder
	// hasData indicates whether any data line has been seen for the current event.
	// hasData 指示当前事件是否已收到任何数据行。
	hasData bool
	// err is the first non-EOF read error encountered.
	// err 是遇到的第一个非 EOF 读取错误。
	err error
	// done indicates whether the scanner has terminated.
	// done 指示扫描器是否已终止。
	done bool
}

// NewSSEScanner returns an SSEScanner that reads events from r.
// NewSSEScanner 返回一个从 r 读取事件的 SSEScanner。
func NewSSEScanner(r io.Reader) *SSEScanner {
	return &SSEScanner{reader: bufio.NewReader(r)}
}

// Next advances to the next event. It returns false when the stream is
// exhausted, the "[DONE]" sentinel is encountered, or a read error occurs.
// After Next returns false, call Err to check for a non-EOF error.
// Next 前进到下一个事件。当流耗尽、遇到 "[DONE]" 哨兵或发生读取错误时返回 false。
// Next 返回 false 后，调用 Err 检查非 EOF 错误。
func (s *SSEScanner) Next() bool {
	if s.done {
		return false
	}

	s.event = ""
	s.data.Reset()
	s.hasData = false

	for {
		line, err := s.reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")

		if line != "" {
			s.handleLine(line)
		} else if err == nil {
			// Empty line: event delimiter. Emit the accumulated event, if any.
			// 空行：事件分隔符。如果有累加的事件，则发出。
			if s.hasData {
				if s.data.String() == sseDoneSentinel {
					s.done = true
					return false
				}
				return true
			}
			// Otherwise keep reading (skip blank lines between events).
			// 否则继续读取（跳过事件之间的空行）。
		}

		if err != nil {
			s.done = true
			if err == io.EOF {
				s.err = nil
				// Flush a trailing event that was not followed by a delimiter.
				// 刷新末尾没有跟随分隔符的事件。
				if s.hasData && s.data.String() != sseDoneSentinel {
					return true
				}
				return false
			}
			s.err = err
			return false
		}
	}
}

// handleLine processes a single non-empty SSE line, updating the scanner's
// accumulated event/data state.
// handleLine 处理单个非空 SSE 行，更新扫描器的累加事件/数据状态。
func (s *SSEScanner) handleLine(line string) {
	switch {
	case strings.HasPrefix(line, sseDataPrefix):
		value := strings.TrimPrefix(line, sseDataPrefix)
		// A single leading space is part of the separator, not the value.
		// 前导空格是分隔符的一部分，不是值的一部分。
		value = strings.TrimPrefix(value, " ")
		if s.hasData {
			// Multiple data lines are joined with '\n' per the SSE spec.
			// 根据 SSE 规范，多个数据行用 '\n' 连接。
			s.data.WriteByte('\n')
		}
		s.data.WriteString(value)
		s.hasData = true
	case strings.HasPrefix(line, sseEventPrefix):
		value := strings.TrimPrefix(line, sseEventPrefix)
		value = strings.TrimPrefix(value, " ")
		s.event = value
	case strings.HasPrefix(line, ":"):
		// Comment / heartbeat line; ignored per spec.
		// 注释/心跳行；按规范忽略。
	default:
		// Unknown field; ignored per spec (browsers ignore unknown fields).
		// 未知字段；按规范忽略（浏览器忽略未知字段）。
	}
}

// Data returns the accumulated data payload of the current event. Multiple
// data lines are joined with '\n'. The result is valid only after a successful
// call to Next.
// Data 返回当前事件的累加数据有效负载。多个数据行用 '\n' 连接。结果仅在一次成功的
// Next 调用后有效。
func (s *SSEScanner) Data() string {
	return s.data.String()
}

// Event returns the event type of the current event, or "" if the server did
// not set one. The result is valid only after a successful call to Next.
// Event 返回当前事件的事件类型，如果服务器未设置则返回 ""。结果仅在一次成功的
// Next 调用后有效。
func (s *SSEScanner) Event() string {
	return s.event
}

// Err returns the first non-EOF read error encountered by the scanner, or nil.
// A nil return after Next returns false indicates a clean end of stream
// (including the "[DONE]" sentinel).
// Err 返回扫描器遇到的第一个非 EOF 读取错误，或 nil。Next 返回 false 后返回 nil
// 表示流干净结束（包括 "[DONE]" 哨兵）。
func (s *SSEScanner) Err() error {
	return s.err
}
