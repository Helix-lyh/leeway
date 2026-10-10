package gui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"leeway/internal/history"
	"leeway/internal/model"
	"leeway/internal/provider"
)

// Version is the release the window reports; the bundle's Info.plist wins
// once the app is installed.
const Version = "0.1.0"

// window sizes; the windows remember their own bounds under these keys.
const (
	mainWidth     = 1040
	mainHeight    = 700
	settingsWidth = 660
	settingsHght  = 600
)

// Focus brings the main window forward, as a second launch asks.
func (a *App) Focus() {
	if w := a.win; w != nil && !w.IsDestroyed() {
		w.Show()
		w.Focus()
	}
}

// appearanceCanvas is the window fill before the first frame paints.
const appearanceCanvas = "light-dark(#f2f2f6, #0d0d11)"

// Open creates the main window. Call it from mygo.App.WhenReady.
func (a *App) Open() {
	a.applyAppearance()
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "leeway",
		Width:           mainWidth,
		Height:          mainHeight,
		MinWidth:        780,
		MinHeight:       500,
		TitleBarStyle:   mygo.TitleBarHidden,
		BackgroundColor: appearanceCanvas,
		StateKey:        "main",
		Content:         ui.View(a.view),
	})
	win.SetMinimumSize(780, 500)
	a.Start(win)
}

// OpenSettings shows the settings window, or brings it forward.
func (a *App) OpenSettings() {
	if w := a.settingsWin; w != nil && !w.IsDestroyed() {
		w.Show()
		w.Focus()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:           "leeway 设置",
		Width:           settingsWidth,
		Height:          settingsHght,
		MinWidth:        560,
		MinHeight:       460,
		TitleBarStyle:   mygo.TitleBarHidden,
		BackgroundColor: appearanceCanvas,
		StateKey:        "settings",
		Content:         ui.View(a.settingsView),
	})
	w.OnClosed(func() { a.settingsWin = nil })
	a.settingsWin = w
}

// applyAppearance asks MyGo to follow the appearance the settings chose.
func (a *App) applyAppearance() {
	switch a.settings.Theme {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// Menu is the application menu. Call it before mygo.App.Run.
func (a *App) Menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "刷新", Accelerator: "CmdOrCtrl+R", Click: func(*mygo.MenuItem, *mygo.Window) { a.Refresh(true) }},
			{Label: "复制数据", Accelerator: "CmdOrCtrl+Shift+C", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.Clipboard.WriteText(a.CopyJSON()) }},
			mygo.Separator(),
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) { a.OpenSettings() }},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleWindowMenu},
	})
}

// --- the main window -------------------------------------------------------

// pageBottomPad is the room the scroll keeps under the page: at the end of a
// long page the last card stops this far above the viewport's bottom edge, so
// the page's own padding still shows under it.
const pageBottomPad = 40

// sidebarWidth is the navigation column's width: fixed, so a page's measure
// does not move when a provider is added. pagePad is the room the scroll keeps
// on either side of the page.
const (
	sidebarWidth = 236
	pagePad      = 32
)

// contentMax is the widest the page's column gets before it stops growing and
// centers itself. It is a measure, not a breakpoint: the window's own default
// is 1040 and the narrowest is 780, so on either of those the page still fills
// what it is given, and the cap only answers a window the reader stretched.
const contentMax = 1120

// contentWidth is the page's measure: the window less the sidebar and the
// page's own padding, capped at contentMax. Two pages lay columns out from the
// window's width — the matrix's account columns and the overview's cards — and
// both have to work out the same number, so it is worked out in one place.
func contentWidth(winW float32) float32 {
	inner := winW - sidebarWidth - 2*pagePad
	if inner > contentMax {
		inner = contentMax
	}
	return inner
}

// overviewCardMin is the narrowest a provider card is drawn: below it the card's
// numbers and bars stop being readable across the room. The overview fits as
// many cards side by side as this leaves room for, and lays them out in that
// many columns.
const overviewCardMin = 340

