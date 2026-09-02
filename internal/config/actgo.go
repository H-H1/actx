package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ACTXgoConfigDir is the directory name for actx project configuration.
// ACTXgoConfigDir 是 actx 项目配置的目录名。
const ACTXgoConfigDir = ".actx"

// ACTXgoConfigFileName is the JSON config file inside the .actx directory.
// ACTXgoConfigFileName 是 .actx 目录中的 JSON 配置文件。
const ACTXgoConfigFileName = "config.json"

// ACTXgoConfig represents the JSON config structure for actx.
// ACTXgoConfig 表示 actx 的 JSON 配置结构。
type ACTXgoConfig struct {
	// ModelProvider selects which provider from model_providers to use.
	// ModelProvider 选择使用 model_providers 中的哪个提供商。
	ModelProvider string `json:"model_provider"`

	// ModelProviders is a map of provider name → provider configuration.
	// Each provider defines its own base_url and models list.
	// ModelProviders 是提供商名称到提供商配置的映射。
	// 每个提供商定义自己的 base_url 和模型列表。
	ModelProviders map[string]providerConfig `json:"model_providers"`
}

// providerConfig defines the configuration for a single model provider.
// providerConfig 定义单个模型提供商的配置。
type providerConfig struct {
	// BaseURL is the API base URL for the provider.
	// BaseURL 是该提供商的 API 基础 URL。
	BaseURL string `json:"base_url"`
	// Models is the list of model identifiers offered by this provider.
	// Models 是该提供商提供的模型标识符列表。
	Models []string `json:"models"`
}

// resolveProvider returns the provider config for the active model_provider.
// Falls back to the first provider if the named one is not found.
// resolveProvider 返回当前 model_provider 的提供商配置。
// 如果找不到指定名称的提供商，则回退到第一个提供商。
func (c *ACTXgoConfig) resolveProvider() *providerConfig {
	if c.ModelProviders == nil {
		return nil
	}
	if c.ModelProvider != "" {
		if pc, ok := c.ModelProviders[c.ModelProvider]; ok {
			return &pc
		}
	}
	// Fallback: return the first provider.
	// 回退：返回第一个提供商。
	for _, pc := range c.ModelProviders {
		return &pc
	}
	return nil
}

// loadACTXGoConfig reads and parses a .actx/config.json file.
// Returns nil if the file does not exist.
// loadACTXGoConfig 读取并解析 .actx/config.json 文件。
// 如果文件不存在则返回 nil。
func loadACTXGoConfig(path string) (*ACTXgoConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfg ACTXgoConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// findProjectConfigDir searches upward from cwd for a directory containing
// a .actx subdirectory. Returns the path to the .actx directory,
// or empty string if not found.
// findProjectConfigDir 从 cwd 向上搜索包含 .actx 子目录的目录。
// 返回 .actx 目录的路径，如果未找到则返回空字符串。
func findProjectConfigDir(cwd string) string {
	dir := cwd
	for {
		cfgDir := filepath.Join(dir, ACTXgoConfigDir)
		if info, err := os.Stat(cfgDir); err == nil && info.IsDir() {
			return cfgDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root.
			// 已到达文件系统根目录。
			return ""
		}
		dir = parent
	}
}

// userCacheDir returns the path to the user's .actx cache directory.
// userCacheDir 返回用户 .actx 缓存目录的路径。
func userCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ACTXgoConfigDir)
}

// applyACTXGoConfig merges a ACTXgoConfig into a Config, overwriting
// existing values for fields that are set.
// applyACTXGoConfig 将 ACTXgoConfig 合并到 Config 中，覆盖已设置字段的现有值。
func applyACTXGoConfig(cfg *Config, c *ACTXgoConfig) {
	if c == nil {
		return
	}
	pc := c.resolveProvider()
	if pc == nil {
		return
	}
	if c.ModelProvider != "" {
		cfg.ModelProvider = c.ModelProvider
	}
	if len(pc.Models) > 0 {
		cfg.Models = pc.Models
		cfg.Model = pc.Models[0]
	}
	if pc.BaseURL != "" {
		cfg.BaseURL = pc.BaseURL
	}
}
