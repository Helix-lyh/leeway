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
	ok, bad, missing := a.overview()
	sub := fmt.Sprintf("%d 正常 · %d 失败 · %d 未配置", ok, bad, missing)
	if took := sinceStart(a.lastTook); took != "" && !a.fetching {
		sub += " · " + took
	}
	a.arrival.hold(c, a.now(), func() {
		pageHead(c, p, "总览", sub, func() {
			if ui.PrimaryButton(c, "刷新").Disabled(a.fetching).Clicked() {
				a.Refresh(true)
			}
		})
	})
	if a.fetchErr != "" {
		errorLine(c, p, "探测失败："+a.fetchErr)
	}

	list := a.providers()
	if len(list) == 0 {
		shell(c, p, func() {
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

	// Nothing answered and nothing failed: the window has a first run to
	// explain, and the guide lists every provider already.
	firstRun := len(ready) == 0 && ok == 0 && bad == 0
	if firstRun {
		a.arrival.hold(c, a.now(), func() { a.startGuide(c, p) })
	}
	if len(ready) > 0 {
		lead := mostConstrained(ready)
		a.arrival.hold(c, a.now(), func() { a.heroCard(c, p, lead) })
		var rest []providerView
		for _, v := range ready {
			if v.ID != lead.ID {
				rest = append(rest, v)
			}
		}
		if len(rest) > 0 {
			a.arrival.hold(c, a.now(), func() {
				ui.Row(c).FillWidth().Wrap().Gap(16).AlignItems(ui.Start).Children(func() {
					for i := range rest {
						a.overviewCard(c, p, rest[i]).MinWidth(340).Grow(1)
					}
				})
			})
		}
	}
	if len(attention) > 0 && !firstRun {
		a.arrival.hold(c, a.now(), func() { a.attentionCard(c, p, attention) })
	}
}

// heroCard is the provider the page leads with, at twice the size of the
// others: its mark, its numbers, and a bar you can read across the room.
func (a *App) heroCard(c *ui.Context, p palette, v providerView) *ui.Element {
	return shell(c, p, func() {
		ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Center).Children(func() {
			mark(c, p, v.ID, v.DisplayName, 46, v.Status)
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(19).
					FontWeight(650).
					LetterSpacing(-0.4).
					SingleLine()
				muted(c, p, providerIdentity(v))
			})
			iconButton(c, p, "查看 "+v.DisplayName, iconChevron, func() {
				a.navigate("provider:" + v.ID)
			})
		})
		for i := range v.Accounts {
			a.heroAccount(c, p, v, v.Accounts[i])
		}
	})
}

// heroAccount is one account's headline window inside the lead card: the
// window closest to its limit, its number, its bar, and its recent history.
func (a *App) heroAccount(c *ui.Context, p palette, v providerView, av accountView) {
	if av.Snapshot == nil {
		return
	}
	windows := displayWindows(av.Snapshot)
	if len(windows) == 0 {
		return
	}
	lead := fullestWindow(windows)
	points := a.historyFor(v.ID, av.Name, lead.ID)
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		if len(v.Accounts) > 1 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				label(c, p, displayAccount(av.Name))
				if av.Email != "" && av.Email != av.Name {
					muted(c, p, av.Email)
				}
			})
		}
		ui.Row(c).FillWidth().AlignItems(ui.End).Gap(16).Children(func() {
			number := ui.Column(c).Gap(6)
			number.Children(func() {
				hero(c, p, tweenText(lead, tweenValue(number, "value", windowValue(lead))), p.amountColor(lead))
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, windowTitle(v.ID, lead.ID, lead.Title)).
						FontSize(sizeBody).
						FontWeight(600).
						SingleLine()
					if detail := amountDetail(lead); detail != "" {
						muted(c, p, detail)
					}
				})
			})
			ui.Spacer(c)
			if meta := resetMeta(lead, a.now()); meta != "" {
				ui.Column(c).AlignItems(ui.End).Gap(4).Children(func() {
					muted(c, p, meta)
				})
			}
		})
		if fraction, ok := windowFraction(lead); ok {
			bar(c, p, fraction, barColor(p, lead), 10)
		}
		if len(points) >= 2 {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				sparkline(c, p, points, p.toneColor(windowTone(lead)), 54)
				muted(c, p, trendCaption(points, lead))
				burnAlarm(c, p, points, lead)
				resetNote(c, p, points)
			})
		}
		for _, w := range windows {
			if w.ID == lead.ID {
				continue
			}
			a.windowRow(c, p, v.ID, w, a.historyFor(v.ID, av.Name, w.ID), 0)
		}
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