func (a *App) view(c *ui.Context) {
	for _, text := range a.spoken {
		c.Announce(text)
	}
	a.spoken = a.spoken[:0]
	p := a.theme(c)
	if a.toast != "" {
		// Toast announces itself to assistive technology, so it is held to the
		// same rule as errorLine: what it shows is a sentence this codebase
		// wrote. A library's own message goes to the log, which is where a
		// reader who wants the exact error is already looking.
		// The message is the ID, so saying the same thing twice replaces the
		// toast instead of stacking a second copy of it.
		c.Toast(a.toast)
		a.toast = ""
	}
	if a.Due() {
		a.Refresh(false)
	}
	ui.Column(c).
		Fill().
		LinearGradient(ui.LinearGradient{From: p.canvasTop, To: p.canvas, Angle: 180, Oklab: true}).
		Children(func() {
			a.titleBar(c, p)
			// The row below the titlebar is given its height outright: a
			// scroll inside it clamps against its own box, and a percentage
			// height would hand it the window's instead, which leaves the end
			// of a long page unreachable.
			_, winH := c.Size()
			ui.Row(c).Height(winH - a.titleBarHeight(c)).Children(func() {
				ui.Sidebar(c, &a.page, func() { a.sidebarItems(c, p) }).
					Width(sidebarWidth).
					FillHeight().
					Padding(12, 10).
					Material(glass.Glass{})
				ui.Scroll(c).
					Grow(1).
					FillHeight().
					Padding(26, pagePad, pageBottomPad, pagePad).
					Children(func() {
						// The measure. A window pulled across a wide display
						// is not a reason for a bar 1600px long or a number
						// 1500px from the label it belongs to: past this the
						// page stops growing and centers itself in what is
						// left, which is where the eye expects the column to
						// be. The app opens at 1040, so this only ever moves
						// on a window the reader stretched themselves.
						ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
							ui.Column(c).
								FillWidth().
								MaxWidth(contentMax).
								Gap(space5).
								Transition(pageMotion).
								Children(func() { a.pageView(c, p) })
						})
					})
			})
		})

	a.promptSheet(c, p)
	a.confirmSheet(c, p)
}

// pageMotion eases a page in when the sidebar switches to another. The page
// only fades: the cards inside it are what rises, so the two do not fight.
var pageMotion = ui.ElementTransition{
	Enter: &ui.Motion{Opacity: 1},
	Exit:  &ui.Motion{Opacity: 0},
}

// titleBar is the window's own title bar: the wordmark and the state of the
// last probe on the left, the refresh controls on the right, on glass.
// titleBarMin is the room the titlebar keeps for its own controls when the
// desktop does not ask for more.
const titleBarMin = 54

// titleBarHeight is how tall the titlebar is: its own content, or the room the
// desktop asks for its traffic lights.
func (a *App) titleBarHeight(c *ui.Context) float32 {
	if h := c.TitleBar().Height; h > titleBarMin {
		return h
	}
	return titleBarMin
}

func (a *App) titleBar(c *ui.Context, p palette) {
	tb := c.TitleBar()
	height := a.titleBarHeight(c)
	left := float32(space4)
	if tb.Left > 0 {
		left = tb.Left + space3
	}
	ui.Row(c).
		FillWidth().
		Height(height).
		Padding(0, space4, 0, left).
		Gap(space3).
		AlignItems(ui.Center).
		DragWindow().
		Material(glass.Glass{}).
		Children(func() {
			// The mark is ink, not accent: it is the app's name written
			// twice, and the one accent in the frame belongs to the one
			// action in it.
			ui.Icon(c, iconApp).FontSize(sizeChrome).TextColor(p.t.Text)
			ui.Text(c, "leeway").
				FontSize(sizeChrome).
				FontWeight(700).
				LetterSpacing(-0.2).
				Shrink(0)
			chromeRule(c, p)
			a.titleState(c, p)
			ui.Spacer(c)
			if !a.lastAt.IsZero() && !a.fetching {
				muted(c, p, agoText(a.lastAt, a.now())+"更新")
			}
			a.titleActions(c, p)
		})
}

