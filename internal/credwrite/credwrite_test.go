package credwrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPatchCreatesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "creds.yaml")
	if err := Patch(p, "OPENCODE_GO_API_KEY", "sk-new"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]string
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["OPENCODE_GO_API_KEY"] != "sk-new" {
		t.Fatalf("value wrong: %v", doc)
	}
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("perms = %o, want 600 (stat err %v)", fi.Mode().Perm(), err)
	}
}

func TestPatchUpdatesAndPreserves(t *testing.T) {
	p := filepath.Join(t.TempDir(), "creds.yaml")
	// Existing doc with a comment, an untouched entry and the key to update.
	orig := "# dsh credentials\nOPENCODE_GO_API_KEY: sk-old\nOTHER_KEY: keep\n"
	if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Patch(p, "OPENCODE_GO_API_KEY", "sk-new"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	text := string(raw)
	if !strings.Contains(text, "# dsh credentials") {
		t.Fatalf("comment lost:\n%s", text)
	}
	if !strings.Contains(text, "OTHER_KEY: keep") {
		t.Fatalf("unrelated entry lost:\n%s", text)
	}
	var doc map[string]string
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["OPENCODE_GO_API_KEY"] != "sk-new" || doc["OTHER_KEY"] != "keep" {
		t.Fatalf("doc wrong: %v", doc)
	}
}

func TestPatchAddsNewKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "creds.yaml")
	if err := os.WriteFile(p, []byte("A: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Patch(p, "NEW_KEY", "x"); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(mustRead(t, p), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["NEW_KEY"] != "x" {
		t.Fatalf("new key missing: %v", doc)
	}
}

func TestLookup(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.yaml")
	if got := Lookup(missing, "K"); got != "" {
		t.Fatalf("missing file = %q", got)
	}
	p := filepath.Join(dir, "creds.yaml")
	if err := os.WriteFile(p, []byte("OPENCODE_GO_API_KEY: sk-live\nOTHER: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Lookup(p, "OPENCODE_GO_API_KEY"); got != "sk-live" {
		t.Fatalf("Lookup = %q", got)
	}
	if got := Lookup(p, "MISSING"); got != "" {
		t.Fatalf("missing key = %q", got)
	}
}

func TestPatchRefusesNonMapping(t *testing.T) {
	p := filepath.Join(t.TempDir(), "creds.yaml")
	if err := os.WriteFile(p, []byte("- a\n- b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Patch(p, "K", "v"); err == nil {
		t.Fatal("non-mapping root must be refused")
	}
	if err := os.WriteFile(p, []byte("{not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Patch(p, "K", "v"); err == nil {
		t.Fatal("unparseable yaml must be refused")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
