package grok

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testJWT(claims map[string]any) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	p, _ := json.Marshal(claims)
	return h + "." + base64.RawURLEncoding.EncodeToString(p) + ".sig"
}

func TestHasOfficialShapeIgnoresAccessTokenClaims(t *testing.T) {
	dir := t.TempDir()
	tok := testJWT(map[string]any{
		"sub": "user-99", "email": "a@x.ai", "given_name": "Ada",
	})
	p := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(p, []byte(`{"https://auth.x.ai::cli":{"key":"`+tok+`","refresh_token":"rt"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if HasOfficialShape(p) {
		t.Fatal("JWT sub must not count as an official session")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"auth_mode"`) || strings.Contains(string(raw), `"user_id"`) {
		t.Fatalf("file should stay skinny: %s", raw)
	}
}

func TestHasOfficialShapeNeedsExistingUserID(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(p, []byte(`{"https://auth.x.ai::cli":{"key":"k","auth_mode":"oidc","user_id":"u1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !HasOfficialShape(p) {
		t.Fatal("existing official fields should pass")
	}
}

func TestWriteBackKeepsOfficialFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GROK_HOME", dir)
	writeGrokAuth(t, dir, `{
  "https://auth.x.ai::cli": {
    "key": "old",
    "refresh_token": "rt",
    "auth_mode": "oidc",
    "user_id": "keep-me",
    "oidc_issuer": "https://auth.x.ai",
    "oidc_client_id": "cid"
  }
}`)
	sess, err := loadGrokSession("")
	if err != nil {
		t.Fatal(err)
	}
	sess.entry.Key = "new"
	if err := sess.writeBack(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	e := m["https://auth.x.ai::cli"]
	if e["key"] != "new" {
		t.Fatalf("key = %v", e["key"])
	}
	if e["auth_mode"] != "oidc" || e["user_id"] != "keep-me" {
		t.Fatalf("official fields stripped: %v", e)
	}
}
