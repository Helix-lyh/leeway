package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/model"

	// Provider registration side effects (each stub registers itself in init),
	// mirroring cmd/leeway/main.go.
	_ "leeway/internal/provider/codex"
	_ "leeway/internal/provider/commandcode"
	_ "leeway/internal/provider/deepseek"
	_ "leeway/internal/provider/grok"
	_ "leeway/internal/provider/kimi"
	_ "leeway/internal/provider/opencodego"
)

// isolateEnv redirects config, cache and credential locations onto temp dirs
// and blanks provider env keys so command dispatch tests never touch real
// sessions, real config or the network.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LEEWAY_CONFIG", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("LEEWAY_CACHE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LEEWAY_SESSIONS", t.TempDir())
	t.Setenv("LEEWAY_CURSOR_IDE_CHECK", "0")
	t.Setenv("KIMI_CODE_HOME", t.TempDir())
	t.Setenv("GROK_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("DSH_HOME", t.TempDir())
	for _, k := range []string{
		"DEEPSEEK_API_KEY", "DEEPSEEK_KEY",
		"KIMI_CODE_API_KEY", "KIMI_AUTH_TOKEN", "kimi_auth_token",
		"OPENCODE_GO_API_KEY",
		"COMMAND_CODE_API_KEY", "COMMANDCODE_API_KEY",
	} {
		t.Setenv(k, "")
	}
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		out  *model.Output
		want int
	}{
		{"empty", &model.Output{}, 2},
		{"all ok", &model.Output{Providers: []model.ProviderResult{
			{Status: "ok"}, {Status: "disabled"},
		}}, 0},
		{"one not configured", &model.Output{Providers: []model.ProviderResult{
			{Status: "ok"}, {Status: "not_configured"},
		}}, 1},
		{"one error", &model.Output{Providers: []model.ProviderResult{
			{Status: "ok"}, {Status: "error"},
		}}, 1},
		{"mixed ok and error", &model.Output{Providers: []model.ProviderResult{
			{Status: "ok"}, {Status: "error"}, {Status: "disabled"},
		}}, 1},
		{"all not configured", &model.Output{Providers: []model.ProviderResult{
			{Status: "not_configured"},
		}}, 1},
	}
	for _, c := range cases {
		if got := exitCodeFor(c.out); got != c.want {
			t.Errorf("%s: exitCodeFor = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestStatusExit(t *testing.T) {
	if got := statusExit(nil, nil); got != 1 {
		t.Fatalf("empty ledger = %d, want 1", got)
	}
	if got := statusExit([]authRow{{status: "ok"}}, nil); got != 0 {
		t.Fatalf("all ok = %d, want 0", got)
	}
	if got := statusExit([]authRow{{status: "ok"}, {status: "not logged in"}}, nil); got != 1 {
		t.Fatalf("one incomplete = %d, want 1", got)
	}
	if got := statusExit([]authRow{{status: "ok"}}, []statusHint{{provider: "kimi"}}); got != 1 {
		t.Fatalf("missing hint = %d, want 1", got)
	}
}

func TestRootHelpMentionsSubscriptions(t *testing.T) {
	if !strings.Contains(usageText, "login") {
		t.Fatal("root help should show login")
	}
	if !strings.Contains(usageText, "status") {
		t.Fatal("root help should show status")
	}
	if strings.Contains(usageText, "leeway auth") || strings.Contains(usageText, "cookie import") {
		t.Fatal("root help should not mention removed commands")
	}
	if !strings.Contains(usageText, "use    PROVIDER ACCOUNT") {
		t.Fatal("root help should show use as a command with PROVIDER ACCOUNT")
	}
	if !strings.Contains(usageText, "--watch") || !strings.Contains(usageText, "--json") || !strings.Contains(usageText, "--last") {
		t.Fatal("root help should list leaf flags next to the commands that take them")
	}
	if !strings.Contains(useHelp, "cursor") || !strings.Contains(useHelp, "grok") {
		t.Fatal("use help should cover cursor/grok local switch")
	}
	if !strings.Contains(loginHelp, "opencode-go") || !strings.Contains(loginHelp, "commandcode") {
		t.Fatal("login help should list API-key providers")
	}
}

func TestRunDispatch(t *testing.T) {
	isolateEnv(t)
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no args", nil, 2},
		{"unknown command", []string{"frobnicate"}, 2},
		{"help", []string{"help"}, 0},
		{"help login", []string{"help", "login"}, 0},
		{"help logout", []string{"help", "logout"}, 0},
		{"help status", []string{"help", "status"}, 0},
		{"help unknown", []string{"help", "nope"}, 2},
		{"help leftover", []string{"help", "usage", "extra"}, 2},
		{"help dash leftover", []string{"--help", "history"}, 2},
		{"help short leftover", []string{"-h", "auth"}, 2},
		{"help dash topic", []string{"--help", "login"}, 0},
		{"help auth gone", []string{"help", "auth"}, 2},
		{"help history gone", []string{"help", "history"}, 2},
		{"login bogus cookie", []string{"login", "bogus", "--cookie"}, 2},
		{"login bogus manual", []string{"login", "bogus", "--manual", "a=b"}, 2},
		{"login bogus file", []string{"login", "bogus", "home", "--file", "/tmp/x"}, 2},
		{"status empty", []string{"status"}, 1},
		{"use missing", []string{"use"}, 2},
		{"use provider only", []string{"use", "cursor"}, 2},
		{"status leftover", []string{"status", "nope"}, 2},
		{"auth gone", []string{"auth"}, 2},
		{"cookie gone", []string{"cookie", "set"}, 2},
		{"config gone", []string{"config", "set-api-key"}, 2},
		{"doctor gone", []string{"doctor"}, 2},
		{"providers gone", []string{"providers"}, 2},
		{"history gone", []string{"history"}, 2},
		{"login empty key", []string{"login", "deepseek", "--key", ""}, 2},
		{"unknown provider flag", []string{"usage", "--provider", "bogus"}, 2},
		{"unknown provider positional", []string{"usage", "bogus"}, 2},
		{"usage positional then json", []string{"usage", "deepseek", "--json"}, 1},
		{"usage unconfigured", []string{"usage"}, 1}, // all providers not_configured
		{"usage last 12 empty", []string{"usage", "--last", "12"}, 0},
		{"usage last 0", []string{"usage", "--last", "0"}, 2},
		{"usage last and watch", []string{"usage", "--last", "12", "--watch", "1"}, 2},
		{"usage last 0 and watch", []string{"usage", "--last", "0", "--watch", "1"}, 2},
	}
	for _, c := range cases {
		if got := Run(c.args); got != c.want {
			t.Errorf("%s: Run(%v) = %d, want %d", c.name, c.args, got, c.want)
		}
	}
}

func TestAuthStatusListsCursorSessions(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	p, err := auth.CursorAuthFile("work")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"accessToken":"tok"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := runStatus(nil)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if code != 1 {
		t.Fatalf("status = %d, want 1 (other providers still missing)", code)
	}
	if !strings.Contains(string(out), "work") {
		t.Fatalf("status missing session account:\n%s", out)
	}
}

func TestRunHistoryJSON(t *testing.T) {
	isolateEnv(t)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	line := fmtLine(now, 93)
	line2 := fmtLine(now.Add(time.Hour), 95)
	p := filepath.Join(os.Getenv("LEEWAY_CACHE"), "history.jsonl")
	if err := os.WriteFile(p, []byte(line+"\n"+line2+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := Run([]string{"usage", "--json", "--last", "2"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if code != 0 {
		t.Fatalf("usage --last --json = %d", code)
	}
	s := string(out)
	if !strings.Contains(s, `"version": 1`) || !strings.Contains(s, `"window": "primary"`) {
		t.Fatalf("usage --last json:\n%s", s)
	}
	if !strings.Contains(s, "93") || !strings.Contains(s, "95") {
		t.Fatalf("points missing:\n%s", s)
	}
}

func fmtLine(ts time.Time, pct float64) string {
	n, _ := json.Marshal(pct)
	return `{"v":1,"ts":"` + ts.Format(time.RFC3339Nano) + `","generatedAt":"` + ts.Format(time.RFC3339Nano) + `","entries":[{"provider":"deepseek","account":"default","windows":[{"id":"primary","usedPercent":` + string(n) + `}]}]}`
}

func TestRunUsageJSONUnconfigured(t *testing.T) {
	isolateEnv(t)
	// With nothing configured, `usage --json` must still emit the contract and
	// exit 1 (data present, providers missing) — never 2 (invocation error).
	if got := Run([]string{"usage", "--json"}); got != 1 {
		t.Fatalf("usage --json (unconfigured) = %d, want 1", got)
	}
}

func TestRunStatusCorruptConfig(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv("LEEWAY_CONFIG", filepath.Join(dir, "config.json"))
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := runStatus(nil); got != 1 {
		t.Fatalf("status with corrupt config = %d, want 1", got)
	}
}

func TestRunUseDispatch(t *testing.T) {
	isolateEnv(t)
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no flags", []string{"use"}, 2},
		{"missing account", []string{"use", "--provider", "opencode-go"}, 2},
		{"unknown account", []string{"use", "--provider", "opencode-go", "--account", "nope"}, 2},
		{"account without key", []string{"use", "--provider", "opencode-go", "--account", "plain"}, 2},
	}
	for _, c := range cases {
		if got := Run(c.args); got != c.want {
			t.Errorf("%s: Run(%v) = %d, want %d", c.name, c.args, got, c.want)
		}
	}
}

func TestRunUseWritesTarget(t *testing.T) {
	isolateEnv(t)
	// Register a named subscription key.
	if got := runLogin([]string{"opencode-go", "work", "--key", "sk-work123"}); got != 0 {
		t.Fatalf("login = %d, want 0", got)
	}
	if got := Run([]string{"use", "--provider", "opencode-go", "--account", "work"}); got != 0 {
		t.Fatalf("use = %d, want 0", got)
	}
	credPath := filepath.Join(os.Getenv("DSH_HOME"), ".credentials.yaml")
	raw, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("credentials file not written: %v", err)
	}
	if !strings.Contains(string(raw), "OPENCODE_GO_API_KEY: sk-work123") {
		t.Fatalf("key not written to target:\n%s", raw)
	}
	if fi, err := os.Stat(credPath); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("target perms = %o, want 600", fi.Mode().Perm())
	}
}

func TestRunSetAccountKeyDoesNotKeepCookieDefault(t *testing.T) {
	isolateEnv(t)
	if got := saveManualCookie("opencode-go", "sess=x"); got != 0 {
		t.Fatalf("manual cookie = %d, want 0", got)
	}
	if got := runLogin([]string{"opencode-go", "work", "--key", "sk-work123"}); got != 0 {
		t.Fatalf("login = %d, want 0", got)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	accs := cfg.Accounts("opencode-go")
	if len(accs) != 1 || accs[0].Name != "work" || accs[0].APIKey != "sk-work123" {
		t.Fatalf("expected only the api-key subscription, got %+v", accs)
	}

	if err := auth.SaveCookieCache("opencode-go", "", "sess=imported", "chrome"); err != nil {
		t.Fatal(err)
	}
	if got := runLogin([]string{"opencode-go", "personal", "--key", "sk-pers456"}); got != 0 {
		t.Fatalf("login personal = %d, want 0", got)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	accs = cfg.Accounts("opencode-go")
	if len(accs) != 2 || accs[0].Name != "work" || accs[1].Name != "personal" {
		t.Fatalf("expected only named api-key accounts, got %+v", accs)
	}
}

func TestAuthStatusListsKimiAccount(t *testing.T) {
	isolateEnv(t)
	if got := runLogin([]string{"kimi", "work", "--key", "sk-kimi"}); got != 0 {
		t.Fatalf("login = %d", got)
	}
	out, code := captureStdout(t, func() int {
		return runStatus(nil)
	})
	if code != 1 {
		t.Fatalf("status = %d, want 1 (other providers still missing)", code)
	}
	if !strings.Contains(out, "kimi") || !strings.Contains(out, "work") {
		t.Fatalf("status missing kimi account:\n%s", out)
	}
}

func TestAuthStatusUnreadableCursorFile(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	p, err := auth.CursorAuthFile("work")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	row := authStatusRow(cfg, "cursor", "work")
	if row.status != "error" {
		t.Fatalf("status = %q cred=%q, want error (unreadable auth.json must not fall through to keychain)", row.status, row.cred)
	}
	if row.cred == "keychain" {
		t.Fatal("unreadable auth.json must not report keychain")
	}
}

func TestGrokAuthExpiryPrefersOIDC(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "auth.json")
	raw := `{
  "https://other.example::x": {"key": "other-key", "expires_at": "2099-01-01T00:00:00Z", "email": "other@example.com"},
  "https://auth.x.ai::scope1": {"key": "oidc-key", "expires_at": "2030-06-01T00:00:00Z", "email": "oidc@example.com"}
}`
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	exp, err := grokAuthExpiry(p)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)
	if !exp.Equal(want) {
		t.Fatalf("expiry = %v, want OIDC %v", exp, want)
	}
	if email := grokAuthEmail(p); email != "oidc@example.com" {
		t.Fatalf("email = %q, want oidc@example.com", email)
	}
}

func TestCookieImportRemovesCorruptCache(t *testing.T) {
	isolateEnv(t)
	cache := filepath.Join(os.Getenv("LEEWAY_CACHE"), "cookies.json")
	if err := os.WriteFile(cache, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = importBrowserCookies("grok", "", "", "", false)
	raw, err := os.ReadFile(cache)
	if err == nil && string(raw) == "{not json" {
		t.Fatal("corrupt cookies.json must be removed before import continues")
	}
}

func TestUseCursorWritesOfficialFiles(t *testing.T) {
	isolateEnv(t)
	sessions := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", sessions)
	stateDB := filepath.Join(t.TempDir(), "state.vscdb")
	authJSON := filepath.Join(t.TempDir(), "auth.json")
	t.Setenv("LEEWAY_CURSOR_STATE_DB", stateDB)
	t.Setenv("LEEWAY_CURSOR_AUTH_JSON", authJSON)

	p, err := auth.CursorAuthFile("work")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"accessToken":"tok-work","refreshToken":"rt-work"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAccountEmail("cursor", "work", "work@example.com"); err != nil {
		t.Fatal(err)
	}

	out, code := captureStdout(t, func() int {
		return Run([]string{"use", "cursor", "work"})
	})
	if code != 0 {
		t.Fatalf("use cursor = %d out=%s", code, out)
	}
	if strings.Contains(out, "tok-work") || strings.Contains(out, "rt-work") {
		t.Fatalf("stdout leaked token:\n%s", out)
	}
	if !strings.Contains(out, `switched cursor to account "work"`) {
		t.Fatalf("stdout = %q", out)
	}
	raw, err := os.ReadFile(authJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "tok-work") {
		t.Fatalf("auth.json not written: %s", raw)
	}
}

func TestUseCursorMissingLogin(t *testing.T) {
	isolateEnv(t)
	if code := Run([]string{"use", "--provider", "cursor", "--account", "missing"}); code != 2 {
		t.Fatalf("missing cursor login = %d, want 2", code)
	}
}

func TestUseGrokWritesOfficialAuth(t *testing.T) {
	isolateEnv(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "auth.json")
	if err := os.WriteFile(src, []byte(`{"https://auth.x.ai::home":{"key":"home-key","email":"home@x.ai","auth_mode":"oidc","user_id":"u1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "official.json")
	if err := os.WriteFile(dst, []byte(`{"https://other.example::keep":{"key":"keep-key"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEEWAY_GROK_OFFICIAL", dst)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAccountAuthFile("grok", "home", src); err != nil {
		t.Fatal(err)
	}

	out, code := captureStdout(t, func() int {
		return Run([]string{"use", "--provider", "grok", "--account", "home", "--json"})
	})
	if code != 0 {
		t.Fatalf("use grok = %d out=%s", code, out)
	}
	if strings.Contains(out, "home-key") {
		t.Fatalf("json leaked token:\n%s", out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got["provider"] != "grok" || got["account"] != "home" {
		t.Fatalf("json = %s", out)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "home-key") || !strings.Contains(string(raw), "keep-key") {
		t.Fatalf("official auth.json = %s", raw)
	}
	if !strings.Contains(string(raw), `"auth_mode"`) || !strings.Contains(string(raw), `"user_id"`) {
		t.Fatalf("official fields missing: %s", raw)
	}
}

func TestUseGrokRefusesSkinnySession(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(src, []byte(`{"https://auth.x.ai::home":{"key":"skinny-key"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "official.json")
	if err := os.WriteFile(dst, []byte(`{"https://auth.x.ai::old":{"key":"keep-me","auth_mode":"oidc","user_id":"u"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEEWAY_GROK_OFFICIAL", dst)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAccountAuthFile("grok", "home", src); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"use", "grok", "home"}); code != 1 {
		t.Fatalf("skinny use grok = %d, want 1", code)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "keep-me") || strings.Contains(string(raw), "skinny-key") {
		t.Fatalf("official file should stay: %s", raw)
	}
	srcRaw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(srcRaw), `"auth_mode"`) {
		t.Fatalf("refused use must not rewrite isolated src: %s", srcRaw)
	}
}

func TestUseGrokRefusesJWTSkinny(t *testing.T) {
	isolateEnv(t)
	h := "eyJhbGciOiJub25lIn0"
	p := "eyJzdWIiOiJ1c2VyLTk5In0"
	tok := h + "." + p + ".sig"
	src := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(src, []byte(`{"https://auth.x.ai::home":{"key":"`+tok+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "official.json")
	if err := os.WriteFile(dst, []byte(`{"https://auth.x.ai::old":{"key":"keep-me","auth_mode":"oidc","user_id":"u"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEEWAY_GROK_OFFICIAL", dst)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAccountAuthFile("grok", "home", src); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"use", "grok", "home"}); code != 1 {
		t.Fatalf("jwt skinny use grok = %d, want 1", code)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), tok) {
		t.Fatalf("official file should stay: %s", raw)
	}
}

func TestLoginGrokFileSnapshots(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(src, []byte(`{"https://auth.x.ai::home":{"key":"snap-key","auth_mode":"oidc","user_id":"u2","email":"b@x.ai"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"grok", "home", "--file", src}); code != 0 {
		t.Fatalf("login grok --file = %d", code)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a, ok := cfg.Account("grok", "home")
	if !ok || a.AuthFile == "" || a.AuthFile == src {
		t.Fatalf("authFile should be an isolated copy, got %+v", a)
	}
	raw, err := os.ReadFile(a.AuthFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "snap-key") || !strings.Contains(string(raw), "u2") {
		t.Fatalf("snapshot = %s", raw)
	}
}

func TestLoginGrokFileRefusesSkinnyKeepsExisting(t *testing.T) {
	isolateEnv(t)
	good := filepath.Join(t.TempDir(), "good.json")
	if err := os.WriteFile(good, []byte(`{"https://auth.x.ai::home":{"key":"keep-key","auth_mode":"oidc","user_id":"u3"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"grok", "home", "--file", good}); code != 0 {
		t.Fatalf("login good = %d", code)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a, ok := cfg.Account("grok", "home")
	if !ok {
		t.Fatal("missing grok account")
	}
	skinny := filepath.Join(t.TempDir(), "skinny.json")
	if err := os.WriteFile(skinny, []byte(`{"https://auth.x.ai::home":{"key":"bad-key"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"grok", "home", "--file", skinny}); code != 1 {
		t.Fatalf("login skinny = %d, want 1", code)
	}
	raw, err := os.ReadFile(a.AuthFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "keep-key") || strings.Contains(string(raw), "bad-key") {
		t.Fatalf("isolated file should stay: %s", raw)
	}
}

func TestLoginGrokRefusesDeviceAuthWhenSnapshotExists(t *testing.T) {
	isolateEnv(t)
	home, err := grokHome("home")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, "auth.json")
	if err := os.WriteFile(p, []byte(`{"https://auth.x.ai::home":{"key":"keep-key","auth_mode":"oidc","user_id":"u4"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"grok", "home"}); code != 1 {
		t.Fatalf("device-auth over snapshot = %d, want 1", code)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "keep-key") || !strings.Contains(string(raw), "u4") {
		t.Fatalf("snapshot should stay: %s", raw)
	}
}

func TestLoginCursorFileSnapshots(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(src, []byte(`{"accessToken":"cur-tok","refreshToken":"cur-rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"cursor", "work", "--file", src}); code != 0 {
		t.Fatalf("login cursor --file = %d", code)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a, ok := cfg.Account("cursor", "work")
	if !ok || a.AuthFile == "" || a.AuthFile == src {
		t.Fatalf("authFile should be an isolated copy, got %+v", a)
	}
	if !strings.Contains(a.AuthFile, os.Getenv("LEEWAY_SESSIONS")) {
		t.Fatalf("cursor snapshot not under sessions: %s", a.AuthFile)
	}
	raw, err := os.ReadFile(a.AuthFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "cur-tok") || !strings.Contains(string(raw), "cur-rt") {
		t.Fatalf("snapshot = %s", raw)
	}
	srcRaw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(srcRaw), "cur-tok") {
		t.Fatal("source file should stay")
	}
}

func TestLoginCursorFileRejectsSkinny(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(src, []byte(`{"accessToken":"only-at"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"cursor", "work", "--file", src}); code != 1 {
		t.Fatalf("skinny cursor --file = %d, want 1", code)
	}
}

func TestLoginKimiFileSnapshots(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "kimi-code.json")
	if err := os.WriteFile(src, []byte(`{"access_token":"kimi-tok","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"kimi", "work", "--file", src}); code != 0 {
		t.Fatalf("login kimi --file = %d", code)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a, ok := cfg.Account("kimi", "work")
	if !ok || a.AuthFile == "" || a.AuthFile == src {
		t.Fatalf("authFile should be an isolated copy, got %+v", a)
	}
	raw, err := os.ReadFile(a.AuthFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "kimi-tok") {
		t.Fatalf("snapshot = %s", raw)
	}
}

func TestUseKimiWritesOfficialSession(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "kimi-code.json")
	if err := os.WriteFile(src, []byte(`{"access_token":"kimi-tok","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAccountAuthFile("kimi", "work", src); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int {
		return Run([]string{"use", "kimi", "work", "--json"})
	})
	if code != 0 {
		t.Fatalf("use kimi = %d out=%s", code, out)
	}
	if strings.Contains(out, "kimi-tok") {
		t.Fatalf("json leaked token:\n%s", out)
	}
	dst := filepath.Join(os.Getenv("KIMI_CODE_HOME"), "credentials", "kimi-code.json")
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "kimi-tok") {
		t.Fatalf("official kimi session = %s", raw)
	}
}

func TestUseKimiRejectsBadSession(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "kimi-code.json")
	if err := os.WriteFile(src, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(os.Getenv("KIMI_CODE_HOME"), "credentials", "kimi-code.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(`{"access_token":"keep-me","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAccountAuthFile("kimi", "work", src); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"use", "kimi", "work"}); code != 1 {
		t.Fatalf("bad kimi use = %d, want 1", code)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "keep-me") {
		t.Fatalf("official kimi session should stay: %s", raw)
	}
}

func TestUseKimiRefusesOfficialBinding(t *testing.T) {
	isolateEnv(t)
	dst := filepath.Join(os.Getenv("KIMI_CODE_HOME"), "credentials", "kimi-code.json")
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(`{"access_token":"live-tok","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SetAccountAuthFile("kimi", "work", dst); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"use", "kimi", "work"}); code != 1 {
		t.Fatalf("official-bound kimi use = %d, want 1", code)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "live-tok") {
		t.Fatalf("official kimi session should stay: %s", raw)
	}
}

func TestUseCodexRefused(t *testing.T) {
	isolateEnv(t)
	if code := Run([]string{"use", "codex", "home"}); code != 2 {
		t.Fatalf("use codex = %d, want 2", code)
	}
}

func TestLoginDeepseekFileRejected(t *testing.T) {
	isolateEnv(t)
	src := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(src, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := runLogin([]string{"deepseek", "home", "--file", src}); code != 2 {
		t.Fatalf("login deepseek --file = %d, want 2", code)
	}
}

func TestUnderDirRefusesRoot(t *testing.T) {
	if underDir("/", "/.codex") || underDir("/", "/Users") {
		t.Fatal("underDir must not treat filesystem root as a session dir")
	}
}

func TestGrokHomeStaysUnderSessions(t *testing.T) {
	isolateEnv(t)
	root := os.Getenv("LEEWAY_SESSIONS")
	home, err := grokHome("../../.codex")
	if err != nil {
		t.Fatal(err)
	}
	if !underDir(root, home) {
		t.Fatalf("grokHome escaped sessions: %s", home)
	}
	if code := runLogout([]string{"grok", "../../.codex"}); code != 0 {
		t.Fatalf("logout grok escaped name = %d", code)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".codex")); err == nil {
		t.Fatal("logout must not create or touch ~/.codex")
	}
}

func captureStdout(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := fn()
	w.Close()
	os.Stdout = old
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), code
}
