// Package switcher writes a leeway-held cursor/grok session into the official
// client location so the local IDE or CLI actually uses that account.
//
// Probe and login stay isolated. This package runs only on an explicit
// `leeway use` for cursor or grok.
package switcher

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"

	"leeway/internal/auth"
)

var errUnsupportedOS = errors.New("cursor/grok switch supports macOS and Linux only")

// Result is what `leeway use` prints: where the session was written, no secrets.
type Result struct {
	Provider string   `json:"provider"`
	Account  string   `json:"account"`
	Email    string   `json:"email,omitempty"`
	Targets  []string `json:"targets"`
	SavedAs  string   `json:"savedPreviousAs,omitempty"`
}

// CursorTargets are the official Cursor files we write. Tests inject temp paths.
type CursorTargets struct {
	StateDB  string
	AuthJSON string
}

// OfficialCursorTargets resolves the Cursor IDE db and CLI auth.json for this OS.
// LEEWAY_CURSOR_STATE_DB / LEEWAY_CURSOR_AUTH_JSON override the paths (tests).
func OfficialCursorTargets() (CursorTargets, error) {
	t := CursorTargets{
		StateDB:  os.Getenv("LEEWAY_CURSOR_STATE_DB"),
		AuthJSON: os.Getenv("LEEWAY_CURSOR_AUTH_JSON"),
	}
	if t.StateDB != "" && t.AuthJSON != "" {
		return t, nil
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return CursorTargets{}, errUnsupportedOS
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return CursorTargets{}, err
	}
	if t.AuthJSON == "" {
		t.AuthJSON = filepath.Join(home, ".cursor", "auth.json")
	}
	if t.StateDB == "" {
		p, err := cursorStateDB(home)
		if err != nil {
			return CursorTargets{}, err
		}
		t.StateDB = p
	}
	return t, nil
}

func cursorStateDB(home string) (string, error) {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb"),
		}
	case "linux":
		candidates = []string{
			filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb"),
			filepath.Join(home, ".var", "app", "com.cursor.Cursor", "config", "Cursor", "User", "globalStorage", "state.vscdb"),
			filepath.Join(home, "snap", "cursor", "current", ".config", "Cursor", "User", "globalStorage", "state.vscdb"),
			filepath.Join(home, "snap", "cursor", "common", ".config", "Cursor", "User", "globalStorage", "state.vscdb"),
		}
	default:
		return "", errUnsupportedOS
	}
	if p := newestExisting(candidates); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("Cursor state.vscdb not found (looked in %s) — open Cursor once, then retry", strings.Join(candidates, ", "))
}

func newestExisting(paths []string) string {
	best, bestTime := "", int64(0)
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if t := fi.ModTime().UnixNano(); best == "" || t > bestTime {
			best, bestTime = p, t
		}
	}
	return best
}

