// Package auth resolves cookie headers for cookie-based providers.
//
// Sources, by preference: manual cookie header (config) > imported browser
// cookies (~/.cache/leeway/cookies.json, populated by `leeway login --cookie`).
// Browser importers (chrome/edge/kimi-desktop) live in this package.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"leeway/internal/config"
)

// ImportTTL bounds the freshness of imported cookies; `login --cookie --force`
// refreshes regardless.
const ImportTTL = 12 * time.Hour

// ErrCookieCacheUnreadable is returned when cookies.json exists but cannot
// be parsed or read. Missing cookies are a different error.
var ErrCookieCacheUnreadable = errors.New("cookie cache unreadable")

// CookieCache is one imported cookie header stored on disk.
type CookieCache struct {
	ProviderID string    `json:"providerID"`
	Profile    string    `json:"profile,omitempty"` // Chrome profile this session came from
	Header     string    `json:"header"`
	Browser    string    `json:"browser"`
	ImportedAt time.Time `json:"importedAt"`
}

// CacheDir returns ~/.cache/leeway (created on demand).
func CacheDir() (string, error) {
	if p := os.Getenv("LEEWAY_CACHE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "leeway"), nil
}

func cacheFile() (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cookies.json"), nil
}

func readCache() (map[string]CookieCache, error) {
	p, err := cacheFile()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]CookieCache{}, nil
	}
	if err != nil {
		return nil, err
	}
	cache := map[string]CookieCache{}
	if err := json.Unmarshal(raw, &cache); err != nil {
		return nil, err
	}
	return cache, nil
}

func writeCache(cache map[string]CookieCache) error {
	p, err := cacheFile()
	if err != nil {
		return err
	}
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}

// cacheKey namespaces the cookie cache by (providerID, profile) so multiple
// browser sessions can coexist per provider: "grok" or "grok@Profile 1".
func cacheKey(providerID, profile string) string {
	if profile == "" {
		return providerID
	}
	return providerID + "@" + profile
}

// LoadCookieCache returns the stored cookie header for a provider (optionally
// from a specific browser profile). The error distinguishes "nothing stored"
// (ok=false, err=nil) from "cache exists but is corrupt or unreadable" — the
// latter should be surfaced so the user is not told to re-import cookies they
// already imported.
func LoadCookieCache(providerID, profile string) (CookieCache, bool, error) {
	cache, err := readCache()
	if err != nil {
		return CookieCache{}, false, err
	}
	cc, ok := cache[cacheKey(providerID, profile)]
	return cc, ok, nil
}

// SaveCookieCache persists an imported cookie header (optionally per profile).
func SaveCookieCache(providerID, profile, header, browser string) error {
	cache, err := readCache()
	if err != nil {
		return err
	}
	cache[cacheKey(providerID, profile)] = CookieCache{
		ProviderID: providerID,
		Profile:    profile,
		Header:     header,
		Browser:    browser,
		ImportedAt: time.Now(),
	}
	return writeCache(cache)
}

// ClearCookieCache removes the stored cookie header for a provider
// (optionally for a specific profile; profile "" clears the provider-level
// entry only).
func ClearCookieCache(providerID, profile string) error {
	cache, err := readCache()
	if err != nil {
		return err
	}
	delete(cache, cacheKey(providerID, profile))
	return writeCache(cache)
}

// AllCookieCaches lists every imported cookie header in stable order
// (provider ID, then import time).
func AllCookieCaches() ([]CookieCache, error) {
	cache, err := readCache()
	if err != nil {
		return nil, err
	}
	out := make([]CookieCache, 0, len(cache))
	for _, cc := range cache {
		out = append(out, cc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProviderID != out[j].ProviderID {
			return out[i].ProviderID < out[j].ProviderID
		}
		return out[i].ImportedAt.Before(out[j].ImportedAt)
	})
	return out, nil
}

