package switcher

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"leeway/internal/auth"
)

func TestApplyCursorWritesVscdbAndAuthJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	targets := CursorTargets{
		StateDB:  filepath.Join(dir, "state.vscdb"),
		AuthJSON: filepath.Join(dir, "auth.json"),
	}
	ca := auth.CursorAuth{AccessToken: "tok-work", RefreshToken: "rt-work"}
	if err := ApplyCursor(targets, ca, "work@example.com"); err != nil {
		t.Fatal(err)
	}

	token, email, err := ReadCursorStateToken(targets.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok-work" || email != "work@example.com" {
		t.Fatalf("vscdb token=%q email=%q", token, email)
	}
	raw, err := os.ReadFile(targets.AuthJSON)
	if err != nil {
		t.Fatal(err)
	}
	var got auth.CursorAuth
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "tok-work" || got.RefreshToken != "rt-work" {
		t.Fatalf("auth.json = %+v", got)
	}
	fi, err := os.Stat(targets.AuthJSON)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("auth.json perms = %o", fi.Mode().Perm())
	}

	db, err := sql.Open("sqlite", targets.StateDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if vscdbGet(db, "cursor.accessToken") != "tok-work" {
		t.Fatal("missing cursor.accessToken")
	}
	if vscdbGet(db, "cursor.email") != "work@example.com" {
		t.Fatal("missing cursor.email")
	}
}

func TestApplyCursorRefusesEmptyToken(t *testing.T) {
	if err := ApplyCursor(CursorTargets{StateDB: filepath.Join(t.TempDir(), "db")}, auth.CursorAuth{}, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestSnapshotCursorIfNeeded(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	targets := CursorTargets{
		StateDB:  filepath.Join(dir, "state.vscdb"),
		AuthJSON: filepath.Join(dir, "auth.json"),
	}
	if err := ApplyCursor(targets, auth.CursorAuth{AccessToken: "tok-old"}, "old@example.com"); err != nil {
		t.Fatal(err)
	}

	saved, unsaved, err := SnapshotCursorIfNeeded(targets, "tok-new")
	if err != nil {
		t.Fatal(err)
	}
	if saved != "old@example.com" || unsaved {
		t.Fatalf("saved = %q unsaved=%v", saved, unsaved)
	}
	ca, ok, err := auth.LoadCursorAuth("old@example.com")
	if err != nil || !ok || ca.AccessToken != "tok-old" {
		t.Fatalf("snapshot: %+v ok=%v err=%v", ca, ok, err)
	}

	// Same token already incoming: no extra snapshot.
	saved, unsaved, err = SnapshotCursorIfNeeded(targets, "tok-old")
	if err != nil || saved != "" || unsaved {
		t.Fatalf("same-token snapshot = %q unsaved=%v err=%v", saved, unsaved, err)
	}
}

func TestSnapshotCursorCollisionLeavesOfficialUnsaved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	if err := auth.WriteCursorAuth("old@example.com", auth.CursorAuth{AccessToken: "tok-held"}); err != nil {
		t.Fatal(err)
	}
	targets := CursorTargets{StateDB: filepath.Join(dir, "state.vscdb")}
	if err := ApplyCursor(targets, auth.CursorAuth{AccessToken: "tok-other"}, "old@example.com"); err != nil {
		t.Fatal(err)
	}
	saved, unsaved, err := SnapshotCursorIfNeeded(targets, "tok-new")
	if err != nil || saved != "" || !unsaved {
		t.Fatalf("saved=%q unsaved=%v err=%v", saved, unsaved, err)
	}
}

func TestApplyGrokMergesOIDCAndKeepsOtherScopes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.json")
	dst := filepath.Join(dir, "dst.json")
	if err := os.WriteFile(src, []byte(`{
  "https://auth.x.ai::home": {"key": "home-key", "email": "home@x.ai", "refresh_token": "rt"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(`{
  "https://auth.x.ai::old": {"key": "old-key", "email": "old@x.ai"},
  "https://other.example::keep": {"key": "keep-key"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyGrok(dst, src); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["https://auth.x.ai::old"]; ok {
		t.Fatal("old OIDC scope should be replaced")
	}
	if m["https://auth.x.ai::home"]["key"] != "home-key" {
		t.Fatalf("home entry = %v", m["https://auth.x.ai::home"])
	}
	if m["https://other.example::keep"]["key"] != "keep-key" {
		t.Fatal("non-xAI scope was dropped")
	}
}

func TestApplyGrokCreatesOfficialFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.json")
	dst := filepath.Join(dir, "missing", "auth.json")
	if err := os.WriteFile(src, []byte(`{"https://auth.x.ai::a":{"key":"k"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyGrok(dst, src); err != nil {
		t.Fatal(err)
	}
	if !fileExists(dst) {
		t.Fatal("dst not created")
	}
}

func TestApplyGrokRejectsNonOIDC(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.json")
	if err := os.WriteFile(src, []byte(`{"https://other::x":{"key":"k"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyGrok(filepath.Join(dir, "dst.json"), src); err == nil {
		t.Fatal("expected error")
	}
}

func TestOfficialPathsHonorOverrides(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "state.vscdb")
	authJSON := filepath.Join(dir, "auth.json")
	grok := filepath.Join(dir, "grok.json")
	t.Setenv("LEEWAY_CURSOR_STATE_DB", db)
	t.Setenv("LEEWAY_CURSOR_AUTH_JSON", authJSON)
	t.Setenv("LEEWAY_GROK_OFFICIAL", grok)

	got, err := OfficialCursorTargets()
	if err != nil {
		t.Fatal(err)
	}
	if got.StateDB != db || got.AuthJSON != authJSON {
		t.Fatalf("targets = %+v", got)
	}
	p, err := OfficialGrokAuthFile()
	if err != nil || p != grok {
		t.Fatalf("grok = %q (%v)", p, err)
	}
}

func TestCheckCursorIDECanBeDisabled(t *testing.T) {
	t.Setenv("LEEWAY_CURSOR_IDE_CHECK", "0")
	if CheckCursorIDE() {
		t.Fatal("check should be off")
	}
}

func TestNewestExistingPrefersNewer(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.db")
	fresh := filepath.Join(dir, "fresh.db")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(fresh, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := newestExisting([]string{old, fresh}); got != fresh {
		t.Fatalf("newestExisting = %q, want %s", got, fresh)
	}
}

func TestDisplayPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := DisplayPath(filepath.Join(home, ".grok", "auth.json"))
	if got != "~/.grok/auth.json" {
		t.Fatalf("DisplayPath = %q", got)
	}
}
