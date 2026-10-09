package gui

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"leeway/internal/history"
	"leeway/internal/model"
)

// overviewPage is the dashboard: the provider closest to its limit leads, the
// rest follow in a grid, and whatever needs a hand gets its own list, so the
// page has a shape instead of a uniform field of cards.
func (a *App) overviewPage(c *ui.Context, p palette) {
	// The title bar already says what the probes found, so the head carries
	// what it does not: how long the last probe took.
	sub := ""
	if took := sinceStart(a.lastTook); took != "" && !a.fetching {
		sub = took
	}
	pageHead(c, p, "总览", sub, func() {
		if ui.PrimaryButton(c, "刷新").Disabled(a.fetching).Clicked() {
			a.Refresh(true)
		}
	})
	if a.fetchErr != "" {
		text, detail := failureText(joinNonEmpty([]string{"这次探测没有答上来", staleNote(a)}, " · "), a.fetchErr)
		errorLine(c, p, text, detail)
	}

	list := a.providers()
	if len(list) == 0 {
		card(c, p, func() {
			body(c, p, "没有可用的 provider。")
		})
		return
	}

	var ready, attention []providerView
	for _, v := range list {
		if v.Enabled && v.Status == "ok" && hasWindows(v) {
			ready = append(ready, v)
			continue
		}
		attention = append(attention, v)
	}

	// The first probe is still out: the page shows the shape the answer will
	// land in rather than a guide it is about to take away.
	if a.out == nil && a.fetching {
		a.probingLine(c, p)
		// Only the providers this machine has a credential for can answer with
		// a number; the rest are rows in 需要处理. Counting them first is what
		// lets the skeleton be the page that is about to land.
		enabled, bare := 0, 0
		for _, v := range list {
			if !v.Enabled {
				continue
			}
			if v.hasSubscription() {
				enabled++
			} else {
				bare++
			}
		}
		// Nothing here has a credential, so no probe can come back with a
		// number: the answer is the guide, and drawing it now means the page
		// does not change under the reader when the probe lands.
		if enabled == 0 {
			a.startGuide(c, p)
			return
		}
		a.skeletonHero(c, p)
		// As many cards as there are providers left to fill them: the answer
		// will have one lead card and the rest here, so the page settles into
		// the shape it was already showing instead of reflowing. Which of them
		// will need a hand is the probe's answer, so the skeleton shows only
		// what every answer has.
		if bare > 0 {
			a.skeletonAttention(c, p)
		}
		if enabled > 1 {
			ui.Row(c).FillWidth().Wrap().Gap(space4).AlignItems(ui.Start).Children(func() {
				for i := 1; i < enabled; i++ {
					a.skeletonCard(c, p).MinWidth(340).Grow(1)
				}
			})
		}
		return
	}

	// Nothing answered and nothing failed: the window has a first run to
	// explain, and the guide lists every provider already.
	answered := false
	for _, v := range list {
		if v.Enabled && (v.Status == "ok" || v.Status == "error") {
			answered = true
			break
		}
	}
	firstRun := len(ready) == 0 && !answered
	if firstRun {
		a.startGuide(c, p)
	}
	if len(ready) > 0 {
		lead := mostConstrained(ready)
		a.heroCard(c, p, lead)
		// What needs a hand comes before the providers that are merely fine.
		// Below the grid it was the one card the window cut in half, and a
		// card at the bottom of a page is a card nobody acts on.
		if len(attention) > 0 && !firstRun {
			a.attentionCard(c, p, attention)
		}
		var rest []providerView
		for _, v := range ready {
			if v.ID != lead.ID {
				rest = append(rest, v)
			}
		}
		if len(rest) > 0 {
			ui.Row(c).FillWidth().Wrap().Gap(space4).AlignItems(ui.Start).Children(func() {
				for i := range rest {
					a.overviewCard(c, p, rest[i]).MinWidth(340).Grow(1)
				}
			})
		}
		return
	}
	if len(attention) > 0 && !firstRun {
		a.attentionCard(c, p, attention)
	}
}

