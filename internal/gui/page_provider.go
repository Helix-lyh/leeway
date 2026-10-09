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
// can be done about each.
func (a *App) providerPage(c *ui.Context, p palette, id string) {
	v := a.providerViewOf(id)
	sub := statusLabel(v.Status, v.firstCred())
	if v.Enabled && !a.lastAt.IsZero() {
		sub += " · 更新于 " + agoText(a.lastAt, a.now())
	}
	pageHeadMarked(c, p, v.ID, v.Status, v.DisplayName, sub, func() {
		if btnLarge(ui.Button(c, "← 总览")).Clicked() {
			a.navigate("overview")
		}
		if btnLarge(ui.PrimaryButton(c, "刷新")).Disabled(a.fetching).Clicked() {
			a.Refresh(true)
		}
		btnLarge(ui.MenuButton(c, "操作", func(m *ui.Menu) {
			if m.Item("复制当前 JSON").Chosen() {
				mygo.Clipboard.WriteText(a.CopyJSON())
				a.toast = "已复制 JSON"
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
		text, detail := failureText(joinNonEmpty([]string{"这一家这次没有答上来", staleNote(a)}, " · "), v.Error)
		errorLine(c, p, text, detail)
	}
	if !v.Enabled {
		card(c, p, func() {
			unconfiguredLine(c, p, "这个 provider 已停用，不会参与探测。")
			if btnLarge(ui.PrimaryButton(c, "启用")).Clicked() {
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
		section(c, p, "凭据位置")
		keyValue(c, p, "会话目录", sessionDirText())
		if path, err := config.Path(); err == nil {
			keyValue(c, p, "配置文件", path)
		}
		if len(v.Accounts) > 1 {
			unconfiguredLine(c, p, "「让本机用它」只影响本机官方客户端读的那份会话，不改变 leeway 自己看哪个账号。")
		}
	})
}

func enabledAction(enabled bool) string {
	if enabled {
		return "停用这个 provider"
	}
	return "启用这个 provider"
}

func sessionDirText() string {
	dir, err := auth.SessionDir()
	if err != nil {
		return "未知"
	}
	return dir
}

// providerAccount is one account's card: identity, credentials, every window
// with its history, and the actions that apply.
func (a *App) providerAccount(c *ui.Context, p palette, v providerView, av accountView) ui.Element {
	return card(c, p, func() {
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
			accountActions(c, a, p, v.ID, av.Name)
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
		switch {
		case av.Status == "ok" && av.Snapshot != nil:
			a.snapshotDetail(c, p, v.ID, av)
		case av.Status == "not_configured":
			notConfiguredLine(c, p, v.ID, av)
			if av.Error != "" {
				text, detail := failureText("这个账号这次没有答上来", av.Error)
				errorLine(c, p, text, detail)
			}
		default:
			if av.Error != "" {
				text, detail := failureText("这个账号这次没有答上来", av.Error)
				errorLine(c, p, text, detail)
			}
			unconfiguredLine(c, p, "凭据可能已失效：重新登录，或让 leeway 再解析一次。")
		}
	})
}

// snapshotDetail draws one account's windows, cost and history.
func (a *App) snapshotDetail(c *ui.Context, p palette, providerID string, av accountView) {
	snap := av.Snapshot
	for i, w := range snap.Windows {
		if i > 0 {
			hairline(c, p)
		}
		a.windowRow(c, p, providerID, w, a.historyFor(providerID, av.Name, w.ID), 46)
	}
	if snap.Cost != nil && snap.Cost.Amount != nil {
		if len(snap.Windows) > 0 {
			hairline(c, p)
		}
		line := money(*snap.Cost.Amount, snap.Cost.Currency)
		if snap.Cost.Limit != nil {
			line += fmt.Sprintf(" / %s", money(*snap.Cost.Limit, snap.Cost.Currency))
		}
		if d := snap.Cost.Delta; d != nil && d.Amount != nil && *d.Amount != 0 {
			line += fmt.Sprintf("（较上次 %+.2f）", *d.Amount)
		}
		ui.Row(c).FillWidth().AlignItems(ui.End).Gap(space3).Children(func() {
			ui.Text(c, periodText(snap.Cost.Period)).FontSize(sizeBody).FontWeight(600).SingleLine()
			ui.Spacer(c)
			value(c, p, line, p.t.Text)
		})
	}
	if len(snap.Windows) == 0 && snap.Cost == nil {
		unconfiguredLine(c, p, "provider 没有报告任何额度窗口。")
	}
}

// historyCaption says what the sparkline covers.
func historyCaption(points []history.Point) string {
	if len(points) < 2 {
		return ""
	}
	first, last := points[0].T, points[len(points)-1].T
	parts := []string{fmt.Sprintf("近 %d 次探测：%s → %s", len(points),
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
		line := ui.Row(c).FillWidth().AlignItems(ui.End).Gap(space2)
		line.Children(func() {
			ui.Text(c, label).FontSize(sizeBody).FontWeight(600).SingleLine()
			if detail := amountDetail(w); detail != "" {
				muted(c, p, detail)
			}
			ui.Spacer(c)
			value(c, p, tweenText(w, tweenValue(line, "value", windowValue(w))), p.amountColor(w))
		})
		if fraction, ok := windowFraction(w); ok {
			bar(c, p, fraction, barColor(p, w), 7)
		}
		if meta := windowMeta(w, points, a.now()); meta != "" {
			muted(c, p, meta)
		}
		if chart > 0 && len(points) >= 2 {
			_, over := a.sparkline(c, p, "provider-trend:"+providerID+"/"+w.ID, "用量走势："+windowTitle(providerID, w.ID, w.Title), points, p.toneColor(windowTone(w)), float32(chart), nil)
			summary := joinNonEmpty([]string{chartScale(points), historyCaption(points)}, " · ")
			muted(c, p, chartCaption(points, over, summary))
		}
		// The reset label sits above the bar, so the projection below it does
		// not say when the reset comes: one row states the countdown once.
		burnAlarmNoReset(c, p, points, w)
		resetNote(c, p, points)
	})
}
