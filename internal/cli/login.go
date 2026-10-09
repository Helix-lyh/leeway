package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/provider"
)

func runLogin(args []string) int {
	var (
		providerID string
		account    string
		useCookie  bool
		manual     string
		authFile   string
		key        string
		email      string
		browser    string
		profile    string
		force      bool
		noBrowser  bool
		timeoutMin int
	)
	fs := newFlagSet("login", nil)
	fs.Help(loginHelp)
	fs.providerAccount(&providerID, &account)
	fs.BoolVar(&useCookie, "cookie", false, "从浏览器导入登录状态")
	fs.StringVar(&manual, "manual", "", "手动粘贴的 Cookie 请求头，例如 a=b; c=d")
	fs.StringVar(&authFile, "file", "", "官方登录文件的路径")
	fs.StringVar(&key, "key", "", "API 密钥（不写就交互输入，不回显）")
	fs.StringVar(&email, "email", "", "账号邮箱，用来区分同一服务商的多个账号")
	fs.StringVar(&browser, "browser", "", "用哪个浏览器：chrome / edge / kimi-desktop（默认 chrome）")
	fs.StringVar(&profile, "profile", "", "用哪个 Chrome 个人资料（不填就自动选）")
	fs.BoolVar(&force, "force", false, "忽略已有缓存，重新导入一次")
	fs.BoolVar(&noBrowser, "no-browser", false, "只打印登录网址，不打开浏览器")
	fs.IntVar(&timeoutMin, "timeout", 15, "等浏览器授权的分钟数（默认 15）")
	if !fs.Parse(args) {
		return 2
	}
	if explicitEmptyFlag(args, "key") {
		fmt.Fprintln(os.Stderr, "密钥是空的，已中止")
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}
	if !knownProvider(providerID) {
		fmt.Fprintf(os.Stderr, "不认识的服务商 %q\n\n", providerID)
		fs.Usage()
		return 2
	}

	if useCookie {
		return importBrowserCookies(providerID, account, browser, profile, force)
	}
	if manual != "" {
		return saveManualCookie(providerID, manual)
	}
	if authFile != "" {
		return saveAccountAuthFile(providerID, account, authFile)
	}

	if key != "" || !officialCLILogin(providerID) {
		return loginAPIKey(providerID, account, email, noBrowser, key)
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
	case "codex":
		return loginCodex(ctx, account)
	case "kimi":
		return loginKimi(ctx, account, noBrowser)
	default:
		fmt.Fprintf(os.Stderr, "不认识的服务商 %q\n\n", providerID)
		fs.Usage()
		return 2
	}
}

func explicitEmptyFlag(args []string, name string) bool {
	long := "--" + name
	for i, a := range args {
		if a == long || a == "-"+name {
			return i+1 >= len(args) || args[i+1] == ""
		}
		if strings.HasPrefix(a, long+"=") && a[len(long)+1:] == "" {
			return true
		}
	}
	return false
}

func knownProvider(id string) bool {
	for _, p := range config.ProviderIDs {
		if p == id {
			return true
		}
	}
	return false
}

// officialCLILogin is the provider's own answer to "is logging in a matter of
// running its CLI": the same AuthSpec the window reads.
func officialCLILogin(id string) bool {
	p, ok := provider.Get(id)
	return ok && p.AuthSpec().OfficialCLI
}

func isolatedSessionHome(providerID, account string) (string, error) {
	dir, err := auth.SessionDir()
	if err != nil {
		return "", err
	}
	name := account
	if name == "" || name == "default" {
		name = "default"
	}
	return filepath.Join(dir, providerID+"-"+safeSessionName(name)), nil
}

// overrideEnv copies the process environment, dropping any existing values
// for the given keys so the child's getenv (first-wins on Unix) sees ours.
func overrideEnv(kv ...string) []string {
	drop := make(map[string]struct{}, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		drop[kv[i]] = struct{}{}
	}
	env := os.Environ()
	out := make([]string, 0, len(env)+len(kv)/2)
	for _, e := range env {
		k, _, ok := strings.Cut(e, "=")
		if ok {
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

// loginCodex runs the official `codex login` into an isolated CODEX_HOME
// under leeway's session dir (macOS and Linux share that layout).
func loginCodex(ctx context.Context, account string) int {
	bin, err := exec.LookPath("codex")
	if err != nil {
		fmt.Fprintln(os.Stderr, "没找到 codex 命令，先安装官方 Codex 工具，再重试")
		return 2
	}

	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	name := strings.TrimSpace(account)
	if name == "" || name == "default" {
		name = "default"
	}
	home, err := isolatedSessionHome("codex", name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "codex home:", err)
		return 1
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create home:", err)
		return 1
	}
	cmd.Env = overrideEnv("CODEX_HOME", home)
	authFile := filepath.Join(home, "auth.json")

	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "codex login:", err)
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	if err := cfg.SetAccountAuthFile("codex", name, authFile); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "\n已登录 codex 账号 %s → %s\n", displayAccount(name), authFile)
	return 0
}

// loginKimi runs `kimi login` into an isolated KIMI_CODE_HOME. If the kimi
// CLI is not on PATH, fall back to pasting an API key.
func loginKimi(ctx context.Context, account string, noBrowser bool) int {
	bin, err := exec.LookPath("kimi")
	if err != nil {
		fmt.Fprintln(os.Stderr, "没找到 kimi 命令，改成粘贴 API 密钥")
		return loginAPIKey("kimi", account, "", noBrowser, "")
	}

	name := account
	if name == "" || name == "default" {
		name = "default"
	}
	home, err := isolatedSessionHome("kimi", name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "kimi home:", err)
		return 1
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create home:", err)
		return 1
	}

	cmd := exec.CommandContext(ctx, bin, "login")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = overrideEnv("KIMI_CODE_HOME", home)
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "kimi login:", err)
		return 1
	}

	authFile := filepath.Join(home, "credentials", "kimi-code.json")
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	if err := cfg.SetAccountAuthFile("kimi", name, authFile); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "\n已登录 kimi 账号 %s → %s\n", displayAccount(name), authFile)
	return 0
}
