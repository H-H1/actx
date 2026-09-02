package agent

import (
	"sync"

	"github.com/H-H1/actx/internal/protocol"
)

// ContextManager holds the conversation history that is sent to the model.
// ContextManager 保存发送给模型的对话历史。
// It maintains an in-memory list of ResponseItems and tracks a version
// 它在内存中维护 ResponseItem 列表，并跟踪版本号以支持压缩和回滚。
// number for compaction and rollback support.
type ContextManager struct {
	// mu guards concurrent access to the conversation history.
	// mu 保护对话历史的并发访问。
	mu sync.RWMutex
	// items is the in-memory list of conversation items sent to the model.
	// items 是发送给模型的内存对话项列表。
	items []protocol.ResponseItem
	// version is the current version number, incremented on compaction or rollback.
	// version 是当前版本号，在压缩或回滚时递增。
	version int
}

// NewContextManager creates a ContextManager initialized with the system
// NewContextManager 创建一个 ContextManager，并将系统指令作为第一项进行初始化。
// instructions as the first item.
func NewContextManager(systemInstructions string) *ContextManager {
	cm := &ContextManager{
		items: make([]protocol.ResponseItem, 0, 32),
	}
	if systemInstructions != "" {
		cm.items = append(cm.items, protocol.NewSystemMessage(systemInstructions))
	}
	return cm
}

// Append adds a ResponseItem to the conversation history.
// Append 向对话历史中添加一个 ResponseItem。
func (cm *ContextManager) Append(item protocol.ResponseItem) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.items = append(cm.items, item)
}

// Items returns a copy of the current conversation history.
// Items 返回当前对话历史的副本。
func (cm *ContextManager) Items() []protocol.ResponseItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]protocol.ResponseItem, len(cm.items))
	copy(result, cm.items)
	return result
}

// Version returns the current version number. The version increments
// Version 返回当前版本号。版本号在压缩或回滚时递增。
// on compaction or rollback.
func (cm *ContextManager) Version() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.version
}

// PendingUserMessages returns all user message items that have not yet
// PendingUserMessages 返回所有尚未在轮次中处理的用户消息项。
// been processed in a turn. In the current implementation, this returns
// 在当前实现中，这返回历史末尾的用户消息。
// user messages from the end of the history.
func (cm *ContextManager) PendingUserMessages() []protocol.ResponseItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var result []protocol.ResponseItem
	for _, item := range cm.items {
		if item.IsMessage() && item.Role == "user" {
			result = append(result, item)
		}
	}
	return result
}

// Compact replaces the conversation history with a compacted version,
// Compact 将对话历史替换为压缩版本，同时递增版本号。
// incrementing the version number. The compacted history typically consists
// 压缩后的历史通常由一条摘要消息和最近的上下文组成。
// of a summary message followed by recent context.
func (cm *ContextManager) Compact(summary string, keepRecent int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var newItems []protocol.ResponseItem
	// Keep the system message if present.
	// 如果存在则保留系统消息。
	if len(cm.items) > 0 && cm.items[0].Role == "system" {
		newItems = append(newItems, cm.items[0])
	}
	// Add the summary as a system message.
	// 将摘要添加为系统消息。
	if summary != "" {
		newItems = append(newItems, protocol.NewSystemMessage(
			"Previous conversation summary:\n"+summary,
		))
	}
	// Keep the most recent items.
	// 保留最近的项目。
	start := len(cm.items) - keepRecent
	if start < 1 {
		start = 1 // Always keep the system message
	}
	if start < len(cm.items) {
		newItems = append(newItems, cm.items[start:]...)
	}
	cm.items = newItems
	cm.version++
}

// Rollback restores the conversation history to a previous version by
// Rollback 通过移除给定索引之后添加的项，将对话历史恢复到之前的版本。
// removing items added after the given index.
func (cm *ContextManager) Rollback(toIndex int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if toIndex < 0 {
		toIndex = 0
	}
	if toIndex > len(cm.items) {
		toIndex = len(cm.items)
	}
	cm.items = cm.items[:toIndex]
	cm.version++
}

// Len returns the number of items in the conversation history.
// Len 返回对话历史中的项数。
func (cm *ContextManager) Len() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.items)
}

// Clear resets the conversation history, keeping only the system message.
// Clear 重置对话历史，仅保留系统消息。
func (cm *ContextManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if len(cm.items) > 0 && cm.items[0].Role == "system" {
		cm.items = cm.items[:1]
	} else {
		cm.items = cm.items[:0]
	}
	cm.version++
}
