package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/provider"
)

func runStatus(args []string) int {
	var jsonOut bool
	fs := newFlagSet("status", &jsonOut)
	fs.Help(statusHelp)
	if !fs.Parse(args) {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读配置失败：", err)
		return 1
	}

	rows := collectAuthRows(cfg)
	hints := collectMissingHints(cfg, rows)
	path := configPath()

	if jsonOut {
		type jsonRow struct {
			Provider      string `json:"provider"`
			Account       string `json:"account"`
			Credential    string `json:"credential"`
			Status        string `json:"status"`
			Expires       string `json:"expires"`
			CookiePresent bool   `json:"cookiePresent"`
			APIKeyPresent bool   `json:"apiKeyPresent"`
		}
		type jsonHint struct {
			Provider string `json:"provider"`
			Hint     string `json:"hint"`
		}
		view := struct {
			Accounts   []jsonRow  `json:"accounts"`
			Hints      []jsonHint `json:"hints"`
			ConfigPath string     `json:"configPath"`
		}{
			Accounts:   make([]jsonRow, 0, len(rows)),
			Hints:      make([]jsonHint, 0, len(hints)),
			ConfigPath: path,
		}
		for _, r := range rows {
			cookie, key := rowPresence(cfg, r)
			view.Accounts = append(view.Accounts, jsonRow{
				Provider: r.provider, Account: r.account, Credential: r.cred,
				Status: r.status, Expires: r.expires,
				CookiePresent: cookie, APIKeyPresent: key,
			})
		}
		for _, h := range hints {
			view.Hints = append(view.Hints, jsonHint{Provider: h.provider, Hint: h.hint})
		}
		if code := newJSONEncoder().Encode(view); code != 0 {
			return code
		}
		return statusExit(rows, hints)
	}

	writeStatusTable(os.Stdout, rows, hints)
	fmt.Fprintln(os.Stderr, "配置文件：", path)
	return statusExit(rows, hints)
}

// The table is for humans; `status --json` is the machine contract and keeps
// the raw tokens built in authStatusRow (apiKey (config) / ok / expires today
// 16:31 / …). These labels are applied when rendering the table only, so the
// JSON values stay byte-for-byte what they were.

// writeStatusTable lays the table out here rather than through text/tabwriter:
// tabwriter aligns columns by rune count, but a CJK character occupies two
// terminal columns, so Chinese cells drift out of line with their header.
func writeStatusTable(w io.Writer, rows []authRow, hints []statusHint) {
	table := [][]string{{"服务商", "账号", "登录信息", "状态", "过期时间"}}
	for _, r := range rows {
		table = append(table, []string{r.provider, r.account, credLabel(r.cred), statusLabel(r.status), expiresLabel(r.expires)})
	}
	for _, h := range hints {
		table = append(table, []string{h.provider, "—", "—", statusLabel("missing"), h.hint})
	}

	widths := make([]int, len(table[0]))
	for _, row := range table {
		for i, c := range row {
			if n := displayWidth(c); n > widths[i] {
				widths[i] = n
			}
		}
	}
	for _, row := range table {
		var line strings.Builder
		for i, c := range row {
			line.WriteString(c)
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-displayWidth(c)+2))
			}
		}
		fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
	}
}

// displayWidth is the number of terminal columns s occupies.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n++
		if isWide(r) {
			n++
		}
	}
	return n
}

// isWide reports whether r occupies two terminal columns.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f, // Hangul Jamo
		r >= 0x2e80 && r <= 0xa4cf, // CJK radicals through Yi
		r >= 0xac00 && r <= 0xd7a3, // Hangul syllables
		r >= 0xf900 && r <= 0xfaff, // CJK compatibility ideographs
		r >= 0xfe30 && r <= 0xfe6f, // CJK compatibility forms
		r >= 0xff00 && r <= 0xff60, // fullwidth forms
		r >= 0xffe0 && r <= 0xffe6: // fullwidth signs
		return true
	}
	return false
}

// credLabel names a credential source in everyday Chinese.
func credLabel(cred string) string {
	switch cred {
	case "apiKey (config)":
		return "配置里的 API 密钥"
	case "authFile (private)", "cursor auth.json (private)":
		return "私有登录文件"
	case "~/.grok (shared)":
		return "官方共享登录文件"
	case "keychain":
		return "系统钥匙串"
	case "cookies":
		return "浏览器登录状态"
	case "codex auth.json", "local session":
		return "官方工具登录文件"
	}
	return cred // "—", or the raw auth file path the row reports
}

// statusLabel names a row state in everyday Chinese.
func statusLabel(status string) string {
	switch status {
	case "ok":
		return "正常"
	case "error":
		return "出错"
	case "not logged in":
		return "未登录"
	case "not configured":
		return "没配置"
	case "missing":
		return "缺登录信息"
	}
	return status
}

// expiresLabel rewrites the expiry wording built by authStatusRow's expiry
// closure. Anything else — a login command, a provider error — passes through.
func expiresLabel(expires string) string {
	switch {
	case expires == "—", expires == "":
		return expires
	case expires == "unknown":
		return "未知"
	case expires == "official CLI session":
		return "官方工具登录"
	case strings.HasPrefix(expires, "expires today "):
		return "今天 " + strings.TrimPrefix(expires, "expires today ") + " 过期"
	case strings.HasPrefix(expires, "expired "):
		return strings.TrimPrefix(expires, "expired ") + " 已过期"
	}
	if days, date, ok := strings.Cut(expires, "d · "); ok {
		if _, err := strconv.Atoi(days); err == nil {
			return days + " 天后 · " + date
		}
	}
	return expires
}

func statusExit(rows []authRow, hints []statusHint) int {
	if len(hints) > 0 || len(rows) == 0 {
		return 1
	}
	for _, r := range rows {
		if r.status != "ok" {
			return 1
		}
	}
	return 0
}

type statusHint struct{ provider, hint string }

func collectMissingHints(cfg *config.Config, rows []authRow) []statusHint {
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.provider] = true
	}
	var hints []statusHint
	for _, p := range provider.All() {
		id := p.ID()
		if !cfg.IsEnabled(id) || seen[id] {
			continue
		}
		if hint := missingHint(cfg, p); hint != "" {
			hints = append(hints, statusHint{id, hint})
		}
	}
	return hints
}

func missingHint(cfg *config.Config, p provider.Provider) string {
	if authStatusHasDefault(cfg, p.ID()) {
		return ""
	}
	if p.ID() == "cursor" {
		for _, a := range cursorAccounts(cfg) {
			if _, has, err := auth.LoadCursorAuth(a.Name); err == nil && has {
				return ""
			}
		}
	}
	return "leeway login " + p.ID()
}

func rowPresence(cfg *config.Config, r authRow) (cookie, apiKey bool) {
	if r.cred == "cookies" {
		cookie = true
	}
	if r.cred == "apiKey (config)" {
		apiKey = true
	}
	if cfg.APIKey(r.provider) != "" {
		apiKey = true
	}
	if a, ok := cfg.Account(r.provider, r.account); ok && a.APIKey != "" {
		apiKey = true
	}
	if h, err := auth.ResolveCookieHeader(r.provider, cfg.Provider(r.provider), r.account); err == nil && h != "" {
		cookie = true
	}
	return cookie, apiKey
}
