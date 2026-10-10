package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/fetch"
	"leeway/internal/provider"
	"leeway/internal/provider/grok"
)

// keyPage is where a provider hands out API keys; `leeway login` opens it
// before prompting for the pasted value.
var keyPage = map[string]string{
	"deepseek":    "https://platform.deepseek.com/api_keys",
	"opencode-go": "https://opencode.ai/zen/go",
	"commandcode": "https://commandcode.ai/studio",
}

// loginCursor delegates the sign-in to the official cursor-agent CLI, run
// with AGENT_CLI_CREDENTIAL_STORE=file and a private HOME under leeway's
// session dir. The official client owns the whole OAuth/browser flow and
// writes <home>/.cursor/auth.json (0600); leeway only reads that file back,
// so nothing about the private protocol is reimplemented here. Each account
// gets its own HOME, so signing in one account never touches the official
// CLI/keychain state or another leeway account.
func loginCursor(ctx context.Context, account string, noBrowser bool) int {
	bin, err := exec.LookPath("cursor-agent")
	if err != nil {
		fmt.Fprintln(os.Stderr, "没找到 cursor-agent 命令，先按 https://cursor.com/cli 安装，再重试")
		return 2
	}

	home, err := auth.CursorHome(account)
	if err != nil {
		fmt.Fprintln(os.Stderr, "找 cursor 目录失败：", err)
		return 1
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "建目录失败：", err)
		return 1
	}

	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	pairs := []string{
		"AGENT_CLI_CREDENTIAL_STORE", "file", // write ~/.cursor/auth.json, not the keychain
		"HOME", home, // …into this account's private HOME
	}
	if noBrowser {
		pairs = append(pairs, "NO_OPEN_BROWSER", "1")
	}
	cmd.Env = overrideEnv(pairs...)
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "委托 cursor-agent 登录失败：", err)
		return 1
	}

	staging := account
	if staging == "" {
		staging = "default"
	}
	ca, ok, err := auth.LoadCursorAuth(staging)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读 cursor 登录文件失败：", err)
		return 1
	}
	if !ok || ca.AccessToken == "" {
		p, _ := auth.CursorAuthFile(staging)
		fmt.Fprintf(os.Stderr, "登录好像没完成：%s 里没有登录信息。请重跑 leeway login cursor\n", p)
		return 1
	}

	email, _ := auth.LookupCursorEmail(ctx, ca.AccessToken)
	name := strings.TrimSpace(account)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读配置失败：", err)
		return 1
	}
	if name == "" || name == "default" {
		if email != "" {
			if existing, ok := cfg.Account("cursor", email); ok {
				name = existing.Name
			} else {
				name = email
			}
		} else {
			name = "default"
		}
	}
	if name != staging {
		if err := auth.RelocateCursorHome(staging, name); err != nil {
			fmt.Fprintln(os.Stderr, "挪 cursor 目录失败：", err)
			return 1
		}
	}
	if err := cfg.SetAccountEmail("cursor", name, email); err != nil {
		fmt.Fprintln(os.Stderr, "保存配置失败：", err)
		return 1
	}

	var expiresIn string
	if exp, ok := auth.JWTExpiry(ca.AccessToken); ok {
		expiresIn = "，有效期到 " + exp.Local().Format("2006-01-02 15:04")
	}
	p, _ := auth.CursorAuthFile(name)
	label := accountStatusName(name, email)
	fmt.Fprintf(os.Stderr, "\n已登录 cursor 账号 %s%s，登录信息由官方工具写入 %s（权限 0600）\n", label, expiresIn, p)
	fmt.Fprintln(os.Stderr, "这个账号和官方 Cursor 工具、keychain 互不影响。要看用量，在 usage 后面加账号名；要让本机 Cursor 用这个账号：")
	fmt.Fprintln(os.Stderr, suffixAccount("leeway use cursor", name))
	return 0
}

