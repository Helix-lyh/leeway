package fetch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/model"
	"leeway/internal/provider"

	// Provider registration side effects (each stub registers itself in init).
	_ "leeway/internal/provider/codex"
	_ "leeway/internal/provider/commandcode"
	_ "leeway/internal/provider/deepseek"
	_ "leeway/internal/provider/grok"
	_ "leeway/internal/provider/kimi"
	_ "leeway/internal/provider/opencodego"
)

// isolateEnv redirects every credential/cache location onto temp dirs and
// blanks the provider env keys, so tests never touch real sessions or the
// network: with no credentials anywhere, every provider reports
// not_configured without making a request.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LEEWAY_CACHE", t.TempDir())
	t.Setenv("LEEWAY_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	// HOME plus GROK_HOME / KIMI_CODE_HOME: LocalSessionPath honors those
	// env overrides, so empty temp homes keep canAttemptFetch off.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KIMI_CODE_HOME", t.TempDir())
	t.Setenv("GROK_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("DSH_HOME", t.TempDir())
	for _, k := range []string{
		"DEEPSEEK_API_KEY", "DEEPSEEK_KEY",
		"KIMI_CODE_API_KEY", "KIMI_AUTH_TOKEN", "kimi_auth_token",
		"DEEPSEEK_PLATFORM_TOKEN", "DEEPSEEK_USER_TOKEN",
		"OPENCODE_GO_API_KEY",
		"COMMAND_CODE_API_KEY", "COMMANDCODE_API_KEY",
	} {
		t.Setenv(k, "")
	}
}

func providerIDs(out *model.Output) []string {
	var ids []string
	for _, p := range out.Providers {
		ids = append(ids, p.ID)
	}
	return ids
}

// A provider/account-filtered run must never read or poison the shared
// snapshot cache: `leeway usage --provider deepseek` followed by a plain
// `leeway usage` within the TTL would otherwise return the filtered snapshot.
func TestFetchCacheKeying(t *testing.T) {
	isolateEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	out, err := FetchAll(ctx, cfg, Options{UseCache: true, ProviderFilter: "deepseek"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Providers) != 1 || out.Providers[0].ID != "deepseek" {
		t.Fatalf("filtered run providers = %v, want only deepseek", providerIDs(out))
	}
	// The filtered run must not have written the shared cache.
	if _, err := os.Stat(filepath.Join(os.Getenv("LEEWAY_CACHE"), "snapshots.json")); err == nil {
		t.Fatal("filtered run must not write the shared snapshot cache")
	}

	out2, err := FetchAll(ctx, cfg, Options{UseCache: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex", "deepseek", "kimi"}
	got := providerIDs(out2)
	if len(got) != len(want) {
		t.Fatalf("unfiltered run providers = %v, want %v (filtered cache must not be served)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unfiltered run providers = %v, want %v", got, want)
		}
	}
}

func TestCacheTTLBoundary(t *testing.T) {
	isolateEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Fresh cache: served without probing.
	fp := cacheFingerprint(cfg)
	fresh := &model.Output{Version: 1, GeneratedAt: time.Now(),
		Providers: []model.ProviderResult{{ID: "fake", DisplayName: "Fake", Status: "ok", UpdatedAt: time.Now()}}}
	if err := saveSnapshotCache(fp, fresh); err != nil {
		t.Fatal(err)
	}
	out, err := FetchAll(ctx, cfg, Options{UseCache: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Providers) != 1 || out.Providers[0].ID != "fake" {
		t.Fatalf("fresh cache must be served without probing, got %v", providerIDs(out))
	}

	// Stale cache: refetched (no credentials anywhere -> all not_configured).
	stale := &model.Output{Version: 1, GeneratedAt: time.Now().Add(-CacheTTL - time.Second),
		Providers: []model.ProviderResult{{ID: "fake", DisplayName: "Fake", Status: "ok", UpdatedAt: time.Now()}}}
	if err := saveSnapshotCache(fp, stale); err != nil {
		t.Fatal(err)
	}
	out2, err := FetchAll(ctx, cfg, Options{UseCache: true})
	if err != nil {
		t.Fatal(err)
	}
	got := providerIDs(out2)
	if len(got) != 3 {
		t.Fatalf("stale cache must be refetched, got %v", got)
	}
}

func TestCacheFingerprintRejectsKeySwap(t *testing.T) {
	isolateEnv(t)
	fake := &model.Output{Version: 1, GeneratedAt: time.Now(),
		Providers: []model.ProviderResult{{ID: "fake", DisplayName: "Fake", Status: "ok", UpdatedAt: time.Now()}}}
	if err := saveSnapshotCache("fp-a", fake); err != nil {
		t.Fatal(err)
	}
	got, err := loadSnapshotCache("fp-b")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("mismatched fingerprint must miss, got %v", providerIDs(got))
	}
	hit, err := loadSnapshotCache("fp-a")
	if err != nil || hit == nil || len(hit.Providers) != 1 || hit.Providers[0].ID != "fake" {
		t.Fatalf("matching fingerprint must hit, got %+v err=%v", hit, err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMMAND_CODE_API_KEY", "key-a")
	fpA := cacheFingerprint(cfg)
	t.Setenv("COMMAND_CODE_API_KEY", "key-b")
	fpB := cacheFingerprint(cfg)
	if fpA == fpB {
		t.Fatal("swapping a provider key must change the cache fingerprint")
	}
	t.Setenv("COMMAND_CODE_API_KEY", "ab cd")
	fpC := cacheFingerprint(cfg)
	t.Setenv("COMMAND_CODE_API_KEY", "ab")
	t.Setenv("OPENCODE_GO_API_KEY", "cd")
	fpD := cacheFingerprint(cfg)
	if fpC == fpD {
		t.Fatal("adjacent secret fields must not collide")
	}
}

func TestResolveCredentialsPrecedence(t *testing.T) {
	isolateEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dp, _ := provider.Get("deepseek")

	// Env beats config.
	t.Setenv("DEEPSEEK_API_KEY", "env-key")
	cfg.APIKeys["deepseek"] = "cfg-key"
	creds, err := ResolveCredentials(dp, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if creds.APIKey != "env-key" {
		t.Fatalf("APIKey = %q, want env-key to win over config", creds.APIKey)
	}

	// Config fallback when env is absent.
	t.Setenv("DEEPSEEK_API_KEY", "")
	creds, err = ResolveCredentials(dp, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if creds.APIKey != "cfg-key" {
		t.Fatalf("APIKey = %q, want config fallback", creds.APIKey)
	}

	// Cookie cache fallback for cookie providers.
	if err := auth.SaveCookieCache("grok", "", "sess=abc; x=1", "chrome"); err != nil {
		t.Fatal(err)
	}
	gp, _ := provider.Get("grok")
	creds, err = ResolveCredentials(gp, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if creds.CookieHeader != "sess=abc; x=1" {
		t.Fatalf("CookieHeader = %q, want imported cache", creds.CookieHeader)
	}

	// Missing everywhere: no error, empty credentials (provider reports it).
	t.Setenv("LEEWAY_CACHE", t.TempDir()) // fresh, empty cache
	creds, err = ResolveCredentials(gp, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if creds.CookieHeader != "" {
		t.Fatalf("CookieHeader = %q, want empty when nothing is stored", creds.CookieHeader)
	}
}

func TestResolveCredentialsCorruptCookieCache(t *testing.T) {
	isolateEnv(t)
	if err := os.WriteFile(filepath.Join(os.Getenv("LEEWAY_CACHE"), "cookies.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	gp, _ := provider.Get("grok")
	_, err = ResolveCredentials(gp, cfg, "")
	if !errors.Is(err, auth.ErrCookieCacheUnreadable) {
		t.Fatalf("err = %v, want ErrCookieCacheUnreadable", err)
	}
}

func TestResolveCredentialsSanitizesCookie(t *testing.T) {
	isolateEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SaveCookieCache("grok", "", "sess=abc; bad\x01x=zzz; y=1", "chrome"); err != nil {
		t.Fatal(err)
	}
	gp, _ := provider.Get("grok")
	creds, err := ResolveCredentials(gp, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if creds.CookieHeader != "sess=abc; y=1" {
		t.Fatalf("CookieHeader = %q, want control characters sanitized", creds.CookieHeader)
	}

	// Account-scoped cookies also go through ResolveCookieHeader.
	pc := cfg.Provider("grok")
	pc.Accounts = []config.AccountConfig{{Name: "work", ManualCookieHeader: "ok=1; bad\x01x=zzz; z=9"}}
	cfg.Providers["grok"] = pc
	h, err := auth.ResolveCookieHeader("grok", cfg.Provider("grok"), "work")
	if err != nil {
		t.Fatal(err)
	}
	if h != "ok=1; z=9" {
		t.Fatalf("account cookie = %q, want sanitized via ResolveCookieHeader", h)
	}
}

func TestOverlayAccountCreds(t *testing.T) {
	base := provider.Credentials{APIKey: "env-key", AuthToken: "env-token", CookieHeader: "prov=cookie"}

	got := overlayAccountCreds(base, config.AccountConfig{Name: "work", APIKey: "sk-work"}, "prov=cookie")
	if got.APIKey != "sk-work" {
		t.Fatalf("named key = %q, want sk-work", got.APIKey)
	}
	if got.AuthToken != "sk-work" {
		t.Fatalf("named AuthToken = %q, want sk-work", got.AuthToken)
	}

	got = overlayAccountCreds(base, config.AccountConfig{Name: "personal", WorkspaceID: "ws_1"}, "sess=x")
	if got.APIKey != "" {
		t.Fatalf("cookie account inherited env key %q", got.APIKey)
	}
	if got.AuthToken != "" {
		t.Fatalf("cookie account inherited AuthToken %q", got.AuthToken)
	}
	if got.CookieHeader != "sess=x" {
		t.Fatalf("CookieHeader = %q, want sess=x", got.CookieHeader)
	}

	got = overlayAccountCreds(base, config.AccountConfig{Name: "default"}, "")
	if got.APIKey != "env-key" {
		t.Fatalf("key-only default = %q, want env-key", got.APIKey)
	}
}

func TestCanAttemptFetch(t *testing.T) {
	isolateEnv(t)
	gp, _ := provider.Get("grok")
	if canAttemptFetch(gp, provider.Credentials{}) {
		t.Fatal("empty credentials: should not probe")
	}
	if !canAttemptFetch(gp, provider.Credentials{CookieHeader: "sess=x"}) {
		t.Fatal("cookie header: should probe")
	}
	cp, _ := provider.Get("codex")
	if canAttemptFetch(cp, provider.Credentials{}) {
		t.Fatal("codex without session: should not probe")
	}
	if !canAttemptFetch(cp, provider.Credentials{CodexAuth: &provider.CodexAuth{AccessToken: "tok"}}) {
		t.Fatal("codex with access token: should probe")
	}
}

// A cache-write failure must not fail an otherwise successful probe: the
// caller renders the valid output and the cache error is only logged.
func TestSaveCacheErrorContract(t *testing.T) {
	dir := t.TempDir()
	isolateEnv(t)
	t.Setenv("LEEWAY_CACHE", dir)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	out, err := FetchAll(context.Background(), cfg, Options{UseCache: true})
	if err != nil {
		t.Fatalf("cache-write failure must not fail the run: %v", err)
	}
	if out == nil || len(out.Providers) != 3 {
		t.Fatalf("valid probe results must be returned, got %d providers", len(out.Providers))
	}
}

func TestTimeoutParsing(t *testing.T) {
	t.Setenv("LEEWAY_TIMEOUT", "5")
	if d := timeout(); d != 5*time.Second {
		t.Fatalf("timeout() = %v, want 5s", d)
	}
	// Huge values must be clamped, not overflow to a negative duration.
	t.Setenv("LEEWAY_TIMEOUT", "9e9")
	if d := timeout(); d != time.Duration(maxTimeoutSeconds)*time.Second {
		t.Fatalf("timeout() = %v, want clamped to %d s", d, maxTimeoutSeconds)
	}
	t.Setenv("LEEWAY_TIMEOUT", "bogus")
	if d := timeout(); d != DefaultTimeout {
		t.Fatalf("timeout() = %v, want default on parse failure", d)
	}
	t.Setenv("LEEWAY_TIMEOUT", "-1")
	if d := timeout(); d != DefaultTimeout {
		t.Fatalf("timeout() = %v, want default on negative", d)
	}
	t.Setenv("LEEWAY_TIMEOUT", "")
	if d := timeout(); d != DefaultTimeout {
		t.Fatalf("timeout() = %v, want default when unset", d)
	}
}

func TestSortProviders(t *testing.T) {
	out := &model.Output{Providers: []model.ProviderResult{
		{ID: "zebra"}, {ID: "alpha"}, {ID: "mike"},
	}}
	sortProviders(out)
	got := providerIDs(out)
	want := []string{"alpha", "mike", "zebra"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortProviders = %v, want %v", got, want)
		}
	}
}

// A grok account whose only credential is an account-bound authFile must be
// let through to the provider (which then reports the file state), not
// short-circuited into not_configured. Regression: the not_configured gate
// used to ignore acc.AuthFile, so multi-account grok setups never fetched.
func TestGrokAccountAuthFileNotBlocked(t *testing.T) {
	isolateEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Point the account at a nonexistent private authFile: the provider
	// should attempt the fetch and fail with a missing-file error rather
	// than the gate reporting not_configured.
	gone := filepath.Join(t.TempDir(), "auth.json")
	if err := cfg.SetAccountAuthFile("grok", "second", gone); err != nil {
		t.Fatal(err)
	}

	out, err := FetchAll(context.Background(), cfg, Options{ProviderFilter: "grok"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Providers) != 1 {
		t.Fatalf("providers = %v, want 1", providerIDs(out))
	}
	grok := out.Providers[0]
	for _, acc := range grok.Accounts {
		if acc.Name == "second" {
			if acc.Status == "not_configured" {
				t.Fatalf("account %q was blocked as not_configured despite authFile=%s", acc.Name, gone)
			}
			if acc.Status != "error" {
				t.Fatalf("account %q status = %q, want error (missing auth file)", acc.Name, acc.Status)
			}
			return
		}
	}
	t.Fatal("grok account \"second\" missing from results")
}

// Profile-bound cookies used to be invisible to the not_configured gate:
// canAttemptFetch ran on provider-level creds before overlay, so a named
// account whose only secret was cookie import --account X --profile P
// never reached Fetch.
func TestProfileBoundCookieCanAttempt(t *testing.T) {
	isolateEnv(t)
	if err := auth.SaveCookieCache("grok", "Profile 1", "sess=account-profile", "chrome"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	acc := config.AccountConfig{Name: "work", Profile: "Profile 1"}
	pc := cfg.Provider("grok")
	pc.Accounts = []config.AccountConfig{acc}
	cfg.Providers["grok"] = pc

	header, err := auth.ResolveCookieHeader("grok", cfg.Provider("grok"), "work")
	if err != nil {
		t.Fatal(err)
	}
	if header != "sess=account-profile" {
		t.Fatalf("header = %q", header)
	}
	gp, _ := provider.Get("grok")
	creds := overlayAccountCreds(provider.Credentials{}, acc, header)
	if !canAttemptFetch(gp, creds) {
		t.Fatal("profile-bound cookie must be enough to probe")
	}
}

func TestCanAttemptFetchHonorsGrokHome(t *testing.T) {
	isolateEnv(t)
	gp, _ := provider.Get("grok")
	if canAttemptFetch(gp, provider.Credentials{}) {
		t.Fatal("empty GROK_HOME must not probe")
	}
	home := t.TempDir()
	t.Setenv("GROK_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !canAttemptFetch(gp, provider.Credentials{}) {
		t.Fatal("GROK_HOME/auth.json must allow probe")
	}
}

func TestCanAttemptFetchHonorsKimiCodeHome(t *testing.T) {
	isolateEnv(t)
	kp, _ := provider.Get("kimi")
	if canAttemptFetch(kp, provider.Credentials{}) {
		t.Fatal("empty KIMI_CODE_HOME must not probe")
	}
	home := t.TempDir()
	t.Setenv("KIMI_CODE_HOME", home)
	cred := filepath.Join(home, "credentials", "kimi-code.json")
	if err := os.MkdirAll(filepath.Dir(cred), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cred, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !canAttemptFetch(kp, provider.Credentials{}) {
		t.Fatal("KIMI_CODE_HOME credentials must allow probe")
	}
}