// barColor is the color of a window's bar: its level, or the accent when it
// is money and so has no level of its own.
func barColor(p palette, w model.RateWindow) ui.Color {
	if windowTone(w) == toneNeutral {
		return p.accent
	}
	return p.toneColor(windowTone(w))
}

// overviewCard is one provider's summary in the grid under the lead card.
func (a *App) overviewCard(c *ui.Context, p palette, v providerView) *ui.Element {
	return shell(c, p, func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			mark(c, p, v.ID, v.DisplayName, 32, v.Status)
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(sizeBody).
					FontWeight(600).
					SingleLine()
				muted(c, p, providerIdentity(v))
			})
			iconButton(c, p, "查看 "+v.DisplayName, iconChevron, func() {
				a.navigate("provider:" + v.ID)
			})
		})
		for i := range v.Accounts {
			a.overviewAccount(c, p, v, v.Accounts[i])
		}
	})
}

// overviewAccount is one account inside a grid card.
func (a *App) overviewAccount(c *ui.Context, p palette, v providerView, av accountView) {
	showName := len(v.Accounts) > 1
	switch {
	case av.Status == "ok" && av.Snapshot != nil:
		if showName {
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, displayAccount(av.Name)).FontSize(sizeSmall).FontWeight(600).SingleLine()
				if av.Email != "" && av.Email != av.Name {
					muted(c, p, av.Email)
				}
			})
		}
		windows := displayWindows(av.Snapshot)
		if len(windows) == 0 {
			unconfiguredLine(c, p, "provider 没有报告任何额度窗口。")
		}
		for _, w := range windows {
			a.windowRow(c, p, v.ID, w, a.historyFor(v.ID, av.Name, w.ID), 0)
		}
	default:
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				if showName {
					ui.Text(c, displayAccount(av.Name)).FontSize(sizeSmall).FontWeight(600).SingleLine()
				}
				if av.Error != "" {
					errorLine(c, p, av.Error)
				} else {
					unconfiguredLine(c, p, accountHint(v.ID, av))
				}
			})
			if av.Status != "ok" {
				if tinyButton(c, "重试").Clicked() {
					a.Refresh(true)
				}
			}
			loginActions(c, a, v.ID, av.Name)
		})
	}
}

// accountHint is the one-line reason an account has no numbers.
func accountHint(id string, av accountView) string {
	if av.Cred != "" {
		return credSentence(av.Cred)
	}
	return providerHint(id)
}

// attentionCard lists what needs a hand: providers that failed, that have no
// credentials yet, or that are switched off.
func (a *App) attentionCard(c *ui.Context, p palette, list []providerView) *ui.Element {
	return shell(c, p, func() {
		section(c, p, "需要处理")
		for i := range list {
			if i > 0 {
				hairline(c, p)
			}
			a.attentionRow(c, p, list[i])
		}
	})
}

// attentionRow is one provider that needs something, in one line.
func (a *App) attentionRow(c *ui.Context, p palette, v providerView) {
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		mark(c, p, v.ID, v.DisplayName, 32, v.Status)
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(sizeBody).
					FontWeight(600).
					SingleLine()
				pill(c, p, statusLabel(v.Status, v.firstCred()), statusTone(v.Status))
			})
			attentionLine(c, p, v)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			switch {
			case !v.Enabled:
				if tinyButton(c, "启用").Clicked() {
					a.Enable(v.ID, true)
				}
			case v.Status == "error":
				if tinyButton(c, "重试").Clicked() {
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
func attentionLine(c *ui.Context, p palette, v providerView) *ui.Element {
	reason := attentionReason(v)
	detail := providerError(v)
	if detail == "" || detail == reason {
		return muted(c, p, reason)
	}
	return ui.RichText(c,
		ui.Span{Text: reason + " · ", Color: p.t.TextMuted},
		ui.Span{Text: detail, Font: p.mono, Color: p.t.TextMuted.Alpha(0.75)},
	).FontSize(sizeSmall).MaxLines(2)
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
		return "探测失败，可以重试或重新登录"
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
