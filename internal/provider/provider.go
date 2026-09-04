// Package provider defines the extension-point contracts of leeway.
//
// A new provider is added by implementing Provider, registering it in init()
// (see the stub packages in internal/provider/<name>), and declaring its
// auth requirements in AuthSpec — the orchestrator, doctor and cookie
// commands derive everything else from AuthSpec.
package provider

import (
	"context"
	"os"
	"path/filepath"

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

// AuthSpec declaratively describes how a provider authenticates. Doctor and
// credential resolution use it to decide "not_configured" vs "error".
type AuthSpec struct {
	EnvAPIKey        []string // env var names checked in order (e.g. ["DEEPSEEK_API_KEY"])
	EnvAuthToken     []string // env var names for JWT/session tokens
	UsesCookies      bool     // provider fetches with a Cookie header
	CookieDomains    []string // e.g. ["grok.com"]
	CookieFallbacks  []string // extra auth fallbacks surfaced by doctor, e.g. "kimi-desktop"
	UsesCodexCLI     bool     // reuse ~/.codex/auth.json session
	UsesKeychain     bool     // provider resolves its token from the macOS keychain itself (cursor)
	LocalAuthFiles   []string // provider-owned CLI session files (e.g. "~/.grok/auth.json"), resolved by the provider itself
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
