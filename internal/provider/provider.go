// Package provider defines the extension-point contracts of leeway.
//
// A new provider is added by implementing Provider, registering it in init()
// (see the packages in internal/provider/<name>), and declaring its auth
// requirements in AuthSpec. Credential checks and the window read AuthSpec,
// while some providers resolve extra sources (private auth.json, GROK_HOME /
// KIMI_CODE_HOME, keychain) inside Fetch.
package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"leeway/internal/model"
)

// WindowKind distinguishes percent-style windows from raw balance windows.
type WindowKind int

const (
	PercentWindow WindowKind = iota
	BalanceWindow
)

// WindowSpec declares a window lane a provider can produce.
type WindowSpec struct {
	ID            string
	Title         string
	WindowMinutes *int
	Kind          WindowKind
}

// AuthSpec declaratively describes how a provider authenticates. Credential
// resolution and the window use it to decide "not_configured" vs "error".
type AuthSpec struct {
	EnvAPIKey        []string // env var names checked in order (e.g. ["DEEPSEEK_API_KEY"])
	EnvAuthToken     []string // env var names for JWT/session tokens
	UsesCookies      bool     // provider fetches with a Cookie header
	CookieDomains    []string // e.g. ["grok.com"]
	CookieFallbacks  []string // extra auth fallbacks surfaced by doctor, e.g. "kimi-desktop"
	UsesCodexCLI     bool     // reuse ~/.codex/auth.json session
	UsesKeychain     bool     // provider resolves its token from the macOS keychain itself (cursor)
	LocalAuthFiles   []string // provider-owned CLI session files (e.g. "~/.grok/auth.json"), resolved by the provider itself
	OfficialCLI      bool     // an official CLI (`leeway login` runs it) signs in: cursor/grok/codex/kimi
	LoginHint        string   // command to restore the session, e.g. "run: grok login"
	SupportsAccounts bool     // per-account fetch loop (opencode-go)
}

// HomePath expands a tilde-prefixed path relative to the user's home.
func HomePath(p string) (string, error) {
	if len(p) == 0 || p[0] != '~' {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[1:]), nil
}

// LocalSessionPath resolves a LocalAuthFiles declaration. GROK_HOME and
// KIMI_CODE_HOME override the default homes the same way the grok and kimi
// Fetch paths do.
func LocalSessionPath(declared string) (string, error) {
	switch declared {
	case "~/.grok/auth.json":
		dir, err := envHomeDir("GROK_HOME", ".grok")
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "auth.json"), nil
	case "~/.kimi-code/credentials/kimi-code.json":
		dir, err := envHomeDir("KIMI_CODE_HOME", ".kimi-code")
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "credentials", "kimi-code.json"), nil
	default:
		return HomePath(declared)
	}
}

func envHomeDir(envKey, defaultRel string) (string, error) {
	if custom := os.Getenv(envKey); custom != "" {
		if strings.HasPrefix(custom, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, custom[2:]), nil
		}
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultRel), nil
}

// CodexAuth is the parsed ~/.codex/auth.json session.
type CodexAuth struct {
	AccessToken string
	AccountID   string
}

// Credentials is the resolved set of secrets handed to Fetch. The fetch
// orchestrator resolves them from env, config and the cookie cache.
type Credentials struct {
	APIKey       string
	AuthToken    string
	CookieHeader string
	CodexAuth    *CodexAuth
}

// FetchRequest is one account's fetch invocation.
type FetchRequest struct {
	Creds       Credentials
	Account     string
	WorkspaceID string
	AuthFile    string
	Email       string
}

// Provider is the unit of extension.
type Provider interface {
	ID() string
	DisplayName() string
	AuthSpec() AuthSpec
	DeclaredWindows() []WindowSpec
	Fetch(ctx context.Context, req FetchRequest) (*model.Snapshot, error)
}
