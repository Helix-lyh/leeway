package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"leeway/internal/config"
)

func TestSanitizeHeader(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a=b; c=d", "a=b; c=d"},
		{"a=b; bad\x01cookie=zzz; c=d", "a=b; c=d"},
		{"a=b;; c=d", "a=b; c=d"},
		{"=novalue; a=b; x", "a=b"},
		{"a=b; c\x7fd=e", "a=b"},
		{"a=b ;  c=d  ", "a=b; c=d"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := sanitizeHeader(c.in); got != c.want {
			t.Errorf("sanitizeHeader(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveCookieHeaderPrecedence(t *testing.T) {
	t.Setenv("LEEWAY_CACHE", t.TempDir())
	now := time.Now()
	cache := map[string]CookieCache{
		"grok": {
			ProviderID: "grok", Header: "sess=provider-level", ImportedAt: now,
		},
		"grok@Profile 1": {
			ProviderID: "grok", Profile: "Profile 1", Header: "sess=account-profile", ImportedAt: now,
		},
	}
	if err := writeCache(cache); err != nil {
		t.Fatal(err)
	}
	pc := config.ProviderConfig{Accounts: []config.AccountConfig{{Name: "work", Profile: "Profile 1"}}}

	// Account bound to a profile resolves the profile-scoped cache entry.
	if h, err := ResolveCookieHeader("grok", pc, "work"); err != nil || h != "sess=account-profile" {
		t.Fatalf("account+profile: h=%q err=%v, want profile-scoped entry", h, err)
	}
	// An account with a manual header wins over everything.
	manual := config.ProviderConfig{
		Accounts: []config.AccountConfig{{
			Name: "work", Profile: "Profile 1", ManualCookieHeader: "sess=manual-account",
		}},
	}
	if h, err := ResolveCookieHeader("grok", manual, "work"); err != nil || h != "sess=manual-account" {
		t.Fatalf("account manual: h=%q err=%v", h, err)
	}
	// Provider-level manual header beats the cache.
	provManual := config.ProviderConfig{ManualCookieHeader: "sess=manual-provider"}
	if h, err := ResolveCookieHeader("grok", provManual, "work"); err != nil || h != "sess=manual-provider" {
		t.Fatalf("provider manual: h=%q err=%v", h, err)
	}
	// An unbound account falls back to the provider-level cache entry.
	if h, err := ResolveCookieHeader("grok", pc, "other"); err != nil || h != "sess=provider-level" {
		t.Fatalf("fallback: h=%q err=%v, want provider-level entry", h, err)
	}
	// Nothing stored anywhere (different provider, no entries): descriptive
	// error naming the fix.
	_, err := ResolveCookieHeader("opencode-go", config.ProviderConfig{}, "default")
	if err == nil || !strings.Contains(err.Error(), "leeway login opencode-go --cookie") {
		t.Fatalf("absent err = %v, want login --cookie hint", err)
	}
}

func TestLoadCookieCacheUnreadable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_CACHE", dir)
	if err := os.WriteFile(filepath.Join(dir, "cookies.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadCookieCache("grok", ""); err == nil {
		t.Fatal("LoadCookieCache must surface a corrupt cache file, not silently report nothing")
	}
	// ResolveCookieHeader must distinguish "unreadable cache" from "nothing
	// imported" instead of telling the user to re-import.
	_, err := ResolveCookieHeader("grok", config.ProviderConfig{}, "default")
	if !errors.Is(err, ErrCookieCacheUnreadable) {
		t.Fatalf("err = %v, want ErrCookieCacheUnreadable", err)
	}
}

func TestAllCookieCachesStableOrder(t *testing.T) {
	t.Setenv("LEEWAY_CACHE", t.TempDir())
	base := time.Now()
	cache := map[string]CookieCache{
		"zebra":  {ProviderID: "zebra", Header: "a=1", ImportedAt: base},
		"alpha":  {ProviderID: "alpha", Header: "a=1", ImportedAt: base},
		"beta@P": {ProviderID: "beta", Profile: "P", Header: "a=1", ImportedAt: base.Add(time.Hour)},
		"beta":   {ProviderID: "beta", Header: "a=1", ImportedAt: base},
	}
	if err := writeCache(cache); err != nil {
		t.Fatal(err)
	}
	out, err := AllCookieCaches()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, cc := range out {
		got = append(got, cc.ProviderID+"/"+cc.Profile)
	}
	want := []string{"alpha/", "beta/", "beta/P", "zebra/"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("AllCookieCaches order = %v, want %v", got, want)
	}
}

func TestExtractCookieHeader(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"-H 'Cookie: a=b; c=d'", "a=b; c=d"},
		{`-H "Cookie: a=b"`, "a=b"},
		{"cookie: a=b; c=d", "a=b; c=d"},
		{"Cookie: a=b", "a=b"},
		{"--cookie 'a=b'", "a=b"},
		{"-b 'a=b'", "a=b"},
		{"-b a=b", "a=b"},
		{"a=b; c=d", ""}, // already-clean header: no curl wrapper to strip
		{"", ""},
	}
	for _, c := range cases {
		if got := ExtractCookieHeader(c.in); got != c.want {
			t.Errorf("ExtractCookieHeader(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
