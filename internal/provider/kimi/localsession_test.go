package kimi

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadKimiCodeCredential(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIMI_CODE_HOME", dir)
	credDir := filepath.Join(dir, "credentials")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"access_token":"eyJhbGciOiJIUzI1NiJ9.test-token","refresh_token":"rt","expires_at":4102444800,"scope":"openid offline_access","token_type":"Bearer"}`
	if err := os.WriteFile(filepath.Join(credDir, "kimi-code.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	tok, err := localCLIToken(context.Background(), now, "")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "eyJhbGciOiJIUzI1NiJ9.test-token" {
		t.Fatalf("unexpected token %q", tok)
	}
}

func TestLoadKimiCodeCredentialAuthFile(t *testing.T) {
	t.Setenv("KIMI_CODE_HOME", t.TempDir())
	isolated := filepath.Join(t.TempDir(), "kimi-work", "credentials", "kimi-code.json")
	if err := os.MkdirAll(filepath.Dir(isolated), 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"access_token":"from-isolated","refresh_token":"rt","expires_at":4102444800}`
	if err := os.WriteFile(isolated, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := localCLIToken(context.Background(), time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC), isolated)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "from-isolated" {
		t.Fatalf("token = %q, want isolated file", tok)
	}
}

func TestLoadKimiCodeCredentialExpired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIMI_CODE_HOME", dir)
	credDir := filepath.Join(dir, "credentials")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// expires_at = 2026-08-06 11:00 UTC, checked at 12:00 UTC.
	content := `{"access_token":"tok","refresh_token":"rt","expires_at":1786014000}`
	if err := os.WriteFile(filepath.Join(credDir, "kimi-code.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// Point the OAuth host at an unreachable address so the auto-refresh
	// attempt fails fast instead of hitting the real auth.kimi.com.
	t.Setenv("KIMI_CODE_OAUTH_HOST", "http://127.0.0.1:1")
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if _, err := localCLIToken(context.Background(), now, ""); err == nil {
		t.Fatal("expected expiry error")
	}
}

func TestLoadKimiCodeCredentialMissing(t *testing.T) {
	t.Setenv("KIMI_CODE_HOME", t.TempDir())
	if _, err := localCLIToken(context.Background(), time.Now(), ""); err == nil {
		t.Fatal("expected missing-file error")
	}
}

func TestKimiAutoRefresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIMI_CODE_HOME", dir)
	credDir := filepath.Join(dir, "credentials")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Expired credential with a refresh_token carrying the client_id claim.
	// client_id in the JWT payload: 17e5f671-d194-4dfb-9706-5516cb48c098.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"kimi-auth","client_id":"17e5f671-d194-4dfb-9706-5516cb48c098","scope":"kimi-code"}`))
	refreshToken := header + "." + payload + ".sig"
	content := fmt.Sprintf(`{"access_token":"old-token","refresh_token":%q,"expires_at":1786014000,"scope":"kimi-code","token_type":"Bearer"}`, refreshToken)
	if err := os.WriteFile(filepath.Join(credDir, "kimi-code.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" ||
			r.Form.Get("refresh_token") != refreshToken ||
			r.Form.Get("client_id") != "17e5f671-d194-4dfb-9706-5516cb48c098" {
			t.Errorf("bad refresh form: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","expires_in":900,"scope":"kimi-code","token_type":"Bearer"}`))
	}))
	defer srv.Close()
	t.Setenv("KIMI_CODE_OAUTH_HOST", srv.URL)

	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	tok, err := localCLIToken(context.Background(), now, "")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "new-token" {
		t.Fatalf("expected refreshed token, got %q", tok)
	}
	raw, err := os.ReadFile(filepath.Join(credDir, "kimi-code.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "new-token") || !strings.Contains(s, "new-refresh") {
		t.Fatalf("refreshed credential not written back:\n%s", s)
	}
}

func TestKimiRefreshFailureHint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIMI_CODE_HOME", dir)
	credDir := filepath.Join(dir, "credentials")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"client_id":"cid-123"}`))
	content := fmt.Sprintf(`{"access_token":"old","refresh_token":"x.y.%s","expires_at":1786014000}`, payload)
	if err := os.WriteFile(filepath.Join(credDir, "kimi-code.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	t.Setenv("KIMI_CODE_OAUTH_HOST", srv.URL)

	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	if _, err := localCLIToken(context.Background(), now, ""); err == nil || !strings.Contains(err.Error(), "leeway login kimi") {
		t.Fatalf("err = %v, want re-login hint", err)
	}
}