// titleState is what the probes found: a dot and a count per state.
func (a *App) titleState(c *ui.Context, p palette) {
	ok, bad, missing := a.overview()
	type entry struct {
		n     int
		text  string
		color ui.Color
	}
	entries := []entry{
		{ok, "正常", p.toneColor(toneOK)},
		{bad, "失败", p.toneColor(toneCritical)},
		// A count is text, so it is held to 4.5:1 like the rest: at 0.5 alpha
		// it measured 2.02:1. The quiet one here is the tone, not the fade.
		{missing, "未配置", p.t.TextMuted},
	}
	busy := a.fetching || a.fetchErr != ""
	if a.fetching {
		entries = []entry{{0, "正在查询…", p.t.TextMuted}}
	} else if a.fetchErr != "" {
		entries = []entry{{0, "查询失败", p.toneColor(toneCritical)}}
	}
	shown := 0
	ui.Row(c).Gap(14).AlignItems(ui.Center).Role(ui.RoleStatus).Children(func() {
		for _, e := range entries {
			if e.n == 0 && !busy {
				continue
			}
			shown++
			entry := e
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				dot(c, entry.color)
				spans := []ui.Span{{Text: entry.text, Color: p.t.TextMuted}}
				if entry.n > 0 {
					spans = append([]ui.Span{{Text: fmt.Sprintf("%d ", entry.n), Weight: 600, Color: p.t.Text}}, spans...)
				}
				ui.RichText(c, spans...).FontSize(sizeBody).SingleLine()
			})
		}
		if shown == 0 {
			muted(c, p, "还没有添加服务商")
		}
	})
}

// titleActions is the refresh control, the interval menu and the settings
// button.
func (a *App) titleActions(c *ui.Context, p palette) {
	if a.fetching {
		titleControl(ui.Row(c).Gap(space1).AlignItems(ui.Center)).Children(func() {
			ui.Spinner(c).Size(14, 14)
		})
	} else if titleControl(ui.Button(c, "").Label("刷新")).Children(func() {
		ui.Icon(c, iconRefresh).FontSize(iconBody)
	}).Tooltip("刷新（⌘R）").Clicked() {
		a.Refresh(true)
	}
	titleControl(ui.MenuButton(c, refreshLabel(a.settings.IntervalSec), func(m *ui.Menu) {
		for _, opt := range refreshOptions {
			if m.Item(opt.label).Checked(a.settings.IntervalSec == opt.sec).Chosen() {
				a.settings.IntervalSec = opt.sec
				a.SaveSettings()
			}
		}
	})).Tooltip("自动刷新")
	if titleControl(ui.Button(c, "").Label("设置")).Children(func() {
		ui.Icon(c, iconSettings).FontSize(iconBody)
	}).Tooltip("设置（⌘,）").Clicked() {
		a.OpenSettings()
	}
}

// titleControl is one control in the title bar: the refresh button, the
// interval menu and the settings button share one padding and one corner, so
// the row reads as a cluster rather than three buttons that happen to be near.
func titleControl(e ui.Element) ui.Element {
	return e.Padding(6, 10).Radius(radiusControl).FontSize(sizeBody)
}

var refreshOptions = []struct {
	sec   int
	label string
}{
	{0, "手动刷新"},
	{30, "每 30 秒"},
	{60, "每分钟"},
	{300, "每 5 分钟"},
	{900, "每 15 分钟"},
}

func refreshLabel(sec int) string {
	for _, o := range refreshOptions {
		if o.sec == sec {
			return o.label
		}
	}
	return "自动刷新"
}

// sidebarItems lists the pages: the dashboard, one item per provider, and
// the pages about the data itself.
func (a *App) sidebarItems(c *ui.Context, p palette) {
	ui.SidebarSection(c, "用量", nil, func() {
		sidebarItem(c, "overview", iconOverview, "总览", a.page == "overview")
		// Only the subscriptions this machine actually has: a provider nobody
		// has set up is a row to read once in the first-run guide, not a page
		// to keep walking past.
		listed := 0
		for _, v := range a.providers() {
			view := v
			if !view.Enabled || !view.hasSubscription() {
				continue
			}
			listed++
			item := ui.SidebarItem(c, "provider:"+view.ID, nil, view.DisplayName)
			// The dot's shape and colour are the mark; the word rides on the
			// row so a keyboard or screen-reader user reads the state too.
			item = item.Tooltip(view.DisplayName + " · " + statusLabel(view.Status, view.firstCred()))
			item.Children(func() {
				// Every provider listed here has at least one subscription, so
				// the count is always worth showing: a row without a number
				// reads as "unknown", not as "one".
				if n := len(view.Accounts); n > 0 {
					// Text, not a badge: a filled chip 9px from the state dot
					// reads as a second state badge, and the count has no label
					// to say which is which.
					muted(c, p, fmt.Sprintf("%d", n))
				}
				// A dot means leeway has something to say: green is fine, red
				// is not, and a ring means it has not answered yet.
				statusDot(c, p, view.Status)
			})
		}
		if listed == 0 {
			muted(c, p, "配置好的服务商会显示在这里")
		}
	})
	ui.SidebarSection(c, "数据", nil, func() {
		sidebarItem(c, "history", iconHistory, "历史", a.page == "history")
		sidebarItem(c, "about", iconAbout, "关于", a.page == "about")
	})
}

