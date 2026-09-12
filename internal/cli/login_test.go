package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"leeway/internal/config"
)

func TestRunLoginMissingProvider(t *testing.T) {
	isolateEnv(t)
	if code := runLogin(nil); code != 2 {
		t.Fatalf("login missing provider = %d, want 2", code)
	}
}

func TestRunLoginUnknownProvider(t *testing.T) {
	isolateEnv(t)
	if code := runLogin([]string{"bogus"}); code != 2 {
		t.Fatalf("login unknown provider = %d, want 2", code)
	}
}

func TestRunLoginDeepseekKey(t *testing.T) {
	isolateEnv(t)
	out, errOut, code := captureStd(t, func() int {
		return runLogin([]string{"deepseek", "--key", "sk-test"})
	})
	if code != 0 {
		t.Fatalf("login deepseek --key = %d, want 0\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	if strings.Contains(out, "sk-test") || strings.Contains(errOut, "sk-test") {
		t.Fatalf("raw key leaked\nstdout=%s\nstderr=%s", out, errOut)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey("deepseek") != "sk-test" {
		t.Fatalf("stored key = %q, want sk-test", cfg.APIKey("deepseek"))
	}
}

func TestRunLoginCursorMissingCLI(t *testing.T) {
	isolateEnv(t)
	t.Setenv("PATH", t.TempDir())
	if code := runLogin([]string{"cursor"}); code != 2 {
		t.Fatalf("login cursor without cursor-agent = %d, want 2", code)
	}
}

func TestOverrideEnvDropsParentHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/real-home")
	t.Setenv("CODEX_HOME", "/tmp/real-codex")
	t.Setenv("GROK_HOME", "/tmp/real-grok")
	env := overrideEnv("HOME", "/iso-home", "CODEX_HOME", "/iso-codex", "GROK_HOME", "/iso-grok")
	got := map[string]int{}
	want := map[string]string{"HOME": "/iso-home", "CODEX_HOME": "/iso-codex", "GROK_HOME": "/iso-grok"}
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, keep := want[k]; !keep {
			continue
		}
		got[k]++
		if v != want[k] {
			t.Fatalf("%s = %q, want %q", k, v, want[k])
		}
	}
	for k := range want {
		if got[k] != 1 {
			t.Fatalf("%s appeared %d times, want 1", k, got[k])
		}
	}
}

func TestRunLoginEmptyKeyFlag(t *testing.T) {
	isolateEnv(t)
	if code := runLogin([]string{"deepseek", "--key", ""}); code != 2 {
		t.Fatalf("login --key empty = %d, want 2", code)
	}
	if code := runLogin([]string{"deepseek", "--key="}); code != 2 {
		t.Fatalf("login --key= = %d, want 2", code)
	}
}

func TestRunLoginCookieWithoutProvider(t *testing.T) {
	isolateEnv(t)
	if code := runLogin([]string{"--cookie"}); code != 2 {
		t.Fatalf("login --cookie without provider = %d, want 2", code)
	}
}

func TestRunStatusJSON(t *testing.T) {
	isolateEnv(t)
	out, _, code := captureStd(t, func() int {
		return runStatus([]string{"--json"})
	})
	if code != 0 {
		t.Fatalf("status --json = %d, want 0\n%s", code, out)
	}
	if !json.Valid([]byte(strings.TrimSpace(out))) {
		t.Fatalf("status --json is not JSON:\n%s", out)
	}
}

func TestRunLogoutCursorMissingSession(t *testing.T) {
	isolateEnv(t)
	home := os.Getenv("HOME")
	code := runLogout([]string{"cursor"})
	if code != 0 && code != 1 {
		t.Fatalf("logout cursor missing session = %d, want 0 or 1", code)
	}
	if os.Getenv("HOME") != home {
		t.Fatal("logout changed HOME")
	}
}

func captureStd(t *testing.T, fn func() int) (stdout, stderr string, code int) {
	t.Helper()
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	code = fn()
	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outB, err := io.ReadAll(rOut)
	if err != nil {
		t.Fatal(err)
	}
	errB, err := io.ReadAll(rErr)
	if err != nil {
		t.Fatal(err)
	}
	return string(outB), string(errB), code
}
