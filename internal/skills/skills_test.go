package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	home := t.TempDir()

	// 创建一个有效的技能
	skillDir := filepath.Join(home, "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: test-skill\ndescription: A test skill\n---\n# Test Skill\nBody content."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 创建一个带引号的技能
	quotedDir := filepath.Join(home, "skills", "quoted-skill")
	if err := os.MkdirAll(quotedDir, 0755); err != nil {
		t.Fatal(err)
	}
	quotedContent := "---\nname: \"quoted-skill\"\ndescription: \"Quoted description\"\n---\n# Quoted"
	if err := os.WriteFile(filepath.Join(quotedDir, "SKILL.md"), []byte(quotedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 创建一个没有 frontmatter 的技能（应使用文件夹名作为名称）
	noFmDir := filepath.Join(home, "skills", "no-frontmatter")
	if err := os.MkdirAll(noFmDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noFmDir, "SKILL.md"), []byte("# No frontmatter"), 0644); err != nil {
		t.Fatal(err)
	}

	// 创建一个不含 SKILL.md 的文件夹（应被跳过）
	emptyDir := filepath.Join(home, "skills", "empty-folder")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(loaded))
	}

	// 验证 test-skill
	ts, ok := FindByName(loaded, "test-skill")
	if !ok {
		t.Fatal("test-skill not found")
	}
	if ts.Description != "A test skill" {
		t.Errorf("expected description 'A test skill', got '%s'", ts.Description)
	}
	if !strings.Contains(ts.Content, "Body content") {
		t.Error("content should contain body")
	}

	// 验证带引号的技能
	qs, ok := FindByName(loaded, "quoted-skill")
	if !ok {
		t.Fatal("quoted-skill not found")
	}
	if qs.Description != "Quoted description" {
		t.Errorf("expected 'Quoted description', got '%s'", qs.Description)
	}

	// 验证无 frontmatter 的技能
	nf, ok := FindByName(loaded, "no-frontmatter")
	if !ok {
		t.Fatal("no-frontmatter not found")
	}
	if nf.Name != "no-frontmatter" {
		t.Errorf("expected name 'no-frontmatter', got '%s'", nf.Name)
	}
}

func TestLoadEmptyDir(t *testing.T) {
	home := t.TempDir()
	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load on non-existent dir should not error: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(loaded))
	}
}

func TestExtractMentions(t *testing.T) {
	skills := []Skill{
		{Name: "code-reviewer", Description: "Code review"},
		{Name: "imagegen", Description: "Image gen"},
	}

	mentions := ExtractMentions("Please $code-reviewer this code", skills)
	if len(mentions) != 1 || mentions[0] != "code-reviewer" {
		t.Fatalf("expected [code-reviewer], got %v", mentions)
	}

	mentions = ExtractMentions("$imagegen and $code-reviewer both", skills)
	if len(mentions) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(mentions))
	}

	// 无提及
	mentions = ExtractMentions("no mentions here", skills)
	if len(mentions) != 0 {
		t.Fatalf("expected 0 mentions, got %d", len(mentions))
	}
}

func TestInjectSkills(t *testing.T) {
	skills := []Skill{
		{Name: "test", Description: "Test", Content: "# Test Skill\nDo things."},
	}

	// 有提及
	injected := InjectSkills("hello world", skills, []string{"test"})
	if !strings.Contains(injected, "# Test Skill") {
		t.Error("injected text should contain skill content")
	}
	if !strings.Contains(injected, "hello world") {
		t.Error("injected text should contain original input")
	}

	// 无提及
	injected = InjectSkills("hello world", skills, nil)
	if injected != "hello world" {
		t.Error("no mentions should return original text")
	}
}