// loginGrok defers to the official Grok CLI's device-code flow rather than
// reimplementing OAuth. Every account — named or the default — gets its own
// private GROK_HOME under leeway's session dir and is bound via
// accounts[].authFile, so grok CLI/browser logins on other accounts never
// touch the credential leeway reads (and vice versa).
func loginGrok(ctx context.Context, account string) int {
	name := account
	if name == "" || name == "default" {
		name = "default"
	}
	home, err := grokHome(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "找 grok 目录失败：", err)
		return 1
	}
	authFile := filepath.Join(home, "auth.json")
	if grok.HasOfficialShape(authFile) {
		fmt.Fprintf(os.Stderr, "grok 账号 %s 已有一份完整官方登录。让本机 grok 用它：leeway use grok %s；换一份：leeway login grok %s --file ~/.grok/auth.json\n", name, name, name)
		return 1
	}

	bin, err := exec.LookPath("grok")
	if err != nil {
		fmt.Fprintln(os.Stderr, "没找到 grok 命令，先安装：curl -fsSL https://x.ai/cli/install.sh | bash")
		return 2
	}

	cmd := exec.CommandContext(ctx, bin, "login", "--device-auth")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = overrideEnv("GROK_HOME", home)
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "委托 grok 登录失败：", err)
		return 1
	}

	shaped := grok.HasOfficialShape(authFile)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读配置失败：", err)
		return 1
	}
	if err := cfg.SetAccountAuthFile("grok", name, authFile); err != nil {
		fmt.Fprintln(os.Stderr, "保存配置失败：", err)
		return 1
	}
	label := name
	if email := grokAuthEmail(authFile); email != "" {
		_ = cfg.SetAccountEmail("grok", name, email)
		label = accountStatusName(name, email)
	}
	fmt.Fprintf(os.Stderr, "\n已登录 grok 账号 %s → %s\n", label, authFile)
	fmt.Fprintln(os.Stderr, "这个账号和官方 grok 工具（~/.grok）互不影响。看用量：leeway usage grok"+suffixFlag(name))
	if shaped {
		fmt.Fprintln(os.Stderr, "要让本机 grok 用这个账号：")
		fmt.Fprintln(os.Stderr, suffixAccount("leeway use grok", name))
	} else {
		fmt.Fprintf(os.Stderr, "这份登录缺官方字段，use 会拒绝写入。要让本机 grok 用它，先备份能打开 grok 的文件：leeway login grok %s --file ~/.grok/auth.json\n", name)
	}
	return 0
}

// grokHome is the isolated GROK_HOME for a named account.
func grokHome(account string) (string, error) {
	dir, err := auth.SessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "grok-"+safeSessionName(account)), nil
}

// loginAPIKey covers providers whose only credential is an API key: open the
// key page, then read the pasted value without echoing it, so the key never
// lands in shell history or the scrollback.
func loginAPIKey(providerID, account, email string, noBrowser bool, key string) int {
	key = strings.TrimSpace(key)
	if key == "" {
		page := keyPage[providerID]
		if page != "" && !noBrowser {
			fmt.Fprintf(os.Stderr, "正在打开 %s，请创建并复制 API 密钥…\n", page)
			if err := auth.OpenBrowser(page); err != nil {
				fmt.Fprintf(os.Stderr, "(没能打开浏览器，请手动访问 %s)\n", page)
			}
		}
		var err error
		key, err = auth.PromptSecret("粘贴 API 密钥（不回显）: ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "读 API 密钥失败：", err)
			return 2
		}
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "密钥是空的，已中止")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读配置失败：", err)
		return 1
	}
	if account != "" && account != "default" {
		err = cfg.SetAccountKey(providerID, account, key)
	} else {
		err = cfg.SetAPIKey(providerID, key)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "保存 API 密钥失败：", err)
		return 1
	}
	// A key alone does not say whose subscription it is. With several accounts
	// on one provider that is the difference between four rows and four
	// readable rows, so an email given here is recorded on the account.
	if email != "" {
		if err := cfg.SetAccountEmail(providerID, account, email); err != nil {
			fmt.Fprintln(os.Stderr, "没能保存邮箱：", err)
			return 1
		}
	}

	fmt.Fprintf(os.Stderr, "已保存 %s 的 API 密钥（%s）\n", providerID, maskKey(key))
	if account != "" && account != "default" {
		fmt.Fprintln(os.Stderr, suffixAccount("看用量：leeway usage "+providerID, account))
	} else {
		fmt.Fprintln(os.Stderr, "看用量：leeway usage "+providerID)
	}
	return 0
}

// authStatus prints a per-account credential ledger for the providers that
// hold multiple independent sign-ins, so the user can see at a glance which
// account is alive and when a re-login is due — without touching any of the
// official CLIs.
type authRow struct{ provider, account, cred, status, expires string }

func collectAuthRows(cfg *config.Config) []authRow {
	var rows []authRow
	for _, id := range config.ProviderIDs {
		if !cfg.IsEnabled(id) {
			continue
		}
		for _, acc := range authStatusAccounts(cfg, id) {
			rows = append(rows, authStatusRow(cfg, id, acc.Name))
		}
	}
	return rows
}

