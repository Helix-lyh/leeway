package gui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/history"
	"leeway/internal/model"
	"leeway/internal/provider"
)

// providerViewOf builds one provider's view, whether or not it was probed.
func (a *App) providerViewOf(id string) providerView {
	if p, ok := provider.Get(id); ok {
		return a.providerView(p)
	}
	return providerView{ID: id, DisplayName: id, Status: "not_configured"}
}

// providerPage is one provider in full: every account, every window, and what
// can be done about each. Pulse's detail pattern: page head, then hero metrics
// and card sections — no decorative borders.
func (a *App) providerPage(c *ui.Context, p palette, id string) {
	v := a.providerViewOf(id)
	sub := statusLabel(v.Status, v.firstCred())
	if v.Enabled && !a.lastAt.IsZero() {
		sub += " · " + agoText(a.lastAt, a.now()) + "更新"
	}
	pageHeadMarked(c, p, v.ID, v.Status, v.DisplayName, sub, func() {
		back := btnLarge(ui.ButtonBase(c).Children(func() {
			ui.Text(c, "← 总览").FontSize(sizeBody).TextColor(p.t.TextMuted)
		})).Cursor(ui.CursorPointer)
		ghostFill(back, p)
		if back.Clicked() {
			a.navigate("overview")
		}
		if btnLarge(ui.PrimaryButton(c, "刷新")).Disabled(a.fetching).Clicked() {
			a.Refresh(true)
		}
		btnLarge(ui.MenuButton(c, "更多", func(m *ui.Menu) {
			if m.Item("复制数据").Chosen() {
				mygo.Clipboard.WriteText(a.CopyJSON())
				a.toast = "已复制"
			}
			if m.Item("添加账号…").Chosen() {
				a.openNewAccount(id)
			}
			m.Separator()
			if m.Item(enabledAction(v.Enabled)).Chosen() {
				a.Enable(id, !v.Enabled)
			}
			if m.Item("打开配置目录").Chosen() {
				if path, err := config.Path(); err == nil {
					mygo.Shell.ShowItemInFolder(path)
				}
			}
		}))
	})

	if v.Error != "" && v.Status == "error" {
		text, detail := failureText(joinNonEmpty([]string{"这次没有结果", staleNote(a)}, " · "), v.Error)
		errorLine(c, p, text, detail)
	}
	if !v.Enabled {
		card(c, p, func() {
			unconfiguredLine(c, p, "这个服务商已移除，不会被查询。")
			if btnLarge(ui.PrimaryButton(c, "添加")).Clicked() {
				a.Enable(id, true)
			}
		})
		return
	}

	if len(v.Accounts) > 1 {
		// Several subscriptions of one provider are read side by side; a
		// single one is read in full.
		a.providerMatrix(c, p, v)
	} else {
		for i := range v.Accounts {
			av := v.Accounts[i]
			a.providerAccount(c, p, v, av)
		}
	}

	card(c, p, func() {
		section(c, p, "登录信息来源")
		keyValue(c, p, "登录文件目录", sessionDirText())
		if path, err := config.Path(); err == nil {
			keyValue(c, p, "配置文件", path)
		}
		if len(v.Accounts) > 1 {
			unconfiguredLine(c, p, "「启用」只影响官方工具读的那份登录文件，不改变 leeway 自己看哪个账号。")
		}
	})
}

func enabledAction(enabled bool) string {
	if enabled {
		return "移除这个服务商"
	}
	return "添加这个服务商"
}

func sessionDirText() string {
	dir, err := auth.SessionDir()
	if err != nil {
		return "未知"
	}
	return dir
}

