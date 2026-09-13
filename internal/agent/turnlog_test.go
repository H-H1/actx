package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H-H1/actx/internal/model"
)

// chdirTemp switches to a fresh temp directory for the test so
// turnLogsDir() resolves inside it (not the repo's .actx), restoring the
// original cwd afterwards.
// chdirTemp 把测试切换到全新临时目录，使 turnLogsDir() 在其内解析
// （而非仓库的 .actx），测试结束后恢复原 cwd。
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Pre-create .actx so turnLogsDir() resolves here instead of walking
	// up to a user-level C:\Users\<name>\.actx directory.
	// 预先创建 .actx，使 turnLogsDir() 就地解析，而不是向上找到
	// 用户级的 C:\Users\<name>\.actx 目录。
	if err := os.MkdirAll(filepath.Join(dir, ".actx"), 0o755); err != nil {
		t.Fatalf("mkdir .actx: %v", err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

// TestLogRequestInstructionsDedup verifies the instructions (system prompt)
// are logged once per thread: the first request writes them, identical
// follow-up requests do not, and a changed instructions text (e.g. after a
// tools-mode switch) is written again.
// TestLogRequestInstructionsDedup 验证 instructions（系统提示词）每线程
// 只记录一次：首个请求写入，后续相同请求不重复，内容变化（如工具模式
// 切换后）时再次写入。
func TestLogRequestInstructionsDedup(t *testing.T) {
	dir := chdirTemp(t)
	logger := NewTurnLogger("thread-test")
	if logger == nil {
		t.Fatal("NewTurnLogger returned nil")
	}
	defer logger.Close()

	p := model.Prompt{Instructions: "be brief and safe"}
	logger.LogRequest(p)
	logger.LogRequest(p)
	logger.LogRequest(model.Prompt{Instructions: "be brief and safe + tool prompt"})

	raw, err := os.ReadFile(filepath.Join(dir, ".actx", "logs", "thread-test.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	content := string(raw)
	if got := strings.Count(content, "be brief and safe"); got != 2 {
		t.Errorf("expected base instructions twice (initial + changed variant), got %d:\n%s", got, content)
	}
	if n := strings.Count(content, "[instructions]"); n != 2 {
		t.Errorf("expected 2 [instructions] sections, got %d:\n%s", n, content)
	}
	if n := strings.Count(content, "Request #"); n != 3 {
		t.Errorf("expected 3 request headers, got %d", n)
	}
}
