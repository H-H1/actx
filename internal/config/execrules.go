package config

import (
	"strings"
)

// ExecRule defines a custom execution rule for command validation.
// ExecRule 定义用于命令验证的自定义执行规则。
type ExecRule struct {
	// Pattern is the command pattern to match (e.g. "git push*").
	// Pattern 是要匹配的命令模式（例如 "git push*"）。
	Pattern string `json:"pattern" toml:"pattern"`
	// Action is what to do: "allow", "deny", "ask".
	// Action 表示执行的操作："allow"（允许）、"deny"（拒绝）、"ask"（询问）。
	Action string `json:"action" toml:"action"`
	// Message is an optional explanation shown to the user.
	// Message 是向用户显示的可选说明。
	Message string `json:"message,omitempty" toml:"message,omitempty"`
}

// ExecRuleSet holds a collection of exec rules.
// ExecRuleSet 保存执行规则的集合。
type ExecRuleSet struct {
	// Rules is the ordered list of rules to evaluate against commands.
	// Rules 是按顺序评估命令的规则列表。
	Rules []ExecRule `json:"rules" toml:"rules"`
}

// CheckCommand evaluates rules against a command and returns the action.
// Returns "ask" if no rule matches.
// CheckCommand 根据规则评估命令并返回操作。
// 如果没有规则匹配，返回 "ask"。
func (rs *ExecRuleSet) CheckCommand(command []string) string {
	cmdStr := strings.Join(command, " ")
	for _, rule := range rs.Rules {
		if matchPattern(rule.Pattern, cmdStr) {
			return rule.Action
		}
	}
	return "ask"
}

// matchPattern checks if a command matches a pattern with wildcard support.
// matchPattern 检查命令是否匹配支持通配符的模式。
func matchPattern(pattern, command string) bool {
	if pattern == "" {
		return false
	}
	// Simple wildcard matching: pattern ends with *
	// 简单的通配符匹配：模式以 * 结尾。
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(command, prefix)
	}
	return pattern == command
}

// DefaultExecRules returns the default rule set.
// DefaultExecRules 返回默认规则集。
func DefaultExecRules() *ExecRuleSet {
	return &ExecRuleSet{
		Rules: []ExecRule{
			{Pattern: "git status*", Action: "allow", Message: "read-only git command"},
			{Pattern: "git diff*", Action: "allow", Message: "read-only git command"},
			{Pattern: "git log*", Action: "allow", Message: "read-only git command"},
			{Pattern: "ls*", Action: "allow", Message: "read-only command"},
			{Pattern: "cat*", Action: "allow", Message: "read-only command"},
			{Pattern: "rm -rf /*", Action: "deny", Message: "dangerous command"},
		},
	}
}
