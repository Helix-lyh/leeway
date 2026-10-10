package gui

import (
	"fmt"
	"sort"
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
	// Page head carries the question; the body is only the answers. How long
	// the last probe took sits in the subtitle when there is one — quiet, not
	// a control strip in the cards.
	sub := "各家还剩多少"
	if took := sinceStart(a.lastTook); took != "" && !a.fetching {
		sub = took
	}
	pageHead(c, p, "总览", sub, func() {
		if ui.PrimaryButton(c, "刷新").Disabled(a.fetching).Clicked() {
			a.Refresh(true)
		}
	})
	if a.fetchErr != "" {
		text, detail := failureText(joinNonEmpty([]string{"这次查询没有结果", staleNote(a)}, " · "), a.fetchErr)
		errorLine(c, p, text, detail)
	}

	list := a.providers()
	if len(list) == 0 {
		card(c, p, func() {
			body(c, p, "还没有可用的服务商。")
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
		if enabled > 1 {
			// One card per row, the shape the answer lands in.
			ui.Column(c).FillWidth().Gap(space5).Children(func() {
				for i := 1; i < enabled; i++ {
					a.skeletonCard(c, p)
				}
			})
		}
		if bare > 0 {
			a.skeletonAttention(c, p)
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
		var rest []providerView
		for _, v := range ready {
			if v.ID != lead.ID {
				rest = append(rest, v)
			}
		}
		// The cards are read in the order of how much they carry: a provider
		// with four accounts is four blocks of numbers to look at, and it is
		// also the card that sets the height of its column. Ties keep the order
		// providers() settled — whoever answered first, then by ID.
		sort.SliceStable(rest, func(i, j int) bool {
			return len(rest[i].Accounts) > len(rest[j].Accounts)
		})
		if len(rest) > 0 {
			// Fill + gap only: stacked full width (no side-by-side holes), with
			// the breathing room Pulse keeps between bento plates.
			ui.Column(c).FillWidth().Gap(space5).Children(func() {
				for i := range rest {
					a.overviewCard(c, p, rest[i])
				}
			})
		}
		// What needs a hand comes after the providers that are merely fine: the
		// cards are what the page is read for, and a list of chores above them
		// pushes every reading below the fold.
		if len(attention) > 0 && !firstRun {
			a.attentionCard(c, p, attention)
		}
		return
	}
	if len(attention) > 0 && !firstRun {
		a.attentionCard(c, p, attention)
	}
}

// heroCard is the provider the page leads with: mark as category anchor, a
// large lead number, quiet secondary readings, meter and trend — Pulse's
// resource-card hierarchy at full width.
func (a *App) heroCard(c *ui.Context, p palette, v providerView) ui.Element {
	open := func() { a.navigate("provider:" + v.ID) }
	return openCard(c, p, "查看 "+v.DisplayName, open, func() {
		ui.Row(c).FillWidth().Gap(space3).AlignItems(ui.Center).Children(func() {
			mark(c, p, v.ID, v.DisplayName, 46, v.Status)
			ui.Column(c).Grow(1).Gap(space1).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(sizeValue).
					FontWeight(700).
					LetterSpacing(-0.4).
					SingleLine()
				label(c, p, providerIdentity(v))
			})
			// The whole plate is the target, so the chevron is the card's
			// own mark rather than a second button inside the first.
			ui.Icon(c, iconChevron).FontSize(iconBody).TextColor(p.t.TextMuted).Role(ui.RoleNone)
		})
		for i := range v.Accounts {
			a.heroAccount(c, p, v, v.Accounts[i], open)
		}
	})
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
		a.heroWindow(c, p, v.ID, lead, heroFacts(v.ID, windows, lead, a.now()))
		if len(points) >= 2 {
			ui.Column(c).FillWidth().Gap(space2).Children(func() {
				// TODO(theme): providerColor(id) for spark/meter ink so mark +
				// series share one category hue; tone stays for warn/critical text.
				series := p.toneColor(windowTone(lead))
				_, over := a.sparkline(c, p, "overview-trend", "用量走势："+windowTitle(v.ID, lead.ID, lead.Title), points, series, 54, open)
				summary := joinNonEmpty([]string{chartScale(points), trendCaption(points, lead)}, " · ")
				muted(c, p, chartCaption(points, over, summary))
				resetNote(c, p, points)
			})
		}
	})
}

// heroFact is one reading beside the lead number: a caption over its value.
type heroFact struct{ caption, value string }

// heroFacts are the readings that stand beside the lead number: when the window
// comes back, and what the provider's other windows are worth. They share the
// number's line rather than taking a row each under a rule, so the card reads
// across instead of down, and nothing in it is said twice.
func heroFacts(providerID string, windows []model.RateWindow, lead model.RateWindow, now time.Time) []heroFact {
	var out []heroFact
	if in := resetIn(lead, now); in != "" {
		out = append(out, heroFact{caption: "重置", value: in})
	}
	for _, w := range windows {
		if w.ID == lead.ID {
			continue
		}
		out = append(out, heroFact{caption: windowTitle(providerID, w.ID, w.Title), value: amountText(w)})
	}
	return out
}

