package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/model"

	// Provider registration side effects (each stub registers itself in init),
	// mirroring cmd/leeway/main.go.
	_ "leeway/internal/provider/codex"
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
	t.Setenv("KIMI_CODE_HOME", t.TempDir())
	t.Setenv("GROK_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("DSH_HOME", t.TempDir())
	for _, k := range []string{
		"DEEPSEEK_API_KEY", "DEEPSEEK_KEY",
		"KIMI_CODE_API_KEY", "KIMI_AUTH_TOKEN", "kimi_auth_token",
		"OPENCODE_GO_API_KEY",
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
		{"cookie set without manual", []string{"cookie", "set"}, 2},
		{"cookie unknown subcommand", []string{"cookie", "bogus"}, 2},
		{"config unknown subcommand", []string{"config", "bogus"}, 2},
		{"set-api-key empty key", []string{"config", "set-api-key", "--provider", "deepseek", "--key", ""}, 2},
		{"unknown provider flag", []string{"usage", "--provider", "bogus"}, 2},
		{"usage unconfigured", []string{"usage"}, 1}, // all providers not_configured
	}
	for _, c := range cases {
		if got := Run(c.args); got != c.want {
			t.Errorf("%s: Run(%v) = %d, want %d", c.name, c.args, got, c.want)
		}
	}
}

func TestRunUsageJSONUnconfigured(t *testing.T) {
	isolateEnv(t)
	// With nothing configured, `usage --json` must still emit the contract and
	// exit 1 (data present, providers missing) — never 2 (invocation error).
	if got := Run([]string{"usage", "--json"}); got != 1 {
		t.Fatalf("usage --json (unconfigured) = %d, want 1", got)
	}
}

func TestRunCookieListCorruptConfig(t *testing.T) {
	isolateEnv(t)
	// A corrupt config.json must yield a "config:" diagnostic and exit 2,
	// not a nil-dereference panic.
	dir := t.TempDir()
	t.Setenv("LEEWAY_CONFIG", filepath.Join(dir, "config.json"))
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Run([]string{"cookie", "list"}); got != 2 {
		t.Fatalf("cookie list with corrupt config = %d, want 2", got)
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
	if got := Run([]string{"config", "set-api-key", "--provider", "opencode-go", "--account", "work", "--key", "sk-work123"}); got != 0 {
		t.Fatalf("set-api-key = %d, want 0", got)
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

func TestRunConfigKeys(t *testing.T) {
	isolateEnv(t)
	if got := Run([]string{"config", "keys"}); got != 2 {
		t.Fatalf("config keys without provider = %d, want 2", got)
	}
	if got := Run([]string{"config", "set-api-key", "--provider", "opencode-go", "--account", "work", "--key", "sk-abc12345"}); got != 0 {
		t.Fatalf("set-api-key = %d, want 0", got)
	}
	// Human output must show the account with a masked key.
	if got := Run([]string{"config", "keys", "--provider", "opencode-go"}); got != 0 {
		t.Fatalf("config keys = %d, want 0", got)
	}
}

func TestRunSetAccountKeyDoesNotKeepCookieDefault(t *testing.T) {
	isolateEnv(t)
	// A manual cookie header (and an imported cache) must NOT spawn a "default"
	// account: the API-key subscription is the only account.
	if got := Run([]string{"cookie", "set", "--provider", "opencode-go", "--manual", "sess=x"}); got != 0 {
		t.Fatalf("cookie set = %d, want 0", got)
	}
	if got := Run([]string{"config", "set-api-key", "--provider", "opencode-go", "--account", "work", "--key", "sk-work123"}); got != 0 {
		t.Fatalf("set-api-key = %d, want 0", got)
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
	if got := Run([]string{"config", "set-api-key", "--provider", "opencode-go", "--account", "personal", "--key", "sk-pers456"}); got != 0 {
		t.Fatalf("set personal = %d, want 0", got)
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

func TestRunConfigKeysMarksActive(t *testing.T) {
	isolateEnv(t)
	if got := Run([]string{"config", "set-api-key", "--provider", "opencode-go", "--account", "work", "--key", "sk-work123"}); got != 0 {
		t.Fatalf("set work = %d", got)
	}
	if got := Run([]string{"config", "set-api-key", "--provider", "opencode-go", "--account", "personal", "--key", "sk-pers456"}); got != 0 {
		t.Fatalf("set personal = %d", got)
	}
	if got := Run([]string{"use", "--provider", "opencode-go", "--account", "work"}); got != 0 {
		t.Fatalf("use = %d", got)
	}
	out, code := captureStdout(t, func() int {
		return Run([]string{"config", "keys", "--provider", "opencode-go", "--json"})
	})
	if code != 0 {
		t.Fatalf("config keys = %d out=%s", code, out)
	}
	var rows []struct {
		Account string `json:"account"`
		Active  bool   `json:"active"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	active := map[string]bool{}
	for _, r := range rows {
		active[r.Account] = r.Active
	}
	if !active["work"] || active["personal"] {
		t.Fatalf("active map = %v, want work only", active)
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
