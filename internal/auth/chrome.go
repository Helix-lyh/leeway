package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// Chrome/Edge cookie importer (L2, macOS). macOS Chromium encrypts cookies
// with AES-128-CBC (IV = 16 spaces, PKCS7), key = PBKDF2-HMAC-SHA1(keychain
// password, salt "saltysalt", 1003 iterations, 16 bytes). Modern blobs carry
// a "v10" prefix; cookie databases with user_version >= 24 bind the
// plaintext to the host with a SHA-256(host_key) prefix that must be
// stripped. Mirrors SweetCookieKit's ChromeCookieImporter (the library
// CodexBar ships).

// keychainKey reads a Chromium-family "Safe Storage" generic password from
// the macOS keychain via /usr/bin/security. providerID names the provider the
// import is for, substituted into the manual-cookie hint so the command is
// copy-paste usable.
func keychainKey(service, providerID string) (string, error) {
	cmd := exec.Command("/usr/bin/security", "find-generic-password", "-w", "-s", service)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// The security(1) output goes last: failureText splits off only the tail
		// after the final colon, so the advice stays on screen and the tool's
		// own words drop to the tooltip.
		detail := strings.TrimSpace(stderr.String())
		return "", fmt.Errorf("读不了 %q 的 keychain 密钥 — 请授权访问 keychain（首次导入会弹窗）或手动填登录状态（leeway login %s --manual \"name=value; ...\"）：%v", service, providerID, detail)
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", fmt.Errorf("%q 的 keychain 密钥是空的 — 请授权访问 keychain，或手动填登录状态（leeway login %s --manual \"name=value; ...\"）", service, providerID)
	}
	return key, nil
}

// chromeUserDataDir returns the browser's macOS user-data directory.
func chromeUserDataDir(browser string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("找不到主目录：%w", err)
	}
	switch browser {
	case "chrome":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome"), nil
	case "edge":
		return filepath.Join(home, "Library", "Application Support", "Microsoft", "Edge"), nil
	default:
		return "", fmt.Errorf("还不支持 %q 这个浏览器", browser)
	}
}

