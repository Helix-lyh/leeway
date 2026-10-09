// Package switcher writes a leeway-held cursor/grok session into the official
// client location so the local IDE or CLI actually uses that account.
//
// Probe and login stay isolated. This package runs only on an explicit
// `leeway use` for cursor or grok.
package switcher

import (
	"bytes"
	"context"
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

var errUnsupportedOS = errors.New("只有 macOS 和 Linux 能把登录切换给官方工具")

// Result is what `leeway use` prints: where the session was written, no secrets.
type Result struct {
	Provider string   `json:"provider"`
	Account  string   `json:"account"`
	Email    string   `json:"email,omitempty"`
	Targets  []string `json:"targets"`
	SavedAs  string   `json:"savedPreviousAs,omitempty"`
	// Unsaved is true when the login that was there before could not be kept as
	// a leeway account, so switching away from it loses it for good.
	Unsaved bool `json:"previousUnsaved,omitempty"`
}

// CursorTargets are the official Cursor stores we write. Tests inject temp
// paths. Keychain is the macOS keychain service the official cursor-agent
// reads; it is empty when there is no such item.
type CursorTargets struct {
	StateDB  string
	AuthJSON string
	Keychain string
}

// OfficialCursorTargets resolves the Cursor IDE db, the cursor-agent keychain
// item and the CLI auth.json for this OS. A machine runs only the agent, only
// the IDE, or both, so every target is optional: what is not there is skipped,
// and a machine with none of them has nothing to switch.
//
// LEEWAY_CURSOR_STATE_DB / LEEWAY_CURSOR_AUTH_JSON redirect the file stores and
// LEEWAY_CURSOR_KEYCHAIN the keychain item (0 disables it). Naming either file
// override keeps the real keychain out of the write; each file store is
// redirected on its own, so a run that names only one still resolves the other
// to the machine's own path.
func OfficialCursorTargets() (CursorTargets, error) {
	t := CursorTargets{
		StateDB:  os.Getenv("LEEWAY_CURSOR_STATE_DB"),
		AuthJSON: os.Getenv("LEEWAY_CURSOR_AUTH_JSON"),
	}
	if v, ok := os.LookupEnv("LEEWAY_CURSOR_KEYCHAIN"); ok {
		if v != "0" {
			t.Keychain = v
		}
	} else if t.StateDB == "" && t.AuthJSON == "" {
		t.Keychain = defaultCursorKeychain()
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
		// Only an auth.json that is already there: writing one would invent a
		// Cursor install on a machine that has none, and `use` has to fail
		// loudly when there is nothing to switch.
		p := filepath.Join(home, ".cursor", "auth.json")
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			t.AuthJSON = p
		}
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

// defaultCursorKeychain is the keychain item the official cursor-agent keeps
// its token in. An item that is not there means the agent is not using the
// keychain (never installed, or its file store is in use), so nothing to write.
func defaultCursorKeychain() string {
	if runtime.GOOS != "darwin" || !auth.CursorKeychainItemExists(context.Background(), auth.CursorKeychainService) {
		return ""
	}
	return auth.CursorKeychainService
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
	return pickCursorStateDB(candidates)
}

func pickCursorStateDB(candidates []string) (string, error) {
	var found []string
	for _, p := range candidates {
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		found = append(found, p)
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("找到 %d 个 Cursor state.vscdb（%s）；请用 LEEWAY_CURSOR_STATE_DB 指定写哪一个", len(found), strings.Join(found, ", "))
	}
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

// ApplyCursor writes tokens into every official store the machine has: the
// cursor-agent keychain item, the IDE sqlite and ~/.cursor/auth.json.
func ApplyCursor(t CursorTargets, ca auth.CursorAuth, email string) error {
	if ca.AccessToken == "" {
		return errors.New("cursor 的登录信息里没有 access token")
	}
	if t.StateDB == "" && t.AuthJSON == "" && t.Keychain == "" {
		return errors.New("这台机器上没找到能写 Cursor 登录的地方（IDE 的 state.vscdb、cursor-agent 的 keychain、~/.cursor/auth.json 都没有）。请先装并登录 Cursor")
	}
	// The keychain write is the one that can be refused (macOS prompts), so it
	// goes first: a refusal then leaves the other stores untouched.
	if t.Keychain != "" {
		if err := auth.SetCursorKeychainToken(context.Background(), t.Keychain, ca.AccessToken); err != nil {
			return err
		}
	}
	if t.StateDB != "" {
		if err := writeCursorStateDB(t.StateDB, ca, email); err != nil {
			return err
		}
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
		return errors.New("cursor state.vscdb 的路径是空的")
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
		return fmt.Errorf("打不开 Cursor 的 state.vscdb：%w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		return fmt.Errorf("准备 Cursor 的 ItemTable 失败：%w", err)
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
		return fmt.Errorf("写入 cursorAuth/accessToken 失败：%w", err)
	}
	if err := upsert("cursorAuth/refreshToken", ca.RefreshToken); err != nil {
		return fmt.Errorf("写入 cursorAuth/refreshToken 失败：%w", err)
	}
	if err := upsert("cursorAuth/cachedEmail", email); err != nil {
		return fmt.Errorf("写入 cursorAuth/cachedEmail 失败：%w", err)
	}
	if err := upsert("cursor.accessToken", ca.AccessToken); err != nil {
		return fmt.Errorf("写入 cursor.accessToken 失败：%w", err)
	}
	if err := upsert("cursor.email", email); err != nil {
		return fmt.Errorf("写入 cursor.email 失败：%w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("没能写入 Cursor 的 state.vscdb：%w", err)
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

// SnapshotCursorIfNeeded copies the logins the switch is about to overwrite
// (the IDE db and the cursor-agent keychain item) into leeway private HOMEs
// when they are not stored yet. saved is the new account name; unsaved is true
// when a different official token was seen but could not be kept.
func SnapshotCursorIfNeeded(t CursorTargets, incomingToken string) (saved string, unsaved bool, err error) {
	dbToken, dbEmail, err := ReadCursorStateToken(t.StateDB)
	if err != nil {
		return "", false, err
	}
	candidates := []string{dbToken}
	emails := []string{dbEmail}
	if t.Keychain != "" {
		kt, kerr := auth.CursorKeychainTokenFor(context.Background(), t.Keychain)
		switch {
		case kerr == nil:
			candidates = append(candidates, kt)
			emails = append(emails, "")
		case auth.IsKeychainItemMissing(kerr):
			// No item, nothing to keep.
		default:
			// The current login could not be read, so it cannot be kept:
			// writing over it now would lose the machine's own session.
			return "", false, fmt.Errorf("读不到本机 cursor 的 keychain 登录，已放弃切换（不然会丢掉当前登录）：%v", kerr)
		}
	}
	for i, token := range candidates {
		s, u, err := snapshotCursorToken(t, token, emails[i], incomingToken)
		if err != nil {
			return saved, unsaved, err
		}
		if s != "" && saved == "" {
			saved = s
		}
		unsaved = unsaved || u
	}
	return saved, unsaved, nil
}

func snapshotCursorToken(t CursorTargets, token, email, incomingToken string) (saved string, unsaved bool, err error) {
	if token == "" && fileExists(t.AuthJSON) {
		if raw, rerr := os.ReadFile(t.AuthJSON); rerr == nil {
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
	if ca.RefreshToken == "" {
		// The keychain and the IDE db hold the access token alone, and a token
		// without its refresh token cannot be written back into a client. Say
		// the login was not kept rather than promise a snapshot that cannot be
		// restored.
		return "", true, nil
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
		return fmt.Errorf("读取 grok 登录文件失败：%w", err)
	}
	var dest []byte
	if fileExists(dst) {
		dest, err = os.ReadFile(dst)
		if err != nil {
			return fmt.Errorf("读取官方 grok auth.json 失败：%w", err)
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
		return nil, fmt.Errorf("解析 grok 登录文件失败：%w", err)
	}
	dest := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(dst)) > 0 {
		dest, err = grokRegistry(dst)
		if err != nil {
			return nil, fmt.Errorf("解析官方 grok auth.json 失败：%w", err)
		}
	}
	var oidc []string
	for k := range source {
		if strings.HasPrefix(k, "https://auth.x.ai::") {
			oidc = append(oidc, k)
		}
	}
	if len(oidc) == 0 {
		return nil, errors.New("grok 的登录文件里没有 xAI 登录信息")
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
		return nil, errors.New("grok 的 auth.json 不是 JSON 对象，请换一份官方登录文件")
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

// DisplayTargets names the stores a Cursor switch wrote, skipping the ones
// this machine does not have.
func DisplayTargets(t CursorTargets) []string {
	var out []string
	if t.Keychain != "" {
		out = append(out, "keychain "+t.Keychain)
	}
	if t.StateDB != "" {
		out = append(out, DisplayPath(t.StateDB))
	}
	if t.AuthJSON != "" {
		out = append(out, DisplayPath(t.AuthJSON))
	}
	return out
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
