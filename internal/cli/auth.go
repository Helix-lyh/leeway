package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/fetch"
	"leeway/internal/provider"
)

// keyPage is where a provider hands out API keys; `auth login` opens it
// before prompting for the pasted value.
var keyPage = map[string]string{
	"deepseek":    "https://platform.deepseek.com/api_keys",
	"opencode-go": "https://opencode.ai/zen/go",
	"commandcode": "https://commandcode.ai/studio",
}

func runAuth(args []string) int {
	if len(args) < 1 || isHelpArg(args[0]) {
		printHelp(os.Stderr, authHelp)
		if len(args) > 0 && isHelpArg(args[0]) {
			return 0
		}
		return 2
	}
	switch args[0] {
	case "login":
		return authLogin(args[1:])
	case "logout":
		return authLogout(args[1:])
	case "status":
		return authStatus(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "auth: unknown subcommand %q\n\n", args[0])
		printHelp(os.Stderr, authHelp)
		return 2
	}
}

func authLogin(args []string) int {
	var (
		providerID string
		account    string
		noBrowser  bool
		timeoutMin int
	)
	fs := newFlagSet("auth login", nil)
	fs.Help(authLoginHelp)
	fs.StringVar(&providerID, "provider", "", "cursor | grok | opencode-go | commandcode | deepseek")
	fs.StringVar(&account, "account", "", "tag / 订阅名；cursor 省略则用邮箱当账号名")
	fs.BoolVar(&noBrowser, "no-browser", false, "只打印登录 URL，不打开浏览器")
	fs.IntVar(&timeoutMin, "timeout", 15, "等待浏览器授权的分钟数")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if timeoutMin > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMin)*time.Minute)
		defer cancel()
	}

	switch providerID {
	case "cursor":
		return loginCursor(ctx, account, noBrowser)
	case "grok":
		return loginGrok(ctx, account)
	default:
		return loginAPIKey(providerID, account, noBrowser)
	}
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
		fmt.Fprintln(os.Stderr, "没找到 cursor-agent CLI，先安装（https://cursor.com/cli），再重试")
		return 2
	}

	home, err := auth.CursorHome(account)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cursor home:", err)
		return 1
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create home:", err)
		return 1
	}

	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(),
		"AGENT_CLI_CREDENTIAL_STORE=file", // write ~/.cursor/auth.json, not the keychain
		"HOME="+home,                      // …into this account's private HOME
	)
	if noBrowser {
		cmd.Env = append(cmd.Env, "NO_OPEN_BROWSER=1")
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cursor-agent login:", err)
		return 1
	}

	staging := account
	if staging == "" {
		staging = "default"
	}
	ca, ok, err := auth.LoadCursorAuth(staging)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read auth:", err)
		return 1
	}
	if !ok || ca.AccessToken == "" {
		p, _ := auth.CursorAuthFile(staging)
		fmt.Fprintf(os.Stderr, "登录似乎没完成：%s 里没有令牌\n", p)
		return 1
	}

	email, _ := auth.LookupCursorEmail(ctx, ca.AccessToken)
	name := strings.TrimSpace(account)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
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
			fmt.Fprintln(os.Stderr, "move cursor home:", err)
			return 1
		}
	}
	if err := cfg.SetAccountEmail("cursor", name, email); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 1
	}

	var expiresIn string
	if exp, ok := auth.JWTExpiry(ca.AccessToken); ok {
		expiresIn = "，有效期到 " + exp.Local().Format("2006-01-02 15:04")
	}
	p, _ := auth.CursorAuthFile(name)
	label := accountStatusName(name, email)
	fmt.Fprintf(os.Stderr, "\n已登录 cursor 账号 %s%s，令牌由官方 CLI 写入 %s（0600）\n", label, expiresIn, p)
	fmt.Fprintln(os.Stderr, "该账号与官方 Cursor CLI / keychain 完全独立，切号只需换 --account（tag 或邮箱），不用再 logout/login")
	fmt.Fprintln(os.Stderr, suffixAccount("查额度：leeway usage --provider cursor", name))
	return 0
}

// loginGrok defers to the official Grok CLI's device-code flow rather than
// reimplementing OAuth. Every account — named or the default — gets its own
// private GROK_HOME under leeway's session dir and is bound via
// accounts[].authFile, so grok CLI/browser logins on other accounts never
// touch the credential leeway reads (and vice versa).
func loginGrok(ctx context.Context, account string) int {
	bin, err := exec.LookPath("grok")
	if err != nil {
		fmt.Fprintln(os.Stderr, "没找到 grok CLI，先安装：curl -fsSL https://x.ai/cli/install.sh | bash")
		return 2
	}

	name := account
	if name == "" || name == "default" {
		name = "default"
	}
	home, err := grokHome(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "grok home:", err)
		return 1
	}

	cmd := exec.CommandContext(ctx, bin, "login", "--device-auth")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "GROK_HOME="+home)
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "grok login:", err)
		return 1
	}

	authFile := filepath.Join(home, "auth.json")
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	if err := cfg.SetAccountAuthFile("grok", name, authFile); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 1
	}
	if email := grokAuthEmail(authFile); email != "" {
		_ = cfg.SetAccountEmail("grok", name, email)
		name = accountStatusName(name, email)
	}
	fmt.Fprintf(os.Stderr, "\n已登录 grok 账号 %s → %s\n", name, authFile)
	fmt.Fprintln(os.Stderr, "该账号与官方 grok CLI（~/.grok）完全独立，切号只需换 --account，不用再 logout/login")
	return 0
}

