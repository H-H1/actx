package applypatch

import (
	"strings"
)

// FormatPatch converts a Patch back to the apply_patch string format.
// FormatPatch 将 Patch 转换回 apply_patch 字符串格式。
func FormatPatch(patch *Patch) string {
	var sb strings.Builder
	sb.WriteString("*** Begin Patch\n")

	for _, change := range patch.Changes {
		switch change.Op {
		case FileOpAdd:
			sb.WriteString("*** Add File: " + change.Path + "\n")
			for _, line := range strings.Split(change.Content, "\n") {
				if line != "" {
					sb.WriteString("+" + line + "\n")
				}
			}
			sb.WriteString("*** End File\n")

		case FileOpUpdate:
			sb.WriteString("*** Update File: " + change.Path + "\n")
			for _, hunk := range change.Hunks {
				sb.WriteString("@@\n")
				for _, line := range hunk.Context {
					sb.WriteString(" " + line + "\n")
				}
				for _, line := range hunk.Deletions {
					sb.WriteString("-" + line + "\n")
				}
				for _, line := range hunk.Additions {
					sb.WriteString("+" + line + "\n")
				}
			}
			sb.WriteString("*** End File\n")

		case FileOpDelete:
			sb.WriteString("*** Delete File: " + change.Path + "\n")
		}
	}

	sb.WriteString("*** End Patch\n")
	return sb.String()
}

// CreateFileChange creates a FileChange for adding a new file.
// CreateFileChange 创建一个用于添加新文件的 FileChange。
func CreateFileChange(path string, content string) FileChange {
	return FileChange{
		Op:      FileOpAdd,
		Path:    path,
		Content: content,
	}
}

// UpdateFileChange creates a FileChange for updating an existing file with
// a single hunk replacing oldLines with newLines.
// UpdateFileChange 创建一个 FileChange，用于通过单个块将 oldLines 替换为 newLines 来更新现有文件。
func UpdateFileChange(path string, oldLines, newLines []string) FileChange {
	return FileChange{
		Op:   FileOpUpdate,
		Path: path,
		Hunks: []Hunk{
			{
				Deletions: oldLines,
				Additions: newLines,
			},
		},
	}
}

// DeleteFileChange creates a FileChange for deleting a file.
// DeleteFileChange 创建一个用于删除文件的 FileChange。
func DeleteFileChange(path string) FileChange {
	return FileChange{
		Op:   FileOpDelete,
		Path: path,
	}
}