// heroCard is the provider the page leads with, at twice the size of the
// others: its mark, its numbers, and a bar you can read across the room.
func (a *App) heroCard(c *ui.Context, p palette, v providerView) ui.Element {
	open := func() { a.navigate("provider:" + v.ID) }
	return a.openCard(c, p, "查看 "+v.DisplayName, open, shell(c, p, func() {
		ui.Row(c).FillWidth().Gap(space4).AlignItems(ui.Center).Children(func() {
			mark(c, p, v.ID, v.DisplayName, 46, v.Status)
			ui.Column(c).Grow(1).Gap(space1).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(sizeValue).
					FontWeight(700).
					LetterSpacing(-0.4).
					SingleLine()
				muted(c, p, providerIdentity(v))
			})
			// The whole plate is the target, so the chevron is the card's
			// own mark rather than a second button inside the first.
			ui.Icon(c, iconChevron).FontSize(iconBody).TextColor(p.t.TextMuted).Role(ui.RoleNone)
		})
		for i := range v.Accounts {
			a.heroAccount(c, p, v, v.Accounts[i], open)
		}
	}))
}

// heroAccount is one account's headline window inside the lead card: the
// window closest to its limit, its number, its bar, its recent history, and
// the facts that support them, in that order.
func (a *App) heroAccount(c *ui.Context, p palette, v providerView, av accountView, open func()) {
	if av.Snapshot == nil {
		return
	}
	windows := displayWindows(av.Snapshot)
	if len(windows) == 0 {
		return
	}
	lead := fullestWindow(windows)
	points := a.historyFor(v.ID, av.Name, lead.ID)
	ui.Column(c).FillWidth().Gap(space4).Children(func() {
		if len(v.Accounts) > 1 {
			ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
				label(c, p, displayAccount(av.Name))
				if av.Email != "" && av.Email != av.Name {
					muted(c, p, av.Email)
				}
			})
		}
		a.heroWindow(c, p, v.ID, lead, points, open)
		if len(points) >= 2 {
			ui.Column(c).FillWidth().Gap(space2).Children(func() {
				_, over := a.sparkline(c, p, "overview-trend", "用量走势："+windowTitle(v.ID, lead.ID, lead.Title), points, p.toneColor(windowTone(lead)), 54, open)
				summary := joinNonEmpty([]string{chartScale(points), trendCaption(points, lead)}, " · ")
				muted(c, p, chartCaption(points, over, summary))
				// The sentence about running out sits with the chart it is
				// read from, not beside a balance where it competes with one.
				burnAlarmNoReset(c, p, points, lead)
				resetNote(c, p, points)
			})
		}
		a.heroFooter(c, p, v.ID, windows, lead)
	})
}

// heroWindow is the lead card's headline: the number, what it counts and when
// the window comes back, with the bar all three are read against directly
// under them, so the block has no band of nothing in the middle of it.
func (a *App) heroWindow(c *ui.Context, p palette, providerID string, w model.RateWindow, points []history.Point, open func()) {
	ui.Column(c).FillWidth().Gap(space2).Children(func() {
		line := ui.Row(c).FillWidth().AlignItems(ui.End).Gap(space3)
		line.Children(func() {
			hero(c, p, tweenText(w, tweenValue(line, "value", windowValue(w))), p.amountColor(w))
			// The scope sits on the number's baseline: at this size a label
			// under the number reads as a caption, and the two together read
			// as one figure.
			ui.Column(c).Gap(space1).Padding(0, 0, space1, 0).Children(func() {
				ui.Text(c, windowTitle(providerID, w.ID, w.Title)).
					FontSize(sizeBody).
					FontWeight(600).
					SingleLine()
				if detail := amountDetail(w); detail != "" {
					muted(c, p, detail)
				}
			})
			ui.Spacer(c)
			if meta := resetMeta(w, a.now()); meta != "" {
				muted(c, p, meta)
			}
		})
		fraction, ok := windowFraction(w)
		if !ok {
			return
		}
		projection := 0.0
		if burn, burning := burnOf(points, &w); burning {
			// Where the line is heading by the time the window comes back.
			projection = (*w.UsedPercent + burn.rate*burn.left.Hours()) / 100
		}
		a.heroBar(c, p, w, fraction, projection)
	})
}

