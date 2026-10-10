package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/provider"
	"leeway/internal/provider/grok"
	"leeway/internal/provider/kimi"
	"leeway/internal/switcher"
)

// --- small shared helpers (the CLI's own live in internal/cli) -------------

func displayAccount(name string) string {
	if strings.TrimSpace(name) == "" {
		return "default"
	}
	return name
}

func safeSessionName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
}

func underDir(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if root == "" || root == string(os.PathSeparator) {
		return false
	}
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(os.PathSeparator))
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return aa == bb
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func isolatedHome(providerID, account string) (string, error) {
	dir, err := auth.SessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, providerID+"-"+safeSessionName(displayAccount(account))), nil
}

func grokHome(account string) (string, error) {
	dir, err := auth.SessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "grok-"+safeSessionName(displayAccount(account))), nil
}

// grokAuthEmail reads the account email out of an official grok auth.json.
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
	for _, e := range m {
		if e.Key != "" && e.Email != "" {
			return e.Email
		}
	}
	return ""
}

// overrideEnv copies the process environment, dropping any existing values for
// the given keys so the child's getenv sees ours (first-wins on Unix).
func overrideEnv(kv ...string) []string {
	drop := make(map[string]struct{}, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		drop[kv[i]] = struct{}{}
	}
	env := os.Environ()
	out := make([]string, 0, len(env)+len(kv)/2)
	for _, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok {
			if _, skip := drop[k]; skip {
				continue
			}
		}
		out = append(out, e)
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, kv[i]+"="+kv[i+1])
	}
	return out
}

// --- credentials -----------------------------------------------------------

// loginAPIKey stores a pasted key. A named account gets an account-level key;
// the default account uses the provider-level key, as `leeway login` does.
func loginAPIKey(providerID, account, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("API 密钥不能为空")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if account != "" && account != "default" {
		return cfg.SetAccountKey(providerID, account, key)
	}
	return cfg.SetAPIKey(providerID, key)
}

// importCookies imports a browser session for a provider and caches the
// header. A fresh cached import is reused unless force is set.
func importCookies(providerID, account, browser, profile string, force bool) (string, error) {
	if browser == "" {
		browser = "chrome"
	}
	if cc, ok, err := auth.LoadCookieCache(providerID, profile); err != nil {
		// An unreadable cache would fail every probe; drop it and re-import.
		if rerr := auth.RemoveCookieCacheFile(); rerr != nil {
			return "", rerr
		}
	} else if ok && !force && time.Since(cc.ImportedAt) < auth.ImportTTL {
		return fmt.Sprintf("本地登录状态还新（%s 导入），跳过", cc.ImportedAt.Local().Format("01-02 15:04")), nil
	}

	header, err := auth.Import(providerID, browser, profile)
	if err != nil {
		return "", err
	}
	if err := auth.SaveCookieCache(providerID, profile, header, browser); err != nil {
		return "", err
	}
	if account != "" && account != "default" && profile != "" {
		if err := bindAccount(providerID, account, func(a *config.AccountConfig) { a.Profile = profile }); err != nil {
			return "", err
		}
	}
	n := 0
	if strings.TrimSpace(header) != "" {
		n = strings.Count(header, ";") + 1
	}
	return fmt.Sprintf("已从 %s 读取 %d 条登录状态", browser, n), nil
}

// saveManualCookie stores a pasted cookie header for a provider or account,
// accepting curl fragments as `leeway login --manual` does.
func saveManualCookie(providerID, account, raw string) error {
	header := auth.ExtractCookieHeader(raw)
	if header == "" {
		header = strings.TrimSpace(raw)
	}
	if !strings.Contains(header, "=") {
		return errors.New("没找到 name=value 格式的内容")
	}
	if account != "" && account != "default" {
		return bindAccount(providerID, account, func(a *config.AccountConfig) { a.ManualCookieHeader = header })
	}
	return bindProvider(providerID, func(pc *config.ProviderConfig) { pc.ManualCookieHeader = header })
}

