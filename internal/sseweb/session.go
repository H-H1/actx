// Package sseweb 将 ACTX 代理暴露为 HTTP 服务器，
// 通过 Server-Sent Events (SSE) 实时推送代理事件。
//
// 每个 HTTP 会话包装一个 agent.ACTX 实例：
//   - REST 端点提交操作（用户输入、中断、审批、压缩）
//   - SSE 端点消费事件流，支持断线重连与历史重放（Last-Event-ID）
package sseweb

import (
	"log/slog"
	"sync"
	"time"

	"github.com/H-H1/actx/internal/agent"
	"github.com/H-H1/actx/internal/config"
	"github.com/H-H1/actx/internal/protocol"
)

// ringCapacity 是每个会话保留的近期事件数量，
// 供重连的 SSE 客户端重放（超出后丢弃最旧事件）。
const ringCapacity = 1024

// subscriberBuffer 是单个 SSE 订阅者通道的容量；
// 消费过慢时事件被丢弃，客户端可通过 Last-Event-ID 重放补齐。
const subscriberBuffer = 256

// sseEntry 是带递增序号的代理事件。
// 序号用于 SSE 的 id 字段，客户端重连时通过 Last-Event-ID 补齐缺失事件。
type sseEntry struct {
	Seq   uint64         `json:"seq"`
	Event protocol.Event `json:"event"`
}

// subscriber 表示一条已附加的 SSE 流。
type subscriber struct {
	id int64
	ch chan sseEntry
}

// session 包装一个 agent.ACTX 实例：
// 后台 drain goroutine 是事件通道的唯一消费者，
// 将事件广播给所有 SSE 订阅者并写入环形历史。
type session struct {
	id        string
	threadID  string
	createdAt time.Time
	cfg       *config.Config
	agent     *agent.ACTX
	logger    *slog.Logger

	mu      sync.Mutex
	seq     uint64                // 已发布事件的最大序号
	ring    []sseEntry            // 近期事件环形历史
	subs    map[int64]*subscriber // 活跃 SSE 订阅者
	nextSub int64                 // 订阅者 ID 分配器
	closed  bool                  // 代理事件通道是否已关闭
}

// newSession 创建会话包装并启动 drain goroutine。
// newSession 创建会话包装并启动后台事件广播循环。
func newSession(cfg *config.Config, ag *agent.ACTX, logger *slog.Logger) *session {
	s := &session{
		id:        ag.SessionID(),
		threadID:  ag.ThreadID(),
		createdAt: time.Now(),
		cfg:       cfg,
		agent:     ag,
		logger:    logger,
		subs:      make(map[int64]*subscriber),
	}
	go s.drain()
	return s
}

// drain 消费代理事件通道：这是 rxEvent 的唯一读取方。
// drain 是代理事件通道的唯一消费者，把每个事件广播给订阅者并写入历史。
// 通道关闭（代理关闭）后标记会话为 closed 并关闭所有订阅者通道。
func (s *session) drain() {
	for ev := range s.agent.Events() {
		s.publish(ev)
	}
	s.finish()
}

// publish 为事件分配序号，写入环形历史并广播给订阅者。
// 订阅者通道满时丢弃该事件（慢消费者可通过 Last-Event-ID 重连补齐）。
func (s *session) publish(ev protocol.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.seq++
	entry := sseEntry{Seq: s.seq, Event: ev}
	s.ring = append(s.ring, entry)
	if len(s.ring) > ringCapacity {
		s.ring = s.ring[len(s.ring)-ringCapacity:]
	}
	for _, sub := range s.subs {
		select {
		case sub.ch <- entry:
		default:
			// 慢消费者：丢弃，客户端可凭序号检测缺口并重连重放。
		}
	}
}

// subscribe 附加一个新的 SSE 订阅者。
// 返回订阅者、订阅时刻的最新序号、会话是否已关闭。
// 调用方应先订阅再重放历史（since < seq <= upTo），
// 以保证订阅与重放之间无事件丢失。
func (s *session) subscribe() (sub *subscriber, upTo uint64, closed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, s.seq, true
	}
	sub = &subscriber{
		id: s.nextSub,
		ch: make(chan sseEntry, subscriberBuffer),
	}
	s.nextSub++
	s.subs[sub.id] = sub
	return sub, s.seq, false
}

// unsubscribe 移除一个 SSE 订阅者（客户端断开时调用）。
func (s *session) unsubscribe(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, sub.id)
}

// history 返回序号在 (since, upTo] 区间内的历史事件，用于重连重放。
func (s *session) history(since, upTo uint64) []sseEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sseEntry
	for _, e := range s.ring {
		if e.Seq > since && e.Seq <= upTo {
			out = append(out, e)
		}
	}
	return out
}

// finish 在代理事件通道关闭后调用：
// 标记会话关闭并关闭所有订阅者通道，让 SSE 流以 session_closed 结束。
func (s *session) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for id, sub := range s.subs {
		close(sub.ch)
		delete(s.subs, id)
	}
}

// isClosed 返回会话是否已关闭。
func (s *session) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// shutdown 优雅关闭底层代理。非阻塞：关闭在后台完成，
// drain 循环结束后会话被标记为 closed。
func (s *session) shutdown() {
	go func() {
		s.agent.Shutdown()
	}()
}