// providerAccount is one account: when it answered, a Pulse-like hero plate
// plus a quieter card for the rest; otherwise one card with the state and the
// actions that apply.
func (a *App) providerAccount(c *ui.Context, p palette, v providerView, av accountView) {
	switch {
	case av.Status == "ok" && av.Snapshot != nil:
		a.providerHero(c, p, v, av)
		a.providerSupport(c, p, v, av)
	default:
		card(c, p, func() {
			a.accountChrome(c, p, v, av)
			switch {
			case av.Status == "not_configured":
				notConfiguredLine(c, p, v.ID, av)
				if av.Error != "" {
					text, detail := failureText("这个账号这次没有结果", av.Error)
					errorLine(c, p, text, detail)
				}
			default:
				if av.Error != "" {
					text, detail := failureText("这个账号这次没有结果", av.Error)
					errorLine(c, p, text, detail)
				}
				unconfiguredLine(c, p, "登录信息可能已失效：重新登录，或让 leeway 再读一次。")
			}
		})
	}
}

// accountChrome is the identity row every account plate shares: who it is,
// how it stands, and the actions that apply — kebab stays secondary.
func (a *App) accountChrome(c *ui.Context, p palette, v providerView, av accountView) {
	ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
		section(c, p, displayAccount(av.Name))
		if av.Email != "" && av.Email != av.Name {
			muted(c, p, av.Email)
		}
		if av.Plan != "" {
			pill(c, p, av.Plan, toneNeutral)
		}
		ui.Spacer(c)
		pill(c, p, statusLabel(av.Status, av.Cred), statusTone(av.Status))
		loginActions(c, a, v.ID, av.Name)
		more := accountActions(c, a, p, v.ID, av.Name)
		ghostFill(more, p)
	})
	cred := av.Cred
	if av.Expires != "" {
		cred += " · " + av.Expires
	}
	if cred != "" {
		ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconKeyhole).FontSize(iconSmall).TextColor(p.t.TextMuted)
			muted(c, p, cred)
		})
	}
}

// providerHero is the account's lead plate: identity, then the window closest
// to its limit as a large number with supporting readings — Pulse detail's
// hero stack, without a nested tray or decorative border.
func (a *App) providerHero(c *ui.Context, p palette, v providerView, av accountView) {
	snap := av.Snapshot
	card(c, p, func() {
		a.accountChrome(c, p, v, av)
		if len(snap.Windows) == 0 {
			if snap.Cost != nil && snap.Cost.Amount != nil {
				a.costLine(c, p, snap)
				return
			}
			unconfiguredLine(c, p, "服务商没有提供额度信息。")
			return
		}
		lead := fullestWindow(snap.Windows)
		points := a.historyFor(v.ID, av.Name, lead.ID)
		a.heroWindow(c, p, v.ID, lead, heroFacts(v.ID, snap.Windows, lead, a.now()))
		if len(points) >= 2 {
			ui.Column(c).FillWidth().Gap(space2).Padding(space2).Radius(radiusControl).
				Background(p.chartWell).Children(func() {
				_, over := a.sparkline(c, p, "provider-trend:"+v.ID+"/"+lead.ID,
					"用量走势："+windowTitle(v.ID, lead.ID, lead.Title),
					points, p.toneColor(windowTone(lead)), 54, nil)
				summary := joinNonEmpty([]string{chartScale(points), historyCaption(points)}, " · ")
				muted(c, p, chartCaption(points, over, summary))
			})
		}
		resetNote(c, p, points)
		// A lone window keeps cost on the hero so the page is still one answer.
		if len(snap.Windows) == 1 {
			a.costLine(c, p, snap)
		}
	})
}

// providerSupport holds the windows that are not the lead, and the cost when
// more than one window already claimed the hero. Gap between plates — no rules.
func (a *App) providerSupport(c *ui.Context, p palette, v providerView, av accountView) {
	snap := av.Snapshot
	if len(snap.Windows) == 0 {
		return
	}
	lead := fullestWindow(snap.Windows)
	var rest []model.RateWindow
	for _, w := range snap.Windows {
		if w.ID != lead.ID {
			rest = append(rest, w)
		}
	}
	needCost := len(snap.Windows) > 1 && snap.Cost != nil && snap.Cost.Amount != nil
	if len(rest) == 0 && !needCost {
		return
	}
	card(c, p, func() {
		for _, w := range rest {
			a.windowRow(c, p, v.ID, w, a.historyFor(v.ID, av.Name, w.ID), 40)
		}
		if needCost {
			a.costLine(c, p, snap)
		}
	})
}