// The marks a quota bar draws over its own track. They are named here because
// the contrast test reads the same two values: a literal written twice is a
// pair that cannot fail.
const (
	barTickAlpha = 0.85
	barRunAlpha  = 0.85
)

// heroBar is the lead card's bar: the same track and fill as every other bar,
// with the scale the number is read against, and — when the series is on
// course to run out before the window comes back — the mark it is heading for.
func (a *App) heroBar(c *ui.Context, p palette, w model.RateWindow, fraction, projection float64) {
	e := bar(c, p, fraction, barColor(p, w), 10)
	// The fill eases to its new width; the scale and the mark are read
	// against where the fill is now, so they move with it.
	fill := tweenValue(e, "fill", fraction)
	alarm := p.toneColor(toneCritical)
	e.DrawOver(func(pt *ui.Painter, r ui.Rect) {
		// The quarter marks are a scale the reader measures against, so they
		// are a graphic carrying meaning and are held to 3:1 against what they
		// are drawn on: over the track, 0.85 of the muted measures 3.13:1.
		// They cross the fill as well, where a lighter line on a darker one is
		// no reading of its own — the scale is read against the empty bar, and
		// stopping the marks at the fill would leave a scale with a gap in it.
		for i := 1; i < 4; i++ {
			x := r.X + r.W*float32(i)/4
			pt.Fill(ui.Rect{X: x, Y: r.Y + 1, W: 1, H: r.H - 2}, p.t.TextMuted.Alpha(barTickAlpha), 0)
		}
		if projection <= 0 {
			return
		}
		// The mark is where the projection crosses the bar, and the run from
		// the fill to it is what the window has left to give.
		mark := r.X + r.W*float32(min(max(projection, 0), 1)) - 1
		mark = min(mark, r.X+r.W-3)
		from := r.X + r.W*float32(min(fill, 1))
		for x := from; x < mark-2; x += 7 {
			// The dashes say how much is left before the window runs out: at
			// 0.5 alpha they measured 2.08:1 against the bar, and a mark the
			// sentence below refers to is a graphic, not a tint.
			pt.Fill(ui.Rect{X: x, Y: r.Y + r.H/2 - 1, W: min(3, mark-2-x), H: 2}, alarm.Alpha(barRunAlpha), 1)
		}
		pt.Fill(ui.Rect{X: mark, Y: r.Y + 1.5, W: 2, H: r.H - 3}, alarm, 1)
	})
}

// heroFooter is the lead card's last row: what the provider's other windows
// are worth, under the card's own rule. The lead window's own number, bar and
// countdown are read above it, so nothing here says them again.
func (a *App) heroFooter(c *ui.Context, p palette, providerID string, windows []model.RateWindow, lead model.RateWindow) {
	var rest []model.RateWindow
	for _, w := range windows {
		if w.ID != lead.ID {
			rest = append(rest, w)
		}
	}
	if len(rest) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(space3).Children(func() {
		hairline(c, p)
		ui.Row(c).FillWidth().Wrap().Gap(space4).AlignItems(ui.Center).Children(func() {
			for _, w := range rest {
				ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
					ui.Text(c, windowTitle(providerID, w.ID, w.Title)).
						FontSize(sizeBody).
						FontWeight(600).
						SingleLine()
					if detail := amountDetail(w); detail != "" {
						muted(c, p, detail)
					}
					chrome(c, p, amountText(w))
				})
			}
		})
	})
}

// resetMeta is the hero's right-hand line: when the window resets, and
// whether the provider says it is rate limiting. The trend caption below the
// chart carries the change, so it is not repeated here.
func resetMeta(w model.RateWindow, now time.Time) string {
	var parts []string
	if r := resetLabel(w, now); r != "" {
		parts = append(parts, r)
	}
	if w.RateLimited {
		parts = append(parts, "provider 报告已限流")
	}
	return strings.Join(parts, " · ")
}

// trendCaption says what the hero's chart covers: how many probes, over how
// long, and where the window moved.
func trendCaption(points []history.Point, w model.RateWindow) string {
	parts := []string{fmt.Sprintf("近 %d 次探测", len(points))}
	// The chart scales itself to the series, so it has to say what its ends
	// are worth: otherwise a climb of two points and a climb of forty draw
	// the same picture.
	if r := trendRange(points); r != "" {
		parts = append(parts, r)
	}
	if span := spanText(points[len(points)-1].T.Sub(points[0].T)); span != "" {
		parts = append(parts, "覆盖 "+span)
	}
	if d := deltaText(points, w); d != "" {
		parts = append(parts, d)
	}
	return strings.Join(parts, " · ")
}

