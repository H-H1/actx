// Package rollout implements session persistence for ACTX CLI.
// rollout 包实现 ACTX CLI 的会话持久化。
// It records conversation history and session metadata to disk so that
// 它将对话历史和会话元数据记录到磁盘，以便
// sessions can be resumed, forked, or reviewed later.
// 会话可以稍后恢复、派生或审查。
package rollout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/H-H1/actx/internal/protocol"
)

// SessionMeta holds metadata about a saved session.
// SessionMeta 保存有关已保存会话的元数据。
type SessionMeta struct {
	// SessionID is the unique identifier of the session.
	// SessionID 是会话的唯一标识符。
	SessionID protocol.SessionId `json:"session_id"`
	// ThreadID is the identifier of the thread within the session.
	// ThreadID 是会话内线程的标识符。
	ThreadID protocol.ThreadId `json:"thread_id"`
	// Model is the model name used by the session.
	// Model 是会话使用的模型名称。
	Model string `json:"model"`
	// Provider is the model provider name.
	// Provider 是模型服务商名称。
	Provider string `json:"provider"`
	// Cwd is the working directory of the session.
	// Cwd 是会话的工作目录。
	Cwd string `json:"cwd"`
	// CreatedAt is the session creation time.
	// CreatedAt 是会话创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the last update time of the session.
	// UpdatedAt 是会话的最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// RolloutEntry represents a single item in the rollout history.
// RolloutEntry 表示灰度发布历史中的单个条目。
type RolloutEntry struct {
	// Timestamp records when the item was recorded.
	// Timestamp 记录条目被记录的时间。
	Timestamp time.Time `json:"timestamp"`
	// Item is the recorded conversation item.
	// Item 是被记录的对话项。
	Item protocol.ResponseItem `json:"item"`
}

// Rollout represents a complete session rollout.
// Rollout 表示一个完整的会话灰度发布。
type Rollout struct {
	// Meta is the metadata of the session.
	// Meta 是会话的元数据。
	Meta SessionMeta `json:"meta"`
	// Entries is the recorded history of the session.
	// Entries 是会话的已记录历史。
	Entries []RolloutEntry `json:"entries"`
}

// Recorder writes session events to disk for persistence.
// Recorder 将会话事件写入磁盘以实现持久化。
type Recorder struct {
	// dir is the directory where rollouts are stored.
	// dir 是存储会话记录的目录。
	dir string
	// meta is the metadata of the session being recorded.
	// meta 是正在记录的会话的元数据。
	meta SessionMeta
	// entries is the accumulated history entries.
	// entries 是已累积的历史条目。
	entries []RolloutEntry
}

// NewRecorder creates a Recorder for the given session directory.
// NewRecorder 为给定的会话目录创建一个 Recorder。
func NewRecorder(rolloutDir string, meta SessionMeta) *Recorder {
	return &Recorder{
		dir:  rolloutDir,
		meta: meta,
	}
}

// Record appends a ResponseItem to the rollout.
// Record 向灰度发布追加一个 ResponseItem。
func (r *Recorder) Record(item protocol.ResponseItem) {
	r.entries = append(r.entries, RolloutEntry{
		Timestamp: time.Now(),
		Item:      item,
	})
}

// Save writes the rollout to disk.
// Save 将灰度发布写入磁盘。
func (r *Recorder) Save() error {
	sessionDir := filepath.Join(r.dir, r.meta.SessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	rollout := Rollout{
		Meta:    r.meta,
		Entries: r.entries,
	}

	data, err := json.MarshalIndent(rollout, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rollout: %w", err)
	}

	path := filepath.Join(sessionDir, "rollout.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write rollout: %w", err)
	}

	return nil
}

// Load reads a rollout from disk.
// Load 从磁盘读取灰度发布。
func Load(rolloutDir, sessionID string) (*Rollout, error) {
	path := filepath.Join(rolloutDir, sessionID, "rollout.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rollout: %w", err)
	}

	var rollout Rollout
	if err := json.Unmarshal(data, &rollout); err != nil {
		return nil, fmt.Errorf("parse rollout: %w", err)
	}
	return &rollout, nil
}

// ListSessions returns metadata for all saved sessions, sorted by
// UpdatedAt in descending order.
// ListSessions 返回所有已保存会话的元数据，按 UpdatedAt 降序排列。
func ListSessions(rolloutDir string) ([]SessionMeta, error) {
	entries, err := os.ReadDir(rolloutDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read rollout dir: %w", err)
	}

	var sessions []SessionMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(rolloutDir, entry.Name(), "rollout.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rollout Rollout
		if err := json.Unmarshal(data, &rollout); err != nil {
			continue
		}
		sessions = append(sessions, rollout.Meta)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// DeleteSession removes a saved session from disk.
// DeleteSession 从磁盘删除已保存的会话。
func DeleteSession(rolloutDir, sessionID string) error {
	path := filepath.Join(rolloutDir, sessionID)
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// LoadSession reads a saved session by its ID and returns the conversation items.
// 加载会话 - 通过 ID 读取已保存的会话并返回对话项。
func LoadSession(rolloutDir, sessionID string) ([]protocol.ResponseItem, error) {
	path := filepath.Join(rolloutDir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session file: %w", err)
	}
	var items []protocol.ResponseItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return items, nil
}

// SaveSession writes conversation items to a session file.
// 保存会话 - 将对话项写入会话文件。
func SaveSession(rolloutDir, sessionID string, items []protocol.ResponseItem) error {
	if err := os.MkdirAll(rolloutDir, 0o700); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}
	path := filepath.Join(rolloutDir, sessionID+".json")
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	return nil
}

// FormatSessionList returns a human-readable list of sessions.
// FormatSessionList 返回人类可读的会话列表。
func FormatSessionList(sessions []SessionMeta) string {
	if len(sessions) == 0 {
		return "No sessions found."
	}
	var sb strings.Builder
	for i, s := range sessions {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, s.SessionID))
		sb.WriteString(fmt.Sprintf("   Model: %s\n", s.Model))
		sb.WriteString(fmt.Sprintf("   Updated: %s\n", s.UpdatedAt.Format(time.RFC3339)))
		if s.Cwd != "" {
			sb.WriteString(fmt.Sprintf("   Cwd: %s\n", s.Cwd))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