// heroWindow is the lead card's headline: quiet caption above a large number,
// secondary readings on the number's own line, meter under — Pulse's "answer
// first" stack without a nested tray.
func (a *App) heroWindow(c *ui.Context, p palette, providerID string, w model.RateWindow, facts []heroFact) {
	ui.Column(c).FillWidth().Gap(space3).Children(func() {
		line := ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(space4)
		line.Children(func() {
			ui.Column(c).Gap(space1).Children(func() {
				ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
					label(c, p, windowTitle(providerID, w.ID, w.Title))
					if detail := amountDetail(w); detail != "" {
						muted(c, p, detail)
					}
				})
				hero(c, p, tweenText(w, tweenValue(line, "value", windowValue(w))), p.amountColor(w))
			})
			for _, f := range facts {
				// Readings share the number's line as one group. End alignment
				// matches box bottoms; the 40px number descends further than a
				// 16px reading, so the shorter face gets the difference back.
				ui.Column(c).
					Gap(space1).
					Padding(0, 0, heroFactDrop(), 0).
					Children(func() {
						label(c, p, f.caption)
						chrome(c, p, f.value)
					})
			}
		})
		meter(c, p, w, 10)
	})
}

// heroFactDrop is how far below the hero number's baseline its box bottom sits
// compared with a reading's: End alignment matches box bottoms, so a row that
// aligns them has to give the shorter face the difference back.
func heroFactDrop() float32 {
	return ui.Font{Size: sizeHero, Weight: 600}.Metrics().Descent -
		ui.Font{Size: sizeChrome, Weight: 600}.Metrics().Descent
}

// resetMeta is the meta line under a matrix cell's bar: when the window resets,
// and whether the provider says it is rate limiting. The chart's own caption
// carries the change, so it is not repeated here.
func resetMeta(w model.RateWindow, now time.Time) string {
	var parts []string
	if r := resetLabel(w, now); r != "" {
		parts = append(parts, r)
	}
	if w.RateLimited {
		parts = append(parts, "服务商说请求太频繁")
	}
	return strings.Join(parts, " · ")
}

// trendCaption says what the hero's chart covers: how many probes, over how
// long, and where the window moved.
func trendCaption(points []history.Point, w model.RateWindow) string {
	parts := []string{fmt.Sprintf("近 %d 次查询", len(points))}
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

// overviewCard is one provider under the lead: Pulse resource-card shape —
// mark, name, chevron, then a large lead number with quiet secondary stats.
func (a *App) overviewCard(c *ui.Context, p palette, v providerView) ui.Element {
	return openCard(c, p, "查看 "+v.DisplayName, func() {
		a.navigate("provider:" + v.ID)
	}, func() {
		ui.Row(c).FillWidth().Gap(space3).AlignItems(ui.Center).Children(func() {
			mark(c, p, v.ID, v.DisplayName, 32, v.Status)
			ui.Column(c).Grow(1).Gap(space1).Children(func() {
				ui.Text(c, v.DisplayName).
					Role(ui.RoleHeading).
					Level(2).
					FontSize(sizeChrome).
					FontWeight(600).
					SingleLine()
				label(c, p, providerIdentity(v))
			})
			ui.Icon(c, iconChevron).FontSize(iconSmall).TextColor(p.t.TextMuted).Role(ui.RoleNone)
		})
		// Two subscriptions are two blocks, separated by Gap alone — no rule,
		// no nested tray.
		for _, av := range v.Accounts {
			a.overviewAccount(c, p, v, av)
		}
	})
}

// probingLine says what the window is doing while the first probe is out: a
// page that will have cards in a moment says so, rather than showing a guide
// it is about to take away.
func (a *App) probingLine(c *ui.Context, p palette) ui.Element {
	return ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c).Size(iconBody, iconBody)
		muted(c, p, "正在查询…")
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
// mark, caption, large number and secondary readings, so the page does not
// jump when the probe answers.
func (a *App) skeletonHero(c *ui.Context, p palette) ui.Element {
	return card(c, p, func() {
		ui.Row(c).FillWidth().Gap(space3).AlignItems(ui.Center).Children(func() {
			placeholder(c, p, 46, 46, 12)
			ui.Column(c).Grow(1).Gap(space2).Children(func() {
				placeholder(c, p, 132, 14, 7)
				placeholder(c, p, 180, 10, 5)
			})
		})
		// Caption + hero number + two readings on the number's line: the shape
		// the answer takes. Bar heights follow the type scale so 1.5x does not
		// move the landed page under a short skeleton.
		ui.Column(c).FillWidth().Gap(space3).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.End).Gap(space4).Children(func() {
				f := sizeBody / baseBody
				ui.Column(c).Gap(space1).Children(func() {
					placeholder(c, p, 96*f, sizeLabel*1.2, 6)
					placeholder(c, p, 96*f, sizeHero*1.2, 12)
				})
				for i := 0; i < 2; i++ {
					ui.Column(c).Gap(space1).Children(func() {
						placeholder(c, p, 48*f, sizeLabel*1.2, 6)
						placeholder(c, p, 72*f, sizeChrome*1.2, 7)
					})
				}
			})
			placeholder(c, p, 0, 10, 5)
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
		ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
			placeholder(c, p, 28, 28, 8)
			placeholder(c, p, 96, 12, 6)
			placeholder(c, p, 52, 16, 8)
			ui.Spacer(c)
			placeholder(c, p, 76, 28, 8)
		})
	})
}

