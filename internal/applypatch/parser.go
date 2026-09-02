// Package applypatch implements the ACTX apply_patch format parser and applier.
// applypatch 包实现 ACTX apply_patch 格式的解析器和应用器。
// The apply_patch format uses special markers to add, update, and delete files.
// apply_patch 格式使用特殊标记来添加、更新和删除文件。
package applypatch

import (
	"fmt"
	"strings"
)

// FileOp enumerates the file operations in a patch.
// FileOp 枚举补丁中的文件操作。
type FileOp int

const (
	FileOpAdd    FileOp = iota // Create a new file
	// 创建新文件
	FileOpUpdate               // Modify an existing file
	// 修改现有文件
	FileOpDelete               // Delete an existing file
	// 删除现有文件
)

// String returns the string representation of the file operation.
// String 返回文件操作的字符串表示。
func (op FileOp) String() string {
	switch op {
	case FileOpAdd:
		return "Add"
	case FileOpUpdate:
		return "Update"
	case FileOpDelete:
		return "Delete"
	default:
		return "Unknown"
	}
}

// FileChange represents a single file change within a patch.
// FileChange 表示补丁中的单个文件更改。
type FileChange struct {
	// Op is the operation to perform on the file.
	// Op 是对文件执行的操作。
	Op FileOp
	// Path is the file path the change applies to.
	// Path 是更改所应用的文件路径。
	Path string
	Content string // Full content for Add; for Update, this holds the result after applying hunks
	// 添加操作：完整内容；更新操作：应用块后的结果
	Hunks   []Hunk // Hunks for Update operations
	// 更新操作的块
}

// Hunk represents a contiguous block of changes within a file update.
// Hunk 表示文件更新中的连续更改块。
type Hunk struct {
	Context   []string // Unchanged context lines
	// 未更改的上下文行
	Deletions []string // Lines to remove
	// 要删除的行
	Additions []string // Lines to add
	// 要添加的行
}

// Patch represents a complete apply_patch operation containing multiple file changes.
// Patch 表示包含多个文件更改的完整 apply_patch 操作。
type Patch struct {
	// Changes lists the file changes contained in the patch.
	// Changes 列出补丁中包含的文件更改。
	Changes []FileChange
}

const (
	markerBeginPatch = "*** Begin Patch"
	markerEndPatch   = "*** End Patch"
	markerAddFile    = "*** Add File: "
	markerUpdateFile = "*** Update File: "
	markerDeleteFile = "*** Delete File: "
	markerEndFile    = "*** End File"
	markerHunk       = "@@"
)

// Parse parses the apply_patch format string into a Patch.
// Parse 将 apply_patch 格式字符串解析为 Patch。
func Parse(input string) (*Patch, error) {
	lines := strings.Split(input, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != markerBeginPatch {
		return nil, fmt.Errorf("patch must start with %q", markerBeginPatch)
	}

	patch := &Patch{}
	var currentChange *FileChange
	var currentHunk *Hunk

	finishHunk := func() {
		if currentHunk != nil && currentChange != nil {
			// Only append non-empty hunks (those with context, deletions, or additions).
			// 仅追加非空块（那些包含上下文、删除或添加的块）。
			if len(currentHunk.Context) > 0 || len(currentHunk.Deletions) > 0 || len(currentHunk.Additions) > 0 {
				currentChange.Hunks = append(currentChange.Hunks, *currentHunk)
			}
			currentHunk = nil
		}
	}

	finishChange := func() {
		finishHunk()
		if currentChange != nil {
			// For Add operations, join content lines.
			// 对于添加操作，连接内容行。
			if currentChange.Op == FileOpAdd && currentChange.Content == "" {
				// Content was accumulated differently
				// 内容以不同的方式累积
			}
			patch.Changes = append(patch.Changes, *currentChange)
			currentChange = nil
		}
	}

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == markerEndPatch:
			finishChange()
			return patch, nil

		case strings.HasPrefix(trimmed, markerAddFile):
			finishChange()
			path := strings.TrimPrefix(trimmed, markerAddFile)
			currentChange = &FileChange{Op: FileOpAdd, Path: path}

		case strings.HasPrefix(trimmed, markerUpdateFile):
			finishChange()
			path := strings.TrimPrefix(trimmed, markerUpdateFile)
			currentChange = &FileChange{Op: FileOpUpdate, Path: path}

		case strings.HasPrefix(trimmed, markerDeleteFile):
			finishChange()
			path := strings.TrimPrefix(trimmed, markerDeleteFile)
			currentChange = &FileChange{Op: FileOpDelete, Path: path}

		case trimmed == markerEndFile:
			finishChange()

		case trimmed == markerHunk:
			// The @@ marker both ends the previous hunk and starts a new one.
			// @@ 标记同时结束前一个块并开始一个新块。
			// If there's no current hunk, start one. If there is one, finish
			// 如果没有当前块，则开始一个。如果有，则结束它
			// it and start a new one (which may be empty if the next marker
			// 并开始一个新块（如果下一个标记是 *** End File，则新块可能为空）。
			// is *** End File).
			if currentHunk != nil && (len(currentHunk.Context) > 0 || len(currentHunk.Deletions) > 0 || len(currentHunk.Additions) > 0) {
				finishHunk()
			}
			if currentChange == nil {
				return nil, fmt.Errorf("hunk marker outside of file change at line %d", i+1)
			}
			currentHunk = &Hunk{}

		case currentChange != nil && currentChange.Op == FileOpAdd:
			// For Add operations, lines starting with + are file content.
			// 对于添加操作，以 + 开头的行是文件内容。
			if strings.HasPrefix(line, "+") {
				currentChange.Content += strings.TrimPrefix(line, "+") + "\n"
			}

		case currentHunk != nil:
			// Within a hunk: + prefix = addition, - prefix = deletion, else context.
			// 块内：+ 前缀 = 添加，- 前缀 = 删除，否则为上下文。
			switch {
			case strings.HasPrefix(line, "+"):
				currentHunk.Additions = append(currentHunk.Additions, strings.TrimPrefix(line, "+"))
			case strings.HasPrefix(line, "-"):
				currentHunk.Deletions = append(currentHunk.Deletions, strings.TrimPrefix(line, "-"))
			default:
				currentHunk.Context = append(currentHunk.Context, line)
			}

		case currentChange != nil && currentChange.Op == FileOpDelete:
			// Delete operations have no content lines.
			// 删除操作没有内容行。

		default:
			// Ignore unknown lines (could be empty lines or comments)
			// 忽略未知行（可能是空行或注释）
		}
	}

	finishChange()
	return patch, nil
}
