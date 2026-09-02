package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApprovalPolicyString(t *testing.T) {
	tests := []struct {
		policy ApprovalPolicy
		want   string
	}{
		{ApprovalNever, "never"},
		{ApprovalOnFailure, "on-failure"},
		{ApprovalOnRequest, "on-request"},
		{ApprovalOnUntrusted, "on-untrusted"},
	}

	for _, tt := range tests {
		if got := tt.policy.String(); got != tt.want {
			t.Errorf("expected %s, got %s", tt.want, got)
		}
	}
}

func TestParseApprovalPolicy(t *testing.T) {
	tests := []struct {
		input string
		want  ApprovalPolicy
	}{
		{"never", ApprovalNever},
		{"on-failure", ApprovalOnFailure},
		{"on-request", ApprovalOnRequest},
		{"on-untrusted", ApprovalOnUntrusted},
	}

	for _, tt := range tests {
		got, err := ParseApprovalPolicy(tt.input)
		if err != nil {
			t.Errorf("ParseApprovalPolicy(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseApprovalPolicy(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSandboxPolicyString(t *testing.T) {
	tests := []struct {
		policy SandboxPolicy
		want   string
	}{
		{SandboxNone, "none"},
		{SandboxReadOnly, "read-only"},
		{SandboxWorkspaceWrite, "workspace-write"},
		{SandboxDangerFullAccess, "danger-full-access"},
	}

	for _, tt := range tests {
		if got := tt.policy.String(); got != tt.want {
			t.Errorf("expected %s, got %s", tt.want, got)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a config.toml in the temp directory.
	configContent := `model = "gpt-4"
model_provider = "openai"
approval_policy = "never"
sandbox_policy = "read-only"
`
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	builder := NewConfigBuilder(tmpDir, nil)
	builder.Cwd = tmpDir // prevent upward search for .actx
	cfg, err := builder.Build()
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if cfg.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", cfg.Model)
	}
	if cfg.ModelProvider != "openai" {
		t.Errorf("expected provider openai, got %s", cfg.ModelProvider)
	}
	if cfg.ApprovalPolicy != ApprovalNever {
		t.Errorf("expected ApprovalNever, got %v", cfg.ApprovalPolicy)
	}
	if cfg.SandboxPolicy != SandboxReadOnly {
		t.Errorf("expected SandboxReadOnly, got %v", cfg.SandboxPolicy)
	}
}

func TestLoadConfigWithOverrides(t *testing.T) {
	tmpDir := t.TempDir()

	overrides := map[string]string{
		"model":           "o4-mini",
		"approval_policy": "on-failure",
		"sandbox_policy":  "workspace-write",
	}

	builder := NewConfigBuilder(tmpDir, overrides)
	builder.Cwd = tmpDir // prevent upward search for .actx
	cfg, err := builder.Build()
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	if cfg.Model != "o4-mini" {
		t.Errorf("expected model o4-mini, got %s", cfg.Model)
	}
	if cfg.ApprovalPolicy != ApprovalOnFailure {
		t.Errorf("expected ApprovalOnFailure, got %v", cfg.ApprovalPolicy)
	}
	if cfg.SandboxPolicy != SandboxWorkspaceWrite {
		t.Errorf("expected SandboxWorkspaceWrite, got %v", cfg.SandboxPolicy)
	}
}

func TestBuiltinProfiles(t *testing.T) {
	if BuiltinReadOnlyProfile.ReadOnly != true {
		t.Error("read-only profile should have ReadOnly=true")
	}
	if BuiltinReadOnlyProfile.ExecuteCommands != false {
		t.Error("read-only profile should not allow commands")
	}
	if BuiltinWorkspaceProfile.ReadOnly != false {
		t.Error("workspace profile should not be read-only")
	}
}

func TestACTXHomeDir(t *testing.T) {
	// Should return something, even if ACTX_HOME is not set.
	home := ACTXHomeDir()
	if home == "" {
		t.Error("expected non-empty ACTX home")
	}

	// Test with ACTX_HOME set.
	os.Setenv("ACTX_HOME", "/custom/ACTX/home")
	defer os.Unsetenv("ACTX_HOME")

	home = ACTXHomeDir()
	if home != "/custom/ACTX/home" {
		t.Errorf("expected /custom/ACTX/home, got %s", home)
	}
}
