// Package cli implements the leeway subcommands.
package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/credwrite"
	"leeway/internal/fetch"
	"leeway/internal/history"
	"leeway/internal/model"
	"leeway/internal/provider"
	"leeway/internal/render"
	"leeway/internal/switcher"
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
	case "login":
		return runLogin(args[1:])
	case "logout":
		return runLogout(args[1:])
	case "status":
		return runStatus(args[1:])
	case "use":
		return runUse(args[1:])
	case "help", "-h", "--help":
		rest := args[1:]
		if len(rest) == 0 {
			fmt.Print(usageText)
			return 0
		}
		if len(rest) > 1 {
			fmt.Fprintf(os.Stderr, "unexpected argument %q\n", rest[1])
			return 2
		}
		return runHelpTopic(rest[0])
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
		last           int
		noCache        bool
		noDelta        bool
	)
	last = -1
	fs := newFlagSet("usage", &jsonOut)
	fs.Help(usageHelp)
	fs.providerAccount(&providerFilter, &accountFilter)
	fs.IntVar(&watchSecs, "watch", 0, "每 N 秒刷新")
	fs.IntVar(&last, "last", -1, "近 N 次探测（≥1 时不联网）")
	fs.BoolVar(&noCache, "no-cache", false, "不走 60s 缓存")
	fs.BoolVar(&noDelta, "no-delta", false, "表上不画相对上次的增量（JSON 仍带 delta）")
	if !fs.Parse(args) {
		return 2
	}
	if last == 0 {
		fmt.Fprintln(os.Stderr, "usage: --last 必须 ≥ 1")
		return 2
	}
	if last >= 1 && watchSecs > 0 {
		fmt.Fprintln(os.Stderr, "usage: --last 和 --watch 不能一起用")
		return 2
	}
	if last >= 1 {
		return usageHistory(last, providerFilter, accountFilter, jsonOut)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var watchPrev map[history.Key]history.Obs
	watchRecorded := false

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
		nextPrev := history.LastSeenFromOutput(out)
		seen, lerr := history.LoadLastSeenBefore(out.GeneratedAt)
		if lerr != nil {
			log.Printf("history: load failed: %v", lerr)
			seen = nil
		}
		if watchPrev != nil {
			history.Attach(out, watchPrev)
		} else {
			history.Attach(out, seen)
		}
		if err := history.Record(out, history.Meta{
			Watch:          watchSecs > 0,
			ProviderFilter: providerFilter,
			AccountFilter:  accountFilter,
			Skip:           watchSecs > 0 && watchRecorded,
		}); err != nil {
			log.Printf("history: write failed (data is still valid): %v", err)
		}
		if watchSecs > 0 {
			watchRecorded = true
		}
		if jsonOut {
			if err := render.RenderJSON(os.Stdout, out); err != nil {
				fmt.Fprintln(os.Stderr, "render:", err)
				return nil, 2
			}
		} else if noDelta {
			render.RenderTableNoDelta(os.Stdout, out, render.IsTTY())
		} else {
			render.RenderTable(os.Stdout, out, render.IsTTY())
		}
		if watchSecs > 0 {
			watchPrev = nextPrev
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

func usageHistory(last int, providerFilter, accountFilter string, jsonOut bool) int {
	series, err := history.Collect(last, providerFilter, accountFilter)
	if err != nil {
		fmt.Fprintln(os.Stderr, "history:", err)
		return 1
	}
	if jsonOut {
		type view struct {
			Version int              `json:"version"`
			Last    int              `json:"last"`
			Series  []history.Series `json:"series"`
		}
		if series == nil {
			series = []history.Series{}
		}
		return newJSONEncoder().Encode(view{Version: 1, Last: last, Series: series})
	}
	blocks := make([]render.SparkBlock, 0, len(series))
	for _, s := range series {
		b := render.SparkBlock{
			Head:  historyHead(s.Provider, s.Account),
			Title: windowTitle(s.Provider, s.Window),
			N:     len(s.Points),
		}
		for _, p := range s.Points {
			v, ok, label := pointValue(p)
			if !ok {
				continue
			}
			b.Vals = append(b.Vals, v)
			b.Gaps = append(b.Gaps, p.Reset)
			if b.First == "" {
				b.First = label
			}
			b.Last = label
		}
		if len(b.Vals) == 0 {
			continue
		}
		blocks = append(blocks, b)
	}
	render.RenderHistory(os.Stdout, blocks, render.IsTTY())
	return 0
}

func historyHead(providerID, account string) string {
	name := providerID
	if p, ok := provider.Get(providerID); ok {
		name = p.DisplayName()
	}
	if account == "" || account == "default" {
		return name
	}
	return name + "  " + account
}

func windowTitle(providerID, windowID string) string {
	if p, ok := provider.Get(providerID); ok {
		for _, w := range p.DeclaredWindows() {
			if w.ID == windowID {
				return w.Title
			}
		}
	}
	switch windowID {
	case "included":
		return "Included"
	case "credits":
		return "Credits"
	case "cost":
		return "Balance"
	case "ondemand":
		return "On-demand"
	case "renewal":
		return "Renewal"
	}
	return windowID
}

func pointValue(p history.Point) (float64, bool, string) {
	if p.UsedPercent != nil {
		return *p.UsedPercent, true, fmt.Sprintf("%.0f%%", *p.UsedPercent)
	}
	if p.Used != nil {
		return *p.Used, true, renderMoney(*p.Used, p.Currency)
	}
	return 0, false, ""
}

func renderMoney(v float64, currency string) string {
	switch currency {
	case "CNY", "¥", "RMB":
		return fmt.Sprintf("¥%.2f", v)
	case "USD", "$":
		return fmt.Sprintf("$%.2f", v)
	default:
		if currency != "" {
			return fmt.Sprintf("%s %.2f", currency, v)
		}
		return fmt.Sprintf("%.2f", v)
	}
}

// --- use ----------------------------------------------------------------

// runUse makes a named account the one the local tool actually uses:
// cursor → Cursor IDE + ~/.cursor/auth.json; grok → ~/.grok/auth.json;
// everything else → the DSH credentials file.
func runUse(args []string) int {
	var (
		providerID string
		account    string
		jsonOut    bool
	)
	fs := newFlagSet("use", &jsonOut)
	fs.Help(useHelp)
	fs.providerAccount(&providerID, &account)
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
	switch providerID {
	case "cursor":
		return useCursor(cfg, account, jsonOut)
	case "grok":
		return useGrok(cfg, account, jsonOut)
	}

	acc, ok := cfg.Account(providerID, account)
	if !ok {
		fmt.Fprintf(os.Stderr, "account %q not found for provider %q\n", account, providerID)
		return 2
	}
	if acc.APIKey == "" {
		fmt.Fprintf(os.Stderr, "account %q of %q has no API key — leeway login %s %s --key\n",
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
	return printUseResult(switcher.Result{
		Provider: providerID,
		Account:  account,
		Targets:  []string{target.Key + " → " + target.File},
	}, jsonOut)
}

func useCursor(cfg *config.Config, account string, jsonOut bool) int {
	name, email := resolveCursorAccount(cfg, account)
	ca, ok, err := auth.LoadCursorAuth(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cursor auth:", err)
		return 1
	}
	if !ok || ca.AccessToken == "" {
		fmt.Fprintf(os.Stderr, "cursor 账号 %q 还没登录 — leeway login cursor %s\n", name, name)
		return 2
	}
	if exp, ok := auth.JWTExpiry(ca.AccessToken); ok && time.Now().After(exp) {
		fmt.Fprintf(os.Stderr, "cursor 账号 %q 的令牌已过期 — leeway login cursor %s\n", name, name)
		return 1
	}
	if block, reason := switcher.CheckCursorIDE(); block {
		fmt.Fprintln(os.Stderr, reason)
		return 1
	}
	targets, err := switcher.OfficialCursorTargets()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cursor targets:", err)
		return 1
	}
	saved, unsaved, err := switcher.SnapshotCursorIfNeeded(targets, ca.AccessToken)
	if err != nil {
		fmt.Fprintln(os.Stderr, "save current cursor session:", err)
		return 1
	}
	if saved != "" {
		fmt.Fprintf(os.Stderr, "已把当前 Cursor 登录收进 leeway 账号 %s\n", saved)
	} else if unsaved {
		fmt.Fprintln(os.Stderr, "当前 Cursor 登录不在 leeway 台账里，切走之后只剩目标号")
	}
	if err := switcher.ApplyCursor(targets, ca, email); err != nil {
		fmt.Fprintln(os.Stderr, "write cursor session:", err)
		return 1
	}
	if !jsonOut {
		fmt.Fprintln(os.Stderr, "重新打开 Cursor 后才会用新号")
	}
	return printUseResult(switcher.Result{
		Provider: "cursor",
		Account:  name,
		Email:    email,
		Targets:  []string{switcher.DisplayPath(targets.StateDB), switcher.DisplayPath(targets.AuthJSON)},
		SavedAs:  saved,
	}, jsonOut)
}

func useGrok(cfg *config.Config, account string, jsonOut bool) int {
	name, src := resolveGrokAuthFile(cfg, account)
	if src == "" {
		fmt.Fprintf(os.Stderr, "grok 账号 %q 还没登录 — leeway login grok %s\n", name, name)
		return 2
	}
	if !fileExists(src) {
		fmt.Fprintf(os.Stderr, "grok 账号 %q 的会话文件不在 %s — leeway login grok %s\n", name, src, name)
		return 2
	}
	dst, err := switcher.OfficialGrokAuthFile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "grok target:", err)
		return 1
	}
	if err := switcher.ApplyGrok(dst, src); err != nil {
		fmt.Fprintln(os.Stderr, "write grok session:", err)
		return 1
	}
	email := grokAuthEmail(src)
	return printUseResult(switcher.Result{
		Provider: "grok",
		Account:  name,
		Email:    email,
		Targets:  []string{switcher.DisplayPath(dst)},
	}, jsonOut)
}

func resolveCursorAccount(cfg *config.Config, account string) (name, email string) {
	name = account
	if a, ok := cfg.Account("cursor", account); ok {
		name = a.Name
		email = a.Email
	}
	return name, email
}

func resolveGrokAuthFile(cfg *config.Config, account string) (name, path string) {
	name = account
	if name == "" {
		name = "default"
	}
	if a, ok := cfg.Account("grok", account); ok {
		name = a.Name
		if a.AuthFile != "" {
			if p, err := provider.LocalSessionPath(a.AuthFile); err == nil {
				return name, p
			}
			return name, a.AuthFile
		}
	}
	home, err := grokHome(name)
	if err != nil {
		return name, ""
	}
	return name, filepath.Join(home, "auth.json")
}

func printUseResult(r switcher.Result, jsonOut bool) int {
	if jsonOut {
		return newJSONEncoder().Encode(r)
	}
	fmt.Printf("switched %s to account %q", r.Provider, r.Account)
	if r.Email != "" && r.Email != r.Account {
		fmt.Printf(" (%s)", r.Email)
	}
	fmt.Printf(" (wrote %s)\n", strings.Join(r.Targets, ", "))
	return 0
}

func saveManualCookie(providerID, header string) int {
	if providerID == "" || header == "" {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	pc := cfg.Provider(providerID)
	pc.ManualCookieHeader = header
	pc.CookieSource = "manual"
	cfg.Providers[providerID] = pc
	if err := cfg.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "config save:", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "saved manual cookie header for %q\n", providerID)
	return 0
}

func importBrowserCookies(providerID, account, browser, profile string, force bool) int {
	if providerID == "" {
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
		fmt.Fprintf(os.Stderr, "cookie cache for %q%s is fresh (imported %s by %s); use --force to re-import\n",
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
	fmt.Fprintf(os.Stderr, "imported %d cookies for %q%s from %s%s\n", n, providerID, cacheSuffix(profile), browser, accountSuffix(account))
	return 0
}

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

func saveAccountAuthFile(providerID, account, authFile string) int {
	if providerID == "" || account == "" || authFile == "" {
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
	fmt.Fprintf(os.Stderr, "saved auth-file %q for account %q of %q\n", authFile, account, providerID)
	return 0
}

func maskKey(key string) string {
	if len(key) < 8 {
		return "sk-…"
	}
	return key[:6] + "…"
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

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
