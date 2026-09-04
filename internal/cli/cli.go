// Package cli implements the leeway subcommands.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/credwrite"
	"leeway/internal/fetch"
	"leeway/internal/model"
	"leeway/internal/provider"
	"leeway/internal/render"
)

const usageText = rootHelp

// Run dispatches subcommands and returns the process exit code.
func Run(args []string) int {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	switch args[0] {
	case "usage":
		return runUsage(args[1:])
	case "use":
		return runUse(args[1:])
	case "providers":
		return runProviders(args[1:])
	case "cookie":
		return runCookie(args[1:])
	case "config":
		return runConfig(args[1:])
	case "auth":
		return runAuth(args[1:])
	case "doctor":
		return runDoctor(args[1:])
	case "help", "-h", "--help":
		if args[0] == "help" && len(args) > 1 {
			return runHelpTopic(args[1])
		}
		fmt.Print(usageText)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[0], usageText)
		return 2
	}
}

func exitCodeFor(out *model.Output) int {
	if len(out.Providers) == 0 {
		return 2
	}
	for _, p := range out.Providers {
		if p.Status == "error" || p.Status == "not_configured" {
			return 1
		}
	}
	return 0
}

// --- usage ---------------------------------------------------------------

func runUsage(args []string) int {
	var (
		providerFilter string
		accountFilter  string
		jsonOut        bool
		watchSecs      int
		noCache        bool
	)
	fs := newFlagSet("usage", &jsonOut)
	fs.Help(usageHelp)
	fs.StringVar(&providerFilter, "provider", "", "只查这一家（cursor / grok / opencode-go / commandcode / …）")
	fs.StringVar(&accountFilter, "account", "", "只查这个账号（tag、邮箱或订阅名）")
	fs.IntVar(&watchSecs, "watch", 0, "每 N 秒刷新")
	fs.BoolVar(&noCache, "no-cache", false, "不走 60s 缓存")
	if !fs.Parse(args) {
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runOnce := func() (*model.Output, int) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "config:", err)
			return nil, 2
		}
		out, err := fetch.FetchAll(ctx, cfg, fetch.Options{
			UseCache:       !noCache,
			ProviderFilter: providerFilter,
			AccountFilter:  accountFilter,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "fetch:", err)
			return nil, 2
		}
		if jsonOut {
			if err := render.RenderJSON(os.Stdout, out); err != nil {
				fmt.Fprintln(os.Stderr, "render:", err)
				return nil, 2
			}
		} else {
			render.RenderTable(os.Stdout, out, render.IsTTY())
		}
		return out, exitCodeFor(out)
	}

	if watchSecs > 0 {
		interval := time.Duration(watchSecs) * time.Second
		for {
			if render.IsTTY() {
				fmt.Print("\x1b[H\x1b[2J")
			}
			_, code := runOnce()
			select {
			case <-ctx.Done():
				return code
			case <-time.After(interval):
			}
		}
	}
	_, code := runOnce()
	return code
}

// --- use ----------------------------------------------------------------

// runUse switches the active named subscription for a provider: it writes that
// subscription's API key into the provider's key target (default: DSH
// credentials file, key "<PROVIDER>_API_KEY"), which hot-reloads on next use.
func runUse(args []string) int {
	var (
		providerID string
		account    string
	)
	fs := newFlagSet("use", nil)
	fs.Help(useHelp)
	fs.StringVar(&providerID, "provider", "", "provider（opencode-go / commandcode / …）")
	fs.StringVar(&account, "account", "", "要写给 DSH 的那个订阅名")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" || account == "" {
		fs.Usage()
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	acc, ok := cfg.Account(providerID, account)
	if !ok {
		fmt.Fprintf(os.Stderr, "account %q not found for provider %q\n", account, providerID)
		return 2
	}
	if acc.APIKey == "" {
		fmt.Fprintf(os.Stderr, "account %q of %q has no API key — set one with: leeway config set-api-key --provider %s --account %s\n",
			account, providerID, providerID, account)
		return 2
	}
	target := cfg.KeyTarget(providerID)
	if target.File == "" || target.Key == "" {
		fmt.Fprintln(os.Stderr, "key target is not configured (file and key required)")
		return 2
	}
	if err := credwrite.Patch(target.File, target.Key, acc.APIKey); err != nil {
		fmt.Fprintln(os.Stderr, "write target:", err)
		return 1
	}
	fmt.Printf("switched %s to account %q (wrote %s to %s)\n", providerID, account, target.Key, target.File)
	return 0
}

