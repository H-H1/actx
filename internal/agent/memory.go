package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// MemoryEntry represents a single memory item stored across sessions.
// MemoryEntry 表示跨会话存储的单个记忆项。
type MemoryEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Session string `json:"session,omitempty"`
}

// MemoryStore manages persistent memory across sessions.
// MemoryStore 管理跨会话的持久记忆。
type MemoryStore struct {
	// mu guards concurrent access to the store.
	// mu 保护存储的并发访问。
	mu sync.RWMutex
	// filePath is the path of the JSON file backing the store.
	// filePath 是支持该存储的 JSON 文件路径。
	filePath string
	// entries maps keys to memory entries.
	// entries 将键映射到记忆项。
	entries map[string]MemoryEntry
}

// NewMemoryStore creates a MemoryStore backed by a JSON file.
// NewMemoryStore 创建一个由 JSON 文件支持的 MemoryStore。
func NewMemoryStore(ACTXHome string) *MemoryStore {
	return &MemoryStore{
		filePath: filepath.Join(ACTXHome, "memory.json"),
		entries:  make(map[string]MemoryEntry),
	}
}

// Load reads the memory file from disk.
// Load 从磁盘读取记忆文件。
func (m *MemoryStore) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &m.entries)
}

// Save writes the memory file to disk.
// Save 将记忆文件写入磁盘。
func (m *MemoryStore) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0o600)
}

// Set stores a memory entry.
// Set 存储一条记忆项。
func (m *MemoryStore) Set(key, value, session string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = MemoryEntry{Key: key, Value: value, Session: session}
}

// Get retrieves a memory entry by key.
// Get 通过键检索一条记忆项。
func (m *MemoryStore) Get(key string) (MemoryEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.entries[key]
	return entry, ok
}

// All returns all memory entries.
// All 返回所有记忆项。
func (m *MemoryStore) All() []MemoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MemoryEntry, 0, len(m.entries))
	for _, e := range m.entries {
		result = append(result, e)
	}
	return result
}

// Delete removes a memory entry by key.
// Delete 通过键删除一条记忆项。
func (m *MemoryStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
}
