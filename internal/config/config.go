// Package config manages ~/.config/leeway/config.json (0600).
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// ProviderIDs are the built-in providers (registry order-independent).
var ProviderIDs = []string{"deepseek", "kimi", "grok", "codex", "opencode-go"}

type AccountConfig struct {
	Name               string `json:"name"`
	CookieSource       string `json:"cookieSource,omitempty"`
	ManualCookieHeader string `json:"manualCookieHeader,omitempty"`
	WorkspaceID        string `json:"workspaceID,omitempty"`
	Profile            string `json:"profile,omitempty"` // Chrome profile this account's session lives in
}

type ProviderConfig struct {
	Enabled            *bool           `json:"enabled,omitempty"` // nil = enabled
	CookieSource       string          `json:"cookieSource,omitempty"`
	ManualCookieHeader string          `json:"manualCookieHeader,omitempty"`
	Accounts           []AccountConfig `json:"accounts,omitempty"`
}

type Config struct {
	Providers map[string]ProviderConfig `json:"providers"`
	APIKeys   map[string]string         `json:"apiKeys,omitempty"`
}

// Path returns the config path; LEEWAY_CONFIG overrides it (test isolation).
func Path() (string, error) {
	if p := os.Getenv("LEEWAY_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "leeway", "config.json"), nil
}

// Load reads the config; a missing file yields a default config (and writes
// it so the file exists with the right permissions).
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		if err := cfg.Save(); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default seeds the provider map with the built-in IDs, all enabled by default.
func Default() *Config {
	providers := map[string]ProviderConfig{}
	for _, id := range ProviderIDs {
		providers[id] = ProviderConfig{}
	}
	return &Config{Providers: providers, APIKeys: map[string]string{}}
}

// Save writes the config atomically with 0600 permissions.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// IsEnabled reports whether a provider is enabled (default: enabled).
func (c *Config) IsEnabled(id string) bool {
	pc, ok := c.Providers[id]
	if !ok {
		return false
	}
	return pc.Enabled == nil || *pc.Enabled
}

// APIKey returns the stored API key for a provider (env vars take precedence
// and are resolved by the fetch layer).
func (c *Config) APIKey(id string) string {
	if c.APIKeys == nil {
		return ""
	}
	return c.APIKeys[id]
}

// SetAPIKey stores an API key and persists the config.
func (c *Config) SetAPIKey(id, key string) error {
	if c.APIKeys == nil {
		c.APIKeys = map[string]string{}
	}
	c.APIKeys[id] = key
	return c.Save()
}

// Accounts returns the configured accounts for a provider; falls back to the
// implicit "default" account.
func (c *Config) Accounts(id string) []AccountConfig {
	if pc, ok := c.Providers[id]; ok && len(pc.Accounts) > 0 {
		return pc.Accounts
	}
	return []AccountConfig{{Name: "default"}}
}

// Provider returns the config entry for a provider.
func (c *Config) Provider(id string) ProviderConfig {
	pc, ok := c.Providers[id]
	if !ok {
		return ProviderConfig{}
	}
	return pc
}

// ManualHeader returns the manual cookie header configured for an account,
// falling back to the provider-level manual header.
func (c *Config) ManualHeader(id, account string) string {
	if pc, ok := c.Providers[id]; ok {
		for _, a := range pc.Accounts {
			if a.Name == account && a.ManualCookieHeader != "" {
				return a.ManualCookieHeader
			}
		}
		if pc.ManualCookieHeader != "" {
			return pc.ManualCookieHeader
		}
	}
	return ""
}