// --- providers -----------------------------------------------------------

func runProviders(args []string) int {
	var jsonOut bool
	fs := newFlagSet("providers", &jsonOut)
	fs.Help(providersHelp)
	if !fs.Parse(args) {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	provs := provider.All()
	if jsonOut {
		type provView struct {
			ID               string   `json:"id"`
			DisplayName      string   `json:"displayName"`
			Enabled          bool     `json:"enabled"`
			EnvKeys          []string `json:"envKeys,omitempty"`
			UsesCookies      bool     `json:"usesCookies"`
			UsesCodexCLI     bool     `json:"usesCodexCLI"`
			SupportsAccounts bool     `json:"supportsAccounts"`
		}
		views := make([]provView, 0, len(provs))
		for _, p := range provs {
			spec := p.AuthSpec()
			views = append(views, provView{
				ID: p.ID(), DisplayName: p.DisplayName(), Enabled: cfg.IsEnabled(p.ID()),
				EnvKeys: spec.EnvAPIKey, UsesCookies: spec.UsesCookies,
				UsesCodexCLI: spec.UsesCodexCLI, SupportsAccounts: spec.SupportsAccounts,
			})
		}
		enc := newJSONEncoder()
		return enc.Encode(views)
	}
	fmt.Printf("%-14s %-8s %s\n", "PROVIDER", "ENABLED", "HOW")
	for _, p := range provs {
		fmt.Printf("%-14s %-8t %s\n", p.DisplayName(), cfg.IsEnabled(p.ID()), loginHow(p))
	}
	return 0
}

func loginHow(p provider.Provider) string {
	switch p.ID() {
	case "cursor":
		return "leeway auth login --provider cursor"
	case "grok":
		return "leeway auth login --provider grok"
	case "opencode-go", "commandcode", "deepseek":
		return "leeway auth login --provider " + p.ID()
	case "codex":
		return "codex login"
	case "kimi":
		return "kimi login 或 leeway cookie import --provider kimi"
	default:
		if h := p.AuthSpec().LoginHint; h != "" {
			return h
		}
		return "leeway doctor"
	}
}

// --- cookie --------------------------------------------------------------

func runCookie(args []string) int {
	if len(args) < 1 || isHelpArg(args[0]) {
		printHelp(os.Stderr, cookieHelp)
		if len(args) > 0 && isHelpArg(args[0]) {
			return 0
		}
		return 2
	}
	switch args[0] {
	case "set":
		return cookieSet(args[1:])
	case "import":
		return cookieImport(args[1:])
	case "list":
		return cookieList(args[1:])
	case "clear":
		return cookieClear(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "cookie: unknown subcommand %q\n\n", args[0])
		printHelp(os.Stderr, cookieHelp)
		return 2
	}
}

func cookieSet(args []string) int {
	var (
		providerID string
		manual     string
	)
	fs := newFlagSet("cookie set", nil)
	fs.Help("leeway cookie set — 手动粘贴 Cookie header\n\n  leeway cookie set --provider grok --manual \"a=b; c=d\"\n")
	fs.StringVar(&providerID, "provider", "", "provider（grok / kimi / opencode-go）")
	fs.StringVar(&manual, "manual", "", "Cookie header，例如 a=b; c=d")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" || manual == "" {
		fs.Usage()
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	pc := cfg.Provider(providerID)
	pc.ManualCookieHeader = manual
	pc.CookieSource = "manual"
	cfg.Providers[providerID] = pc
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 2
	}
	fmt.Printf("saved manual cookie header for %q (config: %s)\n", providerID, configPath())
	return 0
}