// statusColor colors a provider's dot in the sidebar and its badge in a card.
func (p palette) statusColor(status string) ui.Color {
	if status == "disabled" {
		return p.t.Border
	}
	if status == "not_configured" {
		// The dot is the only mark a provider's state has in the sidebar, so
		// it is a graphic carrying meaning and is held to 3:1, which a faded
		// muted (1.87:1) did not reach. Grey still reads as "not configured"
		// beside a green or a red dot.
		return p.t.TextMuted
	}
	return p.toneColor(statusTone(status))
}

// pageView routes to the page the sidebar selected.
func (a *App) pageView(c *ui.Context, p palette) {
	switch {
	case a.page == "history":
		a.historyPage(c, p)
	case a.page == "about":
		a.aboutPage(c, p)
	case strings.HasPrefix(a.page, "provider:"):
		a.providerPage(c, p, strings.TrimPrefix(a.page, "provider:"))
	default:
		a.overviewPage(c, p)
	}
}

// staleNote says that the numbers below it are the last ones that came back,
// for the case where the probe that would have refreshed them failed. A cached
// value otherwise reads exactly like a live one, and the age of the data is a
// title bar away from the number it is about.
func staleNote(a *App) string {
	if a.fetchErr == "" || a.lastAt.IsZero() {
		return ""
	}
	return "下面是 " + agoText(a.lastAt, a.now()) + "的数据"
}

// pageHead is the title, subtitle and actions every page starts with.
func pageHead(c *ui.Context, p palette, name, subtitle string, actions func()) ui.Element {
	return pageHeadMarked(c, p, "", "", name, subtitle, actions)
}

// pageHeadMarked is a page head with a provider's mark in front of its name.
func pageHeadMarked(c *ui.Context, p palette, id, status, name, subtitle string, actions func()) ui.Element {
	return ui.Column(c).FillWidth().Gap(14).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.End).Gap(16).Children(func() {
			if id != "" {
				mark(c, p, id, name, 46, status)
			}
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				title(c, p, name)
				if subtitle != "" {
					muted(c, p, subtitle)
				}
			})
			if actions != nil {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(actions)
			}
		})
		hairline(c, p)
	})
}

// failureText is what a failure says: the app's sentence, and — when the
// message underneath is one of ours, written for a person to read — that
// message. A message with no Chinese in it is a library's error text, which
// goes on the tooltip instead: it names the failure, it does not explain it,
// and what fixes it is the button beside the line.
func failureText(sentence, message string) (string, string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return sentence, ""
	}
	// A quoted path is how an OS names a file it could not open, in Chinese
	// too — 无法打开"/Users/张三/auth.json"：没有那个文件或目录 — and such a
	// message is the machine's whole, wherever its colons fall.
	if hasQuotedPath(message) {
		return sentence, message
	}
	// One that leads with our words and trails a machine's — "写 Cursor
	// keychain 失败：exit status 51" — is split at that last colon, so the
	// sentence stays ours and the tail goes to the tooltip.
	if i := strings.LastIndex(message, "："); i > 0 && hasCJK(message[:i]) && tailIsAMachine(message[i+len("："):]) {
		return joinNonEmpty([]string{sentence, message[:i]}, " · "), strings.TrimSpace(message[i+len("："):])
	}
	if !hasCJK(message) || looksLikeAMachine(message) {
		return sentence, message
	}
	return joinNonEmpty([]string{sentence, message}, " · "), ""
}