// fullestWindow picks the window a card leads with: the one closest to its
// limit, which is the one worth looking at.
func fullestWindow(windows []model.RateWindow) model.RateWindow {
	best, bestFill := windows[0], -1.0
	for _, w := range windows {
		if f, ok := windowFraction(w); ok && f > bestFill {
			best, bestFill = w, f
		}
	}
	return best
}

// barColor is the color of a window's bar: its level, or the money color when
// it is money and so has no level of its own. Money is data, and the accent is
// what the window's actions are drawn in — a series in the accent reads as a
// thing you can press.
func barColor(p palette, w model.RateWindow) ui.Color {
	if windowTone(w) == toneNeutral {
		return p.money
	}
	return p.toneColor(windowTone(w))
}

// overviewCard is one provider's summary in the grid under the lead card.
func (a *App) overviewCard(c *ui.Context, p palette, v providerView) ui.Element {
	return a.openCard(c, p, "查看 "+v.DisplayName, func() {
		a.navigate("provider:" + v.ID)
	}, shell(c, p, func() {
		ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
			mark(c, p, v.ID, v.DisplayName, 32, v.Status)
			ui.Column(c).Grow(1).Gap(space1).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(sizeBody).
					FontWeight(600).
					SingleLine()
				muted(c, p, providerIdentity(v))
			})
			ui.Icon(c, iconChevron).FontSize(iconSmall).TextColor(p.t.TextMuted).Role(ui.RoleNone)
		})
		// Two subscriptions of one provider are two blocks: each account gets
		// its own rule and its own name, so a card never reads as one
		// paragraph about nobody in particular.
		for i := range v.Accounts {
			if i > 0 {
				hairline(c, p)
			}
			a.overviewAccount(c, p, v, v.Accounts[i])
		}
	}))
}

// openCard makes a whole card the target that opens its provider, and names
// it for assistive technology. A card that lightens under the pointer and
// answers nowhere is a lie, so the plate itself takes the click, and it
// answers the press as well as the hover. OnClick is MyGo's clickable path:
// it registers the click and marks the element as one that takes the pointer,
// Enter and Space — so the window's front door opens from the keyboard too,
// not only from the mouse.
func (a *App) openCard(c *ui.Context, p palette, label string, open func(), e ui.Element) ui.Element {
	e.Label(label).Role(ui.RoleButton).Cursor(ui.CursorPointer).Focusable().OnClick(open)
	if e.Pressed() {
		e.DrawOver(func(pt *ui.Painter, r ui.Rect) {
			pt.Fill(r, p.t.Text.Alpha(0.05), radiusTray)
		})
	}
	return e
}

// probingLine says what the window is doing while the first probe is out: a
// page that will have cards in a moment says so, rather than showing a guide
// it is about to take away.
func (a *App) probingLine(c *ui.Context, p palette) ui.Element {
	return ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c).Size(iconBody, iconBody)
		muted(c, p, "正在探测…")
	})
}

// placeholder is a bar standing in for a number that has not arrived.
func placeholder(c *ui.Context, p palette, w, h, radius float32) ui.Element {
	e := ui.Box(c).Height(h).Radius(radius).Background(p.track)
	if w > 0 {
		return e.Width(w)
	}
	return e.FillWidth()
}

