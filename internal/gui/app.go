package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/fetch"
	"leeway/internal/history"
	"leeway/internal/model"
	"leeway/internal/provider"
	"leeway/internal/provider/codex"
)

// App is the window's state: what was probed, what is being done, and what
// the user is looking at. The view reads it on the main thread; the refresh
// goroutine writes it through Window.Update.
type App struct {
	win *mygo.Window

	cfg    *config.Config
	out    *model.Output
	series []history.Series

	fetching bool
	fetchErr string
	lastAt   time.Time
	lastTry  time.Time
	lastTook time.Duration

	settings settings

	page string // "overview", "provider:<id>", "history", "about"

	settingsOpen bool // settings popover over the main window (not a second window)
	settingsTab  int

	toast string
	scrub chartScrub

	// spoken is what the next frame asks a screen reader to read out. The
	// view empties it, so nothing is said twice.
	spoken []string

	// clock is the window's clock. A rendering test pins it, so a snapshot
	// only changes when the window does.
	clock func() time.Time

	// window's.

	prompt  prompt
	confirm confirm
}

// settings are the GUI's own preferences, kept beside config.json so the CLI
// never rewrites them.
type settings struct {
	IntervalSec   int    `json:"intervalSec"` // 0 = manual refresh only
	TimeoutSec    int    `json:"timeoutSec"`  // per-provider probe timeout
	HistoryN      int    `json:"historyN"`    // observations per history chart
	Theme         string `json:"theme"`       // "system", "light" or "dark"
	LastPage      string `json:"lastPage,omitempty"`
	OnOpenFetch   bool   `json:"onOpenFetch"`   // probe as the window opens
	NotifyOnError bool   `json:"notifyOnError"` // system notification when a probe fails
}

func defaultSettings() settings {
	return settings{
		IntervalSec:   60,
		TimeoutSec:    15,
		HistoryN:      24,
		Theme:         "system",
		OnOpenFetch:   true,
		NotifyOnError: true,
	}
}

func settingsPath() (string, error) {
	p, err := config.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "gui.json"), nil
}

func loadSettings() settings {
	s := defaultSettings()
	p, err := settingsPath()
	if err != nil {
		return s
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	if s.IntervalSec < 0 {
		s.IntervalSec = 0
	}
	if s.TimeoutSec < 3 || s.TimeoutSec > 300 {
		s.TimeoutSec = 15
	}
	if s.HistoryN < 1 {
		s.HistoryN = 24
	}
	switch s.Theme {
	case "light", "dark", "system":
	default:
		s.Theme = "system"
	}
	return s
}

func (s settings) save() error {
	p, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// NewApp builds the app state from disk: config, settings and the last
// recorded observations.
func NewApp() *App {
	a := &App{
		settings: loadSettings(),
		page:     "overview",
	}
	if a.settings.LastPage != "" {
		a.page = a.settings.LastPage
	}
	a.cfg, _ = config.Load()
	a.applyEnvironment()
	return a
}

// applyEnvironment hands the probe layer the GUI's timeout, which the fetch
// orchestrator reads from LEEWAY_TIMEOUT.
func (a *App) applyEnvironment() {
	_ = os.Setenv("LEEWAY_TIMEOUT", strconv.Itoa(a.settings.TimeoutSec))
}

// Start wires the app to its window and probes once.
func (a *App) Start(win *mygo.Window) {
	a.win = win
	a.reloadConfig()
	a.loadHistory()
	if a.settings.OnOpenFetch {
		a.Refresh(false)
	}
	go a.heartbeat()
	a.captureWhenAsked()
}

// chartScrub is where the keyboard is reading a chart. A frame is built from
// scratch every time, so this is app state: a chart that forgot would drop the
// reader back at the newest observation on every frame.
type chartScrub struct {
	key string
	at  int
}

// captureWhenAsked writes one PNG of the window and quits, when
// LEEWAY_GUI_CAPTURE names a file. It is how a build gets checked on a
// machine nobody is sitting at.
func (a *App) captureWhenAsked() {
	path := os.Getenv("LEEWAY_GUI_CAPTURE")
	if path == "" {
		return
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		png, err := a.win.CapturePage()
		if err == nil {
			err = os.WriteFile(path, png, 0o644)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "leeway: 窗口截图失败：%v\n", err)
		}
		mygo.App.Quit()
	}()
}

// apply runs fn on the main thread and redraws. Without a window (tests, or
// one that is already gone) it runs fn directly.
func (a *App) apply(fn func()) {
	if w := a.win; w != nil && !w.IsDestroyed() {
		w.Update(fn)
		return
	}
	fn()
}

// heartbeat keeps the relative times fresh and starts a scheduled refresh.
// It never touches the state the view reads.
func (a *App) heartbeat() {
	for range time.Tick(time.Second) {
		if a.win == nil || a.win.IsDestroyed() {
			return
		}
		a.win.Invalidate()
	}
}

// Due reports whether a scheduled refresh is due. The view calls it on the
// main thread.
func (a *App) Due() bool {
	if a.settings.IntervalSec <= 0 || a.fetching || a.lastTry.IsZero() {
		return false
	}
	return time.Since(a.lastTry) >= time.Duration(a.settings.IntervalSec)*time.Second
}

func (a *App) reloadConfig() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("窗口：读配置失败：%v", err)
		return
	}
	a.cfg = cfg
}

