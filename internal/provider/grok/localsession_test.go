package grok

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeGrokAuth(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadGrokSessionPrefersOIDC(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GROK_HOME", dir)
	writeGrokAuth(t, dir, `{
  "https://auth.x.ai::scope1": {"key": "oidc-key", "refresh_token": "rt", "expires_at": "2099-01-01T00:00:00Z", "oidc_issuer": "https://auth.x.ai", "oidc_client_id": "cid"},
  "https://legacy.example::s": {"key": "legacy-key"}
}`)
	sess, err := loadGrokSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.entry.Key != "oidc-key" {
		t.Fatalf("expected OIDC entry, got %q", sess.entry.Key)
	}
}

func TestLoadGrokSessionMissingFile(t *testing.T) {
	t.Setenv("GROK_HOME", t.TempDir())
	if _, err := loadGrokSession(); err == nil {
		t.Fatal("expected error for missing auth.json")
	}
}

func TestGrokExpiry(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	exp := "2026-08-06T11:00:00Z"
	e := &grokSessionEntry{ExpiresAt: &exp}
	if !e.expired(now) {
		t.Fatal("past expiry should be expired")
	}
	future := "2026-08-07T12:00:00Z"
	e.ExpiresAt = &future
	if e.expired(now) {
		t.Fatal("future expiry should not be expired")
	}
	e.ExpiresAt = nil
	if e.expired(now) {
		t.Fatal("no expiry should be usable")
	}
}

func TestGrokRefreshAndWriteBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GROK_HOME", dir)
	issuer := "https://auth.test.example"
	writeGrokAuth(t, dir, `{
  "https://auth.x.ai::scope1": {"key": "old-key", "refresh_token": "rt-1", "expires_at": "2026-08-06T11:00:00Z", "oidc_issuer": "`+issuer+`", "oidc_client_id": "cid"},
  "other": {"key": "keep-me"}
}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "rt-1" {
			t.Errorf("bad form: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"new-key","refresh_token":"rt-2","expires_in":3600}`))
	}))
	defer srv.Close()
	// Rewrite the file so the issuer points at the test server.
	writeGrokAuth(t, dir, `{
  "https://auth.x.ai::scope1": {"key": "old-key", "refresh_token": "rt-1", "expires_at": "2026-08-06T11:00:00Z", "oidc_issuer": "`+srv.URL+`", "oidc_client_id": "cid"},
  "other": {"key": "keep-me"}
}`)
	sess, err := loadGrokSession()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if !sess.entry.expired(now) {
		t.Fatal("precondition: token should be expired")
	}
	if err := sess.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sess.entry.Key != "new-key" {
		t.Fatalf("key not refreshed: %q", sess.entry.Key)
	}
	// File must contain the new key and keep the other entry.
	raw, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !contains(s, "new-key") || !contains(s, "rt-2") {
		t.Fatalf("refreshed entry not written back:\n%s", s)
	}
	if !contains(s, "keep-me") {
		t.Fatalf("unrelated entry lost:\n%s", s)
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
