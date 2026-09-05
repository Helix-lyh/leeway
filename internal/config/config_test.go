package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_CONFIG", filepath.Join(dir, "config.json"))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsEnabled("deepseek") {
		t.Fatal("providers should default to enabled")
	}
	if err := cfg.SetAPIKey("deepseek", "sk-test"); err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey("deepseek") != "sk-test" {
		t.Fatal("api key round trip failed")
	}

	// Fresh load from disk.
	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.APIKey("deepseek") != "sk-test" {
		t.Fatal("api key not persisted")
	}

	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config perms = %o, want 600", perm)
	}
}

func TestAccountsFallback(t *testing.T) {
	cfg := Default()
	accs := cfg.Accounts("opencode-go")
	if len(accs) != 0 {
		t.Fatalf("expected no implicit default account, got %+v", accs)
	}
	cfg.Providers["opencode-go"] = ProviderConfig{
		Accounts: []AccountConfig{{Name: "work", WorkspaceID: "ws_1"}, {Name: "personal"}},
	}
	accs = cfg.Accounts("opencode-go")
	if len(accs) != 2 || accs[0].WorkspaceID != "ws_1" {
		t.Fatalf("accounts mismatch: %+v", accs)
	}
}

func TestManualHeaderResolution(t *testing.T) {
	cfg := Default()
	cfg.Providers["grok"] = ProviderConfig{ManualCookieHeader: "a=b"}
	if got := cfg.ManualHeader("grok", "default"); got != "a=b" {
		t.Fatalf("provider-level manual header not resolved: %q", got)
	}
	cfg.Providers["opencode-go"] = ProviderConfig{
		Accounts: []AccountConfig{{Name: "work", ManualCookieHeader: "w=x"}},
	}
	if got := cfg.ManualHeader("opencode-go", "work"); got != "w=x" {
		t.Fatalf("account-level manual header not resolved: %q", got)
	}
}

func TestSetAccountKeyCreatesAndUpdates(t *testing.T) {
	t.Setenv("LEEWAY_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	cfg := Default()
	if err := cfg.SetAccountKey("opencode-go", "work", "sk-work"); err != nil {
		t.Fatal(err)
	}
	acc, ok := cfg.Account("opencode-go", "work")
	if !ok || acc.APIKey != "sk-work" {
		t.Fatalf("created account wrong: %+v ok=%v", acc, ok)
	}
	if err := cfg.SetAccountKey("opencode-go", "work", "sk-work2"); err != nil {
		t.Fatal(err)
	}
	acc, _ = cfg.Account("opencode-go", "work")
	if acc.APIKey != "sk-work2" {
		t.Fatalf("account key not updated: %+v", acc)
	}
	// Fresh load from disk persists the named key.
	cfg2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	acc2, ok := cfg2.Account("opencode-go", "work")
	if !ok || acc2.APIKey != "sk-work2" {
		t.Fatalf("account key not persisted: %+v ok=%v", acc2, ok)
	}
	if _, ok := cfg2.Account("opencode-go", "missing"); ok {
		t.Fatal("missing account must not resolve")
	}
	if _, ok := cfg2.Account("opencode-go", "default"); ok {
		t.Fatal("api-key-only first account must not invent a cookie default")
	}
}

func TestAccountMatchesNameOrEmail(t *testing.T) {
	a := AccountConfig{Name: "work", Email: "alice@example.com"}
	if !a.Matches("work") || !a.Matches("WORK") || !a.Matches("alice@example.com") {
		t.Fatalf("tag/email should match: %+v", a)
	}
	if a.Matches("") || a.Matches("other") {
		t.Fatal("empty/other must not match")
	}
}

func TestDropDefaultAccounts(t *testing.T) {
	got := DropDefaultAccounts([]AccountConfig{{Name: "default"}, {Name: "work"}})
	if len(got) != 1 || got[0].Name != "work" {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeNamedAccounts(t *testing.T) {
	base := []AccountConfig{{Name: "default"}}
	got := MergeNamedAccounts(base, []string{"work", "default", "personal"})
	if len(got) != 3 || got[1].Name != "work" || got[2].Name != "personal" {
		t.Fatalf("got %+v", got)
	}
	tagged := []AccountConfig{{Name: "work", Email: "a@b.com"}}
	got = MergeNamedAccounts(tagged, []string{"work", "a@b.com", "home"})
	if len(got) != 2 || got[1].Name != "home" {
		t.Fatalf("email/name dedupe: %+v", got)
	}
}

func TestSetAccountEmailAndRemove(t *testing.T) {
	t.Setenv("LEEWAY_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	cfg := Default()
	if err := cfg.SetAccountEmail("cursor", "work", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	acc, ok := cfg.Account("cursor", "alice@example.com")
	if !ok || acc.Name != "work" || acc.Email != "alice@example.com" {
		t.Fatalf("lookup by email: %+v ok=%v", acc, ok)
	}
	if err := cfg.RemoveAccount("cursor", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Account("cursor", "work"); ok {
		t.Fatal("account should be gone")
	}
}

func TestKeyTargetDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DSH_HOME", "")
	cfg := Default()
	got := cfg.KeyTarget("opencode-go")
	if got.File != filepath.Join(home, ".dsh", ".credentials.yaml") {
		t.Fatalf("default file = %q", got.File)
	}
	if got.Key != "OPENCODE_GO_API_KEY" {
		t.Fatalf("default key = %q", got.Key)
	}
}

func TestKeyTargetDSHHomeOverride(t *testing.T) {
	cfg := Default()
	t.Setenv("DSH_HOME", "/custom/dsh")
	got := cfg.KeyTarget("opencode-go")
	if got.File != "/custom/dsh/.credentials.yaml" {
		t.Fatalf("DSH_HOME override = %q", got.File)
	}
}

func TestKeyTargetConfigured(t *testing.T) {
	cfg := Default()
	cfg.Providers["opencode-go"] = ProviderConfig{
		KeyTarget: &KeyTarget{File: "/tmp/other.yaml", Key: "MY_KEY"},
	}
	got := cfg.KeyTarget("opencode-go")
	if got.File != "/tmp/other.yaml" || got.Key != "MY_KEY" {
		t.Fatalf("configured target wrong: %+v", got)
	}
}

func TestKeyTargetPartialAndTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DSH_HOME", "")
	cfg := Default()
	cfg.Providers["opencode-go"] = ProviderConfig{
		KeyTarget: &KeyTarget{File: "~/.dsh/.credentials.yaml"},
	}
	got := cfg.KeyTarget("opencode-go")
	wantFile := filepath.Join(home, ".dsh", ".credentials.yaml")
	if got.File != wantFile {
		t.Fatalf("tilde file = %q, want %q", got.File, wantFile)
	}
	if got.Key != "OPENCODE_GO_API_KEY" {
		t.Fatalf("partial keyTarget must keep default key, got %q", got.Key)
	}
}
