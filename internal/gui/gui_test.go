package gui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	_ "leeway/internal/provider/codex"
	_ "leeway/internal/provider/commandcode"
	_ "leeway/internal/provider/cursor"
	_ "leeway/internal/provider/deepseek"
	_ "leeway/internal/provider/grok"
	_ "leeway/internal/provider/kimi"
	_ "leeway/internal/provider/opencodego"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/history"
	"leeway/internal/model"
	"leeway/internal/provider"
)

// isolate points every leeway path at a temporary directory, so a test never
// reads or writes the user's real config, cache or sessions.
func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	// A home of its own: whether the machine running the tests is signed in
	// to Codex or Grok must not change what the window shows.
	t.Setenv("HOME", dir)
	t.Setenv("LEEWAY_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("LEEWAY_CACHE", filepath.Join(dir, "cache"))
	t.Setenv("LEEWAY_SESSIONS", filepath.Join(dir, "sessions"))
}

// testApp is an app with a probe result already in place: no network, no
// window, deterministic content.
// pinned is the instant every test renders at, so that the snapshot of a
// window changes when the window does and not when the clock moves. A test
// that needs "now" builds it from this.
var pinned = time.Date(2026, 10, 8, 13, 30, 0, 0, time.Local)

func testApp(t *testing.T) *App {
	t.Helper()
	isolate(t)
	if _, err := config.Load(); err != nil {
		t.Fatalf("config: %v", err)
	}
	a := NewApp()
	a.clock = func() time.Time { return pinned }
	a.settings.IntervalSec = 0
	a.settings.OnOpenFetch = false
	// A configured key per provider the fake answer calls ok, so the machine
	// the fixture describes is one that could exist: the skeleton reads these
	// credentials to decide how many cards the answer will have, and an answer
	// that reports a provider ok without one is a state no probe can produce.
	for _, id := range []string{"deepseek", "kimi"} {
		if err := loginAPIKey(id, "", "sk-test"); err != nil {
			t.Fatalf("loginAPIKey(%s): %v", id, err)
		}
	}
	// Cursor has no API key: its credential is a private session file, written
	// through the same call the CLI's `leeway use` makes.
	if err := auth.WriteCursorAuth("default", auth.CursorAuth{AccessToken: "test"}); err != nil {
		t.Fatalf("WriteCursorAuth: %v", err)
	}
	a.reloadConfig()
	a.out = fakeOutput(pinned)
	a.lastAt = a.out.GeneratedAt
	return a
}

// freshApp is a machine nothing has been set up on: no credential anywhere,
// which is the only state the first-run guide is actually the answer to.
func freshApp(t *testing.T) *App {
	t.Helper()
	isolate(t)
	if _, err := config.Load(); err != nil {
		t.Fatalf("config: %v", err)
	}
	a := NewApp()
	a.clock = func() time.Time { return pinned }
	a.settings.IntervalSec = 0
	a.settings.OnOpenFetch = false
	a.reloadConfig()
	return a
}

// fakeOutput mixes the shapes the UI has to show: percent windows, a balance
// window with cost, and a provider that failed.
func fakeOutput(now time.Time) *model.Output {
	pct := 42.0
	weekly := 78.0
	balance := 12.5
	spend := 3.25
	limit := 20.0
	cost := 3.25
	reset := now.Add(52 * time.Hour)
	return &model.Output{
		Version:     1,
		GeneratedAt: now,
		Providers: []model.ProviderResult{
			{
				ID:          "deepseek",
				DisplayName: "DeepSeek",
				Status:      "ok",
				Accounts: []model.AccountResult{{
					Name:   "default",
					Status: "ok",
					Snapshot: &model.Snapshot{
						UpdatedAt: now,
						Identity:  &model.Identity{Account: "work@example.com", Plan: "Pro"},
						Windows: []model.RateWindow{
							{ID: "primary", Title: "Balance", Used: &balance, Currency: "CNY"},
							{ID: "primary-percent", Title: "Weekly", UsedPercent: &weekly, ResetsAt: &reset},
						},
						Cost: &model.Cost{Period: "daily", Currency: "CNY", Amount: &cost},
					},
				}},
			},
			{
				ID:          "grok",
				DisplayName: "Grok",
				Status:      "error",
				Error:       "cookie 已过期，重新登录",
				Accounts:    []model.AccountResult{{Name: "default", Status: "error", Error: "cookie 已过期，重新登录"}},
			},
			{
				ID:          "kimi",
				DisplayName: "Kimi",
				Status:      "ok",
				Accounts: []model.AccountResult{{
					Name:   "work",
					Status: "ok",
					Snapshot: &model.Snapshot{
						UpdatedAt: now,
						Windows:   []model.RateWindow{{ID: "primary", UsedPercent: &pct}},
					},
				}},
			},
			{
				ID:          "cursor",
				DisplayName: "Cursor",
				Status:      "ok",
				Accounts: []model.AccountResult{{
					Name:   "default",
					Status: "ok",
					Snapshot: &model.Snapshot{
						UpdatedAt: now,
						Windows: []model.RateWindow{{
							ID: "included", Title: "Included",
							Used: &spend, Limit: &limit, Currency: "USD",
						}},
					},
				}},
			},
		},
	}
}

// snapshot writes the tester's frame under LEEWAY_GUI_SNAPSHOT, when a review
// of the rendering is wanted.
func snapshot(t *testing.T, tester *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("LEEWAY_GUI_SNAPSHOT")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("snapshot dir: %v", err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, tester.Image()); err != nil {
		t.Fatalf("snapshot encode: %v", err)
	}
}

// snapshotStill captures a window whose toast is still fading in: a window in
// the background holds its toasts still, drawn as they end up, so the image is
// the window at rest and not wherever the engine's clock happened to be.
func snapshotStill(t *testing.T, tester *ui.Tester, name string) {
	tester.SetFocused(false)
	tester.Frame()
	snapshot(t, tester, name)
	tester.SetFocused(true)
}

// --- helpers ---------------------------------------------------------------

func TestSettingsRoundTrip(t *testing.T) {
	isolate(t)
	s := defaultSettings()
	s.IntervalSec = 300
	s.Theme = "dark"
	s.HistoryN = 48
	s.LastPage = "provider:kimi"
	if err := s.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := loadSettings()
	if got.IntervalSec != 300 || got.Theme != "dark" || got.HistoryN != 48 || got.LastPage != "provider:kimi" {
		t.Fatalf("loadSettings = %+v", got)
	}
}

func TestOverridesDropInheritedEnv(t *testing.T) {
	t.Setenv("GROK_HOME", "/should/not/survive")
	env := overrideEnv("GROK_HOME", "/tmp/grok-home")
	seen := 0
	for _, e := range env {
		if e == "GROK_HOME=/tmp/grok-home" {
			seen++
		}
		if e == "GROK_HOME=/should/not/survive" {
			t.Fatalf("inherited GROK_HOME survived: %q", e)
		}
	}
	if seen != 1 {
		t.Fatalf("GROK_HOME set %d times, want 1", seen)
	}
}

func TestUnderDirKeepsDeletesInside(t *testing.T) {
	root := "/home/u/.config/leeway/sessions"
	if !underDir(root, root+"/kimi-work") {
		t.Fatal("a child of the sessions directory must be allowed")
	}
	if underDir(root, "/home/u/.config/leeway") || underDir(root, "/home/u") {
		t.Fatal("paths above the sessions directory must be refused")
	}
	if underDir("", "/home/u/.config/leeway/sessions/x") {
		t.Fatal("an empty root must be refused")
	}
}