// skeletonCard is a grid card before its numbers arrive: caption, large
// number, quiet secondary, meter — the Pulse body the answer lands in.
func (a *App) skeletonCard(c *ui.Context, p palette) ui.Element {
	return card(c, p, func() {
		ui.Row(c).FillWidth().Gap(space3).AlignItems(ui.Center).Children(func() {
			placeholder(c, p, 32, 32, 9)
			ui.Column(c).Grow(1).Gap(space2).Children(func() {
				placeholder(c, p, 96, 12, 6)
				placeholder(c, p, 132, 10, 5)
			})
		})
		f := sizeBody / baseBody
		ui.Column(c).FillWidth().Gap(space2).Children(func() {
			placeholder(c, p, 72*f, sizeLabel*1.2, 6)
			placeholder(c, p, 88*f, sizeValue*1.2, 10)
			ui.Row(c).Gap(space4).Children(func() {
				placeholder(c, p, 48*f, sizeLabel*1.2, 6)
				placeholder(c, p, 56*f, sizeChrome*1.2, 7)
			})
			placeholder(c, p, 0, 8, 4)
		})
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
			unconfiguredLine(c, p, "服务商没有提供额度信息。")
			return
		}
		a.overviewLead(c, p, v.ID, av.Name, windows)
	default:
		if av.Error != "" {
			text, detail := failureText("这个账号这次没有结果", av.Error)
			errorLine(c, p, text, detail)
		} else {
			unconfiguredLine(c, p, accountHint(v.ID, av))
		}
		// What the account needs comes after what it says, at the end of its
		// own block, where the rest of the window puts a row's actions.
		ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
			if av.Status != "ok" {
				if btnSmall(ui.Button(c, "刷新")).Clicked() {
					a.Refresh(true)
				}
			}
			loginActions(c, a, v.ID, av.Name)
		})
	}
}

// overviewLead is the Pulse body of a successful account card: quiet caption,
// large lead number, secondary stats in a quiet row under it, then the meter.
// Other windows stay as facts, not a second equal chrome lane.
func (a *App) overviewLead(c *ui.Context, p palette, providerID, account string, windows []model.RateWindow) {
	lead := fullestWindow(windows)
	facts := heroFacts(providerID, windows, lead, a.now())
	points := a.historyFor(providerID, account, lead.ID)
	ui.Column(c).FillWidth().Gap(space2).Children(func() {
		ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
			label(c, p, windowTitle(providerID, lead.ID, lead.Title))
			if detail := amountDetail(lead); detail != "" {
				muted(c, p, detail)
			}
		})
		host := ui.Column(c).FillWidth().Gap(space2)
		host.Children(func() {
			value(c, p, tweenText(lead, tweenValue(host, "value", windowValue(lead))), p.amountColor(lead))
			if len(facts) > 0 {
				ui.Row(c).FillWidth().Wrap().Gap(space4).AlignItems(ui.Start).Children(func() {
					for _, f := range facts {
						ui.Column(c).Gap(space1).Children(func() {
							label(c, p, f.caption)
							chrome(c, p, f.value)
						})
					}
				})
			}
			meter(c, p, lead, 8)
			resetNote(c, p, points)
		})
	})
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
		for _, v := range broken {
			a.attentionRow(c, p, v)
		}
		for _, v := range waiting {
			a.attentionRow(c, p, v)
		}
		if len(bare) > 0 {
			a.unconfiguredRow(c, p, bare)
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
	ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
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
	ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
		mark(c, p, v.ID, v.DisplayName, 28, v.Status)
		ui.Text(c, v.DisplayName).
			Role(ui.RoleHeading).
			Level(2).
			FontSize(sizeBody).
			FontWeight(600).
			SingleLine()
		pill(c, p, statusLabel(v.Status, v.firstCred()), statusTone(v.Status))
		ui.Box(c).Grow(1).Clip().Children(func() { attentionLine(c, p, v) })
		ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
			switch {
			case !v.Enabled:
				if btnSmall(ui.Button(c, "添加")).Clicked() {
					a.Enable(v.ID, true)
				}
			case v.Status == "error":
				retry := btnSmall(ui.Button(c, "刷新"))
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
		return "已移除，不会被查询。"
	}
	if v.Status == "error" {
		// What to do about it is the two buttons on the row; what happened is
		// the provider's own sentence, appended after this one.
		return "查询失败"
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

// windowMeta is the meta on a window's name line, above its bar: when it
// resets, and how it moved since the probe before.
func windowMeta(w model.RateWindow, points []history.Point, now time.Time) string {
	var parts []string
	if r := resetLabel(w, now); r != "" {
		parts = append(parts, r)
	}
	if d := deltaText(points, w); d != "" {
		parts = append(parts, d)
	}
	if w.RateLimited {
		parts = append(parts, "服务商说请求太频繁")
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
