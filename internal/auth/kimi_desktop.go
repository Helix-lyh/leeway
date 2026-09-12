package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Kimi desktop (L1) importer: the official Kimi desktop app is a Chromium
// shell whose cookie store lives at
// ~/Library/Application Support/kimi-desktop/Cookies. Mirrors the Swift
// KimiDesktopAuthToken reader: the app stores its session as a plaintext
// `value` column (no keychain decryption involved), the newest kimi-auth
// row wins, and a read-only open falls back to immutable mode when the
// WAL sidecars are gone after a clean shutdown.

// kimiDesktopCookieDB returns the official Kimi desktop Chromium cookie
// store path (mirrors KimiDesktopAuthToken.cookiesDatabaseURL).
func kimiDesktopCookieDB() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory: %w", err)
	}
	mac := filepath.Join(home, "Library", "Application Support", "kimi-desktop", "Cookies")
	if runtime.GOOS == "darwin" {
		return mac, nil
	}
	linux := filepath.Join(home, ".config", "kimi-desktop", "Cookies")
	if fi, err := os.Stat(linux); err == nil && fi.Mode().IsRegular() {
		return linux, nil
	}
	if fi, err := os.Stat(mac); err == nil && fi.Mode().IsRegular() {
		return mac, nil
	}
	return linux, nil
}

// kimiDesktopQuery is the exact query from KimiDesktopAuthToken.read:
// plaintext value column, kimi-auth name, matching host keys, newest access
// wins.
const kimiDesktopQuery = `SELECT value
FROM cookies
WHERE name = 'kimi-auth'
  AND host_key IN ('www.kimi.com', '.www.kimi.com', '.kimi.com', 'kimi.com')
ORDER BY last_access_utc DESC
LIMIT 1`

// importKimiDesktop imports the kimi-auth session token from the Kimi
// desktop app's own cookie store.
func importKimiDesktop(providerID string) (string, error) {
	if providerID != "kimi" {
		return "", fmt.Errorf("kimi-desktop cookies only exist for the kimi provider (got %q) — use browser \"chrome\" or manual cookies: leeway login %s --manual \"name=value; ...\"", providerID, providerID)
	}
	path, err := kimiDesktopCookieDB()
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("kimi-desktop cookie store %s not found — open the Kimi desktop app and sign in, then retry `leeway login kimi --cookie --browser kimi-desktop`; or: leeway login kimi --manual", path)
	}
	db, err := openCookieDB(path)
	if err != nil {
		return "", fmt.Errorf("could not open kimi-desktop cookie store %s: %v — close the Kimi desktop app or use manual cookies", path, err)
	}
	defer db.Close()

	var token string
	err = db.QueryRow(kimiDesktopQuery).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("no kimi-auth cookie in %s — sign in to the Kimi desktop app and retry, or use manual cookies", path)
	}
	if err != nil {
		return "", fmt.Errorf("could not read kimi-auth cookie from %s: %v", path, err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("kimi-auth cookie in %s is empty — sign in to the Kimi desktop app and retry, or use manual cookies", path)
	}
	return "kimi-auth=" + token, nil
}