// bindProvider edits one provider's config entry and saves.
func bindProvider(id string, edit func(pc *config.ProviderConfig)) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pc := cfg.Provider(id)
	edit(&pc)
	cfg.Providers[id] = pc
	return cfg.Save()
}

// bindAccount edits one account's config entry (creating it) and saves.
func bindAccount(id, account string, edit func(a *config.AccountConfig)) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pc := cfg.Provider(id)
	for i := range pc.Accounts {
		if pc.Accounts[i].Matches(account) {
			edit(&pc.Accounts[i])
			cfg.Providers[id] = pc
			return cfg.Save()
		}
	}
	a := config.AccountConfig{Name: account}
	edit(&a)
	pc.Accounts = append(pc.Accounts, a)
	cfg.Providers[id] = pc
	return cfg.Save()
}

// setProviderEnabled turns a provider on or off in the config.
func setProviderEnabled(id string, on bool) error {
	return bindProvider(id, func(pc *config.ProviderConfig) { pc.Enabled = &on })
}

// addAccount creates an empty named account (a tag to log into).
func addAccount(id, account string) error {
	account = strings.TrimSpace(account)
	if account == "" {
		return errors.New("账号名不能为空")
	}
	return bindAccount(id, account, func(a *config.AccountConfig) { a.Name = account })
}

// sessionChecks validates a pasted session file per provider.
var sessionChecks = map[string]func([]byte) bool{
	"kimi": func(raw []byte) bool {
		var m struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		return json.Unmarshal(raw, &m) == nil && m.AccessToken != "" && m.RefreshToken != ""
	},
	"cursor": func(raw []byte) bool {
		var m struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		}
		return json.Unmarshal(raw, &m) == nil && m.AccessToken != "" && m.RefreshToken != ""
	},
	"codex": func(raw []byte) bool {
		var m struct {
			Tokens struct {
				AccessToken string `json:"access_token"`
			} `json:"tokens"`
			OpenAIKey string `json:"OPENAI_API_KEY"`
		}
		if json.Unmarshal(raw, &m) != nil {
			return false
		}
		return m.Tokens.AccessToken != "" || m.OpenAIKey != ""
	},
}

