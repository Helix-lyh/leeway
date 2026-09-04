package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CursorAuth mirrors the official cursor-agent file credential store: with
// AGENT_CLI_CREDENTIAL_STORE=file the official CLI writes
// {accessToken, refreshToken, apiKey} to ~/.cursor/auth.json (0600). leeway
// never runs the browser sign-in itself — it asks the official CLI to do so
// inside a private HOME, then reads this file back.
type CursorAuth struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	APIKey       string `json:"apiKey"`
}

// CursorHome returns the private HOME leeway gives the official cursor-agent
// CLI for one account, so every account's login state stays independent of
// the official CLI/keychain and of every other leeway account.
func CursorHome(account string) (string, error) {
	dir, err := SessionDir()
	if err != nil {
		return "", err
	}
	name := "cursor"
	if account != "" && account != "default" {
		name += "-" + safeName(account)
	}
	return filepath.Join(dir, name), nil
}

// CursorAuthFile is where the official CLI writes credentials for an account.
func CursorAuthFile(account string) (string, error) {
	home, err := CursorHome(account)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cursor", "auth.json"), nil
}

// LoadCursorAuth reads an account's official auth.json. ok is false when no
// login has been done for the account yet; a present but unreadable file is
// an error so the user is told to re-login rather than silently falling back
// to another credential source.
func LoadCursorAuth(account string) (CursorAuth, bool, error) {
	p, err := CursorAuthFile(account)
	if err != nil {
		return CursorAuth{}, false, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return CursorAuth{}, false, nil
	}
	if err != nil {
		return CursorAuth{}, false, err
	}
	var ca CursorAuth
	if err := json.Unmarshal(raw, &ca); err != nil {
		return CursorAuth{}, false, fmt.Errorf("cursor auth %s is unreadable: %w", p, err)
	}
	return ca, true, nil
}

// RemoveCursorHome deletes an account's private HOME, dropping the credential
// the official CLI wrote there. The macOS keychain (the official CLI's own
// default store) is deliberately untouched — that state is not leeway's to
// revoke.
func RemoveCursorHome(account string) error {
	home, err := CursorHome(account)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(home); err != nil {
		return err
	}
	return nil
}

// ListCursorSessionAccounts returns account names that have a private
// auth.json under leeway's session dir (cursor → "default", cursor-work → "work").
// Login writes these directories even when config.json has no cursor accounts
// yet; status and usage must still see them.
func ListCursorSessionAccounts() ([]string, error) {
	dir, err := SessionDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		base := e.Name()
		var account string
		switch {
		case base == "cursor":
			account = "default"
		case strings.HasPrefix(base, "cursor-"):
			account = strings.TrimPrefix(base, "cursor-")
		default:
			continue
		}
		if account == "" {
			continue
		}
		p := filepath.Join(dir, base, ".cursor", "auth.json")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		names = append(names, account)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "default" {
			return true
		}
		if names[j] == "default" {
			return false
		}
		return names[i] < names[j]
	})
	return names, nil
}

// RelocateCursorHome moves one account's private HOME to another's name
// (used when login discovers the real email and the session was staged
// under "default"). Same-path is a no-op; an existing destination is replaced.
func RelocateCursorHome(from, to string) error {
	src, err := CursorHome(from)
	if err != nil {
		return err
	}
	dst, err := CursorHome(to)
	if err != nil {
		return err
	}
	if src == dst {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return os.Rename(src, dst)
}
