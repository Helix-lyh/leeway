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
	if len(accs) != 1 || accs[0].Name != "default" {
		t.Fatalf("expected implicit default account, got %+v", accs)
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
