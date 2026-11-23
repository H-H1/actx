package agent

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

// TestFlexibleStringSliceVerbatim 验证单字符串命令保持原样，不被按空白分割。
func TestFlexibleStringSliceVerbatim(t *testing.T) {
	var f flexibleStringSlice
	if err := f.UnmarshalJSON([]byte(`"foreach($i in 1..3) { Write-Output $i }"`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"foreach($i in 1..3) { Write-Output $i }"}
	if !reflect.DeepEqual([]string(f), want) {
		t.Fatalf("got %v, want %v", []string(f), want)
	}
}

// TestStripNestedPowerShell 验证多余的 powershell 包装被剥离，$ 变量得以保留。
func TestStripNestedPowerShell(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "powershell -Command wrapper",
			in:   []string{"powershell", "-NoProfile", "-Command", `Get-ChildItem | Where-Object { $_.FullName }`},
			want: []string{`Get-ChildItem | Where-Object { $_.FullName }`},
		},
		{
			name: "full path with .exe",
			in:   []string{`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.EXE`, "-Command", "echo $env:PATH"},
			want: []string{"echo $env:PATH"},
		},
		{
			name: "pwsh -c short flag",
			in:   []string{"pwsh", "-NoProfile", "-c", "foreach($i in 1..3){ $i }"},
			want: []string{"foreach($i in 1..3){ $i }"},
		},
		{
			name: "non-shell command untouched",
			in:   []string{"git", "log", "--oneline"},
			want: []string{"git", "log", "--oneline"},
		},
		{
			name: "single element untouched",
			in:   []string{"powershell -Command echo hi"},
			want: []string{"powershell -Command echo hi"},
		},
		{
			name: "command flag without value untouched",
			in:   []string{"powershell", "-NoProfile", "-Command"},
			want: []string{"powershell", "-NoProfile", "-Command"},
		},
		{
			name: "positional before flag untouched",
			in:   []string{"powershell", "script.ps1", "-Command", "x"},
			want: []string{"powershell", "script.ps1", "-Command", "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripNestedPowerShell(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEncodePowerShellCommand 验证编码可无损解码回原脚本。
func TestEncodePowerShellCommand(t *testing.T) {
	script := powershellUTF8Prefix + `Get-ChildItem 'D:\目录' | Where-Object { $_.FullName -notmatch 'node_modules' }; foreach($i in 1..3){ Write-Output $i }`
	encoded := encodePowerShellCommand(script)

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
	}
	if got := string(utf16.Decode(units)); got != script {
		t.Fatalf("roundtrip mismatch:\n got %q\nwant %q", got, script)
	}
}

// TestJoinPowerShellCommand 验证含空格/引号的部分用单引号包裹且保持 $ 变量字面量。
func TestJoinPowerShellCommand(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"Get-ChildItem | Measure-Object"}, "Get-ChildItem | Measure-Object"},
		{[]string{"cmd", "/c", "dir /b *.html"}, `cmd /c 'dir /b *.html'`},
		{[]string{"Write-Host", "it's here"}, `Write-Host 'it''s here'`},
		{[]string{"echo", `"say $x"`}, `echo '"say $x"'`},
	}
	for _, tc := range cases {
		if got := joinPowerShellCommand(tc.in); got != tc.want {
			t.Fatalf("joinPowerShellCommand(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPrepareCommandEncoded 验证 Windows 下使用 -EncodedCommand 且解码后脚本包含原命令。
func TestPrepareCommandEncoded(t *testing.T) {
	// 与 Handle 流程一致：先剥离嵌套包装，再构造调用。
	cmd := stripNestedPowerShell([]string{"powershell", "-Command", "Write-Output $PSVersionTable"})
	shell, args := prepareCommand(cmd)
	if !strings.Contains(shell, "pwsh.exe") && !strings.Contains(shell, "powershell.exe") {
		t.Fatalf("unexpected shell %q", shell)
	}
	var enc string
	for i, a := range args {
		if strings.EqualFold(a, "-EncodedCommand") && i+1 < len(args) {
			enc = args[i+1]
		}
	}
	if enc == "" {
		t.Fatalf("expected -EncodedCommand in args %v", args)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	// 脚本应为 UTF-16LE。
	var sb strings.Builder
	for i := 0; i+1 < len(raw); i += 2 {
		sb.WriteRune(rune(uint16(raw[i]) | uint16(raw[i+1])<<8))
	}
	script := sb.String()
	if !strings.Contains(script, "Write-Output $PSVersionTable") {
		t.Fatalf("script %q missing original command", script)
	}
	if !strings.Contains(script, powershellUTF8Prefix) {
		t.Fatalf("script %q missing UTF-8 prefix", script)
	}
	// 嵌套包装不应残留在脚本中。
	if strings.Contains(script, "-Command") {
		t.Fatalf("script %q should not contain nested -Command wrapper", script)
	}
}

// TestIsWriteCommandSingleString 验证单字符串命令按第一个 token 判断写命令。
func TestIsWriteCommandSingleString(t *testing.T) {
	if !isWriteCommand([]string{"Remove-Item foo -Recurse"}) {
		t.Fatal("Remove-Item should be detected as write command")
	}
	if isWriteCommand([]string{"Get-ChildItem -Recurse"}) {
		t.Fatal("Get-ChildItem should not be detected as write command")
	}
}

// TestStripNestedPowerShellEnablesWriteDetection 验证剥离包装后沙箱能看到真实命令。
func TestStripNestedPowerShellEnablesWriteDetection(t *testing.T) {
	stripped := stripNestedPowerShell([]string{"powershell", "-Command", "Remove-Item foo"})
	if !isWriteCommand(stripped) {
		t.Fatal("nested Remove-Item should be blocked in read-only sandbox after stripping")
	}
}