// skeletonHero is the lead card's shape before its numbers arrive: the same
// tray, the same mark and the same bars in the same places, so the page does
// not jump when the probe answers.
func (a *App) skeletonHero(c *ui.Context, p palette) ui.Element {
	return card(c, p, func() {
		ui.Row(c).FillWidth().Gap(space4).AlignItems(ui.Center).Children(func() {
			placeholder(c, p, 46, 46, 12)
			ui.Column(c).Grow(1).Gap(space2).Children(func() {
				placeholder(c, p, 132, 14, 7)
				placeholder(c, p, 180, 10, 5)
			})
		})
		placeholder(c, p, 150, 40, 12)
		placeholder(c, p, 0, 10, 5)
		// The card's last row: the provider's other windows under its rule.
		// How many windows the answer brings is the probe's business, but the
		// row itself is what the common answer has, and reserving it is what
		// keeps the page from jumping when it lands.
		ui.Column(c).FillWidth().Gap(space3).Children(func() {
			hairline(c, p)
			ui.Row(c).FillWidth().Gap(space4).AlignItems(ui.Center).Children(func() {
				placeholder(c, p, 66, 12, 6)
				placeholder(c, p, 92, 12, 6)
			})
		})
	})
}

// skeletonAttention is the shape of the 需要处理 card before the probe answers.
// It has one row and no more: which providers are in it is the probe's answer,
// and only the card itself is certain (there is at least one provider with no
// credential). A second row would be a guess — a credential that fails adds
// one, a credential that answers does not — and a guess that is wrong pushes
// the page down by more than the row it reserved.
func (a *App) skeletonAttention(c *ui.Context, p palette) ui.Element {
	return card(c, p, func() {
		section(c, p, "需要处理")
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			placeholder(c, p, 28, 28, 8)
			placeholder(c, p, 96, 12, 6)
			placeholder(c, p, 52, 16, 8)
			ui.Spacer(c)
			placeholder(c, p, 76, 28, 8)
		})
	})
}

// skeletonCard is a grid card before its numbers arrive.
func (a *App) skeletonCard(c *ui.Context, p palette) ui.Element {
	return card(c, p, func() {
		ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
			placeholder(c, p, 32, 32, 9)
			ui.Column(c).Grow(1).Gap(space2).Children(func() {
				placeholder(c, p, 96, 12, 6)
				placeholder(c, p, 132, 10, 5)
			})
		})
		placeholder(c, p, 0, 12, 6)
		placeholder(c, p, 0, 10, 5)
	})
}

// overviewAccount is one account inside a grid card.
func (a *App) overviewAccount(c *ui.Context, p palette, v providerView, av accountView) {
	if len(v.Accounts) > 1 {
		accountCaption(c, p, av)
	}
	switch {
	case av.Status == "ok" && av.Snapshot != nil:
		windows := displayWindows(av.Snapshot)
		if len(windows) == 0 {
			unconfiguredLine(c, p, "provider 没有报告任何额度窗口。")
		}
		for _, w := range windows {
			a.windowRow(c, p, v.ID, w, a.historyFor(v.ID, av.Name, w.ID), 0)
		}
	default:
		if av.Error != "" {
			text, detail := failureText("这个账号这次没有答上来", av.Error)
			errorLine(c, p, text, detail)
		} else {
			unconfiguredLine(c, p, accountHint(v.ID, av))
		}
		// What the account needs comes after what it says, at the end of its
		// own block, where the rest of the window puts a row's actions.
		ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
			if av.Status != "ok" {
				if btnSmall(ui.Button(c, "重试")).Clicked() {
					a.Refresh(true)
				}
			}
			loginActions(c, a, v.ID, av.Name)
		})
	}
}

// accountCaption names one account inside a card that holds more than one,
// with the address that is the one thing telling two accounts apart.
func accountCaption(c *ui.Context, p palette, av accountView) ui.Element {
	return ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
		label(c, p, displayAccount(av.Name))
		if av.Email != "" && av.Email != av.Name {
			ui.Text(c, av.Email).FontSize(sizeLabel).TextColor(p.t.TextMuted).SingleLine()
		}
	})
}

// accountHint is the one-line reason an account has no numbers.
func accountHint(id string, av accountView) string {
	if av.Cred != "" {
		return credSentence(av.Cred)
	}
	return providerHint(id)
}