func (a *App) costLine(c *ui.Context, p palette, snap *model.Snapshot) {
	if snap.Cost == nil || snap.Cost.Amount == nil {
		return
	}
	line := money(*snap.Cost.Amount, snap.Cost.Currency)
	if snap.Cost.Limit != nil {
		line += fmt.Sprintf(" / %s", money(*snap.Cost.Limit, snap.Cost.Currency))
	}
	if d := snap.Cost.Delta; d != nil && d.Amount != nil && *d.Amount != 0 {
		line += fmt.Sprintf("（比上次 %+.2f）", *d.Amount)
	}
	ui.Row(c).FillWidth().AlignItems(ui.End).Gap(space3).Children(func() {
		ui.Text(c, periodText(snap.Cost.Period)).FontSize(sizeBody).FontWeight(600).SingleLine()
		ui.Spacer(c)
		value(c, p, line, p.t.Text)
	})
}

// historyCaption says what the sparkline covers.
func historyCaption(points []history.Point) string {
	if len(points) < 2 {
		return ""
	}
	first, last := points[0].T, points[len(points)-1].T
	parts := []string{fmt.Sprintf("近 %d 次查询：%s → %s", len(points),
		first.Local().Format("01-02 15:04"), last.Local().Format("01-02 15:04"))}
	if r := trendRange(points); r != "" {
		parts = append(parts, r)
	}
	return strings.Join(parts, " · ")
}

// trendRange says what the chart's current leg is worth at its two ends. It
// reads the same leg the rest of the trend math reads: the points before a
// reset describe a quota that is gone, and their ends would tell a second,
// older story about the same window.
func trendRange(points []history.Point) string {
	leg := seriesLeg(points)
	if len(leg) < 2 {
		return ""
	}
	first, last := pointText(leg[0]), pointText(leg[len(leg)-1])
	if first == last {
		return ""
	}
	return first + " → " + last
}

func periodText(period string) string {
	switch period {
	case "daily":
		return "今日"
	case "monthly":
		return "本月"
	case "balance":
		return "余额"
	case "":
		return "费用"
	}
	return period
}

// windowRow draws one window lane: name, amount, bar, reset and change, and —
// when it is given a chart height — the window's history under them. The
// colored lines that read the window come last, so that whatever a lane is
// made of, the sentence about it is in the same place.
func (a *App) windowRow(c *ui.Context, p palette, providerID string, w model.RateWindow, points []history.Point, chart float32) {
	label := windowTitle(providerID, w.ID, w.Title)
	ui.Column(c).FillWidth().Gap(space2).Children(func() {
		line := ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(space2)
		line.Children(func() {
			ui.Text(c, label).FontSize(sizeBody).FontWeight(600).SingleLine()
			if detail := amountDetail(w); detail != "" {
				muted(c, p, detail)
			}
			// When the window resets, and how far it moved, sit on the name's
			// line rather than on one of their own: that line had the width
			// for them, and the bar keeps the width it had. The bar is the one
			// thing here that has to be read against the other bars, so it
			// spans the card and no row starts it somewhere else.
			if meta := windowMeta(w, points, a.now()); meta != "" {
				muted(c, p, meta)
			}
			ui.Spacer(c)
			value(c, p, tweenText(w, tweenValue(line, "value", windowValue(w))), p.amountColor(w))
		})
		meter(c, p, w, 7)
		if chart > 0 && len(points) >= 2 {
			ui.Column(c).FillWidth().Gap(space2).Padding(space2).Radius(radiusControl).
				Background(p.chartWell).Children(func() {
				_, over := a.sparkline(c, p, "provider-trend:"+providerID+"/"+w.ID, "用量走势："+windowTitle(providerID, w.ID, w.Title), points, p.toneColor(windowTone(w)), float32(chart), nil)
				summary := joinNonEmpty([]string{chartScale(points), historyCaption(points)}, " · ")
				muted(c, p, chartCaption(points, over, summary))
			})
		}
		resetNote(c, p, points)
	})
}
