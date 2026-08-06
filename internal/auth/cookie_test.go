package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// ---- test encryptors (build blobs in the TRUE macOS Chromium layout) ----

// encryptMacBlobForTest builds a macOS Chromium cookie blob:
// ["v10"] + AES-128-CBC(PKCS7) with key = PBKDF2(password, "saltysalt", 1003,
// 16) and IV = 16 spaces; when domainHash is set (user_version >= 24), the
// plaintext is prefixed with SHA-256(hostKey).
func encryptMacBlobForTest(t *testing.T, password, hostKey string, domainHash bool, plaintext []byte) []byte {
	t.Helper()
	key, err := pbkdf2SHA1([]byte(password), macSafeStorageSalt, 1003, 16)
	if err != nil {
		t.Fatal(err)
	}
	var pt []byte
	if domainHash {
		hash := sha256.Sum256([]byte(hostKey))
		pt = append(pt, hash[:]...)
	}
	pt = append(pt, plaintext...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	padLen := aes.BlockSize - len(pt)%aes.BlockSize
	padded := make([]byte, len(pt)+padLen)
	copy(padded, pt)
	for i := len(pt); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, macCBCIV).CryptBlocks(out, padded)
	blob := out
	if len(plaintext) > 0 {
		blob = append([]byte("v10"), out...)
	}
	return blob
}

// ---- quality gate (a): v10 + domain-hash round-trip (modern macOS) ----

func TestV10RoundTrip(t *testing.T) {
	password := "test-chrome-safe-storage"
	hostKey := ".grok.com"
	plaintext := "sso=eyJhbGciOiJIUzI1NiJ9.value.and.more"
	blob := encryptMacBlobForTest(t, password, hostKey, true, []byte(plaintext))
	got, err := decryptValue(blob, password, hostKey, 30)
	if err != nil {
		t.Fatalf("decryptValue: %v", err)
	}
	if string(got) != plaintext {
		t.Fatalf("round trip mismatch: got %q want %q", got, plaintext)
	}
	if _, err := decryptValue(blob, "wrong-password", hostKey, 30); err == nil {
		t.Fatal("expected failure with a wrong password")
	}
	if _, err := decryptValue(blob, password, "different.host", 30); err == nil {
		t.Fatal("expected host-key hash mismatch")
	}
	if _, err := decryptValue([]byte("v10"), password, hostKey, 30); err == nil {
		t.Fatal("expected error for a truncated v10 blob")
	}
}

// ---- quality gate (b): pre-domain-hash CBC round-trip (older macOS) ----

func TestLegacyCBCRoundTrip(t *testing.T) {
	password := "legacy-safe-storage-key"
	for _, plaintext := range []string{"sso=value", "x", "exactly-16-bytes!", "padding-multiple: 32 bytes of cookie data!"} {
		blob := encryptMacBlobForTest(t, password, "", false, []byte(plaintext))
		got, err := decryptValue(blob, password, "", 20)
		if err != nil {
			t.Fatalf("decryptValue(%q): %v", plaintext, err)
		}
		if string(got) != plaintext {
			t.Fatalf("legacy round trip mismatch: got %q want %q", got, plaintext)
		}
	}
	if _, err := decryptValue([]byte("not-a-valid-block-length"), password, "", 20); err == nil {
		t.Fatal("expected error for a malformed legacy blob")
	}
}

// ---- quality gate (c): header building ----

func TestHeaderBuilding(t *testing.T) {
	cookies := []decodedCookie{
		{name: "zzz", value: "1"},
		{name: "sso", value: "token", httpOnly: true, session: true},
		{name: "aaa", value: "2", httpOnly: true},
		{name: "mid", value: "3"},
	}
	got := buildHeader(cookies)
	want := "sso=token; aaa=2; mid=3; zzz=1"
	if got != want {
		t.Fatalf("buildHeader: got %q want %q", got, want)
	}
	if got := buildHeader(nil); got != "" {
		t.Fatalf("buildHeader(nil): got %q want empty", got)
	}
}

// ---- quality gate (d): plaintext fallback + skip rules ----

func TestPlaintextFallback(t *testing.T) {
	password := "fallback-keychain-password"
	rows := []cookieRow{
		{name: "plain", value: "visible-value", hostKey: "grok.com", path: "/"},
		{name: "enc", encrypted: encryptMacBlobForTest(t, password, "grok.com", true, []byte("secret")), hostKey: "grok.com", path: "/"},
		{name: "empty", hostKey: "grok.com", path: "/"},
		{name: "broken", encrypted: []byte("v10waytooshort"), hostKey: "grok.com", path: "/"},
		{name: "whitespace", value: "  ", hostKey: "grok.com", path: "/"},
	}
	decoded, _ := decryptRows(rows, password, 30)
	header := buildHeader(decoded)
	for _, want := range []string{"plain=visible-value", "enc=secret"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header %q missing %q", header, want)
		}
	}
	for _, banned := range []string{"empty=", "broken=", "whitespace="} {
		if strings.Contains(header, banned) {
			t.Fatalf("header %q must skip %s cookies", header, banned)
		}
	}
}