// importAuthFile snapshots a session file the user already has (an official
// CLI's own file) into the account's private session directory.
func importAuthFile(providerID, account, src string) (string, error) {
	path := strings.TrimSpace(src)
	if path == "" {
		return "", errors.New("没有选择文件")
	}
	if expanded, err := provider.HomePath(path); err == nil {
		path = expanded
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var dst string
	switch providerID {
	case "grok":
		if !grok.HasOfficialShape(path) {
			return "", errors.New("这不是完整的 grok 登录文件（缺 profile/email 字段），同步会被拒绝")
		}
		home, err := grokHome(displayAccount(account))
		if err != nil {
			return "", err
		}
		dst = filepath.Join(home, "auth.json")
	case "kimi":
		home, err := isolatedHome("kimi", account)
		if err != nil {
			return "", err
		}
		dst = filepath.Join(home, "credentials", "kimi-code.json")
	case "codex":
		home, err := isolatedHome("codex", account)
		if err != nil {
			return "", err
		}
		dst = filepath.Join(home, "auth.json")
	case "cursor":
		p, err := auth.CursorAuthFile(displayAccount(account))
		if err != nil {
			return "", err
		}
		dst = p
	default:
		return "", fmt.Errorf("%s 的登录文件不是导入的", providerID)
	}
	if check, ok := sessionChecks[providerID]; ok && !check(raw) {
		return "", fmt.Errorf("这份文件不能用作 %s 的登录文件", providerID)
	}
	if err := writeFileAtomic(dst, raw); err != nil {
		return "", err
	}

	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if err := cfg.SetAccountAuthFile(providerID, displayAccount(account), dst); err != nil {
		return "", err
	}
	if providerID == "grok" {
		if email := grokAuthEmail(dst); email != "" {
			_ = cfg.SetAccountEmail("grok", displayAccount(account), email)
		}
	}
	return fmt.Sprintf("已备份 %s 账号 %s", providerID, displayAccount(account)), nil
}

func writeFileAtomic(dst string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp := dst + ".incoming"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// --- official CLI login ----------------------------------------------------

// officialLogin delegates the sign-in to the provider's own CLI, run with a
// private HOME under leeway's session directory, and streams what it prints.
// Nothing about the providers' private protocols is reimplemented here.
func officialLogin(ctx context.Context, providerID, account string, log func(string)) error {
	name := displayAccount(account)
	switch providerID {
	case "cursor":
		return loginCursorCLI(ctx, account, log)
	case "grok":
		return loginGrokCLI(ctx, name, log)
	case "codex":
		return loginCodexCLI(ctx, name, log)
	case "kimi":
		return loginKimiCLI(ctx, name, log)
	default:
		return fmt.Errorf("%s 用 API 密钥登录", providerID)
	}
}

func loginCursorCLI(ctx context.Context, account string, log func(string)) error {
	bin, err := exec.LookPath("cursor-agent")
	if err != nil {
		return errors.New("没找到 cursor-agent，先安装：https://cursor.com/cli")
	}
	staging := displayAccount(account)
	home, err := auth.CursorHome(staging)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Env = overrideEnv(
		"AGENT_CLI_CREDENTIAL_STORE", "file",
		"HOME", home,
	)
	cmd.Stdout, cmd.Stderr = logWriter(log), logWriter(log)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("委托 cursor-agent 登录失败：%w", err)
	}
	ca, ok, err := auth.LoadCursorAuth(staging)
	if err != nil {
		return err
	}
	if !ok || ca.AccessToken == "" {
		return errors.New("登录似乎没完成：官方工具没有写出登录信息")
	}
	email, _ := auth.LookupCursorEmail(ctx, ca.AccessToken)
	name := strings.TrimSpace(account)
	cfg, err := config.Load()
	if err != nil {
		return err
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
			return err
		}
	}
	if err := cfg.SetAccountEmail("cursor", name, email); err != nil {
		return err
	}
	log("\n已登录 cursor 账号 " + name + "\n")
	return nil
}

func loginGrokCLI(ctx context.Context, name string, log func(string)) error {
	home, err := grokHome(name)
	if err != nil {
		return err
	}
	authFile := filepath.Join(home, "auth.json")
	if grok.HasOfficialShape(authFile) {
		return fmt.Errorf("grok 账号 %s 已有完整备份；要替换请用「导入登录文件」", name)
	}
	bin, err := exec.LookPath("grok")
	if err != nil {
		return errors.New("没找到 grok，先安装：curl -fsSL https://x.ai/cli/install.sh | bash")
	}
	cmd := exec.CommandContext(ctx, bin, "login", "--device-auth")
	cmd.Env = overrideEnv("GROK_HOME", home)
	cmd.Stdout, cmd.Stderr = logWriter(log), logWriter(log)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("委托 grok 登录失败：%w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.SetAccountAuthFile("grok", name, authFile); err != nil {
		return err
	}
	if email := grokAuthEmail(authFile); email != "" {
		_ = cfg.SetAccountEmail("grok", name, email)
	}
	log("\n已登录 grok 账号 " + name + "\n")
	if !grok.HasOfficialShape(authFile) {
		log("\n这个登录缺官方资料字段，同步给 grok 会被拒绝；请用「导入登录文件」备份 ~/.grok/auth.json\n")
	}
	return nil
}

func loginCodexCLI(ctx context.Context, name string, log func(string)) error {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return errors.New("没找到 codex，先安装官方 Codex CLI")
	}
	home, err := isolatedHome("codex", name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Env = overrideEnv("CODEX_HOME", home)
	cmd.Stdout, cmd.Stderr = logWriter(log), logWriter(log)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("委托 codex 登录失败：%w", err)
	}
	authFile := filepath.Join(home, "auth.json")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.SetAccountAuthFile("codex", name, authFile); err != nil {
		return err
	}
	log("\n已登录 codex 账号 " + name + "\n")
	return nil
}