// grokHome is the isolated GROK_HOME for a named account.
func grokHome(account string) (string, error) {
	dir, err := auth.SessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "grok-"+account), nil
}

// loginAPIKey covers providers whose only credential is an API key: open the
// key page, then read the pasted value without echoing it, so the key never
// lands in shell history or the scrollback.
func loginAPIKey(providerID, account string, noBrowser bool) int {
	page := keyPage[providerID]
	if page != "" && !noBrowser {
		fmt.Fprintf(os.Stderr, "打开 %s 创建并复制 API key…\n", page)
		if err := auth.OpenBrowser(page); err != nil {
			fmt.Fprintf(os.Stderr, "(打不开浏览器，请手动访问 %s)\n", page)
		}
	}

	key, err := auth.PromptSecret("粘贴 API key（不回显）: ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "read key:", err)
		return 1
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	if account != "" && account != "default" {
		err = cfg.SetAccountKey(providerID, account, key)
	} else {
		err = cfg.SetAPIKey(providerID, key)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "save key:", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "已保存 %s 的 key（%s）\n", providerID, maskKey(key))
	if account != "" && account != "default" {
		fmt.Fprintln(os.Stderr, suffixAccount("查额度：leeway usage --provider "+providerID, account))
	} else {
		fmt.Fprintln(os.Stderr, "查额度：leeway usage --provider "+providerID)
	}
	return 0
}

// authStatus prints a per-account credential ledger for the providers that
// hold multiple independent sign-ins, so the user can see at a glance which
// account is alive and when a re-login is due — without touching any of the
// official CLIs.
type authRow struct{ provider, account, cred, status, expires string }

func authStatus(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}

	var rows []authRow
	for _, id := range config.ProviderIDs {
		if !cfg.IsEnabled(id) {
			continue
		}
		for _, acc := range authStatusAccounts(cfg, id) {
			rows = append(rows, authStatusRow(cfg, id, acc.Name))
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tACCOUNT\tCREDENTIAL\tSTATUS\tEXPIRES")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.provider, r.account, r.cred, r.status, r.expires)
	}
	w.Flush()
	return 0
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
			return authRow{id, label, "cursor auth.json (private)", "not logged in", "leeway auth login --provider cursor" + suffixFlag(account)}
		}
		if (account == "" || account == "default") && keychainCursorToken() != "" {
			return authRow{id, label, "keychain", "ok", "official CLI session"}
		}
		return authRow{id, label, "—", "not logged in", "leeway auth login --provider cursor" + suffixFlag(account)}
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
		return authRow{id, label, authFile, "not logged in", "leeway auth login --provider grok" + suffixFlag(account)}
	default:
		key := ""
		if a, ok := cfg.Account(id, account); ok && a.APIKey != "" {
			key = a.APIKey
		} else if cfg.APIKey(id) != "" {
			key = cfg.APIKey(id)
		}
		if key != "" {
			return authRow{id, account, "apiKey (config)", "ok", "—"}
		}
		if p, ok := provider.Get(id); ok {
			spec := p.AuthSpec()
			if spec.UsesCookies {
				if h, err := auth.ResolveCookieHeader(id, cfg.Provider(id), account); err == nil && h != "" {
					return authRow{id, account, "cookies", "ok", "—"}
				}
			}
			if spec.UsesCodexCLI && fetch.HasCodexAuth() {
				return authRow{id, account, "codex auth.json", "ok", "—"}
			}
			for _, f := range spec.LocalAuthFiles {
				path, err := provider.LocalSessionPath(f)
				if err == nil && fileExists(path) {
					return authRow{id, account, "local session", "ok", "—"}
				}
			}
		}
		return authRow{id, account, "—", "not configured", "leeway auth login --provider " + id + suffixFlag(account)}
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

// suffixFlag renders "--account NAME " for a named account (for hints).
func suffixFlag(account string) string {
	if account != "" && account != "default" {
		return " --account " + account
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

func authLogout(args []string) int {
	var providerID, account string
	fs := newFlagSet("auth logout", nil)
	fs.Help(authLogoutHelp)
	fs.StringVar(&providerID, "provider", "", "cursor | grok | …")
	fs.StringVar(&account, "account", "", "tag / 邮箱 / 订阅名")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}

	switch providerID {
	case "cursor":
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config:", err)
			return 1
		}
		name := account
		if a, ok := cfg.Account("cursor", account); ok {
			name = a.Name
		}
		home, _ := auth.CursorHome(name)
		if err := auth.RemoveCursorHome(name); err != nil {
			fmt.Fprintln(os.Stderr, "clear cursor auth:", err)
			return 1
		}
		_ = cfg.RemoveAccount("cursor", name)
		fmt.Fprintf(os.Stderr, "已清除 cursor 账号的私有登录（%s）；keychain 里的官方会话不受影响\n", home)
	case "grok":
		name := account
		if name == "" || name == "default" {
			name = "default"
		}
		home, err := grokHome(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, "grok home:", err)
			return 1
		}
		if err := os.RemoveAll(home); err != nil {
			fmt.Fprintln(os.Stderr, "clear grok auth:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "已清除 grok 账号 %q 的私有登录（%s）\n", name, home)
	default:
		fmt.Fprintf(os.Stderr, "%s 的 key 存在 config 里（apiKey），没有 leeway 私有会话可清；想移除请用 leeway config 覆盖或手动编辑 %s\n", providerID, configPath())
	}
	return 0
}

// suffixAccount appends the --account filter to a suggested command.
func suffixAccount(base, account string) string {
	if account != "" && account != "default" {
		return base + " --account " + account
	}
	return base
}