// OfficialGrokAuthFile is ~/.grok/auth.json. LEEWAY_GROK_OFFICIAL overrides it.
func OfficialGrokAuthFile() (string, error) {
	if p := os.Getenv("LEEWAY_GROK_OFFICIAL"); p != "" {
		return p, nil
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return "", errUnsupportedOS
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grok", "auth.json"), nil
}

// ApplyCursor writes tokens into the IDE sqlite and ~/.cursor/auth.json.
func ApplyCursor(t CursorTargets, ca auth.CursorAuth, email string) error {
	if ca.AccessToken == "" {
		return errors.New("cursor session has no access token")
	}
	if err := writeCursorStateDB(t.StateDB, ca, email); err != nil {
		return err
	}
	if t.AuthJSON == "" {
		return nil
	}
	raw, err := json.Marshal(ca)
	if err != nil {
		return err
	}
	return writeFile0600(t.AuthJSON, raw)
}

func writeCursorStateDB(path string, ca auth.CursorAuth, email string) error {
	if path == "" {
		return errors.New("cursor state.vscdb path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	q.Set("_pragma", "busy_timeout(1000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return fmt.Errorf("open Cursor state.vscdb: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		return fmt.Errorf("prepare Cursor ItemTable: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	upsert := func(key, value string) error {
		if value == "" {
			return nil
		}
		_, err := tx.Exec(`INSERT OR REPLACE INTO ItemTable (key, value) VALUES (?, ?)`, key, value)
		return err
	}
	if err := upsert("cursorAuth/accessToken", ca.AccessToken); err != nil {
		return fmt.Errorf("write cursorAuth/accessToken: %w", err)
	}
	if err := upsert("cursorAuth/refreshToken", ca.RefreshToken); err != nil {
		return fmt.Errorf("write cursorAuth/refreshToken: %w", err)
	}
	if err := upsert("cursorAuth/cachedEmail", email); err != nil {
		return fmt.Errorf("write cursorAuth/cachedEmail: %w", err)
	}
	if err := upsert("cursor.accessToken", ca.AccessToken); err != nil {
		return fmt.Errorf("write cursor.accessToken: %w", err)
	}
	if err := upsert("cursor.email", email); err != nil {
		return fmt.Errorf("write cursor.email: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Cursor state.vscdb: %w", err)
	}
	return nil
}

// ReadCursorStateToken returns the IDE's current access token and cached email.
func ReadCursorStateToken(stateDB string) (token, email string, err error) {
	if stateDB == "" || !fileExists(stateDB) {
		return "", "", nil
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(stateDB)}
	q := u.Query()
	q.Set("mode", "ro")
	q.Set("_pragma", "busy_timeout(1000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return "", "", err
	}
	defer db.Close()
	token = vscdbGet(db, "cursorAuth/accessToken")
	if token == "" {
		token = vscdbGet(db, "cursor.accessToken")
	}
	email = vscdbGet(db, "cursorAuth/cachedEmail")
	if email == "" {
		email = vscdbGet(db, "cursor.email")
	}
	return token, email, nil
}

func vscdbGet(db *sql.DB, key string) string {
	var value string
	err := db.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

// SnapshotCursorIfNeeded copies the current official Cursor session into a
// leeway private HOME when that token is not already stored. saved is the
// new account name; unsaved is true when a different official token was
// seen but could not be kept.
func SnapshotCursorIfNeeded(t CursorTargets, incomingToken string) (saved string, unsaved bool, err error) {
	token, email, err := ReadCursorStateToken(t.StateDB)
	if err != nil {
		return "", false, err
	}
	if token == "" && fileExists(t.AuthJSON) {
		raw, rerr := os.ReadFile(t.AuthJSON)
		if rerr == nil {
			var ca auth.CursorAuth
			if json.Unmarshal(raw, &ca) == nil {
				token = ca.AccessToken
			}
		}
	}
	if token == "" || token == incomingToken {
		return "", false, nil
	}
	held, err := tokenHeldByCursorSession(token)
	if err != nil {
		return "", false, err
	}
	if held {
		return "", false, nil
	}
	name := strings.TrimSpace(email)
	if name == "" {
		name = "imported"
	}
	if existing, ok, err := auth.LoadCursorAuth(name); err != nil {
		return "", false, err
	} else if ok && existing.AccessToken != "" && existing.AccessToken != token {
		return "", true, nil
	}
	ca := auth.CursorAuth{AccessToken: token}
	if raw, err := os.ReadFile(t.AuthJSON); err == nil {
		var file auth.CursorAuth
		if json.Unmarshal(raw, &file) == nil && file.AccessToken == token {
			ca = file
		}
	}
	if err := auth.WriteCursorAuth(name, ca); err != nil {
		return "", false, err
	}
	return name, false, nil
}

func tokenHeldByCursorSession(token string) (bool, error) {
	names, err := auth.ListCursorSessionAccounts()
	if err != nil {
		return false, err
	}
	for _, name := range names {
		ca, ok, err := auth.LoadCursorAuth(name)
		if err != nil {
			return false, err
		}
		if ok && ca.AccessToken == token {
			return true, nil
		}
	}
	return false, nil
}

// ApplyGrok merges the selected account's auth.json into the official
// ~/.grok/auth.json, replacing xAI OIDC scopes and leaving other registry
// entries alone.
func ApplyGrok(dst, src string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read grok session: %w", err)
	}
	var dest []byte
	if fileExists(dst) {
		dest, err = os.ReadFile(dst)
		if err != nil {
			return fmt.Errorf("read official grok auth.json: %w", err)
		}
	}
	out, err := mergeGrokAuth(dest, raw)
	if err != nil {
		return err
	}
	return writeFile0600(dst, out)
}

func mergeGrokAuth(dst, src []byte) ([]byte, error) {
	source, err := grokRegistry(src)
	if err != nil {
		return nil, fmt.Errorf("parse grok session: %w", err)
	}
	dest := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(dst)) > 0 {
		dest, err = grokRegistry(dst)
		if err != nil {
			return nil, fmt.Errorf("parse official grok auth.json: %w", err)
		}
	}
	var oidc []string
	for k := range source {
		if strings.HasPrefix(k, "https://auth.x.ai::") {
			oidc = append(oidc, k)
		}
	}
	if len(oidc) == 0 {
		return nil, errors.New("grok session has no xAI login")
	}
	for k := range dest {
		if strings.HasPrefix(k, "https://auth.x.ai::") {
			delete(dest, k)
		}
	}
	for _, k := range oidc {
		dest[k] = source[k]
	}
	return json.MarshalIndent(dest, "", "  ")
}

func grokRegistry(raw []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("grok auth.json is not an object")
	}
	return m, nil
}

func writeFile0600(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".leeway-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// DisplayPath shortens a home-rooted path to ~/… for stdout.
func DisplayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		return "~/" + filepath.ToSlash(path[len(prefix):])
	}
	return path
}