func cookieImport(args []string) int {
	var (
		providerID string
		browser    string
		profile    string
		account    string
		force      bool
	)
	fs := newFlagSet("cookie import", nil)
	fs.Help("leeway cookie import — 从浏览器导入 cookie\n\n  leeway cookie import --provider grok\n  leeway cookie import --provider kimi --browser kimi-desktop\n")
	fs.StringVar(&providerID, "provider", "", "provider（grok / kimi / opencode-go）")
	fs.StringVar(&browser, "browser", "", "chrome | edge | kimi-desktop（默认 chrome）")
	fs.StringVar(&profile, "profile", "", "Chrome profile（Default / Profile 1；默认选 cookie 最多的）")
	fs.StringVar(&account, "account", "", "绑成这个账号（需同时给 --profile）")
	fs.BoolVar(&force, "force", false, "忽略缓存，强制重导")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}
	if browser == "" {
		browser = "chrome"
	}
	if cc, ok, err := auth.LoadCookieCache(providerID, profile); err != nil {
		if rerr := auth.RemoveCookieCacheFile(); rerr != nil {
			fmt.Fprintln(os.Stderr, "cache:", rerr)
			return 1
		}
	} else if ok && !force && time.Since(cc.ImportedAt) < auth.ImportTTL {
		fmt.Printf("cookie cache for %q%s is fresh (imported %s by %s); use --force to re-import\n",
			providerID, cacheSuffix(profile), cc.ImportedAt.Format(time.RFC3339), cc.Browser)
		return 0
	}
	header, err := auth.Import(providerID, browser, profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import failed:", err)
		return 1
	}
	if err := auth.SaveCookieCache(providerID, profile, header, browser); err != nil {
		fmt.Fprintln(os.Stderr, "cache save:", err)
		return 1
	}
	if account != "" && profile != "" {
		if err := bindAccount(providerID, account, profile); err != nil {
			fmt.Fprintln(os.Stderr, "account bind:", err)
			return 1
		}
	} else if account != "" {
		fmt.Fprintf(os.Stderr, "warning: --account binding requires --profile; config not updated for account %q (imported cookies are stored at provider level)\n", account)
	}
	n := strings.Count(header, ";") + 1
	if header == "" {
		n = 0
	}
	fmt.Printf("imported %d cookies for %q%s from %s%s\n", n, providerID, cacheSuffix(profile), browser, accountSuffix(account))
	return 0
}

// bindAccount adds/updates a config account entry bound to a profile.
func bindAccount(providerID, account, profile string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pc := cfg.Provider(providerID)
	replaced := false
	for i := range pc.Accounts {
		if pc.Accounts[i].Name == account {
			pc.Accounts[i].Profile = profile
			replaced = true
		}
	}
	if !replaced {
		pc.Accounts = append(pc.Accounts, config.AccountConfig{Name: account, Profile: profile})
	}
	cfg.Providers[providerID] = pc
	return cfg.Save()
}

func cacheSuffix(profile string) string {
	if profile == "" {
		return ""
	}
	return " [" + profile + "]"
}

func accountSuffix(account string) string {
	if account == "" {
		return ""
	}
	return " (account: " + account + ")"
}

