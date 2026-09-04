// Package auth resolves credentials for leeway's providers.
//
// It owns two things: the private per-account directories under
// ~/.config/leeway/sessions that isolate one provider sign-in from another,
// and the interactive login flows that fill them. Cursor credentials are the
// official client's auth.json (leeway only reads what cursor-agent wrote);
// grok credentials live in a private GROK_HOME the official grok CLI wrote.
package auth

import (
	"os"
	"path/filepath"
	"strings"
)

// SessionDir returns the directory holding leeway-owned sessions
// (~/.config/leeway/sessions). LEEWAY_SESSIONS overrides it for tests.
func SessionDir() (string, error) {
	if p := os.Getenv("LEEWAY_SESSIONS"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "leeway", "sessions"), nil
}

// safeName reduces a provider or account name to filename-safe characters so
// a name like "../../etc/passwd" cannot escape the session directory.
func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
}
