//go:build integration

package skills

import (
	"fmt"
	"os"
	"testing"

	"github.com/H-H1/actx/internal/config"
)

func TestLoadRealSkills(t *testing.T) {
	home := config.ACTXHomeDir()
	t.Logf("ACTX home: %s", home)

	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Logf("Loaded %d skills:", len(loaded))
	for _, s := range loaded {
		desc := s.Description
		if len(desc) > 80 {
			desc = desc[:80] + "..."
		}
		fmt.Printf("  $%s - %s\n", s.Name, desc)
		t.Logf("  $%s - %s (path: %s)", s.Name, desc, s.Path)
	}

	if len(loaded) == 0 {
		t.Skip("no skills found in ~/.ACTX/skills/")
	}

	// 验证每个技能都有名称和内容
	for _, s := range loaded {
		if s.Name == "" {
			t.Errorf("skill at %s has empty name", s.Path)
		}
		if s.Content == "" {
			t.Errorf("skill %s has empty content", s.Name)
		}
		if s.Path == "" {
			t.Errorf("skill %s has empty path", s.Name)
		}
		// 验证文件确实存在
		if _, err := os.Stat(s.Path); err != nil {
			t.Errorf("skill %s path does not exist: %s", s.Name, s.Path)
		}
	}
}

func TestMentionRealSkill(t *testing.T) {
	home := config.ACTXHomeDir()
	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(loaded) == 0 {
		t.Skip("no skills found")
	}

	// 用第一个技能测试提及提取和注入
	skill := loaded[0]
	mention := "$" + skill.Name
	text := mention + " please help me"

	mentions := ExtractMentions(text, loaded)
	if len(mentions) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(mentions))
	}
	if mentions[0] != skill.Name {
		t.Fatalf("expected %s, got %s", skill.Name, mentions[0])
	}

	injected := InjectSkills(text, loaded, mentions)
	if len(injected) <= len(text) {
		t.Error("injected text should be longer than original")
	}
	t.Logf("Successfully injected skill '%s' (%d chars -> %d chars)",
		skill.Name, len(text), len(injected))
}
