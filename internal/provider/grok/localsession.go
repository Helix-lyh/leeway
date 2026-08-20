package grok

// Local CLI session reuse: reads the `grok login` session at ~/.grok/auth.json
// (GROK_HOME overrides the directory), mirroring CodexBar's GrokAuth.swift.
//
// The file is a map keyed by OIDC scope URL; entries carry the bearer `key`,
// `refresh_token`, `expires_at`, `principal_type` and the OIDC issuer/client
// id. When the bearer has expired, a standard OIDC refresh is attempted
// against the issuer's token endpoint; the refreshed entry is written back
// (format-preserving, 0600) so the grok CLI and leeway share one session.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"leeway/internal/provider"
)

// grokHomeDir returns ~/.grok (GROK_HOME overrides, tilde-expanded), mirroring
// GrokAuth.grokHomeURL.
func grokHomeDir() (string, error) {
	if custom := os.Getenv("GROK_HOME"); custom != "" {
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
	return filepath.Join(home, ".grok"), nil
}

type grokSessionEntry struct {
	Key           string  `json:"key"`
	RefreshToken  string  `json:"refresh_token"`
	ExpiresAt     *string `json:"expires_at"`
	PrincipalType string  `json:"principal_type"`
	Email         string  `json:"email"`
	OIDCIssuer    string  `json:"oidc_issuer"`
	OIDCClientID  string  `json:"oidc_client_id"`
}

// grokSession pairs a selected entry with its scope key in the file so a
// refreshed entry can be written back to the right slot.
type grokSession struct {
	path  string // auth.json path this entry was loaded from (write-back target)
	scope string
	entry *grokSessionEntry
}

// sessionPath resolves the auth.json to read: an explicit authFile (account
// override, tilde-expanded) wins, otherwise $GROK_HOME/auth.json, else
// ~/.grok/auth.json.
func sessionPath(authFile string) (string, error) {
	if authFile != "" {
		return provider.HomePath(authFile)
	}
	dir, err := grokHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

// loadGrokSession reads the auth file and selects the usable entry: the OIDC
// scope entry (prefix "https://auth.x.ai::") is preferred, otherwise the
// first entry carrying a non-empty key (GrokAuth selection semantics).
// authFile overrides the default location for multi-account setups.
func loadGrokSession(authFile string) (*grokSession, error) {
	p, err := sessionPath(authFile)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("grok auth.json not found (%s) — run: grok login", p)
	}
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing grok auth.json: %w", err)
	}
	var oidc, first *grokSession
	for scope, rawEntry := range m {
		var e grokSessionEntry
		if err := json.Unmarshal(rawEntry, &e); err != nil || e.Key == "" {
			continue
		}
		if strings.HasPrefix(scope, "https://auth.x.ai::") {
			oidc = &grokSession{path: p, scope: scope, entry: &e}
			break
		}
		if first == nil {
			first = &grokSession{path: p, scope: scope, entry: &e}
		}
	}
	if oidc != nil {
		return oidc, nil
	}
	if first != nil {
		return first, nil
	}
	return nil, errors.New("grok auth.json contains no usable access tokens — run: grok login")
}

func (e *grokSessionEntry) expired(now time.Time) bool {
	if e.ExpiresAt == nil || *e.ExpiresAt == "" {
		return false // no expiry recorded — assume usable
	}
	t, err := time.Parse(time.RFC3339Nano, *e.ExpiresAt)
	if err != nil {
		return false
	}
	return now.After(t.Add(-30 * time.Second)) // 30s clock-skew grace
}

// refresh performs the OIDC refresh_token grant against the issuer's token
// endpoint (Ory layout: {issuer}/oauth2/token), mirroring what the grok CLI
// itself does. On success the entry and the file are updated. ctx bounds the
// request so an unresponsive issuer cannot block the fetch past the
// orchestrator's deadline.
func (s *grokSession) refresh(ctx context.Context) error {
	e := s.entry
	if e.OIDCIssuer == "" || e.RefreshToken == "" {
		return errors.New("grok session has no refresh_token/issuer — run: grok login")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", e.RefreshToken)
	form.Set("client_id", e.OIDCClientID)
	form.Set("scope", "openid offline_access profile email")
	endpoint := strings.TrimSuffix(e.OIDCIssuer, "/") + "/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("grok session refresh failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("grok session refresh failed (HTTP %d) — run: grok login", resp.StatusCode)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("grok session refresh response unreadable: %w", err)
	}
	if body.AccessToken == "" {
		return errors.New("grok session refresh returned no access_token — run: grok login")
	}
	e.Key = body.AccessToken
	if body.RefreshToken != "" {
		e.RefreshToken = body.RefreshToken
	}
	if body.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(body.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano)
		e.ExpiresAt = &exp
	}
	return s.writeBack()
}

// writeBack replaces the selected entry in its auth.json, preserving all
// other entries, then restores 0600 permissions. The write is atomic (temp
// file + rename) so a crash mid-write cannot truncate the session file the
// grok CLI itself depends on.
func (s *grokSession) writeBack() error {
	p := s.path
	if p == "" {
		dir, err := grokHomeDir()
		if err != nil {
			return err
		}
		p = filepath.Join(dir, "auth.json")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	b, err := json.Marshal(s.entry)
	if err != nil {
		return err
	}
	m[s.scope] = b
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p, out)
}

// writeFileAtomic writes data to path via a temp file in the same directory
// plus rename, so readers never observe a partially-written file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".leeway-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// bearerTokenFromLocalSession returns a usable bearer token for the web
// billing call, refreshing when needed. Errors carry a re-login hint. ctx
// bounds the refresh request so a hung OIDC issuer cannot block the fetch.
// authFile overrides the default auth.json location (multi-account setups).
func bearerTokenFromLocalSession(ctx context.Context, now time.Time, authFile string) (string, error) {
	sess, err := loadGrokSession(authFile)
	if err != nil {
		return "", err
	}
	if sess.entry.expired(now) {
		if err := sess.refresh(ctx); err != nil {
			return "", err
		}
	}
	return sess.entry.Key, nil
}
