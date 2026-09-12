package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCursorAuth writes an official-format auth.json for an account.
func writeCursorAuth(t *testing.T, account, accessToken string) {
	t.Helper()
	p, err := CursorAuthFile(account)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(CursorAuth{AccessToken: accessToken})
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCursorHomeIsolation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)

	// default and named accounts map to distinct, sanitized directories
	home, err := CursorHome("default")
	if err != nil || home != filepath.Join(dir, "cursor") {
		t.Fatalf("default home = %s (%v)", home, err)
	}
	work, err := CursorHome("work")
	if err != nil || work != filepath.Join(dir, "cursor-work") {
		t.Fatalf("work home = %s (%v)", work, err)
	}

	// hostile account names cannot escape the session dir
	evil, err := CursorHome("../../etc/passwd")
	if err != nil || !strings.HasPrefix(evil, dir+string(filepath.Separator)) {
		t.Fatalf("account escaped session dir: %s (%v)", evil, err)
	}
	if filepath.Base(evil) != "cursor-______etc_passwd" {
		t.Fatalf("sanitized name = %s", filepath.Base(evil))
	}
}

func TestLoadCursorAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)

	// absent file -> not logged in, no error
	if _, ok, err := LoadCursorAuth("work"); err != nil || ok {
		t.Fatalf("absent auth: ok=%v err=%v", ok, err)
	}

	writeCursorAuth(t, "work", "tok-123")
	ca, ok, err := LoadCursorAuth("work")
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if ca.AccessToken != "tok-123" {
		t.Fatalf("accessToken = %q", ca.AccessToken)
	}

	// accounts stay isolated
	if _, ok, err := LoadCursorAuth("other"); err != nil || ok {
		t.Fatalf("account isolation broken: ok=%v err=%v", ok, err)
	}

	// auth.json perms are 0600 like the official CLI writes
	fi, err := os.Stat(filepath.Join(dir, "cursor-work", ".cursor", "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("auth.json perms = %o, want 600", fi.Mode().Perm())
	}
}

func TestWriteCursorAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	if err := WriteCursorAuth("work", CursorAuth{AccessToken: "tok-write", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	ca, ok, err := LoadCursorAuth("work")
	if err != nil || !ok || ca.AccessToken != "tok-write" || ca.RefreshToken != "rt" {
		t.Fatalf("wrote %+v ok=%v err=%v", ca, ok, err)
	}
}

func TestListCursorSessionAccounts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	writeCursorAuth(t, "work", "tok-a")
	writeCursorAuth(t, "personal", "tok-b")

	got, err := ListCursorSessionAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "personal" || got[1] != "work" {
		t.Fatalf("got %v", got)
	}
}

func TestRemoveCursorHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	writeCursorAuth(t, "work", "tok")

	if err := RemoveCursorHome("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cursor-work")); !os.IsNotExist(err) {
		t.Fatalf("home still exists after remove: %v", err)
	}
	if err := RemoveCursorHome("work"); err != nil { // idempotent
		t.Fatal(err)
	}
}

func TestLookupCursorEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aiserver.v1.DashboardService/GetMe" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"email":"alice@example.com"}`))
	}))
	defer srv.Close()
	orig := CursorMeURL
	CursorMeURL = srv.URL + "/aiserver.v1.DashboardService/GetMe"
	defer func() { CursorMeURL = orig }()

	got, err := LookupCursorEmail(context.Background(), "tok")
	if err != nil || got != "alice@example.com" {
		t.Fatalf("LookupCursorEmail = %q, %v", got, err)
	}
}

func TestRelocateCursorHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	writeCursorAuth(t, "default", "tok-move")

	if err := RelocateCursorHome("default", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := LoadCursorAuth("default"); err != nil || ok {
		t.Fatalf("old home still present: ok=%v err=%v", ok, err)
	}
	ca, ok, err := LoadCursorAuth("alice@example.com")
	if err != nil || !ok || ca.AccessToken != "tok-move" {
		t.Fatalf("relocated auth: %+v ok=%v err=%v", ca, ok, err)
	}
	if err := RelocateCursorHome("alice@example.com", "alice@example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestJWTExpiry(t *testing.T) {
	claims, _ := json.Marshal(map[string]int64{"exp": 1792524395})
	payload := base64.RawURLEncoding.EncodeToString(claims)
	tok := "eyJhbGciOiJIUzI1NiJ9." + payload + ".sig"

	exp, ok := JWTExpiry(tok)
	if !ok || exp.Unix() != 1792524395 {
		t.Fatalf("JWTExpiry = %v, %v; want 1792524395, true", exp, ok)
	}

	if _, ok := JWTExpiry("not-a-jwt"); ok {
		t.Fatal("expected ok=false for non-JWT")
	}
	if _, ok := JWTExpiry("a." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":0}`)) + ".c"); ok {
		t.Fatal("expected ok=false for exp=0")
	}
}