// hasQuotedPath is the shape an OS uses to name the file it could not open:
// the path inside quotes, in either script's quotes.
func hasQuotedPath(message string) bool {
	return strings.Contains(message, `"/`) || strings.Contains(message, `"~`) ||
		strings.Contains(message, "“/") || strings.Contains(message, "“~")
}

// tailIsAMachine reports whether what follows a colon is a machine's words: a
// phrase with no Chinese in it, like "exit status 51" or "no such file or
// directory". Our own tails are Chinese — "官方工具没有写出登录信息", "写入 64 字节，
// 读回 32 字节" — and those belong in the sentence with the head that leads to
// them.
func tailIsAMachine(tail string) bool {
	tail = strings.TrimSpace(tail)
	return tail != "" && !hasCJK(tail) && len(strings.Fields(tail)) >= 2
}

// looksLikeAMachine reports whether a message is a machine's rather than a
// person's: the "…: no such file or directory" shape an OS error has, a bare
// exit status, or a path standing on its own. Language alone cannot tell them
// apart — macOS localises its errors, so "打开 /Users/张三/x 被拒绝" is a
// machine's message in Chinese — but the punctuation and the paths give it
// away. A path *inside* one of our sentences is not one of these: "先安装
// https://cursor.com/cli" and "先导入 ~/.grok/auth.json" are ours, and they
// stay on screen.
func looksLikeAMachine(message string) bool {
	if hasQuotedPath(message) {
		return true
	}
	// The "…: no such file or directory" shape, in either colon. The tail has
	// to be a machine's: "cookie 已过期: 请重新登录" is ours and stays.
	if i := strings.LastIndex(message, "："); i > 0 && tailIsAMachine(message[i+len("："):]) {
		return true
	}
	if i := strings.LastIndex(message, ": "); i >= 0 && tailIsAMachine(message[i+2:]) {
		return true
	}
	if strings.Contains(message, "exit status") {
		return true
	}
	if strings.HasPrefix(message, "/") || strings.HasPrefix(message, "~") {
		return true
	}
	if !hasCJK(message) {
		// "stat /Users/x failed": no Chinese at all, a path standing on its
		// own. The path has to start a word, or every URL would count.
		return hasBarePath(message)
	}
	// Chinese with a path in the *middle* of it, and no Latin anywhere outside
	// that path, is an OS error: macOS writes 打开 /Users/张三/x 被拒绝. One of
	// ours either ends on the path — "请先导入 ~/.grok/auth.json", "kimi 账号
	// "work" 的登录文件不在 /Users/…" — or names its tool in Latin letters:
	// "找到 3 个 Cursor state.vscdb（/a, /b）——用 LEEWAY_CURSOR_STATE_DB 指定
	// 要写哪一个", "不删除 /Users/…：它不是 leeway 的登录目录". Those stay.
	words := strings.Fields(message)
	for i, word := range words {
		if strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") {
			if i < len(words)-1 && !latinOutsidePaths(words) {
				return true
			}
			continue
		}
	}
	return false
}

// latinOutsidePaths reports whether a message names anything in Latin letters
// other than a path.
func latinOutsidePaths(words []string) bool {
	for _, word := range words {
		if strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") {
			continue
		}
		for _, r := range word {
			if r < utf8.RuneSelf && unicode.IsLetter(r) {
				return true
			}
		}
	}
	return false
}

// hasBarePath reports a path that starts a word, which is how an OS names one
// and how a URL does not.
func hasBarePath(message string) bool {
	for i := 1; i+1 < len(message); i++ {
		if message[i] != '/' || (message[i-1] != ' ' && message[i-1] != '"' && message[i-1] != '\'') {
			continue
		}
		if unicode.IsLetter(rune(message[i+1])) {
			return true
		}
	}
	return false
}

// errorLine is a failure the window is reporting. The sentence is the app's
// and the detail is whatever the layer underneath said — a Go error string,
// usually — which goes on the tooltip and never in the sentence's place: a
// window is not a log, and "unexpected EOF" in front of a reader explains
// nothing. What fixes the failure is a button beside this line, not a string.
func errorLine(c *ui.Context, p palette, text, detail string) ui.Element {
	return ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, iconAlert).FontSize(iconSmall).TextColor(p.toneColor(toneCritical)).Padding(1, 0, 0, 0)
		// Focusable so the detail is not pointer-only: MyGo shows a keyboard
		// tooltip as soon as the focus ring is visible.
		line := ui.Text(c, text).FontSize(sizeBody).TextColor(p.toneColor(toneCritical)).Grow(1).Focusable()
		if detail != "" && detail != text {
			line.Tooltip(detail)
		}
	})
}