func authStatusRow(cfg *config.Config, id, account string) authRow {
	now := time.Now()
	expiry := func(t time.Time) string {
		t = t.Local()
		switch d := t.Sub(now); {
		case d < 0:
			return "expired " + t.Format("2006-01-02 15:04")
		case d < 24*time.Hour:
			return "expires today " + t.Format("15:04")
		default:
			return fmt.Sprintf("%dd · %s", int(d.Hours()/24), t.Format("2006-01-02"))
		}
	}
	switch id {
	case "cursor":
		label := accountStatusName(account, "")
		if a, ok := cfg.Account(id, account); ok {
			label = accountStatusName(a.Name, a.Email)
			account = a.Name
		}
		ca, ok, err := auth.LoadCursorAuth(account)
		if err != nil {
			return authRow{id, label, "cursor auth.json (private)", "error", err.Error()}
		}
		if ok && ca.AccessToken != "" {
			exp := "unknown"
			if t, ok := auth.JWTExpiry(ca.AccessToken); ok {
				exp = expiry(t)
			}
			return authRow{id, label, "cursor auth.json (private)", "ok", exp}
		}
		if ok {
			return authRow{id, label, "cursor auth.json (private)", "not logged in", "leeway login cursor" + suffixFlag(account)}
		}
		if (account == "" || account == "default") && keychainCursorToken() != "" {
			return authRow{id, label, "keychain", "ok", "official CLI session"}
		}
		return authRow{id, label, "—", "not logged in", "leeway login cursor" + suffixFlag(account)}
	case "grok":
		authFile := ""
		label := accountStatusName(account, "")
		if a, ok := cfg.Account(id, account); ok {
			authFile = a.AuthFile
			label = accountStatusName(a.Name, a.Email)
			account = a.Name
		}
		if authFile == "" {
			authFile = "~/.grok/auth.json" // official CLI shared state
		}
		p := authFile
		if path, err := provider.LocalSessionPath(authFile); err == nil {
			p = path
		} else if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		if exp, err := grokAuthExpiry(p); err == nil {
			src := "authFile (private)"
			if strings.HasPrefix(authFile, "~/.grok") {
				src = "~/.grok (shared)"
			}
			return authRow{id, label, src, "ok", expiry(exp)}
		}
		return authRow{id, label, authFile, "not logged in", "leeway login grok" + suffixFlag(account)}
	default:
		// The email is what tells several subscriptions of one provider apart.
		label := account
		if a, ok := cfg.Account(id, account); ok {
			label = accountStatusName(a.Name, a.Email)
		}
		if a, ok := cfg.Account(id, account); ok && a.AuthFile != "" {
			p := a.AuthFile
			if path, err := provider.LocalSessionPath(a.AuthFile); err == nil {
				p = path
			}
			if fileExists(p) {
				return authRow{id, label, "authFile (private)", "ok", "—"}
			}
			return authRow{id, label, a.AuthFile, "not logged in", "leeway login " + id + suffixFlag(account)}
		}
		key := ""
		if a, ok := cfg.Account(id, account); ok && a.APIKey != "" {
			key = a.APIKey
		} else if cfg.APIKey(id) != "" {
			key = cfg.APIKey(id)
		}
		if key != "" {
			return authRow{id, label, "apiKey (config)", "ok", "—"}
		}
		if p, ok := provider.Get(id); ok {
			spec := p.AuthSpec()
			if spec.UsesCookies {
				if h, err := auth.ResolveCookieHeader(id, cfg.Provider(id), account); err == nil && h != "" {
					return authRow{id, label, "cookies", "ok", "—"}
				}
			}
			if spec.UsesCodexCLI && fetch.HasCodexAuth() {
				return authRow{id, label, "codex auth.json", "ok", "—"}
			}
			for _, f := range spec.LocalAuthFiles {
				path, err := provider.LocalSessionPath(f)
				if err == nil && fileExists(path) {
					return authRow{id, label, "local session", "ok", "—"}
				}
			}
		}
		return authRow{id, label, "—", "not configured", "leeway login " + id + suffixFlag(account)}
	}
}

func authStatusAccounts(cfg *config.Config, id string) []config.AccountConfig {
	if id == "cursor" {
		return cursorAccounts(cfg)
	}
	accs := config.DropDefaultAccounts(cfg.Accounts(id))
	if len(accs) > 0 {
		return accs
	}
	if authStatusHasDefault(cfg, id) {
		return []config.AccountConfig{{Name: "default"}}
	}
	return nil
}

