package auth

// Diagnostic (run: LEEWAY_DIAGNOSE=1 go test ./internal/auth/ -run TestDiagnoseChrome -v)
// Inspects the real Chrome cookie store to explain import failures. Skipped
// by default: it reads the developer's real browser cookies and keychain, and
// is not part of the regular test run.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnoseChrome(t *testing.T) {
	if os.Getenv("LEEWAY_DIAGNOSE") == "" {
		t.Skip("diagnostic: set LEEWAY_DIAGNOSE=1 to inspect the real Chrome cookie store")
	}
	home, _ := os.UserHomeDir()
	userData := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	profiles, err := scanProfileDirs(userData)
	if err != nil {
		t.Fatalf("scanProfileDirs: %v", err)
	}
	t.Logf("profiles: %v", profiles)
	key, kerr := keychainKey("Chrome Safe Storage", "grok")
	if kerr != nil {
		t.Logf("keychain: %v", kerr)
	} else {
		t.Logf("keychain key: %d chars", len(key))
	}
	for _, prof := range profiles {
		path, ok := cookieDBForProfile(userData, prof)
		if !ok {
			continue
		}
		db, err := openCookieDB(path)
		if err != nil {
			t.Logf("%s: open failed: %v", prof, err)
			continue
		}
		total := countAll(db)
		t.Logf("%s: %d total cookies in %s", prof, total, path)
		for _, domain := range []string{"opencode.ai", "grok.com", "kimi.com", "www.kimi.com", "opencode.ai%"} {
			n := countLike(db, "%"+domain+"%")
			if n > 0 {
				t.Logf("  domain %q: %d rows", domain, n)
				names := cookieNames(db, "%"+domain+"%")
				t.Logf("    names: %v", names)
				hosts := cookieHosts(db, "%"+domain+"%")
				t.Logf("    hosts: %v", hosts)
				prefixes := cookiePrefixes(db, "%"+domain+"%", key)
				t.Logf("    encrypted prefixes/decrypt: %v", prefixes)
			}
		}
		db.Close()
	}
}

func countAll(db *sql.DB) int {
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM cookies").Scan(&n)
	return n
}

func countLike(db *sql.DB, like string) int {
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM cookies WHERE host_key LIKE ?", like).Scan(&n)
	return n
}

func cookieNames(db *sql.DB, like string) []string {
	rows, err := db.Query("SELECT name, host_key, LENGTH(encrypted_value) FROM cookies WHERE host_key LIKE ? LIMIT 40", like)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, host string
		var elen int
		if err := rows.Scan(&name, &host, &elen); err != nil {
			continue
		}
		out = append(out, fmt.Sprintf("%s@%s(len=%d)", name, host, elen))
	}
	return out
}

func cookieHosts(db *sql.DB, like string) []string {
	rows, err := db.Query("SELECT DISTINCT host_key FROM cookies WHERE host_key LIKE ?", like)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			continue
		}
		out = append(out, h)
	}
	return out
}

// cookiePrefixes reports encrypted_value prefixes and decrypt success. Only
// lengths and prefixes are logged — never cookie values, even truncated ones.
func cookiePrefixes(db *sql.DB, like, key string) []string {
	rows, err := db.Query("SELECT encrypted_value, value, host_key FROM cookies WHERE host_key LIKE ? LIMIT 40", like)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var enc []byte
		var val, host string
		if err := rows.Scan(&enc, &val, &host); err != nil {
			continue
		}
		prefix := "plain"
		if len(enc) > 0 {
			prefix = fmt.Sprintf("enc:%s(len=%d)", string(enc[:min(3, len(enc))]), len(enc))
		}
		dec := "—"
		if key != "" && len(enc) > 0 {
			if p, err := decryptValue(enc, key, host, 30); err == nil {
				dec = "ok"
				if len(p) > 0 {
					dec = fmt.Sprintf("ok(%d chars)", len(p))
				}
			} else {
				dec = "FAIL: " + err.Error()
			}
		}
		out = append(out, fmt.Sprintf("%s plaintext:%d chars → %s", prefix, len(val), dec))
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