// ResolveCookieHeader resolves a provider's cookie header: manual config
// first, then the imported cache (provider-level, or profile-scoped when the
// caller's account names a profile). Returns a descriptive error when
// absent. Both sources are sanitized: cookies with control characters (which
// Go's HTTP client rejects) are dropped.
func ResolveCookieHeader(providerID string, pc config.ProviderConfig, account string) (string, error) {
	if h := manualHeaderFor(providerID, pc, account); h != "" {
		return sanitizeHeader(h), nil
	}
	// Account-scoped profile first, then provider-level cache.
	for _, a := range pc.Accounts {
		if a.Name == account && a.Profile != "" {
			cc, ok, err := LoadCookieCache(providerID, a.Profile)
			if err != nil {
				return "", cacheUnreadableError(err)
			}
			if ok && cc.Header != "" {
				return sanitizeHeader(cc.Header), nil
			}
		}
	}
	cc, ok, err := LoadCookieCache(providerID, "")
	if err != nil {
		return "", cacheUnreadableError(err)
	}
	if ok && cc.Header != "" {
		return sanitizeHeader(cc.Header), nil
	}
	return "", fmt.Errorf("no cookie credentials for %q — run: leeway login %s --cookie (or: leeway login %s --manual ...)", providerID, providerID, providerID)
}

// cacheUnreadableError distinguishes a corrupt/unreadable cookie cache from
// "nothing imported", so the user is not told to re-import cookies that are
// already stored but unreadable.
func cacheUnreadableError(err error) error {
	p, perr := cacheFile()
	if perr != nil {
		p = "(unresolvable)"
	}
	return fmt.Errorf("%w: %v — delete %s and re-run leeway login --cookie", ErrCookieCacheUnreadable, err, p)
}

// RemoveCookieCacheFile deletes cookies.json. A missing file is success.
func RemoveCookieCacheFile() error {
	p, err := cacheFile()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// sanitizeHeader drops individual cookies whose name or value contains
// control characters, preserving the rest.
func sanitizeHeader(h string) string {
	parts := strings.Split(h, ";")
	kept := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		eq := strings.IndexByte(p, '=')
		if eq <= 0 {
			continue
		}
		if hasControl(p[:eq]) || hasControl(p[eq+1:]) {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "; ")
}

func manualHeaderFor(providerID string, pc config.ProviderConfig, account string) string {
	for _, a := range pc.Accounts {
		if a.Name == account && a.ManualCookieHeader != "" {
			return a.ManualCookieHeader
		}
	}
	return pc.ManualCookieHeader
}

// Import imports cookies for a provider from a browser and returns the
// "name=value; name=value" header. Browsers: "chrome" (default), "edge",
// "kimi-desktop" (Kimi desktop app's own store), or "manual" (use
// `leeway login <id> --manual ...` instead). profile names a
// specific Chrome profile ("Default", "Profile 1", …); empty auto-picks the
// profile with the most matching cookies. Imported headers are cached with
// ImportTTL; `leeway login --cookie --force` refreshes regardless. Concrete
// importers live in chrome.go and kimi_desktop.go.
func Import(providerID, browser, profile string) (string, error) {
	return importImpl(providerID, browser, profile)
}

// curlHeaderPatterns match curl-style header fragments people paste from
// browser devtools, mirroring CookieHeaderNormalizer.headerPatterns. This is
// the shared superset used by the kimi and opencode-go providers.
var curlHeaderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-H\s*'Cookie:\s*([^']+)'`),
	regexp.MustCompile(`(?i)-H\s*"Cookie:\s*([^"]+)"`),
	regexp.MustCompile(`(?i)\bcookie:\s*'([^']+)'`),
	regexp.MustCompile(`(?i)\bcookie:\s*"([^"]+)"`),
	regexp.MustCompile(`(?i)\bcookie:\s*([^\r\n]+)`),
	regexp.MustCompile(`(?i)(?:^|\s)(?:--cookie|-b)\s*'([^']+)'`),
	regexp.MustCompile(`(?i)(?:^|\s)(?:--cookie|-b)\s*"([^"]+)"`),
	regexp.MustCompile(`(?i)(?:^|\s)-b([^\s=]+=[^\s]+)`),
	regexp.MustCompile(`(?i)(?:^|\s)(?:--cookie|-b)\s+([^\s]+)`),
}

// ExtractCookieHeader pulls the "name=value; name=value" cookie header out of
// a pasted curl fragment ("-H 'Cookie: ...'", "--cookie", "-b"), a bare
// "cookie: ..." dump, or an already-clean header. Returns "" when nothing
// matches. Shared by the kimi and opencode-go providers so a quoting fix
// reaches both.
func ExtractCookieHeader(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	for _, re := range curlHeaderPatterns {
		if m := re.FindStringSubmatch(value); m != nil && len(m) > 1 {
			if captured := strings.TrimSpace(m[1]); captured != "" {
				return captured
			}
		}
	}
	return ""
}
