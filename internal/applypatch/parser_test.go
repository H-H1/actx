package applypatch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAddFile(t *testing.T) {
	input := `*** Begin Patch
*** Add File: hello.txt
+Hello, World!
+Line 2
*** End File
*** End Patch`

	patch, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(patch.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(patch.Changes))
	}

	change := patch.Changes[0]
	if change.Op != FileOpAdd {
		t.Errorf("expected FileOpAdd, got %v", change.Op)
	}
	if change.Path != "hello.txt" {
		t.Errorf("expected path hello.txt, got %s", change.Path)
	}
	if change.Content != "Hello, World!\nLine 2\n" {
		t.Errorf("unexpected content: %q", change.Content)
	}
}

func TestParseDeleteFile(t *testing.T) {
	input := `*** Begin Patch
*** Delete File: old.txt
*** End Patch`

	patch, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(patch.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(patch.Changes))
	}

	change := patch.Changes[0]
	if change.Op != FileOpDelete {
		t.Errorf("expected FileOpDelete, got %v", change.Op)
	}
	if change.Path != "old.txt" {
		t.Errorf("expected path old.txt, got %s", change.Path)
	}
}

func TestParseUpdateFile(t *testing.T) {
	input := `*** Begin Patch
*** Update File: test.txt
@@
 line1
-old line
+new line
 line3
@@
*** End File
*** End Patch`

	patch, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(patch.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(patch.Changes))
	}

	change := patch.Changes[0]
	if change.Op != FileOpUpdate {
		t.Errorf("expected FileOpUpdate, got %v", change.Op)
	}
	if len(change.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(change.Hunks))
	}

	hunk := change.Hunks[0]
	if len(hunk.Deletions) != 1 || hunk.Deletions[0] != "old line" {
		t.Errorf("unexpected deletions: %v", hunk.Deletions)
	}
	if len(hunk.Additions) != 1 || hunk.Additions[0] != "new line" {
		t.Errorf("unexpected additions: %v", hunk.Additions)
	}
}

func TestApplyAddFile(t *testing.T) {
	tmpDir := t.TempDir()
	patch := &Patch{
		Changes: []FileChange{
			CreateFileChange("test.txt", "Hello, World!\n"),
		},
	}

	result, err := Apply(patch, tmpDir)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if len(result.FilesAdded) != 1 {
		t.Errorf("expected 1 file added, got %d", len(result.FilesAdded))
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(content) != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", string(content))
	}
}

func TestApplyUpdateFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial file.
	initialContent := "line1\nold line\nline3"
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(initialContent), 0o644)

	// Apply update.
	patch := &Patch{
		Changes: []FileChange{
			{
				Op:   FileOpUpdate,
				Path: "test.txt",
				Hunks: []Hunk{
					{
						Context:   []string{"line1"},
						Deletions: []string{"old line"},
						Additions: []string{"new line"},
					},
				},
			},
		},
	}

	result, err := Apply(patch, tmpDir)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if len(result.FilesUpdated) != 1 {
		t.Errorf("expected 1 file updated, got %d", len(result.FilesUpdated))
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	expected := "line1\nnew line\nline3"
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

func TestApplyDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file to delete.
	os.WriteFile(filepath.Join(tmpDir, "delete_me.txt"), []byte("content"), 0o644)

	patch := &Patch{
		Changes: []FileChange{
			DeleteFileChange("delete_me.txt"),
		},
	}

	result, err := Apply(patch, tmpDir)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if len(result.FilesDeleted) != 1 {
		t.Errorf("expected 1 file deleted, got %d", len(result.FilesDeleted))
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "delete_me.txt")); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted")
	}
}

func TestFormatPatch(t *testing.T) {
	patch := &Patch{
		Changes: []FileChange{
			CreateFileChange("test.txt", "Hello\n"),
		},
	}

	output := FormatPatch(patch)
	if output == "" {
		t.Error("expected non-empty output")
	}

	// Verify round-trip: parse the formatted output.
	parsed, err := Parse(output)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(parsed.Changes) != 1 {
		t.Errorf("expected 1 change after round-trip, got %d", len(parsed.Changes))
	}
}
