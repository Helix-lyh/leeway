// Package fetch orchestrates credential resolution and parallel probing of
// all enabled providers.
package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/model"
	"leeway/internal/provider"
)

// DefaultTimeout bounds each provider fetch (overridable via LEEWAY_TIMEOUT).
const DefaultTimeout = 15 * time.Second

// CacheTTL bounds how old a cached snapshot may be before it is refetched.
const CacheTTL = 60 * time.Second

// MaxAccountConcurrency bounds parallel account fetches per provider
// (anti-abuse on opencode.ai etc.).
const MaxAccountConcurrency = 3

type Options struct {
	UseCache       bool
	ProviderFilter string // "" = all
	AccountFilter  string // "" = all accounts
}

// maxTimeoutSeconds clamps LEEWAY_TIMEOUT so a huge value cannot overflow the
// int64-nanosecond conversion (which would produce a negative duration and an
// immediately-firing context deadline).
const maxTimeoutSeconds = 3600

func timeout() time.Duration {
	if raw := os.Getenv("LEEWAY_TIMEOUT"); raw != "" {
		if secs, err := strconv.ParseFloat(raw, 64); err == nil && secs > 0 {
			return time.Duration(math.Min(secs, maxTimeoutSeconds) * float64(time.Second))
		}
	}
	return DefaultTimeout
}

// ResolveCredentials resolves secrets for a provider: env vars first, then
// config file values, then cookie cache. Missing secrets are not an error —
// the provider (via HasCredentials) decides how to report "not_configured".
func ResolveCredentials(p provider.Provider, cfg *config.Config, account string) provider.Credentials {
	spec := p.AuthSpec()
	creds := provider.Credentials{}

	if len(spec.EnvAPIKey) > 0 {
		if v := lookupEnv(spec.EnvAPIKey); v != "" {
			creds.APIKey = v
		} else if k := cfg.APIKey(p.ID()); k != "" {
			creds.APIKey = k
		}
	}
	if len(spec.EnvAuthToken) > 0 {
		if v := lookupEnv(spec.EnvAuthToken); v != "" {
			creds.AuthToken = v
		} else if k := cfg.APIKey(p.ID()); k != "" {
			creds.AuthToken = k
		}
	}
	if spec.UsesCookies {
		if h, err := auth.ResolveCookieHeader(p.ID(), cfg.Provider(p.ID()), account); err == nil {
			creds.CookieHeader = h
		}
	}
	if spec.UsesCodexCLI {
		if cx, err := loadCodexAuth(); err == nil {
			creds.CodexAuth = cx
		}
	}
	return creds
}

func lookupEnv(keys []string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// HasCredentials reports whether enough secrets exist to attempt a fetch.
func HasCredentials(p provider.Provider, creds provider.Credentials) bool {
	spec := p.AuthSpec()
	if len(spec.EnvAPIKey) > 0 && creds.APIKey != "" {
		return true
	}
	if len(spec.EnvAuthToken) > 0 && creds.AuthToken != "" {
		return true
	}
	if spec.UsesCookies && creds.CookieHeader != "" {
		return true
	}
	if spec.UsesCodexCLI && creds.CodexAuth != nil && creds.CodexAuth.AccessToken != "" {
		return true
	}
	for _, f := range spec.LocalAuthFiles {
		if path, err := provider.HomePath(f); err == nil {
			if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
				return true
			}
		}
	}
	// Providers with no declared auth (should not happen) still get a chance.
	return len(spec.EnvAPIKey) == 0 && len(spec.EnvAuthToken) == 0 && !spec.UsesCookies && !spec.UsesCodexCLI && len(spec.LocalAuthFiles) == 0
}