// ---- quality gate (e): WAL-immutable open + full pipeline ----

func TestWALImmutableOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Cookies")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cookies (
		name TEXT, value TEXT, encrypted_value BLOB, host_key TEXT,
		path TEXT, expires_utc INTEGER, is_httponly INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('sso', '', ?, '.grok.com', '/', 0, 1)`,
		encryptMacBlobForTest(t, "test-key", ".grok.com", true, []byte("grok-token"))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO cookies VALUES ('plain', 'grok-plain', NULL, 'grok.com', '/', 0, 0)`); err != nil {
		t.Fatal(err)
	}
	// Merge everything into the main file, then close and drop both sidecars
	// to reproduce Chromium's clean-shutdown state (WAL-mode file, no
	// -wal/-shm). openCookieDB must still read committed rows via its
	// read-only → immutable fallback.
	if _, err := db.Exec("PRAGMA wal_checkpoint(FULL)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	os.Remove(path + "-wal")
	os.Remove(path + "-shm")
	if !walSidecarsMissing(path) {
		t.Fatal("walSidecarsMissing should report missing sidecars after removal")
	}

	opened, err := openCookieDB(path)
	if err != nil {
		t.Fatalf("openCookieDB (ro with immutable fallback): %v", err)
	}
	defer opened.Close()

	rows, err := loadCookieRows(opened, []string{"grok.com"})
	if err != nil {
		t.Fatalf("loadCookieRows: %v", err)
	}
	decoded, _ := decryptRows(rows, "test-key", 30)
	header := buildHeader(decoded)
	if header != "sso=grok-token; plain=grok-plain" {
		t.Fatalf("pipeline header: got %q", header)
	}

	// The immutable DSN itself must also open the file directly.
	imm, err := openDSN(path, true)
	if err != nil {
		t.Fatalf("openDSN immutable: %v", err)
	}
	var n int
	if err := imm.QueryRow("SELECT COUNT(*) FROM cookies").Scan(&n); err != nil || n != 2 {
		imm.Close()
		t.Fatalf("immutable read: n=%d err=%v", n, err)
	}
	imm.Close()
}

// ---- quality gate (f): provider→domain mapping ----

func TestDomainsForProvider(t *testing.T) {
	cases := []struct {
		provider string
		want     string
	}{
		{"grok", "grok.com"},
		{"kimi", "www.kimi.com,kimi.com"},
		{"opencode-go", "opencode.ai"},
	}
	for _, c := range cases {
		got, err := DomainsForProvider(c.provider)
		if err != nil {
			t.Fatalf("DomainsForProvider(%q): %v", c.provider, err)
		}
		if strings.Join(got, ",") != c.want {
			t.Fatalf("DomainsForProvider(%q): got %v want %q", c.provider, got, c.want)
		}
	}
	if _, err := DomainsForProvider("does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

// ---- dispatch errors (no network, no keychain) ----

func TestImportDispatchErrors(t *testing.T) {
	if _, err := Import("grok", "manual", ""); err == nil || !strings.Contains(err.Error(), "cookie set") {
		t.Fatalf("manual should point at `cookie set`, got %v", err)
	}
	if _, err := Import("grok", "firefox", ""); err == nil || !strings.Contains(err.Error(), "manual cookies") {
		t.Fatalf("unsupported browser should suggest manual cookies, got %v", err)
	}
	// Unknown provider must fail before any keychain access.
	if _, err := Import("bogus", "chrome", ""); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("unknown provider should error before touching the keychain, got %v", err)
	}
}

// ---- kimi-desktop end-to-end (HOME override, no real keychain) ----

func TestKimiDesktopImport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dbPath := filepath.Join(home, "Library", "Application Support", "kimi-desktop", "Cookies")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cookies (name TEXT, value TEXT, host_key TEXT, last_access_utc INTEGER)`); err != nil {
		t.Fatal(err)
	}
	rows := [][]any{
		{"kimi-auth", "stale-token", "kimi.com", int64(100)},
		{"kimi-auth", "fresh-token", "www.kimi.com", int64(200)},
		{"kimi-auth", "wrong-host", "example.com", int64(300)},
		{"other", "ignored", "www.kimi.com", int64(400)},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO cookies VALUES (?, ?, ?, ?)`, r...); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	header, err := Import("kimi", "kimi-desktop", "")
	if err != nil {
		t.Fatalf("kimi-desktop import: %v", err)
	}
	if header != "kimi-auth=fresh-token" {
		t.Fatalf("kimi-desktop header: got %q want %q", header, "kimi-auth=fresh-token")
	}

	if _, err := Import("grok", "kimi-desktop", ""); err == nil {
		t.Fatal("expected an error importing kimi-desktop cookies for grok")
	}

	if err := os.Remove(dbPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Import("kimi", "kimi-desktop", ""); err == nil || !strings.Contains(err.Error(), "kimi-desktop cookie store") {
		t.Fatalf("expected an actionable missing-store error, got %v", err)
	}
}