// notConfiguredLine explains an account with no numbers yet: what is missing,
// or that a credential is already there and only a probe is.
func notConfiguredLine(c *ui.Context, p palette, id string, av accountView) {
	if av.Cred != "" {
		unconfiguredLine(c, p, credSentence(av.Cred))
		return
	}
	unconfiguredLine(c, p, providerHint(id))
}

// unconfiguredLine shows what to do about a provider with no credentials.
func unconfiguredLine(c *ui.Context, p palette, hint string) ui.Element {
	return muted(c, p, hint)
}

// windowFraction is how full a window is, and whether a bar means anything.
func windowFraction(w model.RateWindow) (float64, bool) {
	if w.UsedPercent != nil {
		pct := *w.UsedPercent
		if pct < 0 {
			pct = 0
		}
		return pct / 100, true
	}
	if w.Limit != nil && *w.Limit > 0 && w.Used != nil {
		return *w.Used / *w.Limit, true
	}
	return 0, false
}

// deltaText says how a window changed since the last probe, from the fetch's
// own comparison when it has one, from the recorded history otherwise.
func deltaText(points []history.Point, w model.RateWindow) string {
	if d := w.Delta; d != nil {
		switch {
		case d.UsedPercent != nil && *d.UsedPercent != 0:
			return fmt.Sprintf("比上次 %+.0f%%", *d.UsedPercent)
		case d.Used != nil && *d.Used != 0:
			return fmt.Sprintf("比上次 %+.2f", *d.Used)
		}
		return ""
	}
	if len(points) < 2 {
		return ""
	}
	last, prev := points[len(points)-1], points[len(points)-2]
	if last.Reset {
		// A reset is not a change of usage, and the number it makes (-42%)
		// is the one thing it does not mean. The reset note says it, with
		// the values on both sides.
		return ""
	}
	switch {
	case last.UsedPercent != nil && prev.UsedPercent != nil:
		if d := *last.UsedPercent - *prev.UsedPercent; d != 0 {
			return fmt.Sprintf("比上次 %+.0f%%", d)
		}
	case last.Used != nil && prev.Used != nil:
		if d := *last.Used - *prev.Used; d != 0 {
			return fmt.Sprintf("比上次 %+.2f", d)
		}
	}
	return ""
}

// accountActions is the menu of what can be done with one account. Switching
// the machine's own client only appears when that provider has one to switch.
func accountActions(c *ui.Context, a *App, p palette, providerID, account string) ui.Element {
	ready := accountSwitchReady(a, providerID, account)
	session := accountUsesSession(a, providerID, account)
	return btnSmall(ui.ButtonBase(c).Label("账号")).Cursor(ui.CursorPointer).
		Children(func() { ui.Icon(c, iconMore).FontSize(sizeChrome).TextColor(p.t.TextMuted) }).
		Menu(func(m *ui.Menu) {
			if canSwitchTo(providerID, ready) {
				if m.Item("启用").Chosen() {
					a.askUse(providerID, account)
				}
				m.Separator()
			}
			if m.Item("复制数据").Chosen() {
				mygo.Clipboard.WriteText(a.CopyJSON())
				a.toast = "已复制"
			}
			m.Separator()
			if m.Item(removeAccountLabel(session)).Chosen() {
				a.askLogout(providerID, account)
			}
		})
}

// removeAccountLabel names what removal actually removes: a session from
// leeway's own ledger, or just the stored key.
func removeAccountLabel(session bool) string {
	if session {
		return "移除账号"
	}
	return "删除密钥"
}

// loginActions opens the sheet that gives a provider credentials.
func loginActions(c *ui.Context, a *App, providerID, account string) ui.Element {
	return btnSmall(ui.MenuButton(c, loginButtonLabel(providerID), func(m *ui.Menu) {
		for _, method := range loginMethods(providerID) {
			if m.Item(method.label).Chosen() {
				method.open(a, providerID, account)
			}
		}
	}))
}

type loginMethod struct {
	label string
	open  func(a *App, providerID, account string)
}

