package cli

import (
	"os"
	"path/filepath"
	"testing"

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
	for _, k := range []string{
		"DEEPSEEK_API_KEY", "DEEPSEEK_KEY",
		"KIMI_CODE_API_KEY", "KIMI_AUTH_TOKEN", "kimi_auth_token",
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