func authStatusHasDefault(cfg *config.Config, id string) bool {
	if cfg.APIKey(id) != "" {
		return true
	}
	if a, ok := cfg.Account(id, "default"); ok && a.AuthFile != "" {
		p := a.AuthFile
		if path, err := provider.LocalSessionPath(a.AuthFile); err == nil {
			p = path
		}
		if fileExists(p) {
			return true
		}
	}
	p, ok := provider.Get(id)
	if !ok {
		return false
	}
	spec := p.AuthSpec()
	if spec.UsesCookies {
		if h, err := auth.ResolveCookieHeader(id, cfg.Provider(id), "default"); err == nil && h != "" {
			return true
		}
	}
	if spec.UsesCodexCLI && fetch.HasCodexAuth() {
		return true
	}
	for _, f := range spec.LocalAuthFiles {
		path, err := provider.LocalSessionPath(f)
		if err == nil && fileExists(path) {
			return true
		}
	}
	return false
}

// suffixFlag appends a named account to a suggested command.
func suffixFlag(account string) string {
	if account != "" && account != "default" {
		return " " + account
	}
	return ""
}

// displayAccount normalizes an empty account to "default" for messages.
func displayAccount(account string) string {
	if account == "" {
		return "default"
	}
	return account
}

// accountStatusName is "tag  email" when both exist and differ, else whichever is set.
func accountStatusName(name, email string) string {
	name = displayAccount(name)
	email = strings.TrimSpace(email)
	if email == "" || strings.EqualFold(email, name) {
		return name
	}
	if name == "default" {
		return email
	}
	return name + "  " + email
}

func grokAuthEmail(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var m map[string]struct {
		Key   string `json:"key"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	var first string
	for scope, e := range m {
		if e.Key == "" || e.Email == "" {
			continue
		}
		if strings.HasPrefix(scope, "https://auth.x.ai::") {
			return e.Email
		}
		if first == "" {
			first = e.Email
		}
	}
	return first
}

// grokAuthExpiry reads expires_at from the usable grok auth.json entry.
// The OIDC scope (https://auth.x.ai::) wins, matching fetch's loadGrokSession.
func grokAuthExpiry(path string) (time.Time, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	var m map[string]struct {
		Key       string `json:"key"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return time.Time{}, err
	}
	var first time.Time
	var haveFirst bool
	for scope, e := range m {
		if e.Key == "" || e.ExpiresAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, e.ExpiresAt)
		if err != nil {
			continue
		}
		if strings.HasPrefix(scope, "https://auth.x.ai::") {
			return t, nil
		}
		if !haveFirst {
			first = t
			haveFirst = true
		}
	}
	if haveFirst {
		return first, nil
	}
	return time.Time{}, errors.New("no usable entry")
}

func keychainCursorToken() string {
	tok, err := auth.CursorKeychainToken(context.Background())
	if err != nil || tok == "" {
		return ""
	}
	return tok
}

func logoutCursor(account string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读配置失败：", err)
		return 1
	}
	name := account
	if a, ok := cfg.Account("cursor", account); ok {
		name = a.Name
	}
	home, _ := auth.CursorHome(name)
	if err := auth.RemoveCursorHome(name); err != nil {
		fmt.Fprintln(os.Stderr, "删 cursor 的登录失败：", err)
		return 1
	}
	_ = cfg.RemoveAccount("cursor", name)
	fmt.Fprintf(os.Stderr, "已删除 cursor 账号的登录（%s）；keychain 里的官方登录不受影响\n", home)
	return 0
}

func logoutGrok(account string) int {
	name := account
	if name == "" || name == "default" {
		name = "default"
	}
	home, err := grokHome(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "找 grok 目录失败：", err)
		return 1
	}
	root, err := auth.SessionDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "找登录目录失败：", err)
		return 1
	}
	if !underDir(root, home) {
		fmt.Fprintf(os.Stderr, "不删除 %s：它不是 leeway 的登录目录\n", home)
		return 1
	}
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintln(os.Stderr, "删 grok 的登录失败：", err)
		return 1
	}
	if cfg, err := config.Load(); err == nil {
		_ = cfg.RemoveAccount("grok", name)
	}
	fmt.Fprintf(os.Stderr, "已删除 grok 账号 %q 的登录（%s）\n", name, home)
	return 0
}

// suffixAccount appends a named account to a suggested command.
func suffixAccount(base, account string) string {
	if account != "" && account != "default" {
		return base + " " + account
	}
	return base
}
