package switcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
	if err := ApplyCursor(targets, auth.CursorAuth{AccessToken: "tok-old", RefreshToken: "rt-old"}, "old@example.com"); err != nil {
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
	if block, _ := CheckCursorIDE(); block {
		t.Fatal("check should be off")
	}
}

func TestCheckCursorIDEFailsClosedWithoutPgrep(t *testing.T) {
	t.Setenv("LEEWAY_CURSOR_IDE_CHECK", "1")
	t.Setenv("PATH", t.TempDir())
	block, reason := CheckCursorIDE()
	if !block {
		t.Fatal("missing pgrep should block the write")
	}
	if !strings.Contains(reason, "pgrep") {
		t.Fatalf("reason = %q, want pgrep", reason)
	}
}

func TestPickCursorStateDB(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.db")
	b := filepath.Join(dir, "b.db")
	if err := os.WriteFile(a, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := pickCursorStateDB([]string{filepath.Join(dir, "gone.db")}); err != nil || got != "" {
		t.Fatalf("no db = %q (%v), want empty (the IDE is optional)", got, err)
	}
	got, err := pickCursorStateDB([]string{a, filepath.Join(dir, "gone.db")})
	if err != nil || got != a {
		t.Fatalf("one db = %q (%v), want %s", got, err, a)
	}
	if _, err := pickCursorStateDB([]string{a, b}); err == nil {
		t.Fatal("multiple dbs should refuse")
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

func TestApplyCursorWithoutIDE(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	authJSON := filepath.Join(dir, "auth.json")
	ca := auth.CursorAuth{AccessToken: "tok-agent", RefreshToken: "rt-agent"}
	if err := ApplyCursor(CursorTargets{AuthJSON: authJSON}, ca, "agent@example.com"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(authJSON)
	if err != nil {
		t.Fatal(err)
	}
	var got auth.CursorAuth
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "tok-agent" {
		t.Fatalf("auth.json = %+v", got)
	}
	if err := ApplyCursor(CursorTargets{}, ca, "agent@example.com"); err == nil {
		t.Fatal("a machine with no Cursor store at all should error")
	}
}

// keychainService is a throwaway keychain item so tests never touch the
// machine's own cursor-agent login.
func keychainService(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("no security binary")
	}
	svc := "leeway-test-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	del := func() {
		exec.Command("security", "delete-generic-password", "-a", auth.CursorKeychainAccount, "-s", svc).Run()
	}
	t.Cleanup(del)
	del()
	// A locked or absent keychain (SSH, CI, a machine that has not been
	// unlocked yet) is an environment without a keychain, not a broken write
	// path: probe with security itself so a failure here skips instead of
	// hiding a real bug in SetCursorKeychainToken.
	if err := exec.Command("security", "add-generic-password",
		"-a", auth.CursorKeychainAccount, "-s", svc, "-w", "probe").Run(); err != nil {
		t.Skip("keychain unavailable")
	}
	return svc
}

// A Cursor token is a JWT well past 128 bytes, and security's interactive
// prompt cuts the value there without saying so. The whole token has to land,
// or the official client ends up with a login that cannot work.
func TestSetCursorKeychainTokenKeepsLongToken(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain is macOS only")
	}
	svc := keychainService(t)
	tok := strings.Repeat("J", 1200)
	if err := auth.SetCursorKeychainToken(context.Background(), svc, tok); err != nil {
		t.Fatal(err)
	}
	got, err := auth.CursorKeychainTokenFor(context.Background(), svc)
	if err != nil {
		t.Fatal(err)
	}
	if got != tok {
		t.Fatalf("keychain kept %d of %d bytes", len(got), len(tok))
	}
}

func TestApplyCursorWritesKeychain(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain is macOS only")
	}
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	svc := keychainService(t)
	ca := auth.CursorAuth{AccessToken: "tok-keychain", RefreshToken: "rt"}
	if err := ApplyCursor(CursorTargets{Keychain: svc}, ca, "kc@example.com"); err != nil {
		t.Fatal(err)
	}
	got, err := auth.CursorKeychainTokenFor(context.Background(), svc)
	if err != nil {
		t.Fatal(err)
	}
	if got != "tok-keychain" {
		t.Fatalf("keychain token = %q", got)
	}
}

// The keychain item holds the access token alone, and a token without its
// refresh token cannot be written back into a client: switching away from a
// cursor-agent-only login has to be reported as "not kept", not as a snapshot
// the user could return to.
func TestSnapshotCursorReportsKeychainOnlyLoginAsUnsaved(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain is macOS only")
	}
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	svc := keychainService(t)
	if err := auth.SetCursorKeychainToken(context.Background(), svc, "tok-agent"); err != nil {
		t.Fatal(err)
	}
	saved, unsaved, err := SnapshotCursorIfNeeded(CursorTargets{Keychain: svc}, "tok-new")
	if err != nil {
		t.Fatal(err)
	}
	if saved != "" || !unsaved {
		t.Fatalf("saved=%q unsaved=%v", saved, unsaved)
	}
}

// The keychain is the one target that reaches outside leeway's own files, so
// naming a file target has to keep it out of the write: this is what keeps
// `go test ./...` from touching the developer's real cursor-agent login.
func TestOfficialCursorTargetsKeychainDecision(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LEEWAY_CURSOR_STATE_DB", filepath.Join(t.TempDir(), "state.vscdb"))
	t.Setenv("LEEWAY_CURSOR_AUTH_JSON", filepath.Join(t.TempDir(), "auth.json"))
	got, err := OfficialCursorTargets()
	if err != nil {
		t.Fatal(err)
	}
	if got.Keychain != "" {
		t.Errorf("a file target must keep the real keychain out: %+v", got)
	}
	t.Setenv("LEEWAY_CURSOR_KEYCHAIN", "leeway-test-svc")
	if got, err = OfficialCursorTargets(); err != nil || got.Keychain != "leeway-test-svc" {
		t.Errorf("naming a keychain item = %+v err=%v", got, err)
	}
	t.Setenv("LEEWAY_CURSOR_KEYCHAIN", "0")
	if got, err = OfficialCursorTargets(); err != nil || got.Keychain != "" {
		t.Errorf("LEEWAY_CURSOR_KEYCHAIN=0 = %+v err=%v", got, err)
	}
}

// A machine without Cursor must not get one invented for it: `use` has to
// report that there is nothing to switch, not write a fresh auth.json.
func TestOfficialCursorTargetsSkipsAMachineWithoutCursor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LEEWAY_CURSOR_KEYCHAIN", "0")
	got, err := OfficialCursorTargets()
	if err != nil {
		t.Fatal(err)
	}
	if got.AuthJSON != "" {
		t.Errorf("invented an auth.json: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor")); !os.IsNotExist(err) {
		t.Errorf(".cursor was created on a machine without Cursor: %v", err)
	}
	if err := ApplyCursor(got, auth.CursorAuth{AccessToken: "t", RefreshToken: "r"}, ""); err == nil {
		t.Error("switching on a machine without Cursor should fail")
	}
}
