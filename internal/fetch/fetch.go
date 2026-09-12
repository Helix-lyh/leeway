// Package fetch orchestrates credential resolution and parallel probing of
// all enabled providers.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
// the orchestrator uses canAttemptFetch to decide whether to mark
// not_configured. An unreadable cookie cache or Codex auth.json is returned
// as an error.
func ResolveCredentials(p provider.Provider, cfg *config.Config, account string) (provider.Credentials, error) {
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
		h, err := auth.ResolveCookieHeader(p.ID(), cfg.Provider(p.ID()), account)
		if errors.Is(err, auth.ErrCookieCacheUnreadable) {
			return creds, err
		}
		if err == nil {
			creds.CookieHeader = h
		}
	}
	if spec.UsesCodexCLI {
		cx, err := loadCodexAuth()
		if err == nil {
			creds.CodexAuth = cx
		} else if !errors.Is(err, os.ErrNotExist) {
			return creds, fmt.Errorf("Codex auth.json unreadable: %w — run: leeway login codex", err)
		}
	}
	return creds, nil
}

func lookupEnv(keys []string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// canAttemptFetch reports whether the orchestrator should attempt Fetch
// (enough secrets, or UsesKeychain so Fetch can emit the login hint).
func canAttemptFetch(p provider.Provider, creds provider.Credentials) bool {
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
	if spec.UsesKeychain {
		// The provider resolves its token from the macOS keychain itself;
		// treat it as provisionally present and let Fetch report the real
		// failure (missing/expired) so the error carries a login hint.
		return true
	}
	for _, f := range spec.LocalAuthFiles {
		if path, err := provider.LocalSessionPath(f); err == nil {
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
	fp := ""
	if opts.UseCache && opts.ProviderFilter == "" && opts.AccountFilter == "" {
		fp = cacheFingerprint(cfg)
		if cached, err := loadSnapshotCache(fp); err == nil && cached != nil &&
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
			if len(res.Accounts) == 0 && res.Status != "error" {
				return
			}
			mu.Lock()
			out.Providers = append(out.Providers, res)
			mu.Unlock()
		}(p)
	}
	wg.Wait()

	sortProviders(out)
	if opts.UseCache && opts.ProviderFilter == "" && opts.AccountFilter == "" {
		if fp == "" {
			fp = cacheFingerprint(cfg)
		}
		if err := saveSnapshotCache(fp, out); err != nil {
			// Data is still valid; a cache write failure is non-fatal. Log it
			// so the failure is visible without failing the whole run.
			log.Printf("cache: snapshot cache write failed (data is still valid): %v", err)
		}
	}
	return out, nil
}

func fetchProvider(ctx context.Context, p provider.Provider, cfg *config.Config, opts Options) model.ProviderResult {
	res := model.ProviderResult{ID: p.ID(), DisplayName: p.DisplayName(), Status: "not_configured", UpdatedAt: time.Now()}
	accounts := accountsFor(p, cfg, opts.AccountFilter)
	if opts.AccountFilter != "" && len(accounts) == 0 {
		res.Status = "error"
		res.Error = fmt.Sprintf("account %q not configured", opts.AccountFilter)
		return res
	}
	if len(accounts) == 0 {
		return res
	}

	creds, credsErr := ResolveCredentials(p, cfg, "")

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
			header, cookieErr := auth.ResolveCookieHeader(p.ID(), cfg.Provider(p.ID()), acc.Name)
			if errors.Is(cookieErr, auth.ErrCookieCacheUnreadable) {
				ar.Status = "error"
				ar.Error = cookieErr.Error()
				results[i] = accResult{acc, ar}
				return
			}
			if cookieErr != nil {
				header = ""
			}
			req := provider.FetchRequest{
				Creds:       overlayAccountCreds(creds, acc, header),
				Account:     acc.Name,
				WorkspaceID: acc.WorkspaceID,
				AuthFile:    acc.AuthFile,
				Email:       acc.Email,
			}
			if p.AuthSpec().UsesCodexCLI && acc.AuthFile != "" {
				cx, err := loadCodexAuthFile(acc.AuthFile)
				if err == nil {
					req.Creds.CodexAuth = cx
				} else if !errors.Is(err, os.ErrNotExist) {
					ar.Status = "error"
					ar.Error = err.Error()
					results[i] = accResult{acc, ar}
					return
				}
			}
			if !canAttemptFetch(p, req.Creds) && acc.AuthFile == "" {
				if credsErr != nil && !errors.Is(credsErr, auth.ErrCookieCacheUnreadable) {
					ar.Status = "error"
					ar.Error = credsErr.Error()
					results[i] = accResult{acc, ar}
					return
				}
				ar.Error = missingCredentialHint(p.ID(), p.AuthSpec())
				results[i] = accResult{acc, ar}
				return
			}

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

// accountsFor lists accounts to probe. Named config/session accounts only;
// the implicit "default" (keychain, ~/.grok shared) is skipped unless the
// user passed --account default. Providers that are not multi-account
// (kimi, codex, deepseek) still get one unnamed session so a single CLI
// login keeps working.
func accountsFor(p provider.Provider, cfg *config.Config, filter string) []config.AccountConfig {
	accs := cfg.Accounts(p.ID())
	if p.ID() == "cursor" {
		if extra, err := auth.ListCursorSessionAccounts(); err == nil {
			accs = config.MergeNamedAccounts(accs, extra)
		}
	}
	if filter != "" {
		var hit []config.AccountConfig
		for _, a := range accs {
			if a.Matches(filter) {
				hit = append(hit, a)
			}
		}
		if len(hit) == 0 && filter == "default" {
			return []config.AccountConfig{{Name: "default"}}
		}
		return hit
	}
	accs = config.DropDefaultAccounts(accs)
	if len(accs) == 0 {
		if a, ok := cfg.Account(p.ID(), "default"); ok && a.AuthFile != "" {
			return []config.AccountConfig{a}
		}
		if !p.AuthSpec().SupportsAccounts {
			return []config.AccountConfig{{Name: "default"}}
		}
	}
	return accs
}

// overlayAccountCreds applies per-account secrets. Named apiKey wins;
// cookie/workspace accounts drop the inherited provider/env key so they
// stay on the cookie path.
func overlayAccountCreds(base provider.Credentials, acc config.AccountConfig, cookieHeader string) provider.Credentials {
	creds := base
	creds.CookieHeader = cookieHeader
	if acc.APIKey != "" {
		creds.APIKey = acc.APIKey
		creds.AuthToken = acc.APIKey
	} else if acc.ManualCookieHeader != "" || acc.Profile != "" || acc.WorkspaceID != "" || cookieHeader != "" {
		creds.APIKey = ""
		creds.AuthToken = ""
	}
	return creds
}

func missingCredentialHint(id string, spec provider.AuthSpec) string {
	switch {
	case spec.UsesKeychain:
		return "missing Cursor token — run: leeway login cursor (or set CURSOR_ACCESS_TOKEN)"
	case len(spec.EnvAPIKey) > 0 && spec.UsesCookies:
		if len(spec.EnvAuthToken) > 0 {
			return fmt.Sprintf("missing credentials — set %s or %s or: leeway login %s (or: leeway login %s --cookie)", spec.EnvAPIKey[0], spec.EnvAuthToken[0], id, id)
		}
		return fmt.Sprintf("missing credentials — set %s or: leeway login %s (or: leeway login %s --cookie)", spec.EnvAPIKey[0], id, id)
	case len(spec.EnvAPIKey) > 0:
		return fmt.Sprintf("missing credentials — set %s or: leeway login %s", spec.EnvAPIKey[0], id)
	case len(spec.EnvAuthToken) > 0:
		return fmt.Sprintf("missing credentials — set %s or: leeway login %s", spec.EnvAuthToken[0], id)
	case len(spec.LocalAuthFiles) > 0 && spec.LoginHint != "":
		return "missing local session — " + spec.LoginHint
	case spec.UsesCookies:
		return "missing cookies — run: leeway login grok --manual (or: leeway login kimi --cookie)"
	case spec.UsesCodexCLI:
		return "missing Codex session — run: leeway login codex"
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

type snapshotCacheFile struct {
	FP     string        `json:"fp"`
	Output *model.Output `json:"output"`
}

func cacheFingerprint(cfg *config.Config) string {
	h := sha256.New()
	write := func(s string) { fmt.Fprintf(h, "%d:%s\n", len(s), s) }
	ids := append([]string{}, config.ProviderIDs...)
	sort.Strings(ids)
	for _, id := range ids {
		if !cfg.IsEnabled(id) {
			continue
		}
		p, ok := provider.Get(id)
		if !ok {
			continue
		}
		write(id)
		spec := p.AuthSpec()
		write(lookupEnv(spec.EnvAPIKey))
		write(lookupEnv(spec.EnvAuthToken))
		write(cfg.APIKey(id))
		for _, acc := range accountsFor(p, cfg, "") {
			base, _ := ResolveCredentials(p, cfg, acc.Name)
			header, herr := auth.ResolveCookieHeader(p.ID(), cfg.Provider(p.ID()), acc.Name)
			if errors.Is(herr, auth.ErrCookieCacheUnreadable) {
				write(herr.Error())
				header = ""
			} else if herr != nil {
				header = ""
			}
			creds := overlayAccountCreds(base, acc, header)
			write(acc.Name)
			write(creds.APIKey)
			write(creds.AuthToken)
			write(creds.CookieHeader)
			if creds.CodexAuth != nil {
				write(creds.CodexAuth.AccessToken)
			}
			if id == "cursor" {
				if ca, ok, err := auth.LoadCursorAuth(acc.Name); err == nil && ok {
					write(ca.AccessToken)
				}
			}
			if acc.AuthFile != "" {
				write(acc.AuthFile)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func loadSnapshotCache(fp string) (*model.Output, error) {
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
	var file snapshotCacheFile
	if err := json.Unmarshal(raw, &file); err != nil || file.FP == "" || file.Output == nil {
		return nil, nil
	}
	if file.FP != fp {
		return nil, nil
	}
	return file.Output, nil
}

func saveSnapshotCache(fp string, out *model.Output) error {
	p, err := snapshotCachePath()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(p); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(snapshotCacheFile{FP: fp, Output: out}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}
