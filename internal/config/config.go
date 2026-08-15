// Package config manages ~/.config/leeway/config.json (0600).
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ProviderIDs are the built-in providers (registry order-independent).
var ProviderIDs = []string{"deepseek", "kimi", "grok", "codex", "opencode-go"}

type AccountConfig struct {
	Name               string `json:"name"`
	CookieSource       string `json:"cookieSource,omitempty"`
	ManualCookieHeader string `json:"manualCookieHeader,omitempty"`
	WorkspaceID        string `json:"workspaceID,omitempty"`
	Profile            string `json:"profile,omitempty"` // Chrome profile this account's session lives in
	APIKey             string `json:"apiKey,omitempty"`  // API-key subscription (opencode-go); overrides provider-level key
}

// KeyTarget is where `leeway use` writes the active subscription's API key:
// the YAML file path and the credential reference (key name) inside it.
type KeyTarget struct {
	File string `json:"file,omitempty"`
	Key  string `json:"key,omitempty"`
}

type ProviderConfig struct {
	Enabled            *bool           `json:"enabled,omitempty"` // nil = enabled
	CookieSource       string          `json:"cookieSource,omitempty"`
	ManualCookieHeader string          `json:"manualCookieHeader,omitempty"`
	Accounts           []AccountConfig `json:"accounts,omitempty"`
	KeyTarget          *KeyTarget      `json:"keyTarget,omitempty"` // nil = DSH default
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

// Account returns the named account config for a provider (ok false when absent).
func (c *Config) Account(id, name string) (AccountConfig, bool) {
	for _, a := range c.Providers[id].Accounts {
		if a.Name == name {
			return a, true
		}
	}
	return AccountConfig{}, false
}

// SetAccountKey stores an API key on a named account, creating the account
// entry if it does not exist, and persists the config.
func (c *Config) SetAccountKey(id, name, key string) error {
	pc := c.Provider(id)
	for i := range pc.Accounts {
		if pc.Accounts[i].Name == name {
			pc.Accounts[i].APIKey = key
			c.Providers[id] = pc
			return c.Save()
		}
	}
	pc.Accounts = append(pc.Accounts, AccountConfig{Name: name, APIKey: key})
	c.Providers[id] = pc
	return c.Save()
}

// KeyTarget returns the target file/key for `leeway use` on a provider.
// Configured file/key overlay the DSH default independently; a `~/` file
// expands to the user's home. Default credential reference is the provider
// id uppercased, e.g. "opencode-go" → "OPENCODE_GO_API_KEY".
func (c *Config) KeyTarget(id string) KeyTarget {
	got := defaultKeyTarget(id)
	if pc := c.Provider(id); pc.KeyTarget != nil {
		if pc.KeyTarget.File != "" {
			got.File = expandHome(pc.KeyTarget.File)
		}
		if pc.KeyTarget.Key != "" {
			got.Key = pc.KeyTarget.Key
		}
	}
	return got
}

func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}

func defaultKeyTarget(id string) KeyTarget {
	key := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(id)) + "_API_KEY"
	return KeyTarget{File: defaultDSHCredentialsPath(), Key: key}
}

// defaultDSHCredentialsPath resolves the DeepSeek Harness credentials file:
// $DSH_HOME/.credentials.yaml, defaulting to ~/.dsh.
func defaultDSHCredentialsPath() string {
	if h := os.Getenv("DSH_HOME"); h != "" {
		return filepath.Join(h, ".credentials.yaml")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".dsh", ".credentials.yaml")
	}
	return ".credentials.yaml"
}
