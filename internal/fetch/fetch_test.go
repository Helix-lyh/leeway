package fetch

import (
	"context"
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
	// HOME must be redirected too: HasCredentials resolves provider-owned
	// session files ("~/.grok/auth.json") via the tilde, not GROK_HOME.
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
	want := []string{"codex", "deepseek", "grok", "kimi", "opencode-go"}
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
	fresh := &model.Output{Version: 1, GeneratedAt: time.Now(),
		Providers: []model.ProviderResult{{ID: "fake", DisplayName: "Fake", Status: "ok", UpdatedAt: time.Now()}}}
	if err := saveSnapshotCache(fresh); err != nil {
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
	if err := saveSnapshotCache(stale); err != nil {
		t.Fatal(err)
	}
	out2, err := FetchAll(ctx, cfg, Options{UseCache: true})
	if err != nil {
		t.Fatal(err)
	}
	got := providerIDs(out2)
	if len(got) != 5 {
		t.Fatalf("stale cache must be refetched, got %v", got)
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
	if creds := ResolveCredentials(dp, cfg, ""); creds.APIKey != "env-key" {
		t.Fatalf("APIKey = %q, want env-key to win over config", creds.APIKey)
	}

	// Config fallback when env is absent.
	t.Setenv("DEEPSEEK_API_KEY", "")
	if creds := ResolveCredentials(dp, cfg, ""); creds.APIKey != "cfg-key" {
		t.Fatalf("APIKey = %q, want config fallback", creds.APIKey)
	}

	// Cookie cache fallback for cookie providers.
	if err := auth.SaveCookieCache("grok", "", "sess=abc; x=1", "chrome"); err != nil {
		t.Fatal(err)
	}
	gp, _ := provider.Get("grok")
	if creds := ResolveCredentials(gp, cfg, ""); creds.CookieHeader != "sess=abc; x=1" {
		t.Fatalf("CookieHeader = %q, want imported cache", creds.CookieHeader)
	}

	// Missing everywhere: no error, empty credentials (provider reports it).
	t.Setenv("LEEWAY_CACHE", t.TempDir()) // fresh, empty cache
	if creds := ResolveCredentials(gp, cfg, ""); creds.CookieHeader != "" {
		t.Fatalf("CookieHeader = %q, want empty when nothing is stored", creds.CookieHeader)
	}
}

func TestOverlayAccountCreds(t *testing.T) {
	base := provider.Credentials{APIKey: "env-key", CookieHeader: "prov=cookie"}

	got := overlayAccountCreds(base, config.AccountConfig{Name: "work", APIKey: "sk-work"}, "prov=cookie")
	if got.APIKey != "sk-work" {
		t.Fatalf("named key = %q, want sk-work", got.APIKey)
	}

	got = overlayAccountCreds(base, config.AccountConfig{Name: "personal", WorkspaceID: "ws_1"}, "sess=x")
	if got.APIKey != "" {
		t.Fatalf("cookie account inherited env key %q", got.APIKey)
	}
	if got.CookieHeader != "sess=x" {
		t.Fatalf("CookieHeader = %q, want sess=x", got.CookieHeader)
	}

	got = overlayAccountCreds(base, config.AccountConfig{Name: "default"}, "")
	if got.APIKey != "env-key" {
		t.Fatalf("key-only default = %q, want env-key", got.APIKey)
	}
}

func TestHasCredentials(t *testing.T) {
	isolateEnv(t)
	gp, _ := provider.Get("grok")
	if HasCredentials(gp, provider.Credentials{}) {
		t.Fatal("no creds must report false")
	}
	if !HasCredentials(gp, provider.Credentials{CookieHeader: "sess=x"}) {
		t.Fatal("cookie header must count as configured")
	}
	cp, _ := provider.Get("codex")
	if HasCredentials(cp, provider.Credentials{}) {
		t.Fatal("codex without session must report false")
	}
	if !HasCredentials(cp, provider.Credentials{CodexAuth: &provider.CodexAuth{AccessToken: "tok"}}) {
		t.Fatal("codex with access token must report true")
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
	if out == nil || len(out.Providers) != 5 {
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