// loginMethods lists what leeway can do for a provider, derived from the
// provider's own AuthSpec so the window never offers a credential kind the
// provider does not read.
func loginMethods(providerID string) []loginMethod {
	apiKey := loginMethod{"粘贴 API 密钥…", func(a *App, id, acc string) { a.openAPIKey(id, acc) }}
	cookie := loginMethod{"读取浏览器登录状态…", func(a *App, id, acc string) { a.openCookieImport(id, acc) }}
	manual := loginMethod{"手动粘贴登录状态…", func(a *App, id, acc string) { a.openManualCookie(id, acc) }}
	file := loginMethod{"导入登录文件…", func(a *App, id, acc string) { a.openSessionFile(id, acc) }}
	cli := loginMethod{"用官方工具登录…", func(a *App, id, acc string) { a.openCLI(id, acc) }}

	spec, ok := providerSpec(providerID)
	if !ok {
		return []loginMethod{apiKey}
	}
	var out []loginMethod
	if spec.OfficialCLI {
		out = append(out, cli)
	}
	if len(spec.EnvAPIKey) > 0 || len(spec.EnvAuthToken) > 0 {
		out = append(out, apiKey)
	}
	if spec.UsesCookies {
		out = append(out, cookie, manual)
	}
	// `leeway login <id> --file` takes a session file for exactly the four
	// with an official CLI (saveAccountAuthFile); cursor imports its auth.json
	// through the same path even though it declares no LocalAuthFiles.
	if spec.OfficialCLI || spec.UsesCodexCLI || len(spec.LocalAuthFiles) > 0 {
		out = append(out, file)
	}
	if len(out) == 0 {
		out = append(out, apiKey)
	}
	return out
}

// loginButtonLabel says what the button really does: sign in through the
// official CLI, or fill in the key that queries quota.
func loginButtonLabel(providerID string) string {
	spec, ok := providerSpec(providerID)
	if ok && (spec.OfficialCLI || spec.UsesCodexCLI) {
		return "登录"
	}
	return "设置密钥"
}

// providerSpec is the provider's declared credential kinds.
func providerSpec(providerID string) (provider.AuthSpec, bool) {
	p, ok := provider.Get(providerID)
	if !ok {
		return provider.AuthSpec{}, false
	}
	return p.AuthSpec(), true
}

// canSwitchTo reports whether `启用` can do anything for a provider at
// all: only the three with an official local client. Codex deliberately never
// writes ~/.codex, and key-only providers have no client of their own.
func canSwitchTo(providerID string, ready bool) bool {
	switch providerID {
	case "cursor", "grok", "kimi":
		return ready
	}
	return false
}

// accountUsesSession is true when removing this account also removes a login
// rather than only a stored key: the providers that log in through an official
// CLI keep an isolated session, and any account with a cookie has one cached.
func accountUsesSession(a *App, providerID, account string) bool {
	spec, ok := providerSpec(providerID)
	if !ok {
		return false
	}
	if spec.OfficialCLI || spec.UsesCodexCLI {
		return true
	}
	if !spec.UsesCookies || a.cfg == nil {
		return false
	}
	if acc, ok := a.cfg.Account(providerID, account); ok {
		return acc.CookieSource != "" || acc.ManualCookieHeader != ""
	}
	pc := a.cfg.Provider(providerID)
	return pc.CookieSource != "" || pc.ManualCookieHeader != ""
}

// providerHint is the sentence under a provider's name when it has no
// credentials yet.
func providerHint(id string) string {
	switch id {
	case "deepseek", "opencode-go", "commandcode":
		return "还没有 API 密钥。"
	case "cursor":
		return "还没有登录信息。先用官方 cursor-agent 登录。"
	case "grok":
		return "还没有登录信息。用官方 grok 登录，或导入 ~/.grok/auth.json。"
	case "codex":
		return "还没有登录信息。用官方 codex 登录，或导入 ~/.codex/auth.json。"
	case "kimi":
		return "还没有登录信息。三种都行：官方 kimi 登录、API 密钥、浏览器登录状态。"
	}
	return "还没有登录信息。"
}

// sinceStart renders how long the last probe took.
func sinceStart(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return fmt.Sprintf("用时 %.1f 秒", d.Seconds())
}