func loginKimiCLI(ctx context.Context, name string, log func(string)) error {
	bin, err := exec.LookPath("kimi")
	if err != nil {
		return errors.New("没找到 kimi；可以用「粘贴 API 密钥」或「读取浏览器登录状态」")
	}
	home, err := isolatedHome("kimi", name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Env = overrideEnv("KIMI_CODE_HOME", home)
	cmd.Stdout, cmd.Stderr = logWriter(log), logWriter(log)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("委托 kimi 登录失败：%w", err)
	}
	authFile := filepath.Join(home, "credentials", "kimi-code.json")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.SetAccountAuthFile("kimi", name, authFile); err != nil {
		return err
	}
	log("\n已登录 kimi 账号 " + name + "\n")
	return nil
}

// logWriter turns a child's output into lines handed to log.
type logWriter func(string)

func (w logWriter) Write(p []byte) (int, error) {
	w(string(p))
	return len(p), nil
}

// --- use (make an account the machine's own) -------------------------------

// useAccount makes a leeway account the one the machine's own client uses,
// the same operation as `leeway use`.
func useAccount(providerID, account string) (switcher.Result, error) {
	cfg, err := config.Load()
	if err != nil {
		return switcher.Result{}, err
	}
	switch providerID {
	case "cursor":
		return useCursorAccount(cfg, account)
	case "grok":
		return useGrokAccount(cfg, account)
	case "kimi":
		if acc, ok := cfg.Account("kimi", account); ok {
			return useKimiAccount(acc, account)
		}
		return switcher.Result{}, errors.New("这个 kimi 账号只有密钥或浏览器登录状态，没有登录文件可以写给 kimi-code.json")
	case "codex":
		return switcher.Result{}, errors.New("codex 不写 ~/.codex（避免刷新把官方登录作废）")
	}
	return switcher.Result{}, fmt.Errorf("本机没有 %s 的官方工具可同步", providerID)
}

// accountSwitchReady reports whether `让本机用它` can work for this account: the
// same precondition the action enforces, so the entry never appears only to
// fail.
func accountSwitchReady(a *App, providerID, account string) bool {
	if a.cfg == nil {
		return false
	}
	switch providerID {
	case "cursor":
		_, _, err := cursorSessionForSwitch(a.cfg, account)
		return err == nil
	case "grok":
		_, err := grokSwitchSource(a.cfg, account)
		return err == nil
	case "kimi":
		acc, ok := a.cfg.Account("kimi", account)
		if !ok {
			return false
		}
		_, err := kimiSwitchSource(acc, account)
		return err == nil
	}
	return false
}

// cursorSessionForSwitch is the leeway account's own session, which is what
// switching the machine's client needs: an access token without its refresh
// token cannot be written back into the IDE.
func cursorSessionForSwitch(cfg *config.Config, account string) (string, auth.CursorAuth, error) {
	name, _ := resolveCursorAccount(cfg, account)
	ca, ok, err := auth.LoadCursorAuth(name)
	if err != nil {
		return name, auth.CursorAuth{}, err
	}
	if !ok || ca.AccessToken == "" || ca.RefreshToken == "" {
		return name, auth.CursorAuth{}, fmt.Errorf("cursor 账号 %q 还没登录", name)
	}
	if exp, ok := auth.JWTExpiry(ca.AccessToken); ok && time.Now().After(exp) {
		return name, auth.CursorAuth{}, fmt.Errorf("cursor 账号 %q 的登录已过期，重新登录", name)
	}
	return name, ca, nil
}