// attentionCard lists what needs a hand: providers that failed or were
// switched off, one row each because each has its own reason, and then — in a
// single row — the ones that simply have no credentials yet. Those are one
// situation with one answer, and three rows repeating the same sentence would
// be three rows of the same sentence pushing the providers that are fine off
// the screen.
func (a *App) attentionCard(c *ui.Context, p palette, list []providerView) ui.Element {
	var broken, waiting, bare []providerView
	for _, v := range list {
		switch {
		case !v.Enabled || v.Status == "error":
			broken = append(broken, v)
		case !v.hasSubscription():
			bare = append(bare, v)
		case v.Status != "ok":
			// A credential is here but no answer came back: the header counts
			// it as 正常 because nothing failed, and this card is where the
			// reader learns it has not answered yet. It cannot go in the row
			// below, whose pill says 未配置 — that would be the same window
			// saying two things about one provider.
			waiting = append(waiting, v)
		}
	}
	return card(c, p, func() {
		section(c, p, "需要处理")
		first := true
		row := func(fn func()) {
			if !first {
				hairline(c, p)
			}
			first = false
			fn()
		}
		for i := range broken {
			v := broken[i]
			row(func() { a.attentionRow(c, p, v) })
		}
		for i := range waiting {
			v := waiting[i]
			row(func() { a.attentionRow(c, p, v) })
		}
		if len(bare) > 0 {
			row(func() { a.unconfiguredRow(c, p, bare) })
		}
	})
}

// unconfiguredRow is the one row for every provider with no credentials yet.
// The mark column stays empty — the row is about several providers, so no one
// provider's mark belongs in it — and the names line up with the names above.
func (a *App) unconfiguredRow(c *ui.Context, p palette, list []providerView) {
	names := make([]string, 0, len(list))
	for _, v := range list {
		names = append(names, v.DisplayName)
	}
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(28)
		// SingleLine, not MaxLines: a row with more in it than fits has to
		// truncate its words rather than stack them one character per line.
		ui.Text(c, strings.Join(names, "、")).
			FontSize(sizeBody).
			FontWeight(600).
			SingleLine().
			Tooltip(strings.Join(names, "、"))
		pill(c, p, "未配置", toneNeutral)
		// No sentence about the state: the pill says 未配置 twice already if
		// this row repeats it, and the button says what to do about it.
		ui.Spacer(c)
		if btnSmall(ui.Button(c, "去设置")).Clicked() {
			// The settings window's Provider tab is where each of them is
			// configured, one 详情 at a time.
			a.settingsTab = 1
			a.OpenSettings()
		}
	})
}

// attentionRow is one provider that needs something: its mark, its name, its
// state, why it needs a hand, and the two things that can be done about it —
// all on one line. The reason is the part that gives way, because the name,
// the state and the buttons are what the row is for; a list of these has to
// stay short enough that the providers that are fine are still on the screen.
func (a *App) attentionRow(c *ui.Context, p palette, v providerView) {
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		mark(c, p, v.ID, v.DisplayName, 28, v.Status)
		ui.Text(c, v.DisplayName).
			Role(ui.RoleHeading).
			Level(2).
			FontSize(sizeBody).
			FontWeight(600).
			SingleLine()
		pill(c, p, statusLabel(v.Status, v.firstCred()), statusTone(v.Status))
		ui.Box(c).Grow(1).Clip().Children(func() { attentionLine(c, p, v) })
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			switch {
			case !v.Enabled:
				if btnSmall(ui.Button(c, "启用")).Clicked() {
					a.Enable(v.ID, true)
				}
			case v.Status == "error":
				retry := btnSmall(ui.Button(c, "重试"))
				// The line beside this carries the same text as a tooltip, but
				// it is not focusable — a keyboard or screen-reader user only
				// reaches the detail through the control that fixes it.
				if d := providerError(v); d != "" && d != attentionReason(v) {
					retry = retry.Tooltip(attentionReason(v) + " · " + d)
				}
				if retry.Clicked() {
					a.Refresh(true)
				}
				loginActions(c, a, v.ID, defaultAccount(v))
			default:
				loginActions(c, a, v.ID, defaultAccount(v))
			}
		})
	})
}

