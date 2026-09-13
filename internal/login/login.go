// Package login implements authentication management for ACTX CLI.
// login 包实现 ACTX CLI 的认证管理。
// It handles API key storage, ChatGPT OAuth login, and token retrieval.
// 它处理 API 密钥存储、ChatGPT OAuth 登录和令牌检索。
package login

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuthToken represents stored authentication credentials.
// AuthToken 表示存储的认证凭据。
type AuthToken struct {
	// APIKey is the stored API key, if authenticating with a key.
	// APIKey 是存储的 API 密钥（使用密钥认证时）。
	APIKey string `json:"api_key,omitempty"`
	// AccessToken is the OAuth access token, if authenticated via OAuth.
	// AccessToken 是 OAuth 访问令牌（通过 OAuth 认证时）。
	AccessToken string `json:"access_token,omitempty"`
	// RefreshToken is the OAuth refresh token used to renew the access token.
	// RefreshToken 是用于续期访问令牌的 OAuth 刷新令牌。
	RefreshToken string `json:"refresh_token,omitempty"`
	// ExpiresAt is when the access token expires (zero means no expiry).
	// ExpiresAt 是访问令牌的过期时间（零值表示无过期时间）。
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// AccountID is the authenticated account identifier.
	// AccountID 是已认证账户的标识符。
	AccountID string `json:"account_id,omitempty"`
	// Email is the authenticated account email.
	// Email 是已认证账户的邮箱。
	Email string `json:"email,omitempty"`
}

// IsExpired returns true if the token has expired.
// IsExpired 如果令牌已过期则返回 true。
func (t *AuthToken) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(t.ExpiresAt)
}

// IsValid returns true if the token has either an API key or a non-expired access token.
// IsValid 如果令牌有 API 密钥或未过期的访问令牌则返回 true。
func (t *AuthToken) IsValid() bool {
	return t.APIKey != "" || (t.AccessToken != "" && !t.IsExpired())
}

// authFilePath returns the path to the auth.json file in the ACTX home directory.
// authFilePath 返回 ACTX 主目录中 auth.json 文件的路径。
func authFilePath(ACTXHome string) string {
	return filepath.Join(ACTXHome, "auth.json")
}

// LoadToken reads the stored auth token from the ACTX home directory.
// LoadToken 从 ACTX 主目录读取存储的认证令牌。
// Returns nil if no token file exists.
// 如果令牌文件不存在则返回 nil。
func LoadToken(ACTXHome string) (*AuthToken, error) {
	path := authFilePath(ACTXHome)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read auth file: %w", err)
	}

	var token AuthToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("parse auth file: %w", err)
	}
	return &token, nil
}

// SaveToken writes the auth token to the ACTX home directory.
// SaveToken 将认证令牌写入 ACTX 主目录。
func SaveToken(ACTXHome string, token *AuthToken) error {
	path := authFilePath(ACTXHome)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create ACTX home: %w", err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth token: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write auth file: %w", err)
	}
	return nil
}

// DeleteToken removes the auth token file.
// DeleteToken 删除认证令牌文件。
func DeleteToken(ACTXHome string) error {
	path := authFilePath(ACTXHome)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete auth file: %w", err)
	}
	return nil
}

// loadLocalConfigAPIKey reads the API key from the local config.json file.
// loadLocalConfigAPIKey 从本地 config.json 文件读取 API 密钥。
// Searches .actx/config.json from the current directory upward through parents
// so running inside a subdirectory (e.g. cmd/actx) still finds the project
// root config, then falls back to ACTX home.
// 从当前目录逐级向上搜索 .actx/config.json（在子目录如 cmd/actx 中运行
// 也能找到项目根配置），再回退到 ACTX 主目录。
func loadLocalConfigAPIKey(ACTXHome string) (string, error) {
	var paths []string
	// Walk from cwd upward, collecting .actx/config.json candidates.
	// 从当前目录逐级向上收集 .actx/config.json 候选路径。
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			paths = append(paths, filepath.Join(dir, ".actx", "config.json"))
			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
	}
	paths = append(paths, filepath.Join(ACTXHome, "config.json"))

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("read local config %s: %w", path, err)
		}
		var cfg struct {
			APIKey string `json:"api_key"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return "", fmt.Errorf("parse local config %s: %w", path, err)
		}
		if cfg.APIKey != "" {
			return cfg.APIKey, nil
		}
	}
	return "", nil
}

// GetAPIKey retrieves the API key from the stored token, the OPENAI_API_KEY
// environment variable, or the local config.json file.
// GetAPIKey 从存储的令牌、OPENAI_API_KEY 环境变量或本地 config.json 文件中获取 API 密钥。
func GetAPIKey(ACTXHome string) (string, error) {
	// Check environment variable first.
	// 首先检查环境变量。
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key, nil
	}

	// Fall back to stored token.
	// 回退到存储的令牌。
	token, err := LoadToken(ACTXHome)
	if err != nil {
		return "", err
	}
	if token != nil && token.APIKey != "" {
		return token.APIKey, nil
	}

	// Fall back to local config.json (not committed to git).
	// 回退到本地 config.json（不提交到 git）。
	key, err := loadLocalConfigAPIKey(ACTXHome)
	if err != nil {
		return "", err
	}
	if key != "" {
		return key, nil
	}

	return "", fmt.Errorf("no API key found: set OPENAI_API_KEY, login, or add api_key to .actx/config.json")
}

// StoreAPIKey saves an API key to the auth file.
// StoreAPIKey 将 API 密钥保存到认证文件中。
func StoreAPIKey(ACTXHome, apiKey string) error {
	token, _ := LoadToken(ACTXHome)
	if token == nil {
		token = &AuthToken{}
	}
	token.APIKey = apiKey
	return SaveToken(ACTXHome, token)
}