func useCursorAccount(cfg *config.Config, account string) (switcher.Result, error) {
	name, email := resolveCursorAccount(cfg, account)
	name, ca, err := cursorSessionForSwitch(cfg, account)
	if err != nil {
		return switcher.Result{}, err
	}
	if block, reason := switcher.CheckCursorIDE(); block {
		return switcher.Result{}, errors.New(reason)
	}
	targets, err := switcher.OfficialCursorTargets()
	if err != nil {
		return switcher.Result{}, err
	}
	saved, unsaved, err := switcher.SnapshotCursorIfNeeded(targets, ca.AccessToken)
	if err != nil {
		return switcher.Result{}, err
	}
	if err := switcher.ApplyCursor(targets, ca, email); err != nil {
		return switcher.Result{}, err
	}
	return switcher.Result{
		Provider: "cursor",
		Account:  name,
		Email:    email,
		Targets:  switcher.DisplayTargets(targets),
		SavedAs:  saved,
		Unsaved:  unsaved,
	}, nil
}

func resolveCursorAccount(cfg *config.Config, account string) (name, email string) {
	name = account
	if name == "" {
		name = "default"
	}
	if a, ok := cfg.Account("cursor", account); ok {
		name = a.Name
		if a.Email != "" {
			return name, a.Email
		}
	}
	return name, ""
}

// grokSwitchSource is the session file `让本机用它` would write back: the
// account's own file when it has one, the isolated HOME otherwise.
func grokSwitchSource(cfg *config.Config, account string) (string, error) {
	name := displayAccount(account)
	src := ""
	if a, ok := cfg.Account("grok", account); ok {
		name = a.Name
		if a.AuthFile != "" {
			if p, err := provider.LocalSessionPath(a.AuthFile); err == nil {
				src = p
			} else {
				src = a.AuthFile
			}
		}
	}
	if src == "" {
		home, err := grokHome(name)
		if err != nil {
			return "", err
		}
		src = filepath.Join(home, "auth.json")
	}
	if !fileExists(src) {
		return "", fmt.Errorf("grok 账号 %q 还没登录", name)
	}
	if !grok.HasOfficialShape(src) {
		return "", fmt.Errorf("grok 账号 %q 的登录不完整，同步会逼 grok 重新网页登录；请先导入 ~/.grok/auth.json", name)
	}
	return src, nil
}

func useGrokAccount(cfg *config.Config, account string) (switcher.Result, error) {
	src, err := grokSwitchSource(cfg, account)
	if err != nil {
		return switcher.Result{}, err
	}
	name := displayAccount(account)
	if a, ok := cfg.Account("grok", account); ok {
		name = a.Name
	}
	dst, err := switcher.OfficialGrokAuthFile()
	if err != nil {
		return switcher.Result{}, err
	}
	if err := switcher.ApplyGrok(dst, src); err != nil {
		return switcher.Result{}, err
	}
	return switcher.Result{
		Provider: "grok",
		Account:  name,
		Email:    grokAuthEmail(src),
		Targets:  []string{switcher.DisplayPath(dst)},
	}, nil
}

