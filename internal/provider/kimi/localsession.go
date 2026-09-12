package kimi

// Local CLI session reuse: reads the Kimi Code CLI's OAuth session at
// ~/.kimi-code/credentials/kimi-code.json (KIMI_CODE_HOME overrides the
// directory), mirroring CodexBar's KimiSettingsReader.kimiCodeCredential.
//
// The access_token is a JWT and feeds the same web fetch path as the
// kimi-auth cookie token. If the token has expired the CLI must re-login
// (leeway does not implement the Kimi OAuth refresh).

import (
	"context"
	"encoding/base64"
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

// kimiOAuthHost returns the kimi-code OAuth server (KIMI_CODE_OAUTH_HOST
// overrides, mirroring KimiSettingsReader.codeOAuthHostEnvironmentKeys).
func kimiOAuthHost() string {
	if h := os.Getenv("KIMI_CODE_OAUTH_HOST"); h != "" {
		return strings.TrimSuffix(h, "/")
	}
	return "https://auth.kimi.com"
}

// kimiCodeHomeDir returns ~/.kimi-code (KIMI_CODE_HOME overrides,
// tilde-expanded), mirroring KimiSettingsReader.codeHomeURL.
func kimiCodeHomeDir() (string, error) {
	if custom := os.Getenv("KIMI_CODE_HOME"); custom != "" {
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
	return filepath.Join(home, ".kimi-code"), nil
}

// kimiCodeOAuthCredential mirrors KimiCodeOAuthCredential in Swift.
type kimiCodeOAuthCredential struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	ExpiresAt    float64 `json:"expires_at"` // unix seconds
	Scope        string  `json:"scope"`
	TokenType    string  `json:"token_type"`
}

// errSessionMissing distinguishes "no CLI session file" (falls back to the
// generic missing-token error) from "session exists but unusable" (surfaces
// its own hint).
var errSessionMissing = errors.New("kimi CLI session not found — run: leeway login kimi")

// errSessionExpired marks refresh failures of a present-but-expired session so
// callers can classify them with errors.Is instead of substring matching.
var errSessionExpired = errors.New("kimi CLI session expired")

// loadKimiCodeCredential reads the CLI session file. Returns a descriptive
// error when missing or unreadable.
func kimiCredentialPath(authFile string) (string, error) {
	if authFile != "" {
		return provider.LocalSessionPath(authFile)
	}
	dir, err := kimiCodeHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials", "kimi-code.json"), nil
}

func loadKimiCodeCredential(authFile string) (*kimiCodeOAuthCredential, string, error) {
	p, err := kimiCredentialPath(authFile)
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", errSessionMissing
	}
	if err != nil {
		return nil, "", err
	}
	var c kimiCodeOAuthCredential
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, "", fmt.Errorf("parsing kimi-code.json: %w", err)
	}
	if strings.TrimSpace(c.AccessToken) == "" {
		return nil, "", errors.New("kimi CLI session has no access_token — run: leeway login kimi")
	}
	return &c, p, nil
}

// fresh mirrors KimiSettingsReader.isKimiCodeCredentialFresh: the token is
// usable while it expires at least 60s in the future.
func (c *kimiCodeOAuthCredential) fresh(now time.Time) bool {
	return c.ExpiresAt > float64(now.Add(60*time.Second).Unix())
}

// localCLIToken returns the CLI session JWT, refreshing the OAuth session
// (auth.kimi.com/api/oauth/token) when it has expired, mirroring the kimi
// CLI's oauth-manager. Returns a re-login hint when refresh fails. ctx bounds
// the refresh request so a hung OIDC server cannot block the fetch past the
// orchestrator's deadline.
func localCLIToken(ctx context.Context, now time.Time, authFile string) (string, error) {
	cred, path, err := loadKimiCodeCredential(authFile)
	if err != nil {
		return "", err
	}
	if cred.fresh(now) {
		return strings.TrimSpace(cred.AccessToken), nil
	}
	if err := refreshKimiCredential(ctx, cred, path); err != nil {
		return "", err
	}
	return strings.TrimSpace(cred.AccessToken), nil
}

// refreshKimiCredential performs the refresh_token grant and writes the
// rotated tokens back to kimi-code.json. The client_id is decoded from the
// refresh_token JWT payload (the kimi CLI embeds it there). Failures wrap
// errSessionExpired so callers can classify them with errors.Is.
func refreshKimiCredential(ctx context.Context, cred *kimiCodeOAuthCredential, path string) error {
	clientID := jwtClaim(cred.RefreshToken, "client_id")
	if clientID == "" || cred.RefreshToken == "" {
		return fmt.Errorf("%w with no usable refresh token — run: leeway login kimi", errSessionExpired)
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", cred.RefreshToken)
	form.Set("client_id", clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kimiOAuthHost()+"/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "leeway")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w; refresh failed: %v — run: leeway login kimi", errSessionExpired, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w; refresh failed (HTTP %d) — run: leeway login kimi", errSessionExpired, resp.StatusCode)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("%w; refresh response unreadable: %v — run: leeway login kimi", errSessionExpired, err)
	}
	if body.AccessToken == "" || body.RefreshToken == "" {
		return fmt.Errorf("%w; refresh returned incomplete tokens — run: leeway login kimi", errSessionExpired)
	}
	cred.AccessToken = body.AccessToken
	cred.RefreshToken = body.RefreshToken
	cred.ExpiresAt = float64(time.Now().Unix() + body.ExpiresIn)
	if body.Scope != "" {
		cred.Scope = body.Scope
	}
	if body.TokenType != "" {
		cred.TokenType = body.TokenType
	}
	return cred.writeBack(path)
}

// writeBack persists the refreshed credential to kimi-code.json (0600),
// preserving any unknown fields. The write is atomic (temp file + rename) so
// a crash mid-write cannot truncate the session file the kimi CLI itself
// depends on.
func (c *kimiCodeOAuthCredential) writeBack(p string) error {
	raw, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	merged, err := json.Marshal(c)
	if err != nil {
		return err
	}
	var fresh map[string]json.RawMessage
	if err := json.Unmarshal(merged, &fresh); err != nil {
		return err
	}
	for k, v := range fresh {
		m[k] = v
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(p, out)
}

// atomicWriteFile writes data to path via a temp file in the same directory
// plus rename, so readers never observe a partially-written file.
func atomicWriteFile(path string, data []byte) error {
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

// jwtClaim decodes a JWT payload claim without verifying the signature.
func jwtClaim(token, claim string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload := parts[1]
	if pad := len(payload) % 4; pad != 0 {
		payload += strings.Repeat("=", 4-pad)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(payload, "="))
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return ""
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return ""
	}
	if v, ok := claims[claim].(string); ok {
		return v
	}
	return ""
}

// isExpiredSession matches the expired-session hint by sentinel rather than
// substring so unrelated messages containing "expired" cannot change the
// fallback behavior.
func isExpiredSession(err error) bool {
	return errors.Is(err, errSessionExpired)
}