// FetchAll probes every enabled provider concurrently and returns the
// assembled Output (and caches it).
func FetchAll(ctx context.Context, cfg *config.Config, opts Options) (*model.Output, error) {
	providers := provider.All()
	if opts.ProviderFilter != "" {
		p, ok := provider.Get(opts.ProviderFilter)
		if !ok {
			return nil, fmt.Errorf("unknown provider %q", opts.ProviderFilter)
		}
		providers = []provider.Provider{p}
	}

	// Cache path: return fresh cached data when allowed. The cache is only
	// used for unfiltered runs — a provider/account-filtered run must never
	// serve (or poison) a cache entry whose coverage differs from the request.
	if opts.UseCache && opts.ProviderFilter == "" && opts.AccountFilter == "" {
		if cached, err := loadSnapshotCache(); err == nil && cached != nil &&
			time.Since(cached.GeneratedAt) < CacheTTL {
			return cached, nil
		}
	}

	out := &model.Output{Version: 1, GeneratedAt: time.Now(), Providers: []model.ProviderResult{}}
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, p := range providers {
		if !cfg.IsEnabled(p.ID()) {
			// Disabled providers are omitted entirely: no data is probed and
			// no placeholder row is emitted, so `leeway usage` only shows the
			// providers the user actually wants monitored.
			continue
		}
		wg.Add(1)
		go func(p provider.Provider) {
			defer wg.Done()
			res := fetchProvider(ctx, p, cfg, opts)
			mu.Lock()
			out.Providers = append(out.Providers, res)
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	sortProviders(out)
	if opts.UseCache && opts.ProviderFilter == "" && opts.AccountFilter == "" {
		if err := saveSnapshotCache(out); err != nil {
			// Data is still valid; a cache write failure is non-fatal. Log it
			// so the failure is visible without failing the whole run.
			log.Printf("cache: snapshot cache write failed (data is still valid): %v", err)
		}
	}
	return out, nil
}

func fetchProvider(ctx context.Context, p provider.Provider, cfg *config.Config, opts Options) model.ProviderResult {
	res := model.ProviderResult{ID: p.ID(), DisplayName: p.DisplayName(), Status: "not_configured", UpdatedAt: time.Now()}
	accounts := cfg.Accounts(p.ID())
	if opts.AccountFilter != "" {
		filtered := accounts[:0]
		for _, a := range accounts {
			if a.Name == opts.AccountFilter {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			res.Status = "error"
			res.Error = fmt.Sprintf("account %q not configured", opts.AccountFilter)
			return res
		}
		accounts = filtered
	}

	creds := ResolveCredentials(p, cfg, "")
	hasCreds := HasCredentials(p, creds)

	type accResult struct {
		acc config.AccountConfig
		res model.AccountResult
	}
	results := make([]accResult, len(accounts))
	sem := make(chan struct{}, MaxAccountConcurrency)
	var wg sync.WaitGroup

	for i, acc := range accounts {
		wg.Add(1)
		go func(i int, acc config.AccountConfig) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ar := model.AccountResult{Name: acc.Name, Status: "not_configured"}
			if !hasCreds {
				if acc.ManualCookieHeader == "" && acc.APIKey == "" && creds.CookieHeader == "" && creds.APIKey == "" && creds.AuthToken == "" && creds.CodexAuth == nil {
					ar.Error = missingCredentialHint(p.ID(), p.AuthSpec())
					results[i] = accResult{acc, ar}
					return
				}
			}
			req := provider.FetchRequest{Creds: creds, Account: acc.Name}
			if acc.WorkspaceID != "" {
				req.Extra = map[string]string{"workspaceID": acc.WorkspaceID}
			}
			req.Creds = overlayAccountCreds(creds, acc, resolveAccountCookie(p.ID(), cfg, acc, creds.CookieHeader))

			actx, cancel := context.WithTimeout(ctx, timeout())
			defer cancel()
			snap, err := p.Fetch(actx, req)
			if err != nil {
				ar.Status = "error"
				ar.Error = err.Error()
			} else {
				ar.Status = "ok"
				ar.Snapshot = snap
			}
			results[i] = accResult{acc, ar}
		}(i, acc)
	}
	wg.Wait()

	for _, r := range results {
		res.Accounts = append(res.Accounts, r.res)
		if r.res.Status == "error" {
			res.Status = "error"
			if res.Error == "" {
				res.Error = r.res.Error
			}
		} else if r.res.Status == "ok" && res.Status != "error" {
			res.Status = "ok"
			res.UpdatedAt = r.res.Snapshot.UpdatedAt
		} else if r.res.Status == "not_configured" && res.Status != "error" && res.Status != "ok" {
			res.Status = "not_configured"
			res.Error = r.res.Error
		}
	}

	return res
}

// overlayAccountCreds applies per-account secrets. Named apiKey wins;
// cookie/workspace accounts drop the inherited provider/env key so they
// stay on the cookie path.
func overlayAccountCreds(base provider.Credentials, acc config.AccountConfig, cookieHeader string) provider.Credentials {
	creds := base
	creds.CookieHeader = cookieHeader
	if acc.APIKey != "" {
		creds.APIKey = acc.APIKey
	} else if acc.ManualCookieHeader != "" || acc.Profile != "" || acc.WorkspaceID != "" || cookieHeader != "" {
		creds.APIKey = ""
	}
	return creds
}

// resolveAccountCookie resolves the cookie header for one account: the
// account's manual header wins, then the account-bound profile's imported
// session ("opencode-go@Profile 1"), then the provider-level header.
func resolveAccountCookie(providerID string, cfg *config.Config, acc config.AccountConfig, providerLevel string) string {
	if acc.ManualCookieHeader != "" {
		return acc.ManualCookieHeader
	}
	if acc.Profile != "" {
		if cc, ok, err := auth.LoadCookieCache(providerID, acc.Profile); err == nil && ok && cc.Header != "" {
			return cc.Header
		}
	}
	return providerLevel
}

func missingCredentialHint(id string, spec provider.AuthSpec) string {
	switch {
	case len(spec.EnvAPIKey) > 0 && spec.UsesCookies:
		return fmt.Sprintf("missing credentials — set %s or: leeway config set-api-key --provider %s --account NAME (or: leeway cookie import --provider %s)", spec.EnvAPIKey[0], id, id)
	case len(spec.EnvAPIKey) > 0:
		return fmt.Sprintf("missing credentials — set %s or: leeway config set-api-key", spec.EnvAPIKey[0])
	case spec.UsesCookies:
		return "missing cookies — run: leeway cookie import (or: leeway cookie set --manual ...)"
	case spec.UsesCodexCLI:
		return "missing Codex session — run: codex login"
	case len(spec.LocalAuthFiles) > 0 && spec.LoginHint != "":
		return "missing local session — " + spec.LoginHint
	default:
		return "missing credentials"
	}
}

func sortProviders(out *model.Output) {
	sort.Slice(out.Providers, func(i, j int) bool { return out.Providers[i].ID < out.Providers[j].ID })
}

// --- snapshot cache -------------------------------------------------------

func snapshotCachePath() (string, error) {
	dir, err := auth.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "snapshots.json"), nil
}

func loadSnapshotCache() (*model.Output, error) {
	p, err := snapshotCachePath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &model.Output{}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveSnapshotCache(out *model.Output) error {
	p, err := snapshotCachePath()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(p); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}