func cookieList(args []string) int {
	var jsonOut bool
	fs := newFlagSet("cookie list", &jsonOut)
	fs.Help("leeway cookie list — 已导入 / 手动保存的 cookie\n")
	if !fs.Parse(args) {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	manual := map[string]string{}
	for id, pc := range cfg.Providers {
		if pc.ManualCookieHeader != "" {
			manual[id] = "manual"
		}
		for _, a := range pc.Accounts {
			if a.ManualCookieHeader != "" {
				manual[id] = "manual (account " + a.Name + ")"
			}
		}
	}
	caches, err := auth.AllCookieCaches()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cache:", err)
		return 1
	}
	type row struct {
		ProviderID  string
		Source      string
		ImportedAt  string
		CookieCount int
	}
	rows := []row{}
	for id, src := range manual {
		rows = append(rows, row{ProviderID: id, Source: src})
	}
	for _, cc := range caches {
		src := "imported:" + cc.Browser
		if cc.Profile != "" {
			src += " [" + cc.Profile + "]"
		}
		rows = append(rows, row{ProviderID: cc.ProviderID, Source: src, ImportedAt: cc.ImportedAt.Format(time.RFC3339), CookieCount: strings.Count(cc.Header, ";") + 1})
	}
	// Deterministic output: provider ID, then import time.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ProviderID != rows[j].ProviderID {
			return rows[i].ProviderID < rows[j].ProviderID
		}
		return rows[i].ImportedAt < rows[j].ImportedAt
	})
	if jsonOut {
		type jsonRow struct {
			ProviderID  string `json:"providerID"`
			Source      string `json:"source"`
			ImportedAt  string `json:"importedAt,omitempty"`
			CookieCount int    `json:"cookieCount"`
		}
		jsonRows := make([]jsonRow, 0, len(rows))
		for _, r := range rows {
			jsonRows = append(jsonRows, jsonRow{ProviderID: r.ProviderID, Source: r.Source, ImportedAt: r.ImportedAt, CookieCount: r.CookieCount})
		}
		return newJSONEncoder().Encode(jsonRows)
	}
	fmt.Printf("%-12s %-28s %s\n", "PROVIDER", "SOURCE", "IMPORTED")
	for _, r := range rows {
		if r.ImportedAt == "" {
			fmt.Printf("%-12s %-28s —\n", r.ProviderID, r.Source)
		} else {
			fmt.Printf("%-12s %-28s %s\n", r.ProviderID, r.Source, r.ImportedAt)
		}
	}
	return 0
}

func cookieClear(args []string) int {
	var (
		providerID string
		profile    string
	)
	fs := newFlagSet("cookie clear", nil)
	fs.Help("leeway cookie clear — 清除导入的 cookie 缓存\n\n  leeway cookie clear --provider grok\n")
	fs.StringVar(&providerID, "provider", "", "provider")
	fs.StringVar(&profile, "profile", "", "只清这个 Chrome profile")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}
	if err := auth.ClearCookieCache(providerID, profile); err != nil {
		fmt.Fprintln(os.Stderr, "cache:", err)
		return 1
	}
	fmt.Printf("cleared cookie cache for %q%s\n", providerID, cacheSuffix(profile))
	return 0
}

// --- config --------------------------------------------------------------

func runConfig(args []string) int {
	if len(args) < 1 || isHelpArg(args[0]) {
		printHelp(os.Stderr, configHelp)
		if len(args) > 0 && isHelpArg(args[0]) {
			return 0
		}
		return 2
	}
	switch args[0] {
	case "path":
		fmt.Println(configPath())
		return 0
	case "set-api-key":
		return configSetAPIKey(args[1:])
	case "set-auth-file":
		return configSetAuthFile(args[1:])
	case "keys":
		return configKeys(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "config: unknown subcommand %q\n\n", args[0])
		printHelp(os.Stderr, configHelp)
		return 2
	}
}

// configSetAuthFile binds a session-file override (e.g. grok's auth.json) to
// a named account so one provider can track several CLI sessions. The value
// is plain text (a path), so it is echoed back like the keys listing.
func configSetAuthFile(args []string) int {
	var (
		providerID string
		account    string
		authFile   string
	)
	fs := newFlagSet("config set-auth-file", nil)
	fs.Help("leeway config set-auth-file — 给 grok 账号指定 auth.json\n\n  leeway config set-auth-file --provider grok --account home --auth-file ~/.grok/auth.json\n")
	fs.StringVar(&providerID, "provider", "", "provider（一般是 grok）")
	fs.StringVar(&account, "account", "", "账号名")
	fs.StringVar(&authFile, "auth-file", "", "会话文件路径，例如 ~/.grok/auth.json")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" || account == "" || authFile == "" {
		fs.Usage()
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	if err := cfg.SetAccountAuthFile(providerID, account, authFile); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 2
	}
	fmt.Printf("saved auth-file %q for account %q of %q\n", authFile, account, providerID)
	return 0
}