func TestWindowTitlesComeFromDeclarations(t *testing.T) {
	if got := windowTitle("deepseek", "primary", ""); got != "Balance" {
		t.Fatalf("deepseek primary = %q, want the declared Balance", got)
	}
	if got := windowTitle("nope", "ondemand", ""); got != "On-demand" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestWindowFractionAndDelta(t *testing.T) {
	pct := 25.0
	used, limit := 10.0, 40.0
	f, ok := windowFraction(model.RateWindow{UsedPercent: &pct})
	if !ok || f != 0.25 {
		t.Fatalf("percent fraction = %v %v", f, ok)
	}
	f, ok = windowFraction(model.RateWindow{Used: &used, Limit: &limit})
	if !ok || f != 0.25 {
		t.Fatalf("money fraction = %v %v", f, ok)
	}
	if _, ok := windowFraction(model.RateWindow{}); ok {
		t.Fatal("a window with no numbers has no bar")
	}
	delta := 4.5
	if got := deltaText(nil, model.RateWindow{Delta: &model.WindowDelta{UsedPercent: &delta}}); got != "比上次 +4%" && got != "比上次 +5%" {
		t.Fatalf("delta = %q", got)
	}
	if got := deltaText(nil, model.RateWindow{Delta: &model.WindowDelta{Reset: true}}); got != "" {
		t.Fatalf("a reset is not reported as a change of usage: %q", got)
	}
}

// --- credentials -----------------------------------------------------------

func TestLoginStoresKeysWhereTheCLIReadsThem(t *testing.T) {
	isolate(t)
	if err := loginAPIKey("deepseek", "", "sk-provider"); err != nil {
		t.Fatalf("loginAPIKey: %v", err)
	}
	if err := loginAPIKey("deepseek", "work", "sk-account"); err != nil {
		t.Fatalf("loginAPIKey named: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if got := cfg.APIKey("deepseek"); got != "sk-provider" {
		t.Fatalf("provider key = %q", got)
	}
	acc, ok := cfg.Account("deepseek", "work")
	if !ok || acc.APIKey != "sk-account" {
		t.Fatalf("account = %+v (%v)", acc, ok)
	}
}

func TestManualCookieRoundTrip(t *testing.T) {
	isolate(t)
	if err := saveManualCookie("commandcode", "", "session=abc; other=def"); err != nil {
		t.Fatalf("saveManualCookie: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if got := cfg.ManualHeader("commandcode", "default"); got == "" {
		t.Fatal("the manual cookie header did not reach the config")
	}
}

func TestImportKimiSessionSnapshotsTheFile(t *testing.T) {
	isolate(t)
	src := filepath.Join(t.TempDir(), "kimi-code.json")
	if err := os.WriteFile(src, []byte(`{"access_token":"a","refresh_token":"r"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := importAuthFile("kimi", "work", src); err != nil {
		t.Fatalf("importAuthFile: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	acc, ok := cfg.Account("kimi", "work")
	if !ok || acc.AuthFile == "" {
		t.Fatalf("account = %+v (%v)", acc, ok)
	}
	home, err := isolatedHome("kimi", "work")
	if err != nil {
		t.Fatalf("isolatedHome: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "credentials", "kimi-code.json")); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
	// A file that is not a session is refused rather than stored.
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"nope":true}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := importAuthFile("kimi", "work", bad); err == nil {
		t.Fatal("a file without tokens must be refused")
	}
}

func TestLogoutRemovesThePrivateHomeOnly(t *testing.T) {
	isolate(t)
	home, err := grokHome("work")
	if err != nil {
		t.Fatalf("grokHome: %v", err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := logoutAccount("grok", "work"); err != nil {
		t.Fatalf("logoutAccount: %v", err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("the private home survived: %v", err)
	}
	root, err := auth.SessionDir()
	if err != nil {
		t.Fatalf("SessionDir: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("the sessions directory itself must stay: %v", err)
	}
}

func TestEnableAndAccountsReachTheConfig(t *testing.T) {
	isolate(t)
	if err := setProviderEnabled("kimi", false); err != nil {
		t.Fatalf("setProviderEnabled: %v", err)
	}
	if err := addAccount("kimi", "home"); err != nil {
		t.Fatalf("addAccount: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.IsEnabled("kimi") {
		t.Fatal("kimi should be disabled")
	}
	if _, ok := cfg.Account("kimi", "home"); !ok {
		t.Fatal("the new account is missing")
	}
}

// --- rendering -------------------------------------------------------------

func TestOverviewRendersEveryProvider(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "overview")

	for _, want := range []string{"总览", "DeepSeek", "Kimi Code", "Grok", "Codex", "¥12.50", "78%", "42%"} {
		if !tester.HasText(want) {
			t.Errorf("the overview does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if !tester.HasText("cookie 已过期") {
		t.Error("the failing provider's error is not shown")
	}
	// The lead card leads with the fullest window, and still shows the rest.
	if !tester.HasText("需要处理") {
		t.Error("what needs a hand has no list of its own")
	}
	if !tester.HasText("共 $20.00") {
		t.Error("a window out of a limit should say what the limit is")
	}
	// The narrowest window the app allows still lays out.
	narrow := ui.NewTester(a.view, 780, 560)
	snapshot(t, narrow, "overview-narrow")
	if !narrow.HasText("总览") || !narrow.HasText("DeepSeek") {
		t.Error("the overview does not survive the smallest window")
	}
	// Hovering a card must not break the frame it repaints.
	narrow.Move(600, 300)
	snapshot(t, narrow, "overview-hover")
	if !narrow.HasText("总览") {
		t.Error("the overview does not survive the pointer")
	}
	// And the dark theme is a first-class one, not an afterthought.
	dark := ui.NewTester(a.view, mainWidth, mainHeight)
	dark.SetDark(true)
	snapshot(t, dark, "overview-dark")
	if !dark.HasText("DeepSeek") {
		t.Error("the dark overview does not render")
	}
}

func TestConfiguredProviderSaysSoBeforeAProbe(t *testing.T) {
	a := testApp(t)
	// No probe result at all: a provider whose key the CLI put in the config
	// must say so rather than claim it has none.
	a.out = nil
	a.navigate("provider:deepseek")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	if !tester.HasText("登录信息来自 配置里的 API 密钥") {
		t.Errorf("a configured provider must say where its credential is; texts=%v", tester.Texts())
	}
	if tester.HasText("还没有 API 密钥") {
		t.Error("the page still claims there is no key")
	}
}

func TestCardOpensTheProviderPage(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	if err := tester.Click("查看 DeepSeek"); err != nil {
		t.Fatalf("click: %v", err)
	}
	if a.page != "provider:deepseek" {
		t.Fatalf("page = %q", a.page)
	}
	if !tester.HasText("登录信息来源") {
		t.Errorf("the provider page did not render; texts=%v", tester.Texts())
	}
	snapshot(t, tester, "provider")
}

// The whole card is the target, not only the chevron in its corner: a card
// that lightens under the pointer is a card that answers a press, wherever it
// lands on the plate.
func TestTheWholeCardIsTheTarget(t *testing.T) {
	a := testApp(t)
	// With history the card carries a chart, and the chart takes the pointer
	// for its own keys — so this is the fixture that catches a chart sitting on
	// top of the card's own click.
	withHistory(a)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	card, ok := tester.Find("查看 DeepSeek")
	if !ok {
		t.Fatal("the lead card has no accessible name")
	}
	if card.W < 600 || card.H < 150 {
		t.Errorf("the target is not the whole card: %.0fx%.0f", card.W, card.H)
	}
	// The plate answers the hand as well as the pointer: held down, the whole
	// card takes the wash, and letting go opens the provider.
	x, y := card.X+card.W/2, card.Y+card.H/2
	tester.Press(x, y)
	snapshot(t, tester, "overview-pressed")
	tester.Release(x, y)
	if a.page != "provider:deepseek" {
		t.Fatalf("a press on the card body did not open it; page = %q", a.page)
	}
}

// A card that answers the pointer but not the keyboard is half a control: the
// overview is the window's front door, and it has to open from the keyboard
// alone: focusable is not enough, the card has to be clickable as well. The
// card is rendered on its own, because a window with a refresh button in it
// would answer the first Enter with a real probe.
func TestTheCardOpensFromTheKeyboard(t *testing.T) {
	a := &App{}
	opened := false
	view := func(c *ui.Context) {
		p := a.theme(c)
		a.openCard(c, p, "查看 DeepSeek", func() { opened = true }, shell(c, p, func() {
			ui.Text(c, "DeepSeek").FontSize(sizeValue).TextColor(p.t.Text)
		}))
	}
	tester := ui.NewTester(view, 400, 200)
	tester.Key(0, ui.KeyTab)
	tester.Key(0, ui.KeyEnter)
	tester.Frame()
	if !opened {
		t.Error("the card did not open on Enter: it answers the pointer only")
	}
	if r, ok := tester.Find("查看 DeepSeek"); !ok || r.W <= 0 {
		t.Errorf("the card has no accessible name or no box: %+v ok=%v", r, ok)
	}
}

// Four subscriptions of one provider are read side by side, and the whole
// matrix has to fit the window: an account per card would push the last one
// below the fold, which is the question this layout answers.
func manyAccountApp(t *testing.T) *App {
	a := testApp(t)
	accounts := []struct{ name, email, plan string }{
		{"alpha", "alpha@example.com", "Go"},
		{"work", "work@example.com", "Go"},
		{"side", "side@example.com", "Go"},
		{"delta", "delta@example.com", "Go"},
	}
	for _, acc := range accounts {
		if err := loginAPIKey("opencode-go", acc.name, "oc_sk-test"); err != nil {
			t.Fatalf("loginAPIKey: %v", err)
		}
		if _, err := config.Load(); err != nil {
			t.Fatalf("config: %v", err)
		}
	}
	a.reloadConfig()
	var results []model.AccountResult
	for i, acc := range accounts {
		pct := float64(20 * (i + 1))
		reset := a.lastAt.Add(time.Duration(i+1) * 24 * time.Hour)
		results = append(results, model.AccountResult{
			Name: acc.name, Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: a.lastAt,
				Identity:  &model.Identity{Account: acc.email, Plan: acc.plan},
				Windows: []model.RateWindow{
					{ID: "5h", Title: "5-hour", UsedPercent: &pct, ResetsAt: &reset},
					{ID: "weekly", Title: "Weekly", UsedPercent: &pct, ResetsAt: &reset},
					{ID: "monthly", Title: "Monthly", UsedPercent: &pct, ResetsAt: &reset},
				},
			},
		})
	}
	a.out = &model.Output{Version: 1, GeneratedAt: a.lastAt, Providers: []model.ProviderResult{{
		ID: "opencode-go", DisplayName: "OpenCode Go", Status: "ok", UpdatedAt: a.lastAt, Accounts: results,
	}}}
	return a
}

// Four subscriptions of one provider are read side by side, and the whole
// matrix has to fit the window: a card per account would push the last one
// below the fold, which is the question this layout answers.
func TestManyAccountsOfOneProviderFitOneScreen(t *testing.T) {
	a := manyAccountApp(t)
	a.navigate("provider:opencode-go")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	for _, want := range []string{
		"alpha", "alpha@example.com", "work", "work@example.com",
		"side", "side@example.com", "delta", "delta@example.com",
		"5-hour", "Weekly", "Monthly",
	} {
		if !tester.HasText(want) {
			t.Errorf("the matrix does not show %q; texts=%v", want, tester.Texts())
		}
	}
	// Every account and the last row of the last block have to be inside the
	// scroll's viewport. Texts() also lists text that was clipped away, so the
	// check has to be on the rectangles that were really drawn.
	viewport := float32(mainHeight) - titleBarMin
	for _, want := range []string{"alpha", "delta@example.com", "Monthly"} {
		if r, ok := tester.Find(want); !ok || r.H <= 0 || r.Y+r.H > viewport {
			t.Errorf("%q 不在首屏内：%+v ok=%v viewport=%.0f", want, r, ok, viewport)
		}
	}
	last := ""
	for _, txt := range tester.Texts() {
		if strings.HasSuffix(txt, "后重置") {
			last = txt
		}
	}
	if last == "" {
		t.Fatal("no reset label was drawn at all")
	}
	if r, ok := tester.Find(last); !ok || r.Y+r.H > viewport {
		t.Errorf("矩阵最后一行被折出窗口：%q %+v ok=%v viewport=%.0f", last, r, ok, viewport)
	}
	snapshot(t, tester, "provider-many")
	dark := ui.NewTester(a.view, mainWidth, mainHeight)
	dark.SetDark(true)
	snapshot(t, dark, "provider-many-dark")
}

func TestProviderPageShowsAccountsAndWindows(t *testing.T) {
	a := testApp(t)
	a.navigate("provider:deepseek")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	for _, want := range []string{"DeepSeek", "default", "work@example.com", "Balance", "Weekly", "Pro", "设置密钥", "刷新", "配置里的 API 密钥"} {
		if !tester.HasText(want) {
			t.Errorf("the provider page does not show %q", want)
		}
	}
	if tester.HasText("sk-test") {
		t.Error("the provider page must never show a key's value")
	}
	dark := ui.NewTester(a.view, mainWidth, mainHeight)
	dark.SetDark(true)
	snapshot(t, dark, "provider-dark")
	if !dark.HasText("Balance") {
		t.Error("the dark provider page does not render")
	}
}

func TestHistoryPageRendersSeries(t *testing.T) {
	a := testApp(t)
	a.navigate("history")
	a.series = []history.Series{{
		Provider: "deepseek",
		Account:  "default",
		Window:   "primary",
		Points: []history.Point{
			{T: a.now().Add(-2 * time.Hour), UsedPercent: ptrTo(10.0)},
			{T: a.now().Add(-time.Hour), UsedPercent: ptrTo(20.0)},
			{T: a.now(), UsedPercent: ptrTo(30.0)},
		},
	}}
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	if !tester.HasText("历史") || !tester.HasText("Balance") || !tester.HasText("30%") {
		t.Errorf("the history page is missing its content; texts=%v", tester.Texts())
	}
	snapshot(t, tester, "history")
}

func TestSettingsWindowRendersItsTabs(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.settingsView, settingsWidth, settingsHght)
	snapshot(t, tester, "settings")
	for _, want := range []string{"设置", "自动刷新", "服务商", "数据", "查询超时", "主题"} {
		if !tester.HasText(want) {
			t.Errorf("the settings window does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if err := tester.Click("数据"); err != nil {
		t.Fatalf("click 数据: %v", err)
	}
	snapshot(t, tester, "settings-data")
	if !tester.HasText("重新读取浏览器登录状态") {
		t.Errorf("the data tab did not open; texts=%v", tester.Texts())
	}
	if err := tester.Click("服务商"); err != nil {
		t.Fatalf("click Provider: %v", err)
	}
	snapshot(t, tester, "settings-providers")
	for _, want := range []string{"全部服务商", "DeepSeek", "详情"} {
		if !tester.HasText(want) {
			t.Errorf("the provider tab does not show %q", want)
		}
	}
}

func TestAppearancePickerOffersEveryTheme(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.settingsView, settingsWidth, settingsHght)
	// The swatches are drawn, labelled and hit-testable; the click itself
	// saves, which asks MyGo for a theme the tester has no main loop for.
	for _, name := range []string{"跟随系统", "浅色", "深色"} {
		if !tester.HasText(name) {
			t.Errorf("the appearance picker does not offer %q; texts=%v", name, tester.Texts())
		}
		if _, ok := tester.Find("主题：" + name); !ok {
			t.Errorf("the %q swatch is not clickable", name)
		}
	}
}

func TestAboutPageNamesThePaths(t *testing.T) {
	a := testApp(t)
	a.navigate("about")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "about")
	for _, want := range []string{"关于", "版本", "配置", "缓存", "命令", "leeway usage --json"} {
		if !tester.HasText(want) {
			t.Errorf("the about page does not show %q", want)
		}
	}
}

func TestLoginSheetRendersAndKeepsTheTypedKey(t *testing.T) {
	a := testApp(t)
	a.openAPIKey("deepseek", "work")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "login-key")
	for _, want := range []string{"粘贴 API 密钥", "保存", "取消"} {
		if !tester.HasText(want) {
			t.Errorf("the sheet does not show %q; texts=%v", want, tester.Texts())
		}
	}
	tester.Type("sk-typed")
	if a.prompt.key != "sk-typed" {
		t.Fatalf("typed key = %q", a.prompt.key)
	}
	if a.prompt.reveal {
		t.Fatal("the key starts masked")
	}
	if err := tester.Click("显示出来"); err != nil {
		t.Fatalf("click 显示明文: %v", err)
	}
	if !a.prompt.reveal {
		t.Error("the reveal checkbox is not bound to the sheet")
	}
}

// A row's right edge is where its action is. 详情 used to sit outboard of the
// button that gets the provider working, at the same weight: the row read as
// two actions with the quiet one in the loud position.
func TestTheFirstRunRowEndsWithItsAction(t *testing.T) {
	a := freshApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	detail, ok := tester.Find("详情")
	if !ok {
		t.Fatalf("the guide has no 详情; texts=%v", tester.Texts())
	}
	// The first row is Codex's, whose action is the official tool.
	action, ok := tester.Find("用官方工具登录")
	if !ok {
		t.Fatalf("the guide has no action; texts=%v", tester.Texts())
	}
	if action.X+action.W <= detail.X {
		t.Errorf("the action ends at x=%.0f and 详情 starts at x=%.0f: the quiet control is outboard of the action",
			action.X+action.W, detail.X)
	}
	// And it is a control the reader can see. A ButtonBase with only a Label
	// publishes the name and draws nothing: this one was an invisible button
	// until the ink below was checked, and the name in the frame was enough to
	// pass every test that only asked whether it was there.
	if inkBottom(tester.Image(), detail) < 0 {
		t.Error("详情 is in the frame and has no ink: it is a name, not a control")
	}
}

func TestFirstRunTeachesInsteadOfBlaming(t *testing.T) {
	// Nothing configured and nothing answered: the window has to say what to
	// do about it. This is the state a fresh install is in, and the guide is
	// what the page lands on, not a placeholder for an answer.
	a := freshApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "overview-first-run")
	// A row per provider, each with the action that provider needs first,
	// and the machine's own state rather than a copy of the manual.
	for _, want := range []string{
		"开始使用", "粘贴 API 密钥", "用官方工具登录", "详情", "配好一个就能用：点「刷新」或按 ⌘R。命令行同样可用：leeway usage",
	} {
		if !tester.HasText(want) {
			t.Errorf("the first run does not show %q; texts=%v", want, tester.Texts())
		}
	}
	// The guide is the only list of providers on a first run: the card that
	// lists what needs a hand would be the same seven rows twice.
	if tester.HasText("需要处理") {
		t.Error("a first run lists its providers twice")
	}
	// The guide carries every provider's own icon, so it is also the screen
	// that shows whether they survive the dark theme.
	dark := ui.NewTester(a.view, mainWidth, mainHeight)
	dark.SetDark(true)
	snapshot(t, dark, "overview-first-run-dark")
	// A provider with no session on this machine must not claim one.
	for _, unwanted := range []string{"登录信息来自 Codex CLI 登录文件", "登录信息来自 Cursor 私有登录文件"} {
		if tester.HasText(unwanted) {
			t.Errorf("the first run claims %q without checking; texts=%v", unwanted, tester.Texts())
		}
	}
	if !tester.HasText("还没有登录信息。用官方 codex 登录，或导入 ~/.codex/auth.json。") {
		t.Errorf("the codex row does not say what is missing; texts=%v", tester.Texts())
	}
}

// The guide reads this machine: a session that is there is offered as ready,
// and one that is not is never claimed.
func TestFirstRunSeesTheSessionsThisMachineHas(t *testing.T) {
	a := freshApp(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"tokens":{"access_token":"test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "overview-first-run-ready")
	if !tester.HasText("登录信息来自 Codex CLI 登录文件；刷新一次就能看到用量。") {
		t.Errorf("a Codex session on this machine is not offered; texts=%v", tester.Texts())
	}
	if !tester.HasText("刷新") || !tester.HasText("已找到登录信息") {
		t.Errorf("the ready Codex row is not marked as found; texts=%v", tester.Texts())
	}
}

// The first probe takes a moment. A window that spends that moment showing a
// guide it is about to take away reads as a bug, so it shows the shape the
// answer will land in, and only until the answer lands.
func TestTheFirstProbeShowsTheShapeOfTheAnswer(t *testing.T) {
	a := testApp(t)
	a.out = nil
	a.fetching = true
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "overview-probing")
	if !tester.HasText("正在查询…") {
		t.Errorf("the window does not say what it is doing; texts=%v", tester.Texts())
	}
	if tester.HasText("开始使用") {
		t.Error("the first-run guide is shown while the first probe is still out")
	}

	// The probe lands: the page is the real one, and the skeleton is gone.
	a.fetching = false
	a.out = fakeOutput(a.now())
	a.lastAt = a.out.GeneratedAt
	landed := ui.NewTester(a.view, mainWidth, mainHeight)
	if landed.HasText("正在查询…") {
		t.Error("the skeleton outlived the probe")
	}
	if !landed.HasText("78%") {
		t.Errorf("the answer did not replace the skeleton; texts=%v", landed.Texts())
	}

	// The skeleton predicts where the answer lands, and the readings are the
	// part of the card the spacers place. With one spacer for both, the two
	// placeholders sat together at the card's right edge and the first reading
	// jumped 219px left when the numbers arrived. This measures the two frames
	// against each other: the row with three bars is the readings' row — the
	// number and the two readings — and the last two of its runs are where the
	// answer's readings have to appear.
	row := -1
	for y := 120; y < mainHeight-10; y++ {
		if runs := barRuns(tester.Image(), y); len(runs) == 3 {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatal("the skeleton has no row with a number and two readings")
	}
	// The value bars sit under the caption bars and are wider, so the last
	// row with three runs is the one the readings' left edges come from.
	var placed [][2]int
	for y := row; y < row+30; y++ {
		if runs := barRuns(tester.Image(), y); len(runs) == 3 {
			placed = runs
		}
	}
	if len(placed) != 3 {
		t.Fatalf("the skeleton's readings row measured %v", placed)
	}
	for i, want := range []string{"2 天 4 小时", "¥12.50"} {
		box, ok := landed.Find(want)
		if !ok {
			t.Fatalf("the landed page has no %q; texts=%v", want, landed.Texts())
		}
		got := float32(placed[i+1][0])
		if d := got - box.X; d < -20 || d > 20 {
			t.Errorf("the skeleton puts %q at x=%.0f and the answer puts it at x=%.0f: %.0f off", want, got, box.X, d)
		}
	}

	// A machine with nothing on it has no number coming: the answer is the
	// guide, so the window draws the guide rather than a skeleton for a card
	// that will never arrive.
	fresh := freshApp(t)
	fresh.fetching = true
	waiting := ui.NewTester(fresh.view, mainWidth, mainHeight)
	snapshot(t, waiting, "overview-probing-fresh")
	if !waiting.HasText("开始使用") {
		t.Errorf("a machine with nothing configured waits behind a skeleton for a number that cannot come; texts=%v", waiting.Texts())
	}
	if waiting.HasText("需要处理") {
		t.Error("a machine with nothing configured is promised a 需要处理 card it will not get")
	}
}

// A quota that came back is the other half of a quota that ran out: it is
// said in the same place, at the same weight, wherever the window is read.
func TestAResetIsSaidWhereverTheWindowIsRead(t *testing.T) {
	a := testApp(t)
	now := a.now()
	// 42% and then nothing: the provider handed the window back.
	var pts []history.Point
	for i, v := range []float64{20, 42, 0} {
		val := v
		pts = append(pts, history.Point{T: now.Add(time.Duration(i-2) * 24 * time.Minute), UsedPercent: &val})
	}
	pts[len(pts)-1].Reset = true
	a.series = []history.Series{{Provider: "deepseek", Account: "default", Window: "primary-percent", Points: pts}}
	setWindowPercent(a, "deepseek", "primary-percent", 0)
	const note = "刚重置：42% → 0%，额度已恢复"

	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "history-reset")
	if !saysText(tester, note) {
		t.Errorf("the history row does not say the window came back; texts=%v", tester.Texts())
	}

	a.navigate("overview")
	over := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, over, "overview-reset")
	if !saysText(over, note) {
		t.Errorf("the overview does not say the window came back; texts=%v", over.Texts())
	}

	a.navigate("provider:deepseek")
	detail := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, detail, "provider-reset")
	if !saysText(detail, note) {
		t.Errorf("the provider page does not say the window came back; texts=%v", detail.Texts())
	}
}

// A reset the provider made by moving the window's end is not a drop, and the
// two numbers would say nothing: the sentence has to stand without them.
func TestAResetThatOnlyMovedTheClockIsNotGivenNumbers(t *testing.T) {
	a := testApp(t)
	now := a.now()
	var pts []history.Point
	for i, v := range []float64{20, 42, 45} {
		val := v
		pts = append(pts, history.Point{T: now.Add(time.Duration(i-2) * 24 * time.Minute), UsedPercent: &val})
	}
	pts[len(pts)-1].Reset = true
	a.series = []history.Series{{Provider: "deepseek", Account: "default", Window: "primary-percent", Points: pts}}
	setWindowPercent(a, "deepseek", "primary-percent", 45)

	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	if !saysText(tester, "刚重置，额度已恢复") {
		t.Errorf("a reset with no drop is not stated; texts=%v", tester.Texts())
	}
	if saysText(tester, "42% → 45%") {
		t.Error("a reset that was not a drop was given numbers")
	}
}

// setWindowPercent points a provider's probed window at a value, so a fixture
// can agree with the history it is shown beside.
func setWindowPercent(a *App, providerID, windowID string, pct float64) {
	for pi := range a.out.Providers {
		pr := &a.out.Providers[pi]
		if pr.ID != providerID {
			continue
		}
		for ai := range pr.Accounts {
			snap := pr.Accounts[ai].Snapshot
			if snap == nil {
				continue
			}
			for wi := range snap.Windows {
				if w := &snap.Windows[wi]; w.ID == windowID {
					w.UsedPercent = &pct
				}
			}
		}
	}
}

// Every provider is shown by its own icon. A missing or unreadable asset would
// fall back to a letter we drew, silently, so it is checked here as well.
func TestEveryProviderHasItsOwnIcon(t *testing.T) {
	for _, p := range provider.All() {
		if providerMark(p.ID()) == nil {
			t.Errorf("%s has no icon of its own", p.ID())
		}
	}
	if providerMark("nobody") != nil {
		t.Error("a provider leeway does not have claims an icon")
	}
}

// The sidebar is the way around the window, so it lists the subscriptions this
// machine has and nothing else: a provider nobody has set up is a row in the
// first-run guide, not a page to keep walking past.
func TestTheSidebarListsOnlyWhatThisMachineHas(t *testing.T) {
	a := testApp(t)
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	// The sidebar is the left column; the page behind it names providers too.
	const sidebarWidth = 240.0
	inSidebar := func(name string) bool {
		r, ok := tester.Find(name)
		return ok && r.X < sidebarWidth
	}
	for _, want := range []string{"DeepSeek", "Cursor", "Kimi Code", "Grok"} {
		if !inSidebar(want) {
			t.Errorf("%s has a subscription here and is not in the sidebar", want)
		}
	}
	for _, unwanted := range []string{"Codex", "Command Code", "OpenCode Go"} {
		if inSidebar(unwanted) {
			t.Errorf("%s is not set up here and is in the sidebar", unwanted)
		}
	}
	if !tester.HasText("需要处理") {
		t.Error("the unconfigured providers are not offered anywhere")
	}
}

// Every provider with an answer says in the sidebar how much of its tightest
// window is left, so the comparison across providers is readable without
// opening any of them: the page itself shows one card per provider, and at the
// default window that puts the last of them below the fold.
func TestTheSidebarSaysWhatEachProviderHasLeft(t *testing.T) {
	a := testApp(t)
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	left := 0
	first := ""
	for _, text := range tester.Texts() {
		if strings.HasPrefix(text, "剩 ") {
			left++
			if first == "" {
				first = text
			}
		}
	}
	ready := 0
	for _, v := range a.providers() {
		if v.Enabled && v.Status == "ok" && hasWindows(v) {
			ready++
		}
	}
	if left != ready {
		t.Errorf("%d providers have an answer and %d say what is left of it", ready, left)
	}
	// And it is in the first viewport, which is the whole reason it is in the
	// sidebar: a number the reader has to scroll to is not a comparison.
	box, ok := tester.Find(first)
	if !ok {
		t.Fatalf("%q was listed and cannot be found", first)
	}
	if box.X >= sidebarWidth || box.Y+box.H >= mainHeight {
		t.Errorf("%q sits at %.0f,%.0f, outside the sidebar's first screen", first, box.X, box.Y)
	}
}

// A window with nothing configured says so where the list would be.
func TestASidebarWithNothingConfiguredSaysSo(t *testing.T) {
	a := freshApp(t)
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	r, ok := tester.Find("配置好的服务商会显示在这里")
	if !ok || r.X >= 240 {
		t.Errorf("an empty sidebar does not explain itself; texts=%v", tester.Texts())
	}
}

// A long page has to be scrollable to its end: the last card is the one that
// carries the credential paths, and a scroll that stops short of it hides the
// answer behind a scrollbar that looks like it is already at the bottom.
func TestTheEndOfALongPageIsReachable(t *testing.T) {
	a := testApp(t)
	var windows []model.RateWindow
	for i := 0; i < 10; i++ {
		pct := float64(10 * (i + 1))
		reset := a.lastAt.Add(time.Duration(i+1) * time.Hour)
		windows = append(windows, model.RateWindow{
			ID: fmt.Sprintf("w%d", i), Title: fmt.Sprintf("Window %d", i),
			UsedPercent: &pct, ResetsAt: &reset,
		})
	}
	a.out = &model.Output{Version: 1, GeneratedAt: a.lastAt, Providers: []model.ProviderResult{{
		ID: "deepseek", DisplayName: "DeepSeek", Status: "ok", UpdatedAt: a.lastAt,
		Accounts: []model.AccountResult{{Name: "default", Status: "ok", Snapshot: &model.Snapshot{
			UpdatedAt: a.lastAt, Identity: &model.Identity{Account: "work@example.com", Plan: "Pro"},
			Windows: windows}}},
	}}}
	a.navigate("provider:deepseek")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	for i := 0; i < 80; i++ {
		tester.Scroll(mainWidth-40, mainHeight/2, 0, 200)
		tester.Frame()
	}
	// The page's own bottom padding, and the card's, have to be below the last
	// line of it once the scroll is at the end.
	r, ok := tester.Find("配置文件")
	if !ok {
		t.Fatalf("the last card is not on the page; texts=%v", tester.Texts())
	}
	// The scroll's viewport ends at the window's bottom, and the page's own
	// bottom padding sits inside it: at the end of the page the last line has
	// to clear that padding rather than end inside it. Tester.Find reports
	// window coordinates, so the window's bottom is the viewport's, and the
	// last line is inside the card the page ends with.
	viewport := float32(mainHeight) - pageBottomPad
	if bottom := r.Y + r.H; bottom > viewport-8 {
		t.Errorf("the last line ends at %.0f, inside the scroll's own bottom padding (the padding starts at %.0f)", bottom, viewport)
	}
}

// saysText reports whether the window says a sentence, which the layout may
// have wrapped across lines.
func saysText(tester *ui.Tester, want string) bool {
	return strings.Contains(strings.Join(tester.Texts(), ""), want)
}

// The change chip is the row's own account of the last move, and the caption
// under it is the record the row is drawn from: between them a row says where
// it went and over how many observations, with no projection under it.
func TestTheHistoryRowShowsTheLastMove(t *testing.T) {
	a := testApp(t)
	now := a.now()
	var pts []history.Point
	for i, v := range []float64{18, 19, 20} {
		val := v
		pts = append(pts, history.Point{T: now.Add(time.Duration(i-2) * time.Hour), UsedPercent: &val})
	}
	a.series = []history.Series{{Provider: "deepseek", Account: "default", Window: "primary-percent", Points: pts}}
	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	if !tester.HasText("▲ +1%") {
		t.Errorf("the change chip is missing; texts=%v", tester.Texts())
	}
	if !saysText(tester, "18% → 20%") {
		t.Errorf("the row does not say the range it moved over; texts=%v", tester.Texts())
	}
}

// withHistory gives an app a few probes to draw a trend from.
func withHistory(a *App) {
	now := a.now()
	values := []float64{18, 24, 31, 33, 38, 44, 52, 61, 68, 72, 78}
	for _, w := range []struct {
		provider, account, window string
		offset                    float64
	}{
		{"deepseek", "default", "primary-percent", 0},
		{"kimi", "work", "primary", 12},
	} {
		var series history.Series
		series.Provider, series.Account, series.Window = w.provider, w.account, w.window
		for i, v := range values {
			val := v + w.offset
			series.Points = append(series.Points, history.Point{
				T:           now.Add(time.Duration(i-len(values)) * 20 * time.Minute),
				UsedPercent: &val,
			})
		}
		a.series = append(a.series, series)
	}
}

func TestOverviewDrawsTheTrendUnderTheBar(t *testing.T) {
	a := testApp(t)
	withHistory(a)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshotStill(t, tester, "overview-trend")
	// The chart scales itself to the series, so its caption has to say what
	// the ends are worth.
	for _, want := range []string{"近 11 次查询", "18% → 78%", "覆盖 3 小时 20 分", "比上次 +6%"} {
		if !tester.HasText(want) {
			t.Errorf("the hero trend does not say %q; texts=%v", want, tester.Texts())
		}
	}
	// The countdown is one reading in the hero's strip, a caption over its
	// value, so the caption below the chart is not a second place to read it.
	if !tester.HasText("重置") || !tester.HasText("2 天 4 小时") {
		t.Errorf("the hero does not say when the window comes back; texts=%v", tester.Texts())
	}
	if tester.HasText("2 天 4 小时后重置 · 比上次 +6%") {
		t.Error("the delta should live in the caption, not twice on the card")
	}
	narrow := ui.NewTester(a.view, 780, 560)
	snapshot(t, narrow, "overview-trend-narrow")
	a.navigate("provider:deepseek")
	detail := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, detail, "provider-trend")
	if !detail.HasText("近 11 次查询：") || !detail.HasText("18% → 78%") {
		t.Errorf("the provider page should caption the trend with its own times and range; texts=%v", detail.Texts())
	}
}

// The two levels a percentage window turns color at are cut out of the bar's
// fill as a 1px gap, so the scale is the empty track showing through — the only
// mark that stays legible on both sides of itself — and the fill is flat,
// because the gradient that used to lighten its leading end left the edge the
// bar is read by at 2.66:1 against the track in the light theme.
func TestTheBarCutsItsLevelsOutOfTheFill(t *testing.T) {
	a := &App{}
	var p palette
	used := 0.0
	right := 10.0
	view := func(c *ui.Context) {
		p = a.theme(c)
		ui.Column(c).FillWidth().Padding(10, float32(right), 10, 10).Children(func() {
			meter(c, p, model.RateWindow{ID: "primary", Title: "Weekly", UsedPercent: ptrTo(used)}, 10)
		})
	}
	for _, tc := range []struct {
		used float64
		runs int
	}{
		{35, 1}, // under the warn level: no level has been reached
		{78, 2}, // past 50: the warn level is a gap
		{96, 3}, // past 80: both levels are gaps
	} {
		// The matrix gives a column 65.14px, so the bar's width is not a whole
		// number: a 1px gap has to survive being rounded to device pixels, and
		// a bar that loses its scale in the narrow case has no scale at all.
		for _, pad := range []float64{10, 11.3} {
			used, right = tc.used, pad
			tester := ui.NewTester(view, 400, 40)
			tester.Frame()
			fill := barColor(p, model.RateWindow{UsedPercent: ptrTo(tc.used)})
			runs, extent, ok := fillRuns(tester.Image(), fill)
			if !ok {
				t.Fatalf("used=%.0f%% pad=%.1f: the bar drew no fill", tc.used, pad)
			}
			if len(runs) != tc.runs {
				t.Errorf("used=%.0f%% pad=%.1f: the fill has %d stretches, want %d: %v", tc.used, pad, len(runs), tc.runs, runs)
				continue
			}
			// Each gap sits at the level it stands for, on the bar's own width.
			for i, level := range []float64{0.5, 0.8}[:tc.runs-1] {
				want := extent[0] + int(level*float64(extent[1]-extent[0]))
				if got := runs[i+1][0]; got < want-3 || got > want+3 {
					t.Errorf("used=%.0f%% pad=%.1f: the gap for %.0f%% is at x=%d, want about %d", tc.used, pad, level*100, got, want)
				}
			}
		}
	}
}

// laneApp is one account with three percent windows at three levels: the
// shape that shows whether a card's bars can be read against each other.
func laneApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	if err := loginAPIKey("opencode-go", "alpha", "oc_sk-test"); err != nil {
		t.Fatalf("loginAPIKey: %v", err)
	}
	if _, err := config.Load(); err != nil {
		t.Fatalf("config: %v", err)
	}
	a.reloadConfig()
	reset := a.lastAt.Add(36 * time.Hour)
	var windows []model.RateWindow
	for i, w := range []struct{ id, title string }{{"5h", "5-hour"}, {"weekly", "Weekly"}, {"monthly", "Monthly"}} {
		pct := float64(20 * (i + 1))
		windows = append(windows, model.RateWindow{ID: w.id, Title: w.title, UsedPercent: &pct, ResetsAt: &reset})
	}
	a.out = &model.Output{Version: 1, GeneratedAt: a.lastAt, Providers: []model.ProviderResult{{
		ID: "opencode-go", DisplayName: "OpenCode Go", Status: "ok", UpdatedAt: a.lastAt,
		Accounts: []model.AccountResult{{
			Name: "alpha", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: a.lastAt,
				Identity:  &model.Identity{Account: "alpha@example.com", Plan: "Go"},
				Windows:   windows,
			},
		}},
	}}}
	return a
}

// TestTheBarsInOneCardCanBeReadAgainstEachOther pins the one thing a set of
// bars has to get right: the same span of track means the same amount in every
// row. A bar that starts where its own label happens to end makes two 20%
// windows two different lengths, which is a meter that lies about the numbers
// printed beside it.
func TestTheBarsInOneCardCanBeReadAgainstEachOther(t *testing.T) {
	a := laneApp(t)
	a.navigate("provider:opencode-go")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	img := tester.Image()

	// The card's own surface, and the inside of the card without its border:
	// the border is at the same x on every row, so leaving it in would make
	// every row look like it started in the same place. The surface is not one
	// value — the card is painted with a slight gradient — so a pixel counts as
	// surface when it is close to the most common one.
	const x0, x1 = 276, 1003
	surface := modalColor(img, ui.Rect{X: x0, Y: 180, W: x1 - x0, H: 300})
	near := func(c ui.Color) bool {
		d := func(a, b uint8) int {
			if a > b {
				return int(a) - int(b)
			}
			return int(b) - int(a)
		}
		return d(c.R, surface.R)+d(c.G, surface.G)+d(c.B, surface.B) <= 8
	}
	type bar struct{ left, right, top, bottom int }
	var bars []bar
	for y := 180; y < 560; y++ {
		left, right, ink := -1, -1, 0
		for x := x0; x <= x1; x++ {
			if !near(colorAt(img, x, y)) {
				if left < 0 {
					left = x
				}
				right = x
				ink++
			}
		}
		// A bar is a long, almost unbroken run of ink; a line of text is not,
		// and a rule that reaches both ends of the card is not one either.
		if left < 0 || right-left < 240 || ink < (right-left)*95/100 || left <= x0 || right >= x1 {
			continue
		}
		if n := len(bars); n > 0 && bars[n-1].bottom >= y-2 {
			bars[n-1].bottom = y
			if left < bars[n-1].left {
				bars[n-1].left = left
			}
			if right > bars[n-1].right {
				bars[n-1].right = right
			}
			continue
		}
		bars = append(bars, bar{left: left, right: right, top: y, bottom: y})
	}
	// The rules between rows are one pixel tall and span the same width.
	var drawn []bar
	for _, s := range bars {
		if s.bottom-s.top >= 4 {
			drawn = append(drawn, s)
		}
	}
	if len(drawn) != 3 {
		t.Fatalf("the card drew %d bars that span it, want 3: %v — a bar has to be the width of the card and on a row of its own, or the bars cannot be read against each other", len(drawn), drawn)
	}
	for _, s := range drawn[1:] {
		if s.left != drawn[0].left || s.right != drawn[0].right {
			t.Errorf("the bars do not line up: one spans %d..%d, another %d..%d", drawn[0].left, drawn[0].right, s.left, s.right)
		}
	}
}

// fillRuns finds the row a bar was drawn in and the stretches of its fill
// color on it, with the bar's own left and right edges on that row.
func fillRuns(img *image.RGBA, fill ui.Color) ([][2]int, [2]int, bool) {
	b := img.Bounds()
	canvas := colorKey(modalColor(img, ui.Rect{W: float32(b.Dx()), H: float32(b.Dy())}))
	key := colorKey(fill)
	row, most := -1, 0
	for y := 0; y < b.Dy(); y++ {
		n := 0
		for x := 0; x < b.Dx(); x++ {
			if colorKey(colorAt(img, x, y)) == key {
				n++
			}
		}
		if n > most {
			row, most = y, n
		}
	}
	if row < 0 {
		return nil, [2]int{}, false
	}
	extent := [2]int{-1, -1}
	for x := 0; x < b.Dx(); x++ {
		if colorKey(colorAt(img, x, row)) != canvas {
			if extent[0] < 0 {
				extent[0] = x
			}
			extent[1] = x
		}
	}
	var runs [][2]int
	start := -1
	for x := 0; x < b.Dx(); x++ {
		same := colorKey(colorAt(img, x, row)) == key
		if same && start < 0 {
			start = x
		}
		if !same && start >= 0 {
			runs = append(runs, [2]int{start, x - 1})
			start = -1
		}
	}
	if start >= 0 {
		runs = append(runs, [2]int{start, b.Dx() - 1})
	}
	return runs, extent, true
}

// addSeries records a series for a provider, so the history page has more
// than one shape to draw.
func addSeries(a *App, providerID, account, window string, base, step float64, pct bool, resetAt int) {
	var s history.Series
	s.Provider, s.Account, s.Window = providerID, account, window
	v := base
	for i := 0; i < 11; i++ {
		v += step
		point := history.Point{T: a.now().Add(time.Duration(i-11) * 25 * time.Minute)}
		if pct {
			p := v
			point.UsedPercent = &p
		} else {
			u := v
			point.Used, point.Currency = &u, "USD"
		}
		if resetAt > 0 && i == resetAt {
			point.Reset = true
		}
		s.Points = append(s.Points, point)
	}
	a.series = append(a.series, s)
}

func TestShortenPathKeepsTheFileName(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	for _, tc := range []struct{ in, want string }{
		{home + "/.config/leeway/config.json", "~/.config/leeway/config.json"},
		{"/var/folders/g_/n13_z_mj4x7b4_3_qdvrf5d0000gn/T/TestSettings1234/001/cache", "…/001/cache"},
		{"/tmp/leeway", "/tmp/leeway"},
	} {
		if got := shortenPath(tc.in); got != tc.want {
			t.Errorf("shortenPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSettingsPathsAreReadableAndCopyable(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.settingsView, settingsWidth, settingsHght)
	if err := tester.Click("数据"); err != nil {
		t.Fatalf("click 数据: %v", err)
	}
	// The click copies through MyGo's clipboard, which waits on a main loop
	// the tester has none of; the row itself is what is checked here.
	for _, path := range []string{configPathText(), cacheDirText(), sessionDirText(), historyPathText()} {
		if _, ok := tester.Find(shortenPath(path)); !ok {
			t.Errorf("%q is not on the page or not clickable; texts=%v", shortenPath(path), tester.Texts())
		}
	}
}

// TestTheHistoryCardsAreTheSameWidth pins why the page is a grid and not a
// wrapping row of growing cards: the last card on a page would take the whole
// width, and the trend it draws would be at a different scale from the ones
// beside it, which is two readings of the same line.
// One provider's history gets the whole measure: the fixed card width exists so
// one card's slope can be read against another's, and with a single card a
// half-width tray left half the page empty.
func TestASingleHistoryTraySpansTheMeasure(t *testing.T) {
	a := testApp(t)
	addSeries(a, "codex", "default", "primary", 30, 6, true, 0)
	a.navigate("history")
	var p palette
	page := a.view
	tester := ui.NewTester(func(c *ui.Context) { p = a.theme(c); page(c) }, mainWidth, mainHeight)
	tester.Frame()
	img := tester.Image()
	// The rule under the card's header is as wide as the card; the page's
	// gradient moves the surface under it a little from row to row, so the ink
	// is matched against the row above it.
	wide := 0
	for y := 200; y < mainHeight-10; y++ {
		want := over(p.hairline, colorAt(img, 280, y-3))
		run := 0
		for x := 274; x <= 1006; x++ {
			if close3(colorAt(img, x, y), want) {
				run++
				if run > wide {
					wide = run
				}
				continue
			}
			run = 0
		}
	}
	if wide < 700 {
		t.Errorf("the only history card's rule is %dpx across a 739px measure: one card does not need a two-card width", wide)
	}
}

// close3 reports whether two colours are within three steps of each other: the
// page's gradient moves the surface under a rule a little from row to row, so a
// rule is matched against the row just above it rather than a fixed colour.
func close3(a, b ui.Color) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x) - int(y)
		}
		return int(y) - int(x)
	}
	return d(a.R, b.R)+d(a.G, b.G)+d(a.B, b.B) <= 3
}

func TestTheHistoryCardsAreTheSameWidth(t *testing.T) {
	a := testApp(t)
	addSeries(a, "codex", "default", "primary", 30, 6, true, 0)
	addSeries(a, "codex", "default", "secondary", 10, 2, true, 0)
	addSeries(a, "cursor", "default", "included", 0.4, 0.28, false, 0)
	addSeries(a, "kimi", "home", "primary", 55, 3, true, 7)
	a.navigate("history")
	var p palette
	page := a.view
	tester := ui.NewTester(func(c *ui.Context) { p = a.theme(c); page(c) }, mainWidth, mainHeight)
	tester.Frame()
	img := tester.Image()

	// The rule under a card's header is as wide as the card is: the length of
	// that run is the card's width.
	var widths []int
	rows, paired := 0, 0
	for y := 200; y < 700; y++ {
		want := over(p.hairline, colorAt(img, 280, y-3))
		var runs []int
		run := 0
		for x := 274; x <= 1006; x++ {
			if close3(colorAt(img, x, y), want) {
				run++
				continue
			}
			if run >= 200 {
				runs = append(runs, run)
			}
			run = 0
		}
		if run >= 200 {
			runs = append(runs, run)
		}
		if len(runs) == 0 {
			continue
		}
		rows++
		if len(runs) > 1 {
			paired++
		}
		widths = append(widths, runs...)
	}
	if rows < 2 || paired == 0 {
		t.Fatalf("found %d rows of card rules, %d of them side by side: the page is not two cards across", rows, paired)
	}
	for _, w := range widths[1:] {
		if w < widths[0]-1 || w > widths[0]+1 {
			t.Errorf("the cards are not the same width: one is %d wide, another %d", widths[0], w)
		}
	}
}

func TestHistoryGroupsSeriesUnderTheirProvider(t *testing.T) {
	a := testApp(t)
	addSeries(a, "codex", "default", "primary", 30, 6, true, 0)
	addSeries(a, "codex", "default", "secondary", 10, 2, true, 0)
	addSeries(a, "cursor", "default", "included", 0.4, 0.28, false, 0)
	addSeries(a, "kimi", "home", "primary", 55, 3, true, 7)
	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "history-many")
	// One tray per provider, and the tray says how many series it holds.
	for _, want := range []string{"2 条记录", "1 条记录"} {
		if !tester.HasText(want) {
			t.Errorf("the history page does not say %q; texts=%v", want, tester.Texts())
		}
	}
	// A reset is drawn as a tick on the chart, so the caption points at it.
	if !tester.HasText("期间重置 1 次（竖线就是重置）") {
		t.Errorf("a series with a reset should say so; texts=%v", tester.Texts())
	}
	dark := ui.NewTester(a.view, mainWidth, mainHeight)
	dark.SetDark(true)
	snapshot(t, dark, "history-many-dark")
}

func TestNavigationAndToastsAreAnnounced(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Announcements()
	a.navigate("history")
	tester.Frame()
	if got := tester.Announcements(); len(got) != 1 || got[0] != "历史" {
		t.Errorf("navigating to the history page announced %v, want [历史]", got)
	}
	a.navigate("provider:deepseek")
	tester.Frame()
	if got := tester.Announcements(); len(got) != 1 || got[0] != "DeepSeek" {
		t.Errorf("navigating to a provider announced %v, want [DeepSeek]", got)
	}
	// A toast is often the only answer a click gives.
	a.toast = "已复制"
	tester.Frame()
	if got := tester.Announcements(); len(got) != 1 || got[0] != "已复制" {
		t.Errorf("a toast announced %v, want [已复制]", got)
	}
}

func TestKeyboardNavigationKeepsTheFrameAlive(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	for i := 0; i < 8; i++ {
		tester.Key(0, ui.KeyTab)
	}
	if !tester.HasText("总览") || !tester.HasText("DeepSeek") {
		t.Error("tabbing through the window broke the frame")
	}
}

func TestEmptyHistoryShowsTheShapeOfThePage(t *testing.T) {
	a := testApp(t)
	a.series = nil
	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "history-empty")
	for _, want := range []string{"还没有记录", "去总览页刷新"} {
		if !tester.HasText(want) {
			t.Errorf("the empty history does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if err := tester.Click("去总览页刷新"); err != nil {
		t.Fatalf("click 去总览刷新: %v", err)
	}
	if a.page != "overview" {
		t.Errorf("the empty state should lead to the overview, got %q", a.page)
	}
}

func TestConfirmSheetAsksBeforeLoggingOut(t *testing.T) {
	a := testApp(t)
	a.askLogout("kimi", "work")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "confirm")
	for _, want := range []string{"移除 Kimi Code 的账号 work？", "取消", "移除账号"} {
		if !tester.HasText(want) {
			t.Errorf("the confirm sheet does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if err := tester.Click("取消"); err != nil {
		t.Fatalf("click 取消: %v", err)
	}
	if a.confirm.open {
		t.Error("cancelling should close the sheet")
	}
}

func TestLogoutConfirmForKeyOnlyProviderSaysKey(t *testing.T) {
	a := testApp(t)
	a.askLogout("deepseek", "work")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	for _, want := range []string{
		"删除 DeepSeek 账号 work 的 API 密钥？",
		"只删 leeway 里存的这一份，别处不受影响。",
		"删除密钥",
	} {
		if !saysText(tester, want) {
			t.Errorf("the confirm sheet does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if saysText(tester, "移除账号") {
		t.Error("a key-only provider is not a logout")
	}
}

// A sheet that failed says so in this codebase's words. The credential sheets
// have no log pane to put a library's message in, so the message goes on the
// tooltip — the same rule the pages follow.
func TestAFailureKeepsTheMachinesWordsOutOfTheSentence(t *testing.T) {
	for _, c := range []struct {
		message string
		want    string
		detail  string
	}{
		// Our words, then a machine's: only our half is the sentence.
		{"写 Cursor keychain 失败：exit status 51", "切换失败 · 写 Cursor keychain 失败", "exit status 51"},
		// Ours throughout: it is the sentence, with nothing hidden.
		{"cookie 已过期，重新登录", "切换失败 · cookie 已过期，重新登录", ""},
		// A machine's alone: the sentence is ours and the tooltip has it.
		{"unexpected EOF", "切换失败", "unexpected EOF"},
		// A quoted path is the OS naming a file it could not open, and it is
		// the machine's whole message even when a shell command trails it: the
		// colon split must not pull the OS's own sentence back on screen.
		{`无法打开"/Users/张三/auth.json"：没有那个文件或目录：exit status 1`, "切换失败", `无法打开"/Users/张三/auth.json"：没有那个文件或目录：exit status 1`},
		// Our half, then a library's: the split is what keeps the English tail
		// out of the sentence (switcher.go writes this shape with %w).
		{"读取 /Users/张三/.codex/auth.json 失败：no such file or directory", "切换失败 · 读取 /Users/张三/.codex/auth.json 失败", "no such file or directory"},
		{"打开 Cursor state.vscdb：unable to open database file", "切换失败 · 打开 Cursor state.vscdb", "unable to open database file"},
		// A machine's in Chinese: a localised OS error is still a machine's,
		// and the path in it gives that away where the language does not.
		{"open /Users/张三/Library/auth.json: no such file or directory", "切换失败", "open /Users/张三/Library/auth.json: no such file or directory"},
		{"打开 /Users/张三/x 被拒绝", "切换失败", "打开 /Users/张三/x 被拒绝"},
		{"Codex auth.json unreadable: open /Users/张三/.codex/auth.json: no such file", "切换失败", "Codex auth.json unreadable: open /Users/张三/.codex/auth.json: no such file"},
		// Ours, ending on the path it names, or naming its tool in Latin: a
		// path in the middle of a Chinese sentence with no Latin anywhere is
		// the OS's shape, but these are not that shape and must stay readable.
		{`kimi 账号 "work" 的登录文件不在 /Users/张三/.leeway/sessions/kimi-work`, `切换失败 · kimi 账号 "work" 的登录文件不在 /Users/张三/.leeway/sessions/kimi-work`, ""},
		{"无法写入 /Users/me/auth.json", "切换失败 · 无法写入 /Users/me/auth.json", ""},
		{"没有找到 cursor-agent：/opt/homebrew/bin 里没有它", "切换失败 · 没有找到 cursor-agent：/opt/homebrew/bin 里没有它", ""},
		{"cookie 已过期: 请重新登录", "切换失败 · cookie 已过期: 请重新登录", ""},
		// Ours, with a path or a URL inside the sentence: those stay on
		// screen. A rule that fired on any "/" hid them, and on the toast
		// paths — which throw the detail away — that lost the sentence
		// entirely.
		{"没找到 cursor-agent CLI，先安装 https://cursor.com/cli", "切换失败 · 没找到 cursor-agent CLI，先安装 https://cursor.com/cli", ""},
		{"请先导入 ~/.grok/auth.json", "切换失败 · 请先导入 ~/.grok/auth.json", ""},
		{"找到 3 个 Cursor state.vscdb（/Users/x/Library/…/state.vscdb, /Users/x/.config/…/state.vscdb）——用 LEEWAY_CURSOR_STATE_DB 指定要写哪一个", "切换失败 · 找到 3 个 Cursor state.vscdb（/Users/x/Library/…/state.vscdb, /Users/x/.config/…/state.vscdb）——用 LEEWAY_CURSOR_STATE_DB 指定要写哪一个", ""},
		// Ours on both sides of the colon: the split is only for a machine's
		// tail, so our own explanation is not demoted to a tooltip and the
		// sentence does not stutter ("登录没有完成 · 登录似乎没完成").
		{"登录似乎没完成：官方工具没有写出登录信息", "切换失败 · 登录似乎没完成：官方工具没有写出登录信息", ""},
		{"写 Cursor keychain 后回读不一致：写入 64 字节，读回 32 字节", "切换失败 · 写 Cursor keychain 后回读不一致：写入 64 字节，读回 32 字节", ""},
		// Ours, naming a file it refused to touch: the path is the point.
		{"拒绝删除 /Users/张三/.leeway/sessions/x：不在 leeway 的目录里", "切换失败 · 拒绝删除 /Users/张三/.leeway/sessions/x：不在 leeway 的目录里", ""},
		// A machine's, quoting the file it could not open, in Chinese.
		{`无法打开"/Users/张三/auth.json"：没有那个文件或目录`, "切换失败", `无法打开"/Users/张三/auth.json"：没有那个文件或目录`},
	} {
		got, detail := failureText("切换失败", c.message)
		if got != c.want || detail != c.detail {
			t.Errorf("failureText(%q) = %q / %q, want %q / %q", c.message, got, detail, c.want, c.detail)
		}
	}
}

func TestAFailedSheetKeepsTheLibrarysWordsOffTheSentence(t *testing.T) {
	a := testApp(t)
	a.openSessionFile("grok", "work")
	// The path carries a Chinese directory name on purpose: macOS localises its
	// errors, so the message underneath has Chinese in it too, and the sheet
	// must still say the failure in the app's own words.
	a.prompt.path = filepath.Join(t.TempDir(), "文档", "not-there.json")
	// The sheet's failure path without the goroutine a window needs: the file
	// is read and the message split exactly as `submitPrompt` does it, so the
	// assertion below is about the split and not about a race.
	if _, err := importAuthFile("grok", "work", a.prompt.path); err == nil {
		t.Fatalf("reading %s succeeded; the fixture is wrong", a.prompt.path)
	} else {
		a.prompt.err, a.prompt.errDetail = failureText("导入登录文件失败", err.Error())
	}
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	if !tester.HasText("导入登录文件失败") {
		t.Errorf("the sheet does not say what failed; texts=%v", tester.Texts())
	}
	for _, text := range tester.Texts() {
		if strings.Contains(text, "no such file") || strings.Contains(text, a.prompt.path) {
			t.Errorf("a library's own message is the sheet's sentence: %q", text)
		}
	}
	if a.prompt.errDetail == "" {
		t.Error("the message the reader is owed went nowhere")
	}
}

func TestSessionFileSheetOffersAFilePicker(t *testing.T) {
	a := testApp(t)
	a.openSessionFile("grok", "work")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	if !tester.HasText("导入登录文件") || !tester.HasText("选择…") {
		t.Errorf("the session sheet is incomplete; texts=%v", tester.Texts())
	}
	if a.prompt.path == "" {
		t.Error("the sheet should prefill the provider's own session path")
	}
}

func ptrTo[T any](v T) *T { return &v }

// meanDifference is how far apart two frames are on average, over the three
// channels of every pixel, from 0 for the same picture to 255.
func meanDifference(x, y image.Image) float64 {
	b := x.Bounds()
	total, n := 0.0, 0
	for py := b.Min.Y; py < b.Max.Y; py++ {
		for px := b.Min.X; px < b.Max.X; px++ {
			xr, xg, xb, _ := x.At(px, py).RGBA()
			yr, yg, yb, _ := y.At(px, py).RGBA()
			total += float64(abs(int(xr>>8)-int(yr>>8)) + abs(int(xg>>8)-int(yg>>8)) + abs(int(xb>>8)-int(yb>>8)))
			n += 3
		}
	}
	return total / float64(n)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestProviderActionsMatchWhatTheProviderSupports(t *testing.T) {
	// Key-only providers are configured; the four with an official CLI log in.
	for _, id := range []string{"deepseek", "opencode-go", "commandcode"} {
		if got := loginButtonLabel(id); got != "设置密钥" {
			t.Errorf("%s button = %q, want 设置密钥", id, got)
		}
	}
	for _, id := range []string{"cursor", "grok", "codex", "kimi"} {
		if got := loginButtonLabel(id); got != "登录" {
			t.Errorf("%s button = %q, want 登录", id, got)
		}
	}

	// Only providers with an official local client can be switched, and kimi
	// needs a session file to write.
	for id, want := range map[string]bool{
		"cursor": true, "grok": true, "kimi": true,
		"codex": false, "deepseek": false, "opencode-go": false, "commandcode": false,
	} {
		if got := canSwitchTo(id, true); got != want {
			t.Errorf("canSwitchTo(%s) = %v, want %v", id, got, want)
		}
	}
	if canSwitchTo("kimi", false) {
		t.Error("a kimi key-only account has no session file to write")
	}

	// The credential kinds come from each provider's AuthSpec, so the window
	// never offers something the provider does not read.
	labels := func(id string) []string {
		var out []string
		for _, m := range loginMethods(id) {
			out = append(out, m.label)
		}
		return out
	}
	if got := labels("deepseek"); len(got) != 1 || got[0] != "粘贴 API 密钥…" {
		t.Errorf("deepseek login methods = %v", got)
	}
	if got := labels("commandcode"); len(got) != 1 || got[0] != "粘贴 API 密钥…" {
		t.Errorf("commandcode reads no cookies, methods = %v", got)
	}
	if got := labels("opencode-go"); len(got) != 3 || got[1] != "读取浏览器登录状态…" {
		t.Errorf("opencode-go methods = %v", got)
	}
	if got := labels("cursor"); len(got) != 3 || got[0] != "用官方工具登录…" || got[2] != "导入登录文件…" {
		t.Errorf("cursor methods = %v", got)
	}
	if got := labels("grok"); len(got) != 4 || got[1] != "读取浏览器登录状态…" {
		t.Errorf("grok methods = %v", got)
	}
}

// The switch entry has to follow what the action really needs: cursor writes
// the account's own session back into the client, so a login without its
// refresh token is not something the window may offer.
func TestSwitchEntryFollowsTheRealPrecondition(t *testing.T) {
	a := testApp(t)
	if accountSwitchReady(a, "cursor", "work") {
		t.Error("an account with no session cannot be switched to")
	}
	if err := auth.WriteCursorAuth("work", auth.CursorAuth{AccessToken: "at"}); err != nil {
		t.Fatal(err)
	}
	if accountSwitchReady(a, "cursor", "work") {
		t.Error("an access token without its refresh token cannot be written back")
	}
	if err := auth.WriteCursorAuth("work", auth.CursorAuth{AccessToken: "at", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	if !accountSwitchReady(a, "cursor", "work") {
		t.Error("a full session should be switchable")
	}
	if accountSwitchReady(a, "deepseek", "default") {
		t.Error("a key-only provider has no local client to switch")
	}
}

// `退出登录` promises to remove the account's credentials, and a default
// account keeps its key at provider level: leaving it behind would let the
// provider go on probing with a key the user just deleted.
func TestLogoutClearsProviderLevelKey(t *testing.T) {
	a := testApp(t)
	if err := loginAPIKey("cursor", "", "sk-cursor"); err != nil {
		t.Fatal(err)
	}
	a.reloadConfig()
	if !accountUsesSession(a, "cursor", "") {
		t.Fatal("a cursor account logs in through the official CLI")
	}
	if msg, err := logoutAccount("cursor", ""); err != nil || msg == "" {
		t.Fatalf("logout = %q err=%v", msg, err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.APIKey("cursor"); got != "" {
		t.Errorf("provider key survived logout: %q", got)
	}
}

// The official-CLI sheet's 完成 button has to close the sheet, like 停止 does.
func TestCLISheetDoneClosesTheSheet(t *testing.T) {
	a := testApp(t)
	a.prompt.kind = promptCLI
	a.prompt.done = true
	a.submitPrompt()
	if a.prompt.kind != promptNone {
		t.Errorf("sheet still open: kind=%v", a.prompt.kind)
	}
}

// The rules between the account columns are a stated rule of the matrix, and
// they only read as one line if the rows of a block meet. No text assertion can
// see a divider, so this one measures the pixels.
func TestMatrixColumnsAreSeparatedByContinuousRules(t *testing.T) {
	a := manyAccountApp(t)
	a.navigate("provider:opencode-go")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	img := tester.Image()
	y0, y1 := 150, mainHeight-10
	// A rule pixel is darker than the card three pixels to either side of it.
	rule := func(x, y int) bool {
		b := rgbSum(img, x, y)
		return b < rgbSum(img, x-3, y)-30 && b < rgbSum(img, x+3, y)-30
	}
	var rules []int
	for x := 300; x < mainWidth-40; x++ {
		n := 0
		for y := y0; y < y1; y++ {
			if rule(x, y) {
				n++
			}
		}
		if n >= 200 {
			rules = append(rules, x)
		}
	}
	if len(rules) != 3 {
		t.Fatalf("matrix rules = %v, want the 3 between four accounts", rules)
	}
	// The table is as tall as its rows, not as tall as the window, so what the
	// rules are measured against is their own span: the rows where all three
	// are drawn. A dash per row leaves a fifth of that missing.
	top, bottom := -1, -1
	for y := y0; y < y1; y++ {
		all := true
		for _, x := range rules {
			if !rule(x, y) {
				all = false
				break
			}
		}
		if all {
			if top < 0 {
				top = y
			}
			bottom = y
			continue
		}
		// The rule between two blocks is one pixel tall; anything longer than
		// that is the page below the table, not a row of it.
		if top >= 0 && y-bottom > 3 {
			break
		}
	}
	if bottom-top < 200 {
		t.Fatalf("the rules of the matrix share only %d..%d: they do not run down the table", top, bottom)
	}
	for _, x := range rules {
		n := 0
		for y := top; y <= bottom; y++ {
			if rule(x, y) {
				n++
			}
		}
		if n < (bottom-top+1)*95/100 {
			t.Errorf("x=%d 的竖线只覆盖 %d/%d 行：分割线是断的", x, n, bottom-top+1)
		}
	}
}

// barRuns is the x runs of one row's drawn bars: pixels that are darker than
// the card they sit on. The skeleton's bars are the meter's track colour, which
// nothing else on the card draws.
func barRuns(img *image.RGBA, y int) [][2]int {
	var runs [][2]int
	for x := 270; x < img.Bounds().Dx()-20; x++ {
		if rgbSum(img, x, y) >= 740 {
			continue
		}
		if n := len(runs); n > 0 && x-runs[n-1][1] <= 2 {
			runs[n-1][1] = x
			continue
		}
		runs = append(runs, [2]int{x, x})
	}
	var wide [][2]int
	for _, r := range runs {
		if r[1]-r[0] >= 40 {
			wide = append(wide, r)
		}
	}
	return wide
}

func rgbSum(img *image.RGBA, x, y int) int {
	r, g, b, _ := img.At(x, y).RGBA()
	return int(r>>8) + int(g>>8) + int(b>>8)
}

// The email typed at login is what tells several subscriptions of one provider
// apart, so a probe reporting the address from before must not overwrite it.
func TestLoginEmailSurvivesAProbe(t *testing.T) {
	a := manyAccountApp(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// The fixture reports work@example.com for this account; the address that
	// was typed at login is the one that identifies the subscription.
	if err := cfg.SetAccountEmail("opencode-go", "work", "billing@example.com"); err != nil {
		t.Fatal(err)
	}
	a.reloadConfig()
	p, ok := provider.Get("opencode-go")
	if !ok {
		t.Fatal("opencode-go is not registered")
	}
	for _, av := range a.providerView(p).Accounts {
		if av.Name != "work" {
			continue
		}
		if av.Email != "billing@example.com" {
			t.Errorf("email = %q, want the one from login", av.Email)
		}
		return
	}
	t.Fatal("the work account is not in the view")
}

// The account menu is where the user meets `让本机用它`, and the rule is that it
// only appears when this account really has a session to write back.
func TestSwitchEntryOnlyAppearsForASwitchableAccount(t *testing.T) {
	a := testApp(t)
	menuFor := func() []string {
		t.Helper()
		tester := ui.NewTester(a.view, mainWidth, mainHeight)
		if err := tester.Click("账号"); err != nil {
			t.Fatalf("no account menu: %v", err)
		}
		return tester.Menu()
	}
	has := func(items []string, want string) bool {
		for _, it := range items {
			if it == want {
				return true
			}
		}
		return false
	}

	// An access token without its refresh token cannot be written back into the
	// IDE, so the entry must not be there.
	if err := auth.WriteCursorAuth("work", auth.CursorAuth{AccessToken: "at"}); err != nil {
		t.Fatal(err)
	}
	a.navigate("provider:cursor")
	if items := menuFor(); has(items, "启用") {
		t.Errorf("an access-only login was offered as switchable: %v", items)
	}

	// With the refresh token it is a session the client can use.
	if err := auth.WriteCursorAuth("work", auth.CursorAuth{AccessToken: "at", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	a.navigate("provider:cursor")
	if items := menuFor(); !has(items, "启用") {
		t.Errorf("a full session was not offered: %v", items)
	}

	// A provider with no local client never offers it at all.
	a.navigate("provider:opencode-go")
	if items := menuFor(); has(items, "启用") {
		t.Errorf("opencode-go has no client to switch: %v", items)
	}
}

// A missing observation is a gap in the record, not a value: the line is drawn
// in the stretches the series really has, so a hole stays a hole instead of
// becoming a straight line through a number the provider never reported.
func TestAGapInTheSeriesStaysAGap(t *testing.T) {
	runs := chartRuns([]chartPoint{{at: 0, v: 10}, {at: 1, v: 20}, {at: 3, v: 40}, {at: 4, v: 50}, {at: 7, v: 80}})
	if len(runs) != 2 {
		t.Fatalf("a series with one hole and one lone point has %d runs, want 2: %+v", len(runs), runs)
	}
	if len(runs[0]) != 2 || runs[0][0].at != 0 || runs[0][1].at != 1 {
		t.Errorf("the first run is %+v, want the two points before the hole", runs[0])
	}
	if len(runs[1]) != 2 || runs[1][0].at != 3 || runs[1][1].at != 4 {
		t.Errorf("the second run is %+v, want the two points after the hole", runs[1])
	}
}

// The system's text size reaches every size the window sets: the six sizes are
// the window's own, so the window is the one that has to scale them, and a
// reader who has asked for larger text gets it on every page.
func TestTheSystemTextSizeReachesEveryPage(t *testing.T) {
	for _, page := range []string{"overview", "provider:deepseek", "history", "about"} {
		a := testApp(t)
		a.navigate(page)
		plain := ui.NewTester(a.view, mainWidth, mainHeight)
		plain.Frame()
		big := ui.NewTester(a.view, mainWidth, mainHeight)
		big.SetPreferences(ui.Preferences{TextScale: 1.5})
		big.Frame()
		snapshot(t, big, "text-scale-"+strings.ReplaceAll(page, ":", "-"))
		if d := meanDifference(plain.Image(), big.Image()); d < 1 {
			t.Errorf("%s renders the same at 1.5x text (mean difference %v)", page, d)
		}
	}
	// The matrix is the page whose row header rides on the number's line, so it
	// is where 1.5x text has the least room: at the default width it used to
	// cut every window name, and no snapshot rendered that page at 1.5x.
	matrix := manyAccountApp(t)
	matrix.navigate("provider:opencode-go")
	bigMatrix := ui.NewTester(matrix.view, mainWidth, mainHeight)
	bigMatrix.SetPreferences(ui.Preferences{TextScale: 1.5})
	bigMatrix.Frame()
	snapshot(t, bigMatrix, "text-scale-provider-many")
	// The settings window is a second window with its own header, so it gets
	// its own render: its title line is the one that has the least room.
	a := testApp(t)
	big := ui.NewTester(a.settingsView, settingsWidth, settingsHght)
	big.SetPreferences(ui.Preferences{TextScale: 1.5})
	big.Frame()
	snapshot(t, big, "text-scale-settings")
}

// The caption column is a measure of text, so the system text size scales it
// with the type. A column pinned at 76px cut 登录文件目录 to 登录文… at 1.5x,
// which is a reader who asked for larger text losing a word; the property is
// that a caption which fits at 1x fits at any scale, because the column and
// the type grow together.
func TestTheCaptionColumnScalesWithTheText(t *testing.T) {
	a := testApp(t)
	a.navigate("provider:deepseek")
	width := func(scale float32) float32 {
		tr := ui.NewTester(a.view, mainWidth, mainHeight)
		if scale != 1 {
			tr.SetPreferences(ui.Preferences{TextScale: scale})
		}
		tr.Frame()
		box, ok := tr.Find("登录文件目录")
		if !ok {
			t.Fatalf("the credential card has no 登录文件目录 caption at %vx; texts=%v", scale, tr.Texts())
		}
		return box.W
	}
	plain := width(1)
	if plain != labelColumnBase {
		t.Fatalf("the caption column at 1x is %v, want %v", plain, labelColumnBase)
	}
	if big := width(1.5); big < 1.4*plain {
		t.Errorf("the caption column at 1.5x is %v against %v at 1x: the type grew and the column it sits in did not", big, plain)
	}
}

// rewriteWindows replaces every account's windows in the fixture, so a test can
// describe a provider whose own window names — or window count — differ from the
// ones the shared fixture happens to use.
func rewriteWindows(t *testing.T, a *App, windows func(i int) []model.RateWindow) {
	t.Helper()
	out := *a.out
	prov := out.Providers[0]
	accts := make([]model.AccountResult, len(prov.Accounts))
	copy(accts, prov.Accounts)
	for i := range accts {
		snap := *accts[i].Snapshot
		snap.Windows = windows(i)
		accts[i].Snapshot = &snap
	}
	prov.Accounts = accts
	out.Providers = []model.ProviderResult{prov}
	a.out = &out
}

// overviewMixApp is the shape the overview's own layout is about: providers
// carrying different numbers of accounts, so their cards come out different
// heights, plus one that needs a hand. Kimi is the hero — the closest to its
// limit — so the cards are the other three.
func overviewMixApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	if err := loginAPIKey("opencode-go", "", "oc_sk-test"); err != nil {
		t.Fatalf("loginAPIKey: %v", err)
	}
	if _, err := config.Load(); err != nil {
		t.Fatalf("config: %v", err)
	}
	account := func(name string, used float64) model.AccountResult {
		pct := used
		return model.AccountResult{
			Name: name, Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: pinned,
				Identity:  &model.Identity{Account: name + "@example.com", Plan: "Pro"},
				Windows:   []model.RateWindow{{ID: "weekly", Title: "Weekly", UsedPercent: &pct}},
			},
		}
	}
	a.out = &model.Output{Version: 1, GeneratedAt: pinned, Providers: []model.ProviderResult{
		{
			ID: "deepseek", DisplayName: "DeepSeek", Status: "ok", UpdatedAt: pinned,
			Accounts: []model.AccountResult{
				account("deep-1", 11), account("deep-2", 12), account("deep-3", 13), account("deep-4", 14),
			},
		},
		{
			ID: "cursor", DisplayName: "Cursor", Status: "ok", UpdatedAt: pinned,
			Accounts: []model.AccountResult{account("cursor-1", 21), account("cursor-2", 22)},
		},
		{
			ID: "opencode-go", DisplayName: "OpenCode Go", Status: "ok", UpdatedAt: pinned,
			Accounts: []model.AccountResult{account("og-1", 31)},
		},
		{
			ID: "kimi", DisplayName: "Kimi", Status: "ok", UpdatedAt: pinned,
			Accounts: []model.AccountResult{account("kimi-1", 99)},
		},
		{
			ID: "grok", DisplayName: "Grok", Status: "error", UpdatedAt: pinned,
			Error:    "cookie 已过期，重新登录",
			Accounts: []model.AccountResult{{Name: "default", Status: "error", Error: "cookie 已过期，重新登录"}},
		},
	}}
	a.lastAt = a.out.GeneratedAt
	return a
}

// The overview lists its cards one per row and full width. Side by side, a
// provider with four accounts next to one with a single account leaves a hole
// under the short card the height of the tall one, and nothing fills it: the
// cards are read in the order of how much they carry, which is the order they
// stack in.
func TestTheOverviewStacksItsCardsFullWidth(t *testing.T) {
	a := overviewMixApp(t)
	// A window tall enough to lay the whole column out: text below the fold has
	// no box to measure.
	tester := ui.NewTester(a.view, mainWidth, 2400)
	tester.Frame()

	at := func(text string) ui.Rect {
		t.Helper()
		box, ok := tester.Find(text)
		if !ok {
			t.Fatalf("the overview does not show %q; texts=%v", text, tester.Texts())
		}
		return box
	}
	// Full width: each card's number is right-aligned inside a card as wide as
	// the page, so all of them end at the page's right edge.
	edge := float32(sidebarWidth+pagePad) + contentWidth(mainWidth)
	for _, label := range []string{"11%", "21%", "31%"} {
		if box := at(label); box.X+box.W < edge-60 {
			t.Errorf("the card holding %s ends at x=%.0f, short of the page's %.0f: the cards are not full width",
				label, box.X+box.W, edge)
		}
	}
	// One per row, in the order the cards carry: four accounts, then two, then
	// one. The second card starts below the first card's last row.
	four, two, one := at("11%"), at("21%"), at("31%")
	if !(four.Y < two.Y && two.Y < one.Y) {
		t.Errorf("the cards run y=%.0f, y=%.0f, y=%.0f: they are not stacked in account-count order",
			four.Y, two.Y, one.Y)
	}
	if last := at("14%"); two.Y <= last.Y {
		t.Errorf("the two-account card starts at y=%.0f, at or above the four-account card's last row at y=%.0f: the cards are still side by side",
			two.Y, last.Y)
	}
	// What needs a hand comes after the cards, not above them.
	if cho := at("需要处理"); cho.Y <= one.Y {
		t.Errorf("需要处理 sits at y=%.0f, above the cards (the last starts at y=%.0f)", cho.Y, one.Y)
	}
}

// sevenAccountApp is the widest matrix the page has to hold: seven accounts in
// one provider, each reporting a different number, the first of them the widest
// string the cell ever holds ("100%").
func sevenAccountApp(t *testing.T) *App {
	a := testApp(t)
	names := []string{"a", "b", "c", "d", "e", "f", "g"}
	for _, n := range names {
		if err := loginAPIKey("opencode-go", n, "oc_sk-test"); err != nil {
			t.Fatalf("loginAPIKey: %v", err)
		}
		if _, err := config.Load(); err != nil {
			t.Fatalf("config: %v", err)
		}
	}
	a.reloadConfig()
	var results []model.AccountResult
	for i, n := range names {
		pct := float64(100 - 10*i)
		reset := a.lastAt.Add(time.Duration(i+1) * 24 * time.Hour)
		results = append(results, model.AccountResult{
			Name: n, Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: a.lastAt,
				Identity:  &model.Identity{Account: n + "@example.com", Plan: "Go"},
				Windows: []model.RateWindow{
					{ID: "weekly", Title: "Weekly", UsedPercent: &pct, ResetsAt: &reset},
				},
			},
		})
	}
	a.out = &model.Output{Version: 1, GeneratedAt: a.lastAt, Providers: []model.ProviderResult{{
		ID: "opencode-go", DisplayName: "OpenCode Go", Status: "ok", UpdatedAt: a.lastAt, Accounts: results,
	}}}
	return a
}

// inkSpan is the first and last column of dark ink inside a box: what a reader
// sees of a string, which its box is not. A number the column cannot hold is
// laid out at its own width and borrows from the gutter, so it draws ink wider
// than the cell it belongs to.
func inkSpan(img *image.RGBA, box ui.Rect) (int, int) {
	lo, hi := -1, -1
	for x := int(box.X); x < int(box.X+box.W) && x < img.Bounds().Dx(); x++ {
		for y := int(box.Y); y < int(box.Y+box.H) && y < img.Bounds().Dy(); y++ {
			c := img.RGBAAt(x, y)
			if int(c.R)+int(c.G)+int(c.B) < 400 {
				if lo < 0 {
					lo = x
				}
				hi = x
				break
			}
		}
	}
	return lo, hi
}

// Seven accounts is the worst matrix the page holds: at the narrowest window a
// column is 49px, which at 1x holds every number but "100%" (55px) and at 1.5x
// holds none of them (60px). Two things must stay true there: a number the cell
// cannot hold is shortened rather than drawn over its neighbour — two numbers the
// eye runs together read as one, which is the worst thing a monitor can do — and
// it is never shortened away to nothing.
func TestTheMatrixNumbersSurviveSevenColumns(t *testing.T) {
	ink := func(w int, scale float32) [][2]int {
		a := sevenAccountApp(t)
		a.navigate("provider:opencode-go")
		tester := ui.NewTester(a.view, w, mainHeight)
		if scale != 1 {
			tester.SetPreferences(ui.Preferences{TextScale: scale})
		}
		tester.Frame()
		img := tester.Image()
		spans := make([][2]int, 0, 7)
		for i := 0; i < 7; i++ {
			want := fmt.Sprintf("%d%%", 100-10*i)
			box, ok := tester.Find(want)
			if !ok {
				t.Fatalf("at %dx/%vx the matrix does not say %s; texts=%v", w, scale, want, tester.Texts())
			}
			lo, hi := inkSpan(img, box)
			if lo < 0 {
				t.Fatalf("at %dx/%vx %s drew no ink", w, scale, want)
			}
			spans = append(spans, [2]int{lo, hi})
		}
		return spans
	}
	for _, scale := range []float32{1, 1.5} {
		wide := ink(mainWidth, scale)
		narrow := ink(780, scale)
		for i := range wide {
			label := fmt.Sprintf("%d%%", 100-10*i)
			whole, cut := wide[i][1]-wide[i][0], narrow[i][1]-narrow[i][0]
			// One pixel of slack: the two widths put the cell on a
			// half-pixel of a different subpixel phase, and a glyph's ink
			// extent moves by one when it does. A cut number loses a third
			// of its ink or more, which the check below still catches.
			if scale == 1 && i > 0 && cut < whole-1 {
				t.Errorf("at 780/1x %s draws %dpx of ink against %dpx at the default width: the cell holds it, so it must not be cut",
					label, cut+1, whole+1)
			}
			// Even cut, a number keeps a third of its ink: a cell that ends up
			// drawing the ellipsis alone tells the reader nothing at all.
			if cut < whole/3 {
				t.Errorf("at 780/%vx %s draws %dpx of ink against %dpx at the default width: the number is cut away to nothing",
					scale, label, cut+1, whole+1)
			}
			if i > 0 {
				if gap := narrow[i][0] - narrow[i-1][1]; gap < 2 {
					t.Errorf("at 780/%vx %s and %d%% are %dpx apart: the numbers ran together",
						scale, label, 100-10*(i-1), gap)
				}
			}
		}
	}
}

// A provider names its own windows, and those names are not all short: with
// "Codex Spark Weekly" on the row header's line, the number — the one thing in
// the cell that must not be cut — was what got cut, drawing "20%" as "2…" and
// dropping its bar 16px below its neighbours'.
func TestALongWindowNameDoesNotCutTheNumber(t *testing.T) {
	long := manyAccountApp(t)
	rewriteWindows(t, long, func(i int) []model.RateWindow {
		pct, reset := float64(20*(i+1)), long.lastAt.Add(24*time.Hour)
		return []model.RateWindow{{ID: "weekly", Title: "Codex Spark Weekly", UsedPercent: &pct, ResetsAt: &reset}}
	})
	long.navigate("provider:opencode-go")
	short := manyAccountApp(t)
	rewriteWindows(t, short, func(i int) []model.RateWindow {
		pct, reset := float64(20*(i+1)), short.lastAt.Add(24*time.Hour)
		return []model.RateWindow{{ID: "weekly", Title: "Weekly", UsedPercent: &pct, ResetsAt: &reset}}
	})
	short.navigate("provider:opencode-go")
	ref := ui.NewTester(short.view, mainWidth, mainHeight)
	ref.Frame()
	got := ui.NewTester(long.view, mainWidth, mainHeight)
	got.Frame()
	for _, want := range []string{"20%", "40%", "60%", "80%"} {
		wantBox, ok := ref.Find(want)
		if !ok {
			t.Fatalf("the short-name matrix does not say %q; texts=%v", want, ref.Texts())
		}
		box, ok := got.Find(want)
		if !ok {
			t.Fatalf("the long-name matrix does not say %q; texts=%v", want, got.Texts())
		}
		if box.W != wantBox.W || box.Y != wantBox.Y {
			t.Errorf("%s is %.1f wide at y=%.1f against %.1f at y=%.1f under a long window name: the name cut the number",
				want, box.W, box.Y, wantBox.W, wantBox.Y)
		}
	}
}

// The width at which a matrix cell's name fits on the number's line is a
// measure of text like the caption column, so it scales with the type and it
// has to hold the line it gates. At 140 it did neither: a four-account matrix
// cut "Monthly" in the windows between 946 and 1010, and at 1.5x text it cut it
// at the default 1040.
func TestTheMatrixRowHeaderFitsTheLineItRidesOn(t *testing.T) {
	header := func(w int, scale float32) float32 {
		a := manyAccountApp(t)
		a.navigate("provider:opencode-go")
		tr := ui.NewTester(a.view, w, mainHeight)
		if scale != 1 {
			tr.SetPreferences(ui.Preferences{TextScale: scale})
		}
		tr.Frame()
		box, ok := tr.Find("Monthly")
		if !ok {
			t.Fatalf("no row header at %dx/%vx; texts=%v", w, scale, tr.Texts())
		}
		return box.W
	}
	for _, tc := range []struct {
		w     int
		scale float32
	}{
		{946, 1},         // the narrow band where the row still rides the line
		{mainWidth, 1.5}, // the same row set in type a third larger
	} {
		// A window this wide gives the header room to spare, so its box is the
		// width the text itself needs.
		natural := header(1400, tc.scale)
		if got := header(tc.w, tc.scale); got != natural {
			t.Errorf("at %dx/%vx the matrix row header is %v against the %v it needs: the name is cut to fit a line measured at 1x",
				tc.w, tc.scale, got, natural)
		}
	}
}

// A window whose only reading is its reset keeps that reading beside the number:
// the spacers spread the readings across the card, and with one reading the
// slack belongs after it — before it, the reading lands at the card's far edge,
// which is the pattern the readings row was rebuilt to remove.
func TestTheOnlyReadingSitsBesideTheNumber(t *testing.T) {
	a := testApp(t)
	a.out = fakeOutput(a.now())
	rewriteWindows(t, a, func(i int) []model.RateWindow {
		pct, reset := 42.0, a.lastAt.Add(48*time.Hour)
		return []model.RateWindow{{ID: "weekly", Title: "Weekly", UsedPercent: &pct, ResetsAt: &reset}}
	})
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	fact, ok := tester.Find("2 天")
	if !ok {
		t.Fatalf("the lead card does not read its reset; texts=%v", tester.Texts())
	}
	if fact.X > 600 {
		t.Errorf("the only reading starts at x=%.0f in a %d-wide window: the slack landed in front of it", fact.X, mainWidth)
	}
}

// The history head does not claim a last record over a card that says there is
// none: a probe that answered without producing a window is exactly the
// first-run machine.
func TestTheEmptyHistoryHeadDoesNotClaimARecord(t *testing.T) {
	a := testApp(t)
	a.lastAt = pinned
	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	if saysText(tester, "最近一次") {
		t.Errorf("the empty history head claims a last record; texts=%v", tester.Texts())
	}
	if !saysText(tester, "还没有记录") {
		t.Errorf("the empty history page does not say it has no record; texts=%v", tester.Texts())
	}
}

// The skeleton's bars are the landed line boxes, so they take the size the
// reader asked for: at fixed heights the lead card was 55px short at 1.5x, and
// the answer then moved what the skeleton had just drawn. This measures the
// tallest bar in the card — the number's — at 1x and at 1.5x.
func TestTheProbingSkeletonScalesWithTheText(t *testing.T) {
	numberBar := func(scale float32) int {
		a := testApp(t)
		a.out = nil
		a.fetching = true
		tester := ui.NewTester(a.view, mainWidth, mainHeight)
		tester.SetPreferences(ui.Preferences{TextScale: scale})
		tester.Frame()
		img := tester.Image()
		best, run := 0, 0
		for y := 120; y < mainHeight-10; y++ {
			long := false
			for _, r := range barRuns(img, y) {
				if r[1]-r[0] >= 90 {
					long = true
				}
			}
			if !long {
				run = 0
				continue
			}
			if run++; run > best {
				best = run
			}
		}
		return best
	}
	one, big := numberBar(1), numberBar(1.5)
	if big*4 < one*5 {
		t.Errorf("the skeleton's number bar is %dpx at 1.5x against %dpx at 1x: the bars do not scale with the text", big, one)
	}
}

// The two preferences a reader can set that change what the window looks like
// are rendered, not only computed: Increase Contrast has to reach the lines
// the window draws faintest, and the focus ring has to be findable, so both
// states are snapshots a reviewer can measure.
func TestTheContrastAndFocusStatesAreVisible(t *testing.T) {
	contrast := testApp(t)
	contrast.navigate("overview")
	strong := ui.NewTester(contrast.view, mainWidth, mainHeight)
	strong.SetPreferences(ui.Preferences{HighContrast: true})
	strong.Frame()
	snapshotStill(t, strong, "overview-contrast")

	focus := testApp(t)
	focus.navigate("overview")
	ring := ui.NewTester(focus.view, mainWidth, mainHeight)
	ring.Frame()
	// Tab reaches the first control on the page; the ring is what says so.
	ring.Key(0, ui.KeyTab)
	ring.Frame()
	snapshot(t, ring, "overview-focus")
}

// The history page is a grid of charts, and every percentage chart on it is
// drawn on the same 0–100 axis with the same two thresholds, out of the same
// queries. Under each chart that is the page repeating itself; under the head
// it is a legend, and the line each chart keeps is the one about that chart.
func TestTheHistoryPageSaysWhatItsChartsShareOnce(t *testing.T) {
	a := testApp(t)
	addSeries(a, "codex", "default", "primary", 30, 6, true, 0)
	addSeries(a, "codex", "default", "secondary", 10, 2, true, 0)
	addSeries(a, "cursor", "default", "included", 0.4, 0.28, false, 0)
	addSeries(a, "kimi", "home", "primary", 55, 3, true, 7)
	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	shared, charts := 0, 0
	for _, text := range tester.Texts() {
		if strings.Contains(text, "50% 提醒") {
			shared++
		}
		// The legend carries the range of the record, a chart the range of its
		// own series.
		if strings.Contains(text, " → ") && !strings.Contains(text, "查询：") {
			charts++
		}
	}
	if shared != 1 {
		t.Errorf("the shared scale is said %d times, want once; texts=%v", shared, tester.Texts())
	}
	if charts != 4 {
		t.Errorf("%d charts say the range of their own series, want 4; texts=%v", charts, tester.Texts())
	}
}

// A window pulled across a wide display is not a reason for a bar 1600px long
// or a number 1500px from the label it belongs to. Past the measure the page
// stops growing and centers itself in what is left, so the widest thing on it
// is the measure rather than the window.
func TestThePageStopsGrowingOnAWideWindow(t *testing.T) {
	const wide = 2400
	a := testApp(t)
	a.navigate("overview")
	tester := ui.NewTester(a.view, wide, 900)
	img := tester.Image()
	// The rule under the page's head spans the page's column, so it is exactly
	// as wide as the content and the only line on the page that is.
	rule := func(x, y int) bool { return rgbSum(img, x, y) < rgbSum(img, x, y-3)-30 }
	best, left, right := 0, 0, 0
	for y := 60; y < 300; y++ {
		n, x0, x1 := 0, -1, -1
		for x := sidebarWidth; x < wide-8; x++ {
			if rule(x, y) {
				n++
				if x0 < 0 {
					x0 = x
				}
				x1 = x
			}
		}
		if n > best {
			best, left, right = n, x0, x1
		}
	}
	if best > contentMax+1 || best < contentMax-1 {
		t.Errorf("the page's column is %d wide on a %dpx window, want the %d measure", best, wide, contentMax)
	}
	// Centered in the scroll area: the room the cap leaves is split evenly, not
	// left as a gutter beside the sidebar.
	l := float32(left) - sidebarWidth - pagePad
	r := float32(wide) - pagePad - float32(right)
	if l-r > 2 || r-l > 2 {
		t.Errorf("the column sits %.0f from the sidebar and %.0f from the edge, want the same", l, r)
	}
}

// The matrix's numbers have to stay on one line across the table: the row's
// name shares the number's line while the column is wide enough for both, and
// takes a line of its own when it is not — kept, empty, in every other column.
// Both are pinned here, with the bars, because a name that pushes its own
// column's bar down is a table that no longer reads across.
func TestTheMatrixKeepsItsNumbersOnOneLine(t *testing.T) {
	for _, tc := range []struct {
		width   int
		stacked bool
	}{{mainWidth, false}, {780, true}} {
		a := manyAccountApp(t)
		a.navigate("provider:opencode-go")
		tester := ui.NewTester(a.view, tc.width, 700)
		tester.Frame()
		title, ok := tester.Find("5-hour")
		if !ok {
			t.Fatalf("width %d: the row has no name", tc.width)
		}
		value, ok := tester.Find("20%")
		if !ok {
			t.Fatalf("width %d: the row has no number", tc.width)
		}
		if stacked := value.Y > title.Y+2; stacked != tc.stacked {
			t.Errorf("width %d: the name is on a line of its own = %v, want %v", tc.width, stacked, tc.stacked)
		}
		img := tester.Image()
		// The bar under a number: a row of continuous ink where the number's
		// own width was card before. Text does not fill a run like that.
		bar := func(v ui.Rect) int {
			for y := int(v.Y + v.H); y < int(v.Y+v.H)+40; y++ {
				n := 0
				for x := int(v.X); x < int(v.X+v.W); x++ {
					if rgbSum(img, x, y) < rgbSum(img, x, y-7)-24 {
						n++
					}
				}
				if n >= int(v.W)-4 {
					return y
				}
			}
			return -1
		}
		first := bar(value)
		if first < 0 {
			t.Fatalf("width %d: no bar under the number", tc.width)
		}
		for _, text := range []string{"40%", "60%", "80%"} {
			v, ok := tester.Find(text)
			if !ok {
				t.Fatalf("width %d: the row has no %s", tc.width, text)
			}
			if got := bar(v); got != first {
				t.Errorf("width %d: %s's bar starts at y=%d, the first column's at y=%d: the numbers are not on one line", tc.width, text, got, first)
			}
		}
	}
}

// The lead number's unit is a caption, not a digit: the % is set smaller than
// the digits it counts and sits on their baseline. The two are one text, so
// the number is still one string to read and to announce.
func TestTheLeadNumbersUnitIsSetSmaller(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	box, ok := tester.Find("78%")
	if !ok {
		t.Fatalf("the lead card has no number; texts=%v", tester.Texts())
	}
	img := tester.Image()
	// The glyphs' ink, column by column, so the first glyph (a 7) can be
	// measured against the last (the %).
	top, bottom := map[int]int{}, map[int]int{}
	for x := int(box.X); x < int(box.X+box.W); x++ {
		for y := int(box.Y); y < int(box.Y+box.H); y++ {
			if rgbSum(img, x, y) >= 600 {
				continue
			}
			if _, seen := top[x]; !seen {
				top[x] = y
			}
			bottom[x] = y
		}
	}
	var runs [][2]int // first and last ink column of each glyph
	for x := int(box.X); x < int(box.X+box.W); x++ {
		if _, seen := top[x]; !seen {
			continue
		}
		if n := len(runs); n > 0 && x-runs[n-1][1] <= 1 {
			runs[n-1][1] = x
			continue
		}
		runs = append(runs, [2]int{x, x})
	}
	if len(runs) < 3 {
		t.Fatalf("the number drew %d glyphs, want the digits and the unit; runs=%v", len(runs), runs)
	}
	height := func(r [2]int) (int, int) {
		lo, hi := int(box.Y+box.H), int(box.Y)
		for x := r[0]; x <= r[1]; x++ {
			if top[x] < lo {
				lo = top[x]
			}
			if bottom[x] > hi {
				hi = bottom[x]
			}
		}
		return lo, hi
	}
	_, digitBottom := height(runs[0])
	unitTop, unitBottom := height(runs[len(runs)-1])
	digitTop, _ := height(runs[0])
	if unit := unitBottom - unitTop; unit*2 >= digitBottom-digitTop {
		t.Errorf("the unit is %d tall against the digits' %d: it is not set smaller", unit, digitBottom-digitTop)
	}
	if d := unitBottom - digitBottom; d < -2 || d > 2 {
		t.Errorf("the unit's baseline is %d off the digits', want them on one line", d)
	}
}

// inkBottom is the lowest row of dark ink inside a box. That is the line a
// reader sees the text sitting on; the box bottom is not, because a box carries
// its face's descent and two faces of different sizes descend differently.
func inkBottom(img *image.RGBA, box ui.Rect) int {
	bottom := -1
	for y := int(box.Y); y < int(box.Y+box.H) && y < img.Bounds().Dy(); y++ {
		for x := int(box.X); x < int(box.X+box.W) && x < img.Bounds().Dx(); x++ {
			c := img.RGBAAt(x, y)
			if int(c.R)+int(c.G)+int(c.B) < 740 {
				bottom = y
				break
			}
		}
	}
	return bottom
}

// The lead card's readings share the number's baseline instead of taking a row
// each under a rule: the card is the page's headline, and the band of nothing
// between the number and the facts that support it is what this saves. The
// readings are compared by their ink, not their boxes: the 40px number's box
// descends 5px further than a 16px reading's, so box bottoms that match put the
// readings' ink 5px low.
func TestTheHeroReadsAcrossNotDown(t *testing.T) {
	a := testApp(t)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	img := tester.Image()
	number, ok := tester.Find("78%")
	if !ok {
		t.Fatalf("the lead card has no number; texts=%v", tester.Texts())
	}
	base := inkBottom(img, number)
	for _, want := range []string{"2 天 4 小时", "¥12.50"} {
		fact, ok := tester.Find(want)
		if !ok {
			t.Fatalf("the lead card does not say %q; texts=%v", want, tester.Texts())
		}
		if got := inkBottom(img, fact); got-base < -2 || got-base > 2 {
			t.Errorf("%q sits %d off the number's baseline, want it on the number's own", want, got-base)
		}
	}
	// What the number counts is named above it, where the number can have the
	// line to itself.
	scope, ok := tester.Find("Weekly")
	if !ok {
		t.Fatal("the lead card does not name the window")
	}
	if scope.Y+scope.H > number.Y {
		t.Errorf("the window's name is at y=%0.f, below the number at y=%0.f", scope.Y, number.Y)
	}
}

// The pointer on a chart names the observation under it. There is no tooltip:
// the caption that describes the series appends the point it is on, and the
// chart marks it — the reading is added, nothing above it moves, and the
// summary the reader was already using stays where it was.
func TestTheChartReadsThePointUnderThePointer(t *testing.T) {
	a := testApp(t)
	now := a.now()
	var pts []history.Point
	for i := range 11 {
		v := float64(30 + i*4)
		pts = append(pts, history.Point{T: now.Add(time.Duration(i) * time.Hour), UsedPercent: &v})
	}
	a.series = []history.Series{{Provider: "deepseek", Account: "default", Window: "primary-percent", Points: pts}}
	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	if !tester.HasText("近 11 次查询") {
		t.Fatalf("the chart has no summary caption to replace; texts=%v", tester.Texts())
	}
	// The caption sits under the chart; the chart is the row above it. What it
	// says is what is left of the summary once the page has said its own part
	// above the grid: the range the series moved over.
	summary := trendRange(pts)
	at, ok := tester.Find(summary)
	if !ok {
		t.Fatalf("no caption showing the summary; texts=%v", tester.Texts())
	}
	tester.ClickAt(at.X+at.W/2, at.Y-24)
	tester.Frame()
	under := ""
	for _, text := range tester.Texts() {
		if scrubLine.MatchString(text) {
			under = text
			break
		}
	}
	if under == "" {
		t.Fatalf("the caption does not name the point under the pointer; texts=%v", tester.Texts())
	}
	if !strings.Contains(under, summary) {
		t.Errorf("the reading took the series summary off the screen: %q", under)
	}
	snapshot(t, tester, "history-scrub")
}

// A probe that failed says so, and says what the numbers under it are: the last
// ones that came back, not live ones. Cached data that looks live is the one
// thing a monitor must never show.
func TestAFailedProbeMarksTheNumbersAsOld(t *testing.T) {
	a := testApp(t)
	a.lastAt = pinned.Add(-12 * time.Minute)
	a.fetchErr = "unexpected EOF"
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	if !tester.HasText("下面是 12 分钟前的数据") {
		t.Errorf("the cached numbers are not marked as old; texts=%v", tester.Texts())
	}
	snapshot(t, tester, "overview-stale")
}

// The chart's hover cells take the pointer, and the whole card is the target —
// chart included. A chart that swallows the click of the card it sits in is a
// dead zone in the middle of the page's main navigation.
func TestTheChartDoesNotEatTheCardsClick(t *testing.T) {
	a := testApp(t)
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	card, ok := tester.Find("查看 DeepSeek")
	if !ok {
		t.Fatalf("the hero card is not a labelled button; texts=%v", tester.Texts())
	}
	// The chart is the last band inside the card: above the caption and the
	// card's own padding, below the bar.
	tester.ClickAt(card.X+card.W/2, card.Y+card.H-70)
	tester.Frame()
	if a.page != "provider:deepseek" {
		t.Errorf("clicking the chart left the page at %q; the hover cells ate the card's click", a.page)
	}
}

// A matrix column is narrow enough at the window's minimum width that an
// address has to be cut rather than wrapped: an address has no space in it, so
// a second line breaks it mid-word ("alpha@example.c" / "om") and the column
// reads as broken. It is one line with an ellipsis, and the whole address is
// on the tooltip.
func TestTheMatrixCutsAddressesRatherThanBreakingThem(t *testing.T) {
	a := manyAccountApp(t)
	a.navigate("provider:opencode-go")
	tester := ui.NewTester(a.view, 780, 500)
	tester.Frame()
	snapshot(t, tester, "provider-many-narrow")
	for _, acc := range []string{"alpha", "work", "side", "delta"} {
		at, ok := tester.Find(acc + "@example.com")
		if !ok {
			t.Fatalf("%s's address is not shown; texts=%v", acc, tester.Texts())
		}
		if at.H > 18 {
			t.Errorf("%s's address is %.0fpx tall, so it wrapped instead of being cut", acc, at.H)
		}
	}
}

// A raw library error never takes the sentence's place in the window: the
// sentence is the app's, and the exact string is on the tooltip. What this
// codebase writes for a person to read is Chinese, so a message with no
// Chinese in it is a library's — and that is the one that has to move.
func TestRawErrorTextStaysOutOfTheSentence(t *testing.T) {
	a := testApp(t)
	a.lastAt = pinned.Add(-12 * time.Minute)
	a.fetchErr = "unexpected EOF"
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	if tester.HasText("unexpected EOF") {
		t.Error("a raw library error is shown where the window's own sentence belongs")
	}
	if !tester.HasText("这次查询没有结果 · 下面是 12 分钟前的数据") {
		t.Errorf("the failure is not explained in the app's own words; texts=%v", tester.Texts())
	}

	// The other half of the rule: a message this codebase wrote is still shown.
	b := testApp(t)
	b.navigate("overview")
	for i := range b.out.Providers {
		b.out.Providers[i].Error = "cookie 已过期，重新登录"
	}
	b.navigate("overview")
	other := ui.NewTester(b.view, mainWidth, mainHeight)
	other.Frame()
	if !other.HasText("查询失败 · cookie 已过期，重新登录") {
		t.Errorf("our own message was hidden as if it were a library's; texts=%v", other.Texts())
	}
}

// The line is a picture of numbers, so the numbers have to be reachable
// without seeing it: the chart takes the focus, the arrows walk it, and each
// observation is announced as the same sentence the pointer readout shows.
func TestTheChartAnswersTheArrowKeys(t *testing.T) {
	a := testApp(t)
	withHistory(a)
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	// MyGo reports Focused() only while the window itself has the focus.
	tester.SetFocused(true)
	tester.Frame()

	// Tab to the chart: nothing else claims the scrub state.
	for i := 0; i < 40 && a.scrub.key != "overview-trend"; i++ {
		tester.Key(0, ui.KeyTab)
		tester.Frame()
	}
	if a.scrub.key != "overview-trend" {
		t.Fatalf("the chart never took the focus; texts=%v", tester.Texts())
	}
	// The caption under the chart carries the series summary and the
	// observation the keyboard is on, and what the frame asked a screen reader
	// to say is that observation.
	read := func() string {
		for _, text := range tester.Texts() {
			if scrubLine.MatchString(text) {
				return text
			}
		}
		return ""
	}
	point := func() string { return scrubLine.FindString(read()) }
	newest := point()
	if newest == "" {
		t.Fatalf("the caption does not read an observation; texts=%v", tester.Texts())
	}
	if !strings.Contains(read(), "近 11 次查询") {
		t.Errorf("the reading took the summary off the screen: %q", read())
	}
	if said := tester.Announcements(); len(said) == 0 || said[len(said)-1] != newest {
		t.Errorf("focus announced %v, want the observation the caption reads (%q)", said, newest)
	}
	if a.scrub.at <= 0 {
		t.Fatalf("focus landed on point %d, want a later one", a.scrub.at)
	}
	snapshot(t, tester, "overview-chart-focus")

	// The left arrow walks back one observation and announces that one.
	at := a.scrub.at
	tester.Key(0, ui.KeyLeft)
	tester.Frame()
	if a.scrub.at != at-1 {
		t.Errorf("left arrow left the cursor at %d, want %d", a.scrub.at, at-1)
	}
	back := point()
	if back == "" || back == newest {
		t.Errorf("the caption still reads %q after the left arrow", back)
	}
	if said := tester.Announcements(); len(said) == 0 || said[len(said)-1] != back {
		t.Errorf("the left arrow announced %v, want %q", said, back)
	}

	// And the right arrow comes back to where it was.
	tester.Key(0, ui.KeyRight)
	tester.Frame()
	if a.scrub.at != at || point() != newest {
		t.Errorf("right arrow left the cursor at %d reading %q, want %d reading %q",
			a.scrub.at, point(), at, newest)
	}
}

// scrubLine is one observation as the caption reads it: "01-02 15:04 · 78%".
var scrubLine = regexp.MustCompile(`\d\d-\d\d \d\d:\d\d · \d+(\.\d+)?%`)

// The attention row has its own copy of the "is this ours or a machine's" rule
// once: it asked only whether the message had Chinese in it, so an OS error
// whose *path* was Chinese — the same shape that leaked into the failure line
// — was drawn in the row as the provider's own words.
func TestTheAttentionRowKeepsAMachinesPathOffTheRow(t *testing.T) {
	a := testApp(t)
	a.navigate("overview")
	path := "open /Users/张三/.codex/auth.json: no such file or directory"
	for i := range a.out.Providers {
		if a.out.Providers[i].ID == "grok" {
			a.out.Providers[i].Error = path
			a.out.Providers[i].Accounts[0].Error = path
		}
	}
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	if tester.HasText(path) {
		t.Errorf("a machine's path is drawn as the row's own words; texts=%v", tester.Texts())
	}
	if !tester.HasText("查询失败 · cookie 已过期，重新登录") && !tester.HasText("查询失败") {
		t.Errorf("the row lost the app's own sentence; texts=%v", tester.Texts())
	}
}

// The matrix is the densest page, and at the window's enforced minimum width
// (780) a column is about 57 px wide. The name is the column's identity, so it
// keeps a floor there instead of collapsing: measured 0x0 without the floor
// (every column, seven accounts) and 20x18.8 with it. The floor must not cost
// the column its control, so this pins that too.
func TestTheColumnNameSurvivesTheNarrowestMatrix(t *testing.T) {
	a := testApp(t)
	idx := -1
	for i := range a.out.Providers {
		if a.out.Providers[i].ID == "kimi" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("the fixture no longer reports kimi")
	}
	first := a.out.Providers[idx].Accounts[0]
	a.out.Providers[idx].Accounts = nil
	for i := 0; i < 7; i++ {
		cp := first
		cp.Name = fmt.Sprintf("acct-%d", i)
		cp.Snapshot = nil
		a.out.Providers[idx].Accounts = append(a.out.Providers[idx].Accounts, cp)
	}
	a.reloadConfig()
	a.navigate("provider:kimi")
	tester := ui.NewTester(a.view, 780, 700)
	tester.Frame()
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("acct-%d", i)
		r, ok := tester.Find(name)
		if !ok {
			t.Fatalf("%s is not on the page at all", name)
		}
		if r.W < 12 || r.H < 6 {
			t.Errorf("%s is squeezed to %v at the minimum width", name, r)
		}
		if r, ok := tester.Find("账号"); !ok || r.W < 12 {
			t.Errorf("the column lost its menu control: %v %v", r, ok)
		}
	}
}