// openDSN opens a SQLite database read-only. immutable reads the file as a
// static snapshot and works even when the WAL sidecars are gone.
func openDSN(path string, immutable bool) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	q.Set("mode", "ro")
	if immutable {
		q.Set("immutable", "1")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// openCookieDB opens a Chromium cookie store read-only. A plain read-only
// open is tried first so committed WAL records of a running browser stay
// visible; if that fails (Chromium can leave the main file in WAL mode with
// both sidecars removed after a clean shutdown), fall back to immutable
// mode which reads the idle file without recreating sidecars.
func openCookieDB(path string) (*sql.DB, error) {
	db, err := openDSN(path, false)
	if err == nil {
		return db, nil
	}
	if !walSidecarsMissing(path) {
		return nil, err
	}
	db, err = openDSN(path, true)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func walSidecarsMissing(path string) bool {
	_, werr := os.Stat(path + "-wal")
	_, serr := os.Stat(path + "-shm")
	return errors.Is(werr, os.ErrNotExist) && errors.Is(serr, os.ErrNotExist)
}

// pbkdf2SHA1 is a hand-rolled PBKDF2-HMAC-SHA1 (kept local to avoid adding
// golang.org/x/crypto as a dependency).
func pbkdf2SHA1(password, salt []byte, iterations, keyLen int) ([]byte, error) {
	if iterations <= 0 || keyLen <= 0 {
		return nil, errors.New("pbkdf2：迭代次数和密钥长度必须大于 0")
	}
	h := hmac.New(sha1.New, password)
	hashLen := h.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	dk := make([]byte, 0, numBlocks*hashLen)
	var block [sha1.Size]byte
	for i := 1; i <= numBlocks; i++ {
		h.Reset()
		h.Write(salt)
		var be [4]byte
		binary.BigEndian.PutUint32(be[:], uint32(i))
		h.Write(be[:])
		u := h.Sum(nil)
		copy(block[:], u)
		for j := 1; j < iterations; j++ {
			h.Reset()
			h.Write(u)
			u = h.Sum(nil)
			for k := range block {
				block[k] ^= u[k]
			}
		}
		dk = append(dk, block[:]...)
	}
	return dk[:keyLen], nil
}

// macOS Chromium cookie encryption (mirrors SweetCookieKit's
// ChromeCookieImporter, which is what CodexBar ships):
//
//   - key = PBKDF2-HMAC-SHA1(password, salt="saltysalt", 1003 iters, 16 bytes)
//   - encrypted_value = ["v10"] + AES-128-CBC(key, IV=16×0x20, PKCS7)
//   - cookie databases with user_version >= 24 additionally prefix the
//     plaintext with SHA-256(host_key) (32 bytes), which must be stripped.
//
// The "v10" prefix on macOS wraps this CBC scheme (NOT the Windows/Linux
// AES-256-GCM scheme — a common trap when porting from the Chromium docs).

// macSafeStorageSalt is Chromium's hardcoded macOS key-derivation salt.
var macSafeStorageSalt = []byte("saltysalt")

// macCBCIV is the fixed CBC IV: 16 space bytes.
var macCBCIV = bytes.Repeat([]byte{0x20}, aes.BlockSize)

// decryptValue decrypts a Chromium cookie's encrypted_value (macOS scheme).
func decryptValue(encrypted []byte, password, hostKey string, dbVersion int) ([]byte, error) {
	payload := encrypted
	if len(encrypted) >= 3 && string(encrypted[:3]) == "v10" {
		payload = encrypted[3:]
	}
	key, err := pbkdf2SHA1([]byte(password), macSafeStorageSalt, 1003, 16)
	if err != nil {
		return nil, err
	}
	plain, err := decryptCBC(payload, key, macCBCIV)
	if err != nil {
		return nil, err
	}
	if dbVersion >= 24 {
		if len(plain) < sha256.Size {
			return nil, errors.New("明文比主机名哈希前缀还短")
		}
		hash := sha256.Sum256([]byte(hostKey))
		if !hmac.Equal(hash[:], plain[:sha256.Size]) {
			return nil, errors.New("登录状态的主机名哈希对不上")
		}
		plain = plain[sha256.Size:]
	}
	return plain, nil
}

// decryptCBC decrypts AES-128-CBC with PKCS7 padding validation.
func decryptCBC(encrypted, key, iv []byte) ([]byte, error) {
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, errors.New("数据长度不是 AES 块大小的整数倍")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, encrypted)
	// Strip PKCS7 padding.
	padLen := int(plain[len(plain)-1])
	if padLen == 0 || padLen > aes.BlockSize || padLen > len(plain) {
		return nil, errors.New("数据的 PKCS7 填充不合法")
	}
	for _, b := range plain[len(plain)-padLen:] {
		if int(b) != padLen {
			return nil, errors.New("数据的 PKCS7 填充不合法")
		}
	}
	return plain[:len(plain)-padLen], nil
}

// cookieDatabaseVersion reads the cookies table's version, used for the
// host-key hash check (>= 24). Chromium stores it in the meta table
// (key='version'); PRAGMA user_version is 0 on modern Chrome and must NOT be
// used (SweetCookieKit reads the same meta table).
func cookieDatabaseVersion(db *sql.DB) int {
	var v int
	err := db.QueryRow("SELECT CAST(value AS INTEGER) FROM meta WHERE key = 'version'").Scan(&v)
	if err == nil {
		return v
	}
	// Fallback for stores without a meta table.
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0
	}
	return v
}

// cookieRow is one raw row of a Chromium cookies table.
type cookieRow struct {
	name, value, hostKey, path string
	expiresUTC                 int64
	httpOnly                   bool
	encrypted                  []byte
}

func (c cookieRow) isSession() bool { return c.expiresUTC == 0 }

// preferRecord mirrors the Swift importers' merge rule: session cookies (no
// expiry) beat persistent ones, otherwise the later expiry wins.
func preferRecord(a, b cookieRow) bool {
	switch {
	case a.isSession() && !b.isSession():
		return true
	case !a.isSession() && b.isSession():
		return false
	default:
		return b.expiresUTC > a.expiresUTC
	}
}

