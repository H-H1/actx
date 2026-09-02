package applypatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ApplyResult records the outcome of applying a patch.
// ApplyResult 记录应用补丁的结果。
type ApplyResult struct {
	// FilesAdded lists paths of files created by the patch.
	// FilesAdded 列出补丁创建的文件路径。
	FilesAdded []string
	// FilesUpdated lists paths of files modified by the patch.
	// FilesUpdated 列出补丁修改的文件路径。
	FilesUpdated []string
	// FilesDeleted lists paths of files deleted by the patch.
	// FilesDeleted 列出补丁删除的文件路径。
	FilesDeleted []string
	// Errors collects error messages from failed operations.
	// Errors 收集失败操作的错误消息。
	Errors []string
}

// HasErrors returns true if any errors occurred during application.
// HasErrors 如果在应用过程中发生任何错误则返回 true。
func (r *ApplyResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Summary returns a human-readable summary of the result.
// Summary 返回人类可读的结果摘要。
func (r *ApplyResult) Summary() string {
	parts := []string{}
	if len(r.FilesAdded) > 0 {
		parts = append(parts, fmt.Sprintf("%d file(s) added", len(r.FilesAdded)))
	}
	if len(r.FilesUpdated) > 0 {
		parts = append(parts, fmt.Sprintf("%d file(s) updated", len(r.FilesUpdated)))
	}
	if len(r.FilesDeleted) > 0 {
		parts = append(parts, fmt.Sprintf("%d file(s) deleted", len(r.FilesDeleted)))
	}
	if len(r.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", len(r.Errors)))
	}
	return strings.Join(parts, ", ")
}

// Apply applies the patch to files relative to baseDir.
// Apply 将补丁应用到相对于 baseDir 的文件。
func Apply(patch *Patch, baseDir string) (*ApplyResult, error) {
	result := &ApplyResult{}

	for _, change := range patch.Changes {
		fullPath := filepath.Join(baseDir, change.Path)

		switch change.Op {
		case FileOpAdd:
			if err := applyAdd(fullPath, change); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("add %s: %v", change.Path, err))
			} else {
				result.FilesAdded = append(result.FilesAdded, change.Path)
			}

		case FileOpUpdate:
			if err := applyUpdate(fullPath, change); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("update %s: %v", change.Path, err))
			} else {
				result.FilesUpdated = append(result.FilesUpdated, change.Path)
			}

		case FileOpDelete:
			if err := os.Remove(fullPath); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", change.Path, err))
			} else {
				result.FilesDeleted = append(result.FilesDeleted, change.Path)
			}
		}
	}

	return result, nil
}

// applyAdd creates a new file with the given content.
// applyAdd 使用给定内容创建新文件。
func applyAdd(fullPath string, change FileChange) error {
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	content := change.Content
	if strings.HasSuffix(content, "\n") {
		content = strings.TrimSuffix(content, "\n")
	}
	return os.WriteFile(fullPath, []byte(content), 0o644)
}

// applyUpdate modifies an existing file by applying hunks.
// applyUpdate 通过应用块来修改现有文件。
func applyUpdate(fullPath string, change FileChange) error {
	existing, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(existing), "\n")

	for _, hunk := range change.Hunks {
		lines, err = applyHunk(lines, hunk)
		if err != nil {
			return fmt.Errorf("apply hunk: %w", err)
		}
	}

	newContent := strings.Join(lines, "\n")
	return os.WriteFile(fullPath, []byte(newContent), 0o644)
}

// applyHunk applies a single hunk to a slice of lines. It searches for the
// context + deletion lines as a contiguous block and replaces them with
// context + addition lines.
// applyHunk 将单个块应用到行切片中。它搜索
// 上下文 + 删除行作为一个连续块，并用上下文 + 添加行替换它们。
func applyHunk(lines []string, hunk Hunk) ([]string, error) {
	// Build the search pattern: context lines followed by deletion lines.
	// 构建搜索模式：上下文行后跟删除行。
	searchPattern := make([]string, 0, len(hunk.Context)+len(hunk.Deletions))
	searchPattern = append(searchPattern, hunk.Context...)
	searchPattern = append(searchPattern, hunk.Deletions...)

	// Build the replacement: context lines followed by addition lines.
	// 构建替换内容：上下文行后跟添加行。
	replacement := make([]string, 0, len(hunk.Context)+len(hunk.Additions))
	replacement = append(replacement, hunk.Context...)
	replacement = append(replacement, hunk.Additions...)

	if len(searchPattern) == 0 {
		// No context or deletions: append additions at the end.
		// 没有上下文或删除：在末尾追加添加内容。
		return append(lines, replacement...), nil
	}

	// Search for the pattern in lines.
	// 在行中搜索模式。
	found := false
	for i := 0; i <= len(lines)-len(searchPattern); i++ {
		match := true
		for j, search := range searchPattern {
			if i+j >= len(lines) || strings.TrimSpace(lines[i+j]) != strings.TrimSpace(search) {
				match = false
				break
			}
		}
		if match {
			// Replace the matched block with the replacement.
			// 用替换内容替换匹配的块。
			result := make([]string, 0, len(lines)-len(searchPattern)+len(replacement))
			result = append(result, lines[:i]...)
			result = append(result, replacement...)
			result = append(result, lines[i+len(searchPattern):]...)
			lines = result
			found = true
			break
		}
	}

	if !found {
		return lines, fmt.Errorf("hunk context not found in file")
	}

	return lines, nil
}