// attentionLine is the reason a provider needs a hand: the app's own sentence,
// then whatever the provider said, in its own words.
func attentionLine(c *ui.Context, p palette, v providerView) ui.Element {
	reason := attentionReason(v)
	detail := providerError(v)
	// The same rule as failureText: our own words (Chinese) are shown, a
	// machine's go on the tooltip. This had its own copy of the rule — plain
	// hasCJK — and so leaked an OS error whose path happened to be Chinese.
	if !hasCJK(detail) || looksLikeAMachine(detail) || detail == reason {
		return ui.Text(c, reason).
			FontSize(sizeBody).
			TextColor(p.t.TextMuted).
			MaxLines(1).
			Tooltip(joinNonEmpty([]string{reason, detail}, " · "))
	}
	return ui.RichText(c,
		ui.Span{Text: reason + " · ", Color: p.t.TextMuted},
		// Full muted, not a faded muted: this is text, and text is held to
		// 4.5:1 — at 0.75 alpha it measured 3.11:1 on a card and 4.06:1 in
		// dark, under the bar in both.
		ui.Span{Text: detail, Font: p.mono, Color: p.t.TextMuted},
	).FontSize(sizeBody).MaxLines(1).Tooltip(reason + " · " + detail)
}

// providerError is the provider's own error text, when it reported one.
func providerError(v providerView) string {
	if v.Error != "" {
		return v.Error
	}
	for _, av := range v.Accounts {
		if av.Error != "" {
			return av.Error
		}
	}
	return ""
}

// attentionReason says, in one line, what a provider that needs a hand is
// waiting for.
func attentionReason(v providerView) string {
	if !v.Enabled {
		return "已在设置里停用，不会参与探测。"
	}
	if v.Status == "error" {
		// What to do about it is the two buttons on the row; what happened is
		// the provider's own sentence, appended after this one.
		return "探测失败"
	}
	if av, ok := firstAccount(v); ok {
		if av.Cred != "" {
			return credSentence(av.Cred)
		}
	}
	return providerHint(v.ID)
}

// defaultAccount is the account an action applies to when the provider has
// several: the one leeway probes first.
func defaultAccount(v providerView) string {
	if av, ok := firstAccount(v); ok {
		return av.Name
	}
	return ""
}

func firstAccount(v providerView) (accountView, bool) {
	if len(v.Accounts) == 0 {
		return accountView{}, false
	}
	return v.Accounts[0], true
}

// providerIdentity is the line under a provider's name: who it is for.
func providerIdentity(v providerView) string {
	av, ok := firstAccount(v)
	if !ok {
		return statusText(v.Status)
	}
	var parts []string
	if av.Name != "" && av.Name != "default" {
		parts = append(parts, displayAccount(av.Name))
	}
	if av.Email != "" && av.Email != av.Name {
		parts = append(parts, av.Email)
	}
	if av.Plan != "" {
		parts = append(parts, av.Plan)
	}
	if len(parts) == 0 {
		return statusText(v.Status)
	}
	return strings.Join(parts, " · ")
}

// windowMeta is the line under a window's bar: when it resets, and how it
// moved since the probe before.
func windowMeta(w model.RateWindow, points []history.Point, now time.Time) string {
	var parts []string
	if r := resetLabel(w, now); r != "" {
		parts = append(parts, r)
	}
	if d := deltaText(points, w); d != "" {
		parts = append(parts, d)
	}
	if w.RateLimited {
		parts = append(parts, "provider 报告已限流")
	}
	return strings.Join(parts, " · ")
}

// hasWindows reports whether a provider has numbers to lead with.
func hasWindows(v providerView) bool {
	for _, av := range v.Accounts {
		if av.Snapshot == nil {
			continue
		}
		if len(av.Snapshot.Windows) > 0 || av.Snapshot.Cost != nil {
			return true
		}
	}
	return false
}

// mostConstrained picks the provider whose fullest window is closest to its
// limit: the one the page should lead with.
func mostConstrained(list []providerView) providerView {
	best, bestFill := list[0], -1.0
	for _, v := range list {
		for _, av := range v.Accounts {
			if av.Snapshot == nil {
				continue
			}
			for _, w := range av.Snapshot.Windows {
				if f, ok := windowFraction(w); ok && f > bestFill {
					best, bestFill = v, f
				}
			}
		}
	}
	return best
}

// displayWindows picks the lanes a card shows: the first two, which is what
// providers order as primary, secondary and extras.
func displayWindows(snap *model.Snapshot) []model.RateWindow {
	windows := snap.Windows
	if len(windows) > 2 {
		windows = windows[:2]
	}
	return windows
}