func configSetAPIKey(args []string) int {
	var (
		providerID string
		account    string
		key        string
	)
	fs := newFlagSet("config set-api-key", nil)
	fs.Help(configSetKeyHelp)
	fs.StringVar(&providerID, "provider", "", "provider（deepseek / opencode-go / commandcode）")
	fs.StringVar(&account, "account", "", "订阅名（多 key 时必填）")
	fs.StringVar(&key, "key", "", "API key（省略则交互输入，不回显）")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}
	if key == "" {
		fmt.Fprintf(os.Stderr, "API key for %s: ", providerID)
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read:", err)
			fmt.Fprintln(os.Stderr, "stdin is not a terminal — use: leeway config set-api-key --provider", providerID, "--key VALUE")
			return 2
		}
		key = strings.TrimSpace(string(raw))
	}
	if key == "" {
		fmt.Fprintln(os.Stderr, "empty key; aborting")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	if account != "" {
		if err := cfg.SetAccountKey(providerID, account, key); err != nil {
			fmt.Fprintln(os.Stderr, "config save:", err)
			return 2
		}
		fmt.Printf("saved API key for account %q of %q\n", account, providerID)
		return 0
	}
	if err := cfg.SetAPIKey(providerID, key); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 2
	}
	fmt.Printf("saved API key for %q\n", providerID)
	return 0
}