func (a *App) loadHistory() {
	series, err := history.Collect(a.settings.HistoryN, "", "")
	if err != nil {
		log.Printf("窗口：读历史失败：%v", err)
		return
	}
	a.series = series
}

// Refresh probes every enabled provider in the background and records what it
// saw, exactly as `leeway usage` does. force skips the 60s snapshot cache.
func (a *App) Refresh(force bool) {
	if a.fetching {
		return
	}
	a.fetching = true
	a.fetchErr = ""
	n := a.settings.HistoryN
	notify := a.settings.NotifyOnError
	a.lastTry = time.Now()
	started := time.Now()

	go func() {
		cfg, err := config.Load()
		var (
			out    *model.Output
			series []history.Series
			ferr   string
		)
		if err != nil {
			ferr = err.Error()
		} else {
			out, err = fetch.FetchAll(context.Background(), cfg, fetch.Options{UseCache: !force})
			if err != nil {
				ferr = err.Error()
			} else {
				seen, lerr := history.LoadLastSeenBefore(out.GeneratedAt)
				if lerr != nil {
					log.Printf("窗口：读历史失败：%v", lerr)
				}
				history.Attach(out, seen)
				if rerr := history.Record(out, history.Meta{}); rerr != nil {
					log.Printf("窗口：记历史失败：%v", rerr)
				}
				if s, serr := history.Collect(n, "", ""); serr == nil {
					series = s
				}
			}
		}

		a.apply(func() {
			a.fetching = false
			if cfg != nil {
				a.cfg = cfg
			}
			if ferr != "" {
				a.fetchErr = ferr
				// What is spoken and what is notified are the same sentence the
				// page shows: a layer's own message is not written for a person,
				// so it stays out of all three and lives in the tooltip.
				said, _ := failureText("查询失败", ferr)
				a.say(said)
				if notify {
					a.notifyFailure(said)
				}
			} else {
				a.out = out
				a.series = series
				a.lastAt = out.GeneratedAt
				a.fetchErr = ""
				a.say("刷新完成：" + a.summary())
			}
			a.lastTook = time.Since(started)
		})
	}()
}

// summary is the probe result in one line, for the title bar and for a
// screen reader.
func (a *App) summary() string {
	ok, bad, missing := a.overview()
	return fmt.Sprintf("%d 正常 · %d 失败 · %d 未配置", ok, bad, missing)
}

// notifyFailure tells the user about a failed probe when the window is not in
// front, which is when the schedule matters most.
func (a *App) notifyFailure(reason string) {
	if !mygo.App.IsReady() || !mygo.NotificationsSupported() {
		return
	}
	if w := a.win; w != nil && w.IsFocused() {
		return
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "leeway-fetch-failed",
		Title: "leeway 查询失败",
		Body:  reason,
	})
	if err := n.Show(); err != nil {
		log.Printf("窗口：发通知失败：%v", err)
	}
}

// --- views over the probed data -------------------------------------------

// providerResult finds a provider's probe result, if it was probed.
func (a *App) providerResult(id string) *model.ProviderResult {
	if a.out == nil {
		return nil
	}
	for i := range a.out.Providers {
		if a.out.Providers[i].ID == id {
			return &a.out.Providers[i]
		}
	}
	return nil
}

