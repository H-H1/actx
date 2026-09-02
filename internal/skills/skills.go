// Package skills 提供技能的加载、解析和查询功能。
// 技能是 ~/.ACTX/skills/ 目录下的文件夹，每个文件夹包含一个 SKILL.md 文件，
// SKILL.md 的 YAML frontmatter 中定义了 name 和 description。
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillName 是技能文件的固定文件名。
const SkillFileName = "SKILL.md"

// Skill represents a single loaded skill.
// Skill 表示一个已加载的技能。
type Skill struct {
	// Name 是技能名称，来自 SKILL.md frontmatter。
	Name string
	// Description 是技能描述，来自 SKILL.md frontmatter。
	Description string
	// Path 是 SKILL.md 文件的绝对路径。
	Path string
	// Dir 是技能文件夹的绝对路径。
	Dir string
	// Content 是 SKILL.md 的完整内容（含 frontmatter）。
	Content string
}

// Load 扫描 actxHome 下的 skills/ 目录，加载所有技能。
// 返回的技能列表按名称排序。
func Load(actxHome string) ([]Skill, error) {
	skillsDir := filepath.Join(actxHome, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取技能目录 %s: %w", skillsDir, err)
	}

	var result []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill, err := loadSkillDir(filepath.Join(skillsDir, entry.Name()))
		if err != nil {
			// 跳过无法加载的技能文件夹，但不中断整体加载。
			continue
		}
		if skill != nil {
			result = append(result, *skill)
		}
	}
	return result, nil
}

// loadSkillDir 从指定文件夹加载一个技能。
func loadSkillDir(dir string) (*Skill, error) {
	skillPath := filepath.Join(dir, SkillFileName)
	content, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, err
	}

	name, desc := parseFrontmatter(string(content))
	if name == "" {
		// 如果 frontmatter 中没有 name，使用文件夹名作为技能名。
		name = filepath.Base(dir)
	}

	return &Skill{
		Name:        name,
		Description: desc,
		Path:        skillPath,
		Dir:         dir,
		Content:     string(content),
	}, nil
}

// parseFrontmatter 从 Markdown 内容中解析 YAML frontmatter 的 name 和 description 字段。
// frontmatter 格式：
//
//	---
//	name: skill-name
//	description: 技能描述
//	---
func parseFrontmatter(content string) (name, description string) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}

	inFrontmatter := false
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if !inFrontmatter {
			inFrontmatter = true
		}

		// 解析 "key: value" 格式
		idx := strings.Index(trimmed, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])

		// 去除引号
		val = strings.Trim(val, "\"'")

		switch key {
		case "name":
			name = val
		case "description":
			description = val
		}
	}
	return name, description
}

// FindByName 在技能列表中按名称查找技能。
func FindByName(skills []Skill, name string) (*Skill, bool) {
	for i := range skills {
		if skills[i].Name == name {
			return &skills[i], true
		}
	}
	return nil, false
}

// ExtractMentions 从用户输入文本中提取 $skill-name 形式的技能提及。
// 返回被提及的技能名称列表（去重）。
func ExtractMentions(text string, skills []Skill) []string {
	seen := make(map[string]bool)
	var names []string

	for _, skill := range skills {
		mention := "$" + skill.Name
		if strings.Contains(text, mention) {
			if !seen[skill.Name] {
				seen[skill.Name] = true
				names = append(names, skill.Name)
			}
		}
	}
	return names
}

// InjectSkills 将被提及的技能内容作为系统消息注入到用户输入文本前面。
// 返回注入后的完整文本。
func InjectSkills(text string, skills []Skill, mentioned []string) string {
	if len(mentioned) == 0 {
		return text
	}

	var b strings.Builder
	b.WriteString("以下技能已被激活，请参考其指引：\n\n")
	for _, name := range mentioned {
		skill, ok := FindByName(skills, name)
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("--- 技能: %s ---\n", skill.Name))
		b.WriteString(skill.Content)
		b.WriteString("\n\n")
	}
	b.WriteString("--- 用户输入 ---\n")
	b.WriteString(text)
	return b.String()
}