// configKeys lists the named API-key subscriptions for a provider, masking the
// values. Active is whichever stored key currently matches the target file
// (the last `leeway use` write).
func configKeys(args []string) int {
	var (
		providerID string
		jsonOut    bool
	)
	fs := newFlagSet("config keys", &jsonOut)
	fs.Help(configKeysHelp)
	fs.StringVar(&providerID, "provider", "", "provider（opencode-go / commandcode）")
	if !fs.Parse(args) {
		return 2
	}
	if providerID == "" {
		fs.Usage()
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	target := cfg.KeyTarget(providerID)
	live := credwrite.Lookup(target.File, target.Key)
	type row struct {
		Account string `json:"account"`
		Key     string `json:"key"`
		Set     bool   `json:"set"`
		Active  bool   `json:"active"`
	}
	rows := []row{}
	for _, a := range cfg.Accounts(providerID) {
		rows = append(rows, row{
			Account: a.Name,
			Key:     maskKey(a.APIKey),
			Set:     a.APIKey != "",
			Active:  a.APIKey != "" && live != "" && a.APIKey == live,
		})
	}
	if jsonOut {
		return newJSONEncoder().Encode(rows)
	}
	for _, r := range rows {
		mark := ""
		if r.Active {
			mark = "  active"
		}
		if r.Set {
			fmt.Printf("%-12s %s%s\n", r.Account, r.Key, mark)
		} else {
			fmt.Printf("%-12s (no key)%s\n", r.Account, mark)
		}
	}
	return 0
}

func maskKey(key string) string {
	if len(key) < 8 {
		return "sk-…"
	}
	return key[:6] + "…"
}

// --- doctor --------------------------------------------------------------

func runDoctor(args []string) int {
	var jsonOut bool
	fs := newFlagSet("doctor", &jsonOut)
	fs.Help(doctorHelp)
	if !fs.Parse(args) {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	type check struct {
		Provider string `json:"provider"`
		Check    string `json:"check"`
		OK       bool   `json:"ok"`
		Hint     string `json:"hint,omitempty"`
	}
	checks := []check{}
	for _, p := range provider.All() {
		spec := p.AuthSpec()
		var apiOK, tokenOK, cookieOK bool
		var apiHint, tokenHint, cookieHint string
		if len(spec.EnvAPIKey) > 0 {
			for _, k := range spec.EnvAPIKey {
				if os.Getenv(k) != "" {
					apiOK = true
				}
			}
			if !apiOK && cfg.APIKey(p.ID()) != "" {
				apiOK = true
			}
			if !apiOK {
				for _, a := range cfg.Accounts(p.ID()) {
					if a.APIKey != "" {
						apiOK = true
						break
					}
				}
			}
			hint := "leeway auth login --provider " + p.ID()
			if spec.SupportsAccounts {
				hint += " --account NAME"
			}
			apiHint = hintIf(!apiOK, hint)
		}
		if len(spec.EnvAuthToken) > 0 {
			for _, k := range spec.EnvAuthToken {
				if os.Getenv(k) != "" {
					tokenOK = true
				}
			}
			if !tokenOK && cfg.APIKey(p.ID()) != "" {
				tokenOK = true
			}
			hint := "leeway auth login --provider " + p.ID()
			if spec.SupportsAccounts {
				hint += " --account NAME"
			}
			tokenHint = hintIf(!tokenOK, hint)
		}
		if spec.UsesCookies {
			pc := cfg.Provider(p.ID())
			// Sessions bound via `cookie import --account X --profile P` live
			// under the account-scoped profile key, so the check must run per
			// configured account (falling back to "default"); the provider is
			// configured if any account resolves.
			accounts := pc.Accounts
			if len(accounts) == 0 {
				accounts = []config.AccountConfig{{Name: "default"}}
			}
			for _, a := range accounts {
				if h, err := auth.ResolveCookieHeader(p.ID(), pc, a.Name); err == nil && h != "" {
					cookieOK = true
					break
				}
			}
			cookieHint = hintIf(!cookieOK, "leeway cookie import --provider "+p.ID())
			if !cookieOK {
				cookieHint += " (or --manual)"
				for _, fb := range spec.CookieFallbacks {
					cookieHint += "; or: leeway cookie import --provider " + p.ID() + " --browser " + fb
				}
			}
		}
		// Dual-path (api key / token + cookies) is configured if either
		// method works; do not AND-require the unused one.
		dual := (len(spec.EnvAPIKey) > 0 || len(spec.EnvAuthToken) > 0) && spec.UsesCookies
		anyAuth := apiOK || tokenOK || cookieOK
		if len(spec.EnvAPIKey) > 0 && (!dual || apiOK || !anyAuth) {
			checks = append(checks, check{p.DisplayName(), "api key", apiOK, apiHint})
		}
		if len(spec.EnvAuthToken) > 0 && (!dual || tokenOK || !anyAuth) {
			checks = append(checks, check{p.DisplayName(), "auth token", tokenOK, tokenHint})
		}
		if spec.UsesCookies && (!dual || cookieOK || !anyAuth) {
			checks = append(checks, check{p.DisplayName(), "cookies", cookieOK, cookieHint})
		}
		if spec.UsesKeychain {
			ok := false
			for _, k := range spec.EnvAuthToken {
				if os.Getenv(k) != "" {
					ok = true
					break
				}
			}
			if !ok {
				for _, a := range cursorAccounts(cfg) {
					if _, has, err := auth.LoadCursorAuth(a.Name); err == nil && has {
						ok = true
						break
					}
				}
			}
			checks = append(checks, check{p.DisplayName(), "login", ok, hintIf(!ok, "leeway auth login --provider "+p.ID())})
		}
		if spec.UsesCodexCLI {
			ok := fetch.HasCodexAuth()
			checks = append(checks, check{p.DisplayName(), "codex session", ok, hintIf(!ok, "codex login")})
		}
		for _, f := range spec.LocalAuthFiles {
			path, err := provider.HomePath(f)
			ok := err == nil && fileExists(path)
			checks = append(checks, check{p.DisplayName(), "local session", ok, hintIf(!ok, spec.LoginHint)})
		}
	}
	if jsonOut {
		return newJSONEncoder().Encode(checks)
	}
	for _, c := range checks {
		mark := "[✓]"
		if !c.OK {
			mark = "[✗]"
		}
		line := fmt.Sprintf("%-12s %-14s %s", c.Provider, c.Check, mark)
		if !c.OK && c.Hint != "" {
			line += "  " + c.Hint
		}
		fmt.Println(line)
	}
	ok := true
	for _, c := range checks {
		if !c.OK {
			ok = false
		}
	}
	if ok {
		return 0
	}
	return 1
}

func hintIf(cond bool, hint string) string {
	if cond {
		return hint
	}
	return ""
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// --- helpers -------------------------------------------------------------

func cursorAccounts(cfg *config.Config) []config.AccountConfig {
	accs := cfg.Accounts("cursor")
	extra, err := auth.ListCursorSessionAccounts()
	if err != nil {
		return config.DropDefaultAccounts(accs)
	}
	return config.DropDefaultAccounts(config.MergeNamedAccounts(accs, extra))
}

func configPath() string {
	p, err := config.Path()
	if err != nil {
		return "(unresolvable)"
	}
	return p
}

func newJSONEncoder() *jsonEncoder { return &jsonEncoder{} }