// accountView is one account of one provider as the UI shows it.
type accountView struct {
	Name     string
	Email    string
	Plan     string
	Status   string
	Error    string
	Snapshot *model.Snapshot
	Cred     string // where the credential comes from, when known
	Expires  string
}

// providerView is one provider as the UI shows it.
type providerView struct {
	ID          string
	DisplayName string
	Enabled     bool
	Status      string
	Error       string
	Accounts    []accountView
}

// firstCred is the credential sentence of a provider's first account, for the
// places that name a provider's state rather than one account's.
func (v providerView) firstCred() string {
	if len(v.Accounts) == 0 {
		return ""
	}
	return v.Accounts[0].Cred
}

// hasSubscription reports whether a provider is one this machine actually has:
// a probe got an answer out of it, or a credential for it is on disk. What is
// left is a provider to set up, which the first-run guide is for.
func (v providerView) hasSubscription() bool {
	if v.Status == "ok" || v.Status == "error" {
		return true
	}
	for _, av := range v.Accounts {
		if av.Cred != "" {
			return true
		}
	}
	return false
}

// providers lists every built-in provider, probed or not, in a stable order:
// working ones first, then the rest alphabetically.
func (a *App) providers() []providerView {
	var out []providerView
	for _, p := range provider.All() {
		out = append(out, a.providerView(p))
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := statusRank(out[i].Status), statusRank(out[j].Status)
		if ri != rj {
			return ri < rj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func statusRank(status string) int {
	switch status {
	case "ok":
		return 0
	case "error":
		return 1
	case "not_configured":
		return 2
	default:
		return 3
	}
}

func (a *App) providerView(p provider.Provider) providerView {
	id := p.ID()
	v := providerView{ID: id, DisplayName: p.DisplayName(), Enabled: true, Status: "disabled"}
	if a.cfg != nil {
		v.Enabled = a.cfg.IsEnabled(id)
	}
	if !v.Enabled {
		return v
	}
	v.Status = "not_configured"
	pr := a.providerResult(id)
	if pr != nil {
		v.Status = pr.Status
		v.Error = pr.Error
	}
	for _, acc := range a.accountsToShow(p, pr) {
		av := accountView{Name: acc.Name, Email: acc.Email}
		if pr != nil {
			for _, ar := range pr.Accounts {
				if ar.Name == acc.Name || ar.Name == "" && acc.Name == "default" {
					av.Status = ar.Status
					av.Error = ar.Error
					av.Snapshot = ar.Snapshot
					break
				}
			}
		}
		if av.Status == "" {
			if pr == nil {
				av.Status = "not_configured"
			} else {
				av.Status = "not_configured"
			}
		}
		if av.Snapshot != nil && av.Snapshot.Identity != nil {
			// The email set at login wins: with several subscriptions of one
			// provider it is the only thing that tells them apart, so a probe
			// must not overwrite it.
			if av.Email == "" && av.Snapshot.Identity.Account != "" {
				av.Email = av.Snapshot.Identity.Account
			}
			av.Plan = av.Snapshot.Identity.Plan
		}
		av.Cred, av.Expires = a.credentialInfo(p, acc)
		v.Accounts = append(v.Accounts, av)
	}
	if len(v.Accounts) == 0 {
		v.Accounts = append(v.Accounts, accountView{
			Name:   "default",
			Status: "not_configured",
			Cred:   a.credentialInfoOnly(p, config.AccountConfig{Name: "default"}),
		})
	}
	if v.Status == "ok" && v.Error != "" {
		v.Error = ""
	}
	return v
}

// accountsToShow lists the accounts a provider has: the configured ones, and
// any the probe reported that the config does not name yet (cursor session
// directories, for one).
func (a *App) accountsToShow(p provider.Provider, pr *model.ProviderResult) []config.AccountConfig {
	refs := a.accountsFor(p)
	if pr == nil {
		return refs
	}
	seen := map[string]bool{}
	for _, acc := range refs {
		seen[displayAccount(acc.Name)] = true
	}
	for _, ar := range pr.Accounts {
		name := displayAccount(ar.Name)
		if seen[name] {
			continue
		}
		seen[name] = true
		refs = append(refs, config.AccountConfig{Name: name})
	}
	return refs
}

// accountsFor lists the accounts to show for a provider: the configured ones,
// cursor session directories, or the implicit default session.
func (a *App) accountsFor(p provider.Provider) []config.AccountConfig {
	var accs []config.AccountConfig
	if a.cfg != nil {
		accs = a.cfg.Accounts(p.ID())
		if p.ID() == "cursor" {
			if extra, err := auth.ListCursorSessionAccounts(); err == nil {
				accs = config.MergeNamedAccounts(accs, extra)
			}
		}
	}
	accs = config.DropDefaultAccounts(accs)
	if len(accs) == 0 {
		if a.cfg != nil {
			if acc, ok := a.cfg.Account(p.ID(), "default"); ok && acc.AuthFile != "" {
				return []config.AccountConfig{acc}
			}
		}
		if !p.AuthSpec().SupportsAccounts {
			return []config.AccountConfig{{Name: "default"}}
		}
	}
	return accs
}

// credentialInfo names where an account's credential comes from, without
// reading or showing its value. It only names a source it can see: a provider
// whose session file is not on disk has no credential, and naming one anyway
// would promise numbers that cannot arrive.
func (a *App) credentialInfo(p provider.Provider, acc config.AccountConfig) (kind, expires string) {
	spec := p.AuthSpec()
	switch {
	case acc.APIKey != "":
		kind = "账号的 API 密钥"
	case len(spec.EnvAPIKey) > 0 && os.Getenv(spec.EnvAPIKey[0]) != "":
		kind = "环境变量 " + spec.EnvAPIKey[0]
	case len(spec.EnvAuthToken) > 0 && os.Getenv(spec.EnvAuthToken[0]) != "":
		kind = "环境变量 " + spec.EnvAuthToken[0]
	case a.cfg != nil && a.cfg.APIKey(p.ID()) != "":
		kind = "配置里的 API 密钥"
	case acc.AuthFile != "":
		kind = "登录文件 " + shortenPath(acc.AuthFile)
	case acc.ManualCookieHeader != "":
		kind = "手动填的登录状态"
	case spec.UsesCookies:
		if a.cfg != nil {
			if h, err := auth.ResolveCookieHeader(p.ID(), a.cfg.Provider(p.ID()), acc.Name); err == nil && h != "" {
				kind = "浏览器登录状态"
			}
		}
	case spec.UsesCodexCLI:
		if path, err := codex.AuthFilePath(); err == nil && fileExists(path) {
			kind = "Codex CLI 登录文件"
		}
	case len(spec.LocalAuthFiles) > 0:
		if path, ok := localSessionFile(spec); ok {
			kind = "登录文件 " + shortenPath(path)
		}
	}
	if p.ID() == "cursor" && kind == "" {
		// cursor resolves its token environment first, then the configured key,
		// and only then the private session, so this is the fallback branch.
		if ca, ok, err := auth.LoadCursorAuth(acc.Name); err == nil && ok && ca.AccessToken != "" {
			kind = "Cursor 私有登录文件"
			if exp, ok := auth.JWTExpiry(ca.AccessToken); ok {
				expires = exp.Local().Format("01-02 15:04") + " 已过期"
			}
		}
	}
	return kind, expires
}

// localSessionFile is the first session file a provider declares that is
// actually on disk.
func localSessionFile(spec provider.AuthSpec) (string, bool) {
	for _, declared := range spec.LocalAuthFiles {
		path, err := provider.LocalSessionPath(declared)
		if err != nil {
			continue
		}
		if fileExists(path) {
			return path, true
		}
	}
	return "", false
}

func (a *App) credentialInfoOnly(p provider.Provider, acc config.AccountConfig) string {
	kind, _ := a.credentialInfo(p, acc)
	return kind
}

// historyFor returns the recorded series of one provider's window.
func (a *App) historyFor(providerID, account, windowID string) []history.Point {
	for _, s := range a.series {
		if s.Provider == providerID && s.Window == windowID && (s.Account == account || account == "" || s.Account == "") {
			return s.Points
		}
	}
	return nil
}

// overview counts what the dashboard shows in its header: how many providers
// this machine has, and how the last probe went. A provider with a credential
// on disk is one this machine has — counting it as 未配置 is what put "已找到
// 凭据" on a row under a header that said the opposite — so it counts as 正常
// until a probe says otherwise.
func (a *App) overview() (ok, warn, missing int) {
	for _, v := range a.providers() {
		if !v.Enabled {
			continue
		}
		switch {
		case v.Status == "error":
			warn++
		case v.Status == "ok" || v.hasSubscription():
			ok++
		default:
			missing++
		}
	}
	return
}

// --- actions ---------------------------------------------------------------

// Use makes an account the machine's own, as `leeway use` does.
func (a *App) Use(providerID, account string) {
	go func() {
		res, err := useAccount(providerID, account)
		a.apply(func() {
			if err != nil {
				log.Printf("窗口：切到 %s/%s 失败：%v", providerID, account, err)
				a.toast, _ = failureText("切换失败", err.Error())
				return
			}
			msg := "已把 " + res.Provider + " 切换到 " + displayAccount(res.Account)
			if res.SavedAs != "" {
				msg += "（原账号已备份为 " + res.SavedAs + "）"
			}
			if res.Unsaved {
				msg += "；原登录不在 leeway 记录里，切换后只剩目标账号"
			}
			if len(res.Targets) > 0 {
				msg += "（写入 " + strings.Join(res.Targets, "、") + "）"
			}
			a.toast = msg
			a.reloadConfig()
		})
	}()
}

// Logout removes an account's credentials.
func (a *App) Logout(providerID, account string) {
	go func() {
		msg, err := logoutAccount(providerID, account)
		a.apply(func() {
			if err != nil {
				log.Printf("窗口：移除 %s/%s 失败：%v", providerID, account, err)
				a.toast, _ = failureText("移除失败", err.Error())
				return
			}
			a.toast = msg
			a.reloadConfig()
		})
	}()
}

// SaveSettings persists the GUI's preferences and applies the theme.
func (a *App) SaveSettings() {
	a.settings.LastPage = a.page
	if err := a.settings.save(); err != nil {
		log.Printf("窗口：保存设置失败：%v", err)
		a.toast, _ = failureText("设置保存失败", err.Error())
		return
	}
	a.applyEnvironment()
	a.applyAppearance()
}

// ClearCache drops the cached browser-cookie imports, so the next probe reads
// the browsers again.
func (a *App) ClearCache() {
	go func() {
		err := auth.RemoveCookieCacheFile()
		a.apply(func() {
			if err != nil {
				log.Printf("窗口：重新读取浏览器登录状态失败：%v", err)
				a.toast, _ = failureText("重新读取失败", err.Error())
				return
			}
			a.toast = "已清除，下次会重新读取"
			a.Refresh(true)
		})
	}()
}

// Enable turns a provider on or off and probes again when it is turned on.
func (a *App) Enable(providerID string, on bool) {
	if err := setProviderEnabled(providerID, on); err != nil {
		log.Printf("窗口：保存 %s 的开关失败：%v", providerID, err)
		a.toast, _ = failureText("保存失败", err.Error())
		return
	}
	a.reloadConfig()
	if on {
		a.Refresh(true)
	} else {
		a.out = nil
		a.Refresh(true)
	}
}

// CopyJSON puts the current probe result on the clipboard, for an agent or a
// bug report. It never contains credentials.
func (a *App) CopyJSON() string {
	if a.out == nil {
		return "{}"
	}
	raw, err := json.MarshalIndent(a.out, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// navigate switches the shown page and remembers it.
func (a *App) navigate(page string) {
	a.page = page
	a.settings.LastPage = page
	_ = a.settings.save()
	a.say(pageName(page))
}

// now is what the window calls the time.
func (a *App) now() time.Time {
	if a.clock != nil {
		return a.clock()
	}
	return time.Now()
}

// say queues something for a screen reader to read out once. The view hands
// it to the frame it is building; a queue that only grows would repeat
// itself every frame.
func (a *App) say(text string) {
	if text != "" {
		a.spoken = append(a.spoken, text)
	}
}

// pageName is a page as the sidebar names it, for assistive technology.
func pageName(page string) string {
	switch {
	case page == "overview":
		return "总览"
	case page == "history":
		return "历史"
	case page == "about":
		return "关于"
	case strings.HasPrefix(page, "provider:"):
		return providerName(strings.TrimPrefix(page, "provider:"))
	}
	return page
}