// kimiSwitchSource is the CLI session `让本机用它` would write back; a kimi
// account with only a key or cookie has none.
func kimiSwitchSource(a config.AccountConfig, account string) (string, error) {
	if a.AuthFile == "" {
		return "", fmt.Errorf("这个 kimi 账号只有密钥或浏览器登录状态，没有登录文件可以写给 kimi-code.json")
	}
	src := a.AuthFile
	if p, err := provider.LocalSessionPath(a.AuthFile); err == nil {
		src = p
	}
	if !fileExists(src) {
		return "", fmt.Errorf("kimi 账号 %q 的登录文件不在 %s", account, src)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if !sessionChecks["kimi"](raw) {
		return "", fmt.Errorf("kimi 账号 %q 的登录不完整", account)
	}
	return src, nil
}

func useKimiAccount(a config.AccountConfig, account string) (switcher.Result, error) {
	src, err := kimiSwitchSource(a, account)
	if err != nil {
		return switcher.Result{}, err
	}
	dst, err := kimi.OfficialCredentialPath()
	if err != nil {
		return switcher.Result{}, err
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return switcher.Result{}, err
	}
	if samePath(src, dst) {
		return switcher.Result{}, errors.New("这个账号就绑在官方登录文件上，先备份一份再切")
	}
	if err := writeFileAtomic(dst, raw); err != nil {
		return switcher.Result{}, err
	}
	return switcher.Result{
		Provider: "kimi",
		Account:  account,
		Targets:  []string{switcher.DisplayPath(dst)},
	}, nil
}

// --- logout ----------------------------------------------------------------

// logoutAccount removes one account's credentials, as `leeway logout` does.
func logoutAccount(providerID, account string) (string, error) {
	_ = auth.ClearCookieCache(providerID, "")

	switch providerID {
	case "cursor":
		name := displayAccount(account)
		if a, ok, err := accountOf(providerID, account); err == nil && ok {
			name = a.Name
		}
		if err := auth.RemoveCursorHome(name); err != nil {
			return "", err
		}
		removeAccountFromConfig(providerID, name)
		if err := clearProviderCredentials(providerID, account); err != nil {
			return "", err
		}
		return fmt.Sprintf("已清除 cursor 账号 %s 的登录信息", name), nil
	case "grok", "codex", "kimi":
		msg, err := logoutIsolated(providerID, account)
		if err != nil {
			return "", err
		}
		if err := clearProviderCredentials(providerID, account); err != nil {
			return "", err
		}
		return msg, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	name := displayAccount(account)
	if err := cfg.RemoveAccount(providerID, name); err != nil {
		return "", err
	}
	if account == "" || account == "default" {
		if cfg.APIKeys != nil {
			delete(cfg.APIKeys, providerID)
			if err := cfg.Save(); err != nil {
				return "", err
			}
		}
	}
	return fmt.Sprintf("已移除 %s 账号 %s", providerID, name), nil
}

// clearProviderCredentials drops the provider-level key and cookie, which is
// where a default account's credentials live: without it `退出登录` would leave
// the key behind and the provider would keep probing with it.
func clearProviderCredentials(providerID, account string) error {
	if account != "" && account != "default" {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	changed := false
	if _, ok := cfg.APIKeys[providerID]; ok {
		delete(cfg.APIKeys, providerID)
		changed = true
	}
	if pc := cfg.Provider(providerID); pc.ManualCookieHeader != "" || pc.CookieSource != "" {
		pc.ManualCookieHeader, pc.CookieSource = "", ""
		cfg.Providers[providerID] = pc
		changed = true
	}
	if !changed {
		return nil
	}
	return cfg.Save()
}

func logoutIsolated(providerID, account string) (string, error) {
	name := displayAccount(account)
	var home string
	var err error
	if providerID == "grok" {
		home, err = grokHome(name)
	} else {
		home, err = isolatedHome(providerID, name)
	}
	if err != nil {
		return "", err
	}
	root, err := auth.SessionDir()
	if err != nil {
		return "", err
	}
	if !underDir(root, home) {
		return "", fmt.Errorf("拒绝删除 %s：不在 leeway 的目录里", home)
	}
	if err := os.RemoveAll(home); err != nil {
		return "", err
	}
	removeAccountFromConfig(providerID, name)
	return fmt.Sprintf("已清除 %s 账号 %s 的私有登录", providerID, name), nil
}

func removeAccountFromConfig(providerID, name string) {
	cfg, err := config.Load()
	if err != nil {
		return
	}
	_ = cfg.RemoveAccount(providerID, name)
}

// accountOf looks a configured account up by tag or email.
func accountOf(id, account string) (config.AccountConfig, bool, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.AccountConfig{}, false, err
	}
	a, ok := cfg.Account(id, account)
	return a, ok, nil
}

// --- refresh log -----------------------------------------------------------

// loginLog collects what an official CLI prints, for the login sheet to show.
type loginLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *loginLog) write(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, strings.Split(s, "\n")...)
	if len(l.lines) > 400 {
		l.lines = l.lines[len(l.lines)-400:]
	}
}

// String returns the log's tail, at most lines lines.
func (l *loginLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func (l *loginLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = nil
}