// loadCookieRows reads every cookie whose host_key matches one of the
// domains, deduplicating by name|host_key|path (later expiry preferred).
// The is_httponly column is absent in older stores; the query retries
// without it in that case.
func loadCookieRows(db *sql.DB, domains []string) ([]cookieRow, error) {
	const fullQuery = `SELECT name, value, encrypted_value, host_key, path, expires_utc, is_httponly
FROM cookies
WHERE host_key LIKE '%' || ? || '%'`
	const basicQuery = `SELECT name, value, encrypted_value, host_key, path, expires_utc
FROM cookies
WHERE host_key LIKE '%' || ? || '%'`

	seen := map[string]cookieRow{}
	for _, domain := range domains {
		rows, err := db.Query(fullQuery, domain)
		hasHttpOnly := err == nil
		if err != nil {
			if rows, err = db.Query(basicQuery, domain); err != nil {
				return nil, fmt.Errorf("查询 %q 的登录状态：%w", domain, err)
			}
		}
		for rows.Next() {
			var r cookieRow
			if hasHttpOnly {
				var h int64
				if err := rows.Scan(&r.name, &r.value, &r.encrypted, &r.hostKey, &r.path, &r.expiresUTC, &h); err != nil {
					rows.Close()
					return nil, fmt.Errorf("读登录状态那一行失败：%w", err)
				}
				r.httpOnly = h != 0
			} else if err := rows.Scan(&r.name, &r.value, &r.encrypted, &r.hostKey, &r.path, &r.expiresUTC); err != nil {
				rows.Close()
				return nil, fmt.Errorf("读登录状态那一行失败：%w", err)
			}
			key := r.name + "|" + r.hostKey + "|" + r.path
			if old, ok := seen[key]; ok {
				if preferRecord(old, r) {
					seen[key] = r
				}
			} else {
				seen[key] = r
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("逐行读登录状态失败：%w", err)
		}
	}
	out := make([]cookieRow, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	return out, nil
}

// decodedCookie is a cookie ready to go into the request header.
type decodedCookie struct {
	name, value string
	httpOnly    bool
	session     bool
}

// decryptRows decrypts encrypted_value blobs, falling back to the plaintext
// value column, and drops empty or undecryptable cookies. dropped counts rows
// that were present but could not be decrypted — callers use it to tell
// "no matching cookies" from "cookies exist but the Safe Storage key is wrong".
func decryptRows(rows []cookieRow, password string, dbVersion int) (out []decodedCookie, dropped, decrypted int) {
	out = make([]decodedCookie, 0, len(rows))
	for _, r := range rows {
		v := r.value
		if len(r.encrypted) > 0 {
			plain, err := decryptValue(r.encrypted, password, r.hostKey, dbVersion)
			if err != nil {
				dropped++ // undecryptable — skip
				continue
			}
			decrypted++
			v = string(plain)
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, decodedCookie{name: r.name, value: v, httpOnly: r.httpOnly, session: r.isSession()})
	}
	return out, dropped, decrypted
}

// encryptedCookiesUnusable is true when every encrypted cookie failed to
// decrypt. Leftover plaintext must not be treated as a usable session.
func encryptedCookiesUnusable(dropped, decrypted int) bool {
	return dropped > 0 && decrypted == 0
}

// buildHeader joins cookies into a "name=value; name=value" header. Order is
// deterministic: HttpOnly session cookies first, then by name. Cookies whose
// name or value contains control characters are dropped — Go's HTTP client
// rejects header values with control bytes, and a single bad cookie would
// otherwise poison the whole import.
func buildHeader(cookies []decodedCookie) string {
	sort.Slice(cookies, func(i, j int) bool {
		a, b := cookies[i], cookies[j]
		prio := func(c decodedCookie) int {
			p := 0
			if !c.session {
				p++
			}
			if !c.httpOnly {
				p++
			}
			return p
		}
		if ap, bp := prio(a), prio(b); ap != bp {
			return ap < bp
		}
		if a.name != b.name {
			return a.name < b.name
		}
		return a.value < b.value
	})
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if c.name == "" || hasControl(c.name) || hasControl(c.value) {
			continue
		}
		parts = append(parts, c.name+"="+c.value)
	}
	return strings.Join(parts, "; ")
}

// hasControl reports whether s contains bytes below 0x20 (invalid in HTTP
// header values).
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

// scanProfileDirs lists Chromium profile directories (Default, Profile N),
// Default first, then numerically.
var profileRe = regexp.MustCompile(`^Profile \d+$`)

func scanProfileDirs(userData string) ([]string, error) {
	entries, err := os.ReadDir(userData)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && (e.Name() == "Default" || profileRe.MatchString(e.Name())) {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i] == "Default" {
			return true
		}
		if dirs[j] == "Default" {
			return false
		}
		return profileNumber(dirs[i]) < profileNumber(dirs[j])
	})
	return dirs, nil
}

func profileNumber(name string) int {
	var n int
	fmt.Sscanf(strings.TrimPrefix(name, "Profile "), "%d", &n)
	return n
}

// cookieDBForProfile returns the cookie DB path for a profile (Network/
// Cookies on modern Chromium, plain Cookies on older installs).
func cookieDBForProfile(userData, profile string) (string, bool) {
	for _, p := range []string{
		filepath.Join(userData, profile, "Network", "Cookies"),
		filepath.Join(userData, profile, "Cookies"),
	} {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, true
		}
	}
	return "", false
}

