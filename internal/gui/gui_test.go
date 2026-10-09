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
	if got := deltaText(nil, model.RateWindow{Delta: &model.WindowDelta{UsedPercent: &delta}}); got != "较上次 +4%" && got != "较上次 +5%" {
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
	if !tester.HasText("凭据来自 配置里的 API key") {
		t.Errorf("a configured provider must say where its credential is; texts=%v", tester.Texts())
	}
	if tester.HasText("还没有 API key") {
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
	if !tester.HasText("凭据位置") {
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

// Every account in the matrix gets its own prediction, under its own column,
// so a line like "约 1 小时 40 分后用满" is never left to be guessed at.
func TestManyAccountsShowTheirOwnPrediction(t *testing.T) {
	a := manyAccountApp(t)
	for _, acc := range []string{"alpha", "work", "side", "delta"} {
		addSeries(a, "opencode-go", acc, "5h", 10, 6, true, 0)
	}
	a.navigate("provider:opencode-go")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, tester, "provider-many-burn")
	for _, acc := range []string{"alpha", "work", "side", "delta"} {
		if !tester.HasText(acc + "：约") {
			t.Errorf("%s has no prediction of its own; texts=%v", acc, tester.Texts())
		}
	}
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
	for _, want := range []string{"DeepSeek", "default", "work@example.com", "Balance", "Weekly", "Pro", "配置 key", "刷新", "配置里的 API key"} {
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
	for _, want := range []string{"设置", "自动刷新", "Provider", "数据", "探测超时", "主题"} {
		if !tester.HasText(want) {
			t.Errorf("the settings window does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if err := tester.Click("数据"); err != nil {
		t.Fatalf("click 数据: %v", err)
	}
	snapshot(t, tester, "settings-data")
	if !tester.HasText("清除 cookie 缓存") {
		t.Errorf("the data tab did not open; texts=%v", tester.Texts())
	}
	if err := tester.Click("Provider"); err != nil {
		t.Fatalf("click Provider: %v", err)
	}
	snapshot(t, tester, "settings-providers")
	for _, want := range []string{"启用的 provider", "DeepSeek", "详情"} {
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
	for _, want := range []string{"粘贴 API key", "保存", "取消"} {
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
	if err := tester.Click("显示明文"); err != nil {
		t.Fatalf("click 显示明文: %v", err)
	}
	if !a.prompt.reveal {
		t.Error("the reveal checkbox is not bound to the sheet")
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
		"开始使用", "粘贴 API key", "用官方 CLI 登录", "详情", "配好一个就能用：点「刷新」或按 ⌘R。命令行照旧：leeway usage",
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
	for _, unwanted := range []string{"凭据来自 Codex CLI 会话", "凭据来自 Cursor 本机登录"} {
		if tester.HasText(unwanted) {
			t.Errorf("the first run claims %q without checking; texts=%v", unwanted, tester.Texts())
		}
	}
	if !tester.HasText("还没有 codex 会话。用官方 codex 登录，或导入 ~/.codex/auth.json。") {
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
	if !tester.HasText("凭据来自 Codex CLI 会话；刷新一次就能看到用量。") {
		t.Errorf("a Codex session on this machine is not offered; texts=%v", tester.Texts())
	}
	if !tester.HasText("刷新看看") || !tester.HasText("已找到凭据") {
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
	if !tester.HasText("正在探测…") {
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
	if landed.HasText("正在探测…") {
		t.Error("the skeleton outlived the probe")
	}
	if !landed.HasText("78%") {
		t.Errorf("the answer did not replace the skeleton; texts=%v", landed.Texts())
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

// A window climbing toward a limit it will reach before the provider gives it
// back is the one thing this tool exists to say out loud.
func TestTheWindowThatRunsOutFirstIsNamedAloud(t *testing.T) {
	a := testApp(t)
	now := a.now()
	// 20% → 70% over two hours: 25 points an hour, so the 30 points left are
	// gone in an hour and twelve minutes, and the reset is two days out.
	var pts []history.Point
	for i, v := range []float64{20, 30, 40, 50, 60, 70} {
		val := v
		pts = append(pts, history.Point{T: now.Add(time.Duration(i-5) * 24 * time.Minute), UsedPercent: &val})
	}
	a.series = []history.Series{{Provider: "deepseek", Account: "default", Window: "primary-percent", Points: pts}}
	const alarm = "最近 2 小时每小时涨 25%，约 1 小时 12 分后用满；重置还有 2 天 4 小时"
	// The overview draws its own reset line above the bar, so its projection
	// stops at when the window is full: the countdown is read once there, and
	// on the history row — which has no reset label of its own — it is not.
	const alarmOnOverview = "最近 2 小时每小时涨 25%，约 1 小时 12 分后用满"

	a.navigate("history")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshotStill(t, tester, "history-burn")
	if !saysText(tester, alarm) {
		t.Errorf("the history row does not project the burn; texts=%v", tester.Texts())
	}

	// The overview leads with the same window, so it says the same thing.
	a.navigate("overview")
	over := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshotStill(t, over, "overview-burn")
	if !saysText(over, alarmOnOverview) {
		t.Errorf("the overview does not project the burn; texts=%v", over.Texts())
	}
	if saysText(over, "刚重置") {
		t.Error("a window on its way up was reported as having just reset")
	}
}

// The projection reaches the user wherever they are looking: it is the one
// thing worth saying without being asked, and worth saying only once. On the
// overview the sentence is already on screen in the lead card, so the page
// gets the announcement and not a second copy of it in a toast.
func TestTheBurnWarningIsToldOnce(t *testing.T) {
	a := testApp(t)
	now := a.now()
	var pts []history.Point
	for i, v := range []float64{20, 30, 40, 50, 60, 70} {
		val := v
		pts = append(pts, history.Point{T: now.Add(time.Duration(i-5) * 24 * time.Minute), UsedPercent: &val})
	}
	a.series = []history.Series{{Provider: "deepseek", Account: "default", Window: "primary-percent", Points: pts}}
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshotStill(t, tester, "overview-burn-spoken")
	const warning = "DeepSeek Weekly 按现在的速度 1 小时 12 分后用满"
	if got := tester.Announcements(); !containsText(got, warning) {
		t.Errorf("the warning was not announced; got %v", got)
	}
	// The next frame says nothing: a probe that repeats the warning is not
	// news, and the toast would otherwise come back every minute.
	tester.Frame()
	if got := tester.Announcements(); len(got) != 0 {
		t.Errorf("the warning repeated itself: %v", got)
	}
}

func containsText(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
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

// A window with nothing configured says so where the list would be.
func TestASidebarWithNothingConfiguredSaysSo(t *testing.T) {
	a := freshApp(t)
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	r, ok := tester.Find("配置过的 provider 会出现在这里")
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
	// window coordinates, so the window's bottom is the viewport's.
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

// The projection is silent when the reset comes first: a window with room
// left that resets long before it fills is not news.
func TestAWindowThatResetsFirstIsNotAnAlarm(t *testing.T) {
	a := testApp(t)
	now := a.now()
	var pts []history.Point
	for i, v := range []float64{18, 19, 20} {
		val := v
		pts = append(pts, history.Point{T: now.Add(time.Duration(i-2) * time.Hour), UsedPercent: &val})
	}
	a.series = []history.Series{{Provider: "deepseek", Account: "default", Window: "primary-percent", Points: pts}}
	a.navigate("overview")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	if saysText(tester, "用满") {
		t.Errorf("a window that resets first is not out of quota; texts=%v", tester.Texts())
	}
	// The row still says what moved, which is the part that is true.
	a.navigate("history")
	historyView := ui.NewTester(a.view, mainWidth, mainHeight)
	if !historyView.HasText("▲ +1%") {
		t.Errorf("the change chip is missing; texts=%v", historyView.Texts())
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
	for _, want := range []string{"近 11 次探测", "18% → 78%", "覆盖 3 小时 20 分", "较上次 +6%", "2 天 4 小时后重置"} {
		if !tester.HasText(want) {
			t.Errorf("the hero trend does not say %q; texts=%v", want, tester.Texts())
		}
	}
	// The reset countdown stays in the row above, so the caption is not a
	// second place to read the same thing.
	if tester.HasText("2 天 4 小时后重置 · 较上次 +6%") {
		t.Error("the delta should live in the caption, not twice on the card")
	}
	narrow := ui.NewTester(a.view, 780, 560)
	snapshot(t, narrow, "overview-trend-narrow")
	a.navigate("provider:deepseek")
	detail := ui.NewTester(a.view, mainWidth, mainHeight)
	snapshot(t, detail, "provider-trend")
	if !detail.HasText("近 11 次探测：") || !detail.HasText("18% → 78%") {
		t.Errorf("the provider page should caption the trend with its own times and range; texts=%v", detail.Texts())
	}
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
	for _, want := range []string{"2 条序列", "1 条序列"} {
		if !tester.HasText(want) {
			t.Errorf("the history page does not say %q; texts=%v", want, tester.Texts())
		}
	}
	// A reset is drawn as a tick on the chart, so the caption points at it.
	if !tester.HasText("期间重置 1 次（图上的竖线）") {
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
	a.toast = "已复制 JSON"
	tester.Frame()
	if got := tester.Announcements(); len(got) != 1 || got[0] != "已复制 JSON" {
		t.Errorf("a toast announced %v, want [已复制 JSON]", got)
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
	for _, want := range []string{"还没有历史点", "去总览刷新"} {
		if !tester.HasText(want) {
			t.Errorf("the empty history does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if err := tester.Click("去总览刷新"); err != nil {
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
	for _, want := range []string{"退出 kimi 账号 work？", "取消", "退出登录"} {
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
		"删除 deepseek 账号 work 的 API key？",
		"只删掉 leeway 里存的这个 key，本机其它地方不受影响。",
		"删除 API key",
	} {
		if !saysText(tester, want) {
			t.Errorf("the confirm sheet does not show %q; texts=%v", want, tester.Texts())
		}
	}
	if saysText(tester, "退出登录") {
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
		// A machine's in Chinese: a localised OS error is still a machine's,
		// and the path in it gives that away where the language does not.
		{"open /Users/张三/Library/auth.json: no such file or directory", "切换失败", "open /Users/张三/Library/auth.json: no such file or directory"},
		{"打开 /Users/张三/x 被拒绝", "切换失败", "打开 /Users/张三/x 被拒绝"},
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
	a.prompt.path = filepath.Join(t.TempDir(), "not-there.json")
	// The sheet's failure path without the goroutine a window needs: the file
	// is read and the message split exactly as `submitPrompt` does it, so the
	// assertion below is about the split and not about a race.
	if _, err := importAuthFile("grok", "work", a.prompt.path); err == nil {
		t.Fatalf("reading %s succeeded; the fixture is wrong", a.prompt.path)
	} else {
		a.prompt.err, a.prompt.errDetail = failureText("导入会话文件失败", err.Error())
	}
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	if !tester.HasText("导入会话文件失败") {
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
	if !tester.HasText("导入会话文件") || !tester.HasText("选择…") {
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
		if got := loginButtonLabel(id); got != "配置 key" {
			t.Errorf("%s button = %q, want 配置 key", id, got)
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
	if got := labels("deepseek"); len(got) != 1 || got[0] != "粘贴 API key…" {
		t.Errorf("deepseek login methods = %v", got)
	}
	if got := labels("commandcode"); len(got) != 1 || got[0] != "粘贴 API key…" {
		t.Errorf("commandcode reads no cookies, methods = %v", got)
	}
	if got := labels("opencode-go"); len(got) != 3 || got[1] != "从浏览器导入 cookie…" {
		t.Errorf("opencode-go methods = %v", got)
	}
	if got := labels("cursor"); len(got) != 3 || got[0] != "用官方 CLI 登录…" || got[2] != "导入会话文件…" {
		t.Errorf("cursor methods = %v", got)
	}
	if got := labels("grok"); len(got) != 4 || got[1] != "从浏览器导入 cookie…" {
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

// The prediction of each account belongs to its own column: that is what the
// matrix is for, and four predictions stacked in the first column would pass
// any text-only assertion.
func TestEachPredictionStaysInItsOwnColumn(t *testing.T) {
	a := manyAccountApp(t)
	for _, acc := range []string{"alpha", "work", "side", "delta"} {
		addSeries(a, "opencode-go", acc, "5h", 10, 6, true, 0)
	}
	a.navigate("provider:opencode-go")
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	last := float32(-1)
	for _, acc := range []string{"alpha", "work", "side", "delta"} {
		r, ok := rectOf(tester, acc+"：")
		if !ok {
			t.Fatalf("%s has no prediction", acc)
		}
		if r.X <= last {
			t.Errorf("%s 的预测不在自己那一列：x=%v，前一列 x=%v", acc, r.X, last)
		}
		last = r.X
	}
}

// rectOf finds the drawn box of the first text with this prefix. Find matches
// a whole label, and the labels the matrix draws carry values.
func rectOf(tester *ui.Tester, prefix string) (ui.Rect, bool) {
	for _, txt := range tester.Texts() {
		if strings.HasPrefix(txt, prefix) {
			return tester.Find(txt)
		}
	}
	return ui.Rect{}, false
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
	covered := func(x int) int {
		n := 0
		for y := y0; y < y1; y++ {
			b := rgbSum(img, x, y)
			if b < rgbSum(img, x-3, y)-30 && b < rgbSum(img, x+3, y)-30 {
				n++
			}
		}
		return n
	}
	var rules []int
	for x := 300; x < mainWidth-40; x++ {
		if covered(x) >= 300 {
			rules = append(rules, x)
		}
	}
	if len(rules) != 3 {
		t.Fatalf("matrix rules = %v, want the 3 between four accounts", rules)
	}
	// A dash per row leaves roughly a quarter of the line missing; a line that
	// runs the height of the matrix covers all of it bar a stray pixel.
	for _, x := range rules {
		if n := covered(x); n < (y1-y0)*85/100 {
			t.Errorf("x=%d 的竖线只覆盖 %d/%d 行：分割线是断的", x, n, y1-y0)
		}
	}
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
		if err := tester.Click("账号操作"); err != nil {
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
	if items := menuFor(); has(items, "让本机用它") {
		t.Errorf("an access-only login was offered as switchable: %v", items)
	}

	// With the refresh token it is a session the client can use.
	if err := auth.WriteCursorAuth("work", auth.CursorAuth{AccessToken: "at", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	a.navigate("provider:cursor")
	if items := menuFor(); !has(items, "让本机用它") {
		t.Errorf("a full session was not offered: %v", items)
	}

	// A provider with no local client never offers it at all.
	a.navigate("provider:opencode-go")
	if items := menuFor(); has(items, "让本机用它") {
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
	// The settings window is a second window with its own header, so it gets
	// its own render: its title line is the one that has the least room.
	a := testApp(t)
	big := ui.NewTester(a.settingsView, settingsWidth, settingsHght)
	big.SetPreferences(ui.Preferences{TextScale: 1.5})
	big.Frame()
	snapshot(t, big, "text-scale-settings")
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
	if !tester.HasText("近 11 次探测") {
		t.Fatalf("the chart has no summary caption to replace; texts=%v", tester.Texts())
	}
	// The caption sits under the chart; the chart is the row above it.
	summary := joinNonEmpty([]string{chartScale(pts), historyCaption(pts)}, " · ")
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
	if !tester.HasText("这次探测没有答上来 · 下面是 12 分钟前的数据") {
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
	if !other.HasText("探测失败 · cookie 已过期，重新登录") {
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
	if !strings.Contains(read(), "近 11 次探测") {
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