func countMatchingCookies(path string, domains []string) (int, error) {
	db, err := openCookieDB(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	total := 0
	for _, d := range domains {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM cookies WHERE host_key LIKE '%' || ? || '%'", d).Scan(&n); err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// pickCookieDB picks the cookie store to import from. When profile is
// non-empty, that profile's store is used directly; otherwise the profile
// whose store holds the most cookies for the target domains wins (Default
// wins ties).
func pickCookieDB(userData, profile string, domains []string) (string, error) {
	if profile != "" {
		path, ok := cookieDBForProfile(userData, profile)
		if !ok {
			return "", fmt.Errorf("配置 %q 不在 %s 下", profile, userData)
		}
		n, err := countMatchingCookies(path, domains)
		if err != nil {
			return "", fmt.Errorf("配置 %q 的登录状态读不了：%w", profile, err)
		}
		if n <= 0 {
			return "", fmt.Errorf("没找到 %v 的登录状态（配置 %q）", domains, profile)
		}
		return path, nil
	}
	profiles, err := scanProfileDirs(userData)
	if err != nil {
		return "", fmt.Errorf("没找到 %s 的配置目录：%w", userData, err)
	}
	best, bestCount := "", -1
	for _, p := range profiles {
		path, ok := cookieDBForProfile(userData, p)
		if !ok {
			continue
		}
		n, err := countMatchingCookies(path, domains)
		if err != nil {
			continue // unreadable or locked profile — skip
		}
		if n > bestCount {
			best, bestCount = path, n
		}
	}
	if bestCount <= 0 {
		return "", fmt.Errorf("没找到 %v 的登录状态，%s 下哪个配置里都没有", domains, userData)
	}
	return best, nil
}

// importChrome implements the L2 Chrome/Edge importer for macOS. profile
// names a specific profile directory; empty auto-picks the profile with the
// most matching cookies.
func importChrome(providerID, browser, profile string) (string, error) {
	domains, err := DomainsForProvider(providerID)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("读浏览器登录状态只能在 macOS 上用 — Linux 上请用 leeway login %s --manual \"name=value; ...\"", providerID)
	}
	userData, err := chromeUserDataDir(browser)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(userData); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s 没装，或者它的配置目录 %s 不在 — 请先在 %s 里登录，或手动填登录状态（leeway login %s --manual \"name=value; ...\"）", browser, userData, browser, providerID)
	}
	service := "Chrome Safe Storage"
	if browser == "edge" {
		service = "Microsoft Edge Safe Storage"
	}
	password, err := keychainKey(service, providerID)
	if err != nil {
		return "", err
	}
	dbPath, err := pickCookieDB(userData, profile, domains)
	if err != nil {
		return "", fmt.Errorf("从 %s 读登录状态失败：%v — 先登录 %s（用 %s），再跑 `leeway login %s --cookie --force`；或改用 --manual", browser, err, providerID, browser, providerID)
	}
	db, err := openCookieDB(dbPath)
	if err != nil {
		return "", fmt.Errorf("打不开 %s 的登录状态数据库 %s，请关掉浏览器或手动填登录状态：%v", browser, dbPath, err)
	}
	defer db.Close()
	rows, err := loadCookieRows(db, domains)
	if err != nil {
		return "", fmt.Errorf("读不了 %s 里的登录状态（%s）：%v", browser, dbPath, err)
	}
	dbVersion := cookieDatabaseVersion(db)
	decoded, dropped, decrypted := decryptRows(rows, password, dbVersion)
	if encryptedCookiesUnusable(dropped, decrypted) {
		return "", fmt.Errorf("找到 %d 条 %s 的登录状态（%s），但一条都解不开 — Chrome Safe Storage 密钥可能过期，或者 keychain 访问被拒绝；请授权 keychain（security find-generic-password -s %q）后重试，或手动填登录状态（leeway login %s --manual \"name=value; ...\"）", dropped, providerID, browser, service, providerID)
	}
	header := buildHeader(decoded)
	if header == "" {
		if dropped > 0 {
			// Cookies were found but every encrypted value failed to decrypt:
			// re-logging-in cannot fix that, the key is stale or keychain access
			// was denied mid-import.
			return "", fmt.Errorf("找到 %d 条 %s 的登录状态（%s），但一条都解不开 — Chrome Safe Storage 密钥可能过期，或者 keychain 访问被拒绝；请授权 keychain（security find-generic-password -s %q）后重试，或手动填登录状态（leeway login %s --manual \"name=value; ...\"）", dropped, providerID, browser, service, providerID)
		}
		return "", fmt.Errorf("没找到 %s 的登录状态（%v），%s 里没有 — 请先登录 %s（用 %s），再跑 `leeway login %s --cookie --force`", providerID, domains, browser, providerID, browser, providerID)
	}
	return header, nil
}
