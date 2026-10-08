package gui

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo/ui"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/history"
	"leeway/internal/model"
	"leeway/internal/provider"
)

// version is the release the window reports; the bundle's Info.plist wins.
const version = Version

func goVersion() string { return runtime.Version() }

// historyPage shows the recorded observations of every provider, account and
// window as one row each.
func (a *App) historyPage(c *ui.Context, p palette) {
	sub := fmt.Sprintf("每个窗口最近 %d 次探测 · 最近一次 %s", a.settings.HistoryN, agoText(a.lastAt, a.now()))
	if path, err := history.Path(); err == nil {
		sub = fmt.Sprintf("每个窗口最近 %d 次探测 · 记录在 %s · 最近一次 %s",
			a.settings.HistoryN, filepath.Base(path), agoText(a.lastAt, a.now()))
	}
	a.arrival.hold(c, a.now(), func() {
		pageHead(c, p, "历史", sub, func() {
			if tinyButton(c, "重新读取").Clicked() {
				a.loadHistory()
			}
		})
	})

	if len(a.series) == 0 {
		a.arrival.hold(c, a.now(), func() { historyEmpty(c, p, a) })
		return
	}
	byProvider := groupSeries(a.series)
	for _, id := range providerOrder(byProvider) {
		series := byProvider[id]
		a.arrival.hold(c, a.now(), func() {
			shell(c, p, func() {
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					mark(c, p, id, providerName(id), 24, "")
					ui.Text(c, providerName(id)).
						Role(ui.RoleHeading).
						Level(2).
						FontSize(15).
						FontWeight(650).
						LetterSpacing(-0.2).
						SingleLine()
					ui.Spacer(c)
					label(c, p, fmt.Sprintf("%d 条序列", len(series)))
				})
				hairline(c, p)
				for i, s := range series {
					if i > 0 {
						hairline(c, p)
					}
					a.historyRow(c, p, s)
				}
			})
		})
	}
}

// historyEmpty is the history page before anything has been recorded: it shows
// the shape the page will take, and the one thing that fills it.
func historyEmpty(c *ui.Context, p palette, a *App) *ui.Element {
	return shell(c, p, func() {
		ui.Column(c).FillWidth().Gap(14).Children(func() {
			ui.Box(c).FillWidth().Height(72).Draw(func(pt *ui.Painter, r ui.Rect) {
				// The first observation, and the line the ones after it will
				// draw: the page starts here and has nowhere to go but right.
				y := r.Y + r.H*0.62
				pt.Fill(ui.Rect{X: r.X + 2, Y: y - 4, W: 9, H: 9}, p.accent, 4.5)
				for x := r.X + 22; x < r.X+r.W; x += 14 {
					pt.Fill(ui.Rect{X: x, Y: y, W: 7, H: 2}, p.track, 1)
				}
			})
			ui.Text(c, "还没有历史点").
				FontSize(sizeValue).
				FontWeight(650).
				LetterSpacing(-0.2)
			ui.Text(c, "每次探测都会记一个点，记满之后这里就有走势：涨得快说明用得猛，掉下去说明重置了。").
				FontSize(sizeBody).
				TextColor(p.t.TextMuted).
				LineHeight(1.55).
				MaxWidth(560)
			ui.Row(c).Gap(8).Padding(4, 0, 0, 0).Children(func() {
				if ui.PrimaryButton(c, "去总览刷新").Padding(7, 16).Radius(9).FontSize(sizeBody).Clicked() {
					a.navigate("overview")
					a.Refresh(true)
				}
			})
		})
	})
}

// historyRow is one series inside its provider's tray: the window, its shape,
// and what the shape covers.
func (a *App) historyRow(c *ui.Context, p palette, s history.Series) {
	last := s.Points[len(s.Points)-1]
	window := model.RateWindow{ID: s.Window, UsedPercent: last.UsedPercent}
	t := windowTone(window)
	// The series carries the window's ID, not the name the provider gives it:
	// the probe that saw it last is where the name comes from.
	live := a.liveWindow(s)
	title := ""
	if live != nil {
		title = live.Title
	}
	ui.Column(c).FillWidth().Gap(9).Children(func() {
		line := ui.Row(c).FillWidth().Gap(10).AlignItems(ui.End)
		line.Children(func() {
			ui.Text(c, windowTitle(s.Provider, s.Window, title)).FontSize(sizeBody).FontWeight(600).SingleLine()
			if s.Account != "" && s.Account != "default" {
				muted(c, p, displayAccount(s.Account))
			}
			ui.Spacer(c)
			if text, dir := changeChip(s.Points); text != "" {
				muted(c, p, chipText(text, dir))
			}
			value(c, p, pointText(last), p.toneColor(t))
		})
		// The same color rule as a bar: a level has a tone, money does not.
		sparkline(c, p, s.Points, barColor(p, window), 40)
		parts := []string{historyCaption(s.Points)}
		if n := countResets(s.Points); n > 0 {
			parts = append(parts, fmt.Sprintf("期间重置 %d 次（图上的竖线）", n))
		}
		muted(c, p, joinNonEmpty(parts, " · "))
		if live != nil {
			burnAlarm(c, p, s.Points, *live)
		}
		resetNote(c, p, s.Points)
	})
}

// pointText renders one recorded observation.
func pointText(pt history.Point) string {
	switch {
	case pt.UsedPercent != nil:
		return percentText(*pt.UsedPercent)
	case pt.Used != nil:
		return money(*pt.Used, pt.Currency)
	}
	return "—"
}

func countResets(points []history.Point) int {
	n := 0
	for _, pt := range points {
		if pt.Reset {
			n++
		}
	}
	return n
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for _, s := range parts {
		if s == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += s
	}
	return out
}

func groupSeries(series []history.Series) map[string][]history.Series {
	byProvider := map[string][]history.Series{}
	for _, s := range series {
		byProvider[s.Provider] = append(byProvider[s.Provider], s)
	}
	return byProvider
}

// providerOrder lists the providers in the history in the order the registry
// declares them, so the page does not jump around between probes.
func providerOrder(byProvider map[string][]history.Series) []string {
	var order []string
	for _, p := range provider.All() {
		if _, ok := byProvider[p.ID()]; ok {
			order = append(order, p.ID())
		}
	}
	for id := range byProvider {
		if _, ok := provider.Get(id); !ok {
			order = append(order, id)
		}
	}
	return order
}

func providerName(id string) string {
	if p, ok := provider.Get(id); ok {
		return p.DisplayName()
	}
	return id
}

// --- about -----------------------------------------------------------------

func (a *App) aboutPage(c *ui.Context, p palette) {
	a.arrival.hold(c, a.now(), func() {
		pageHead(c, p, "关于", "leeway · AI 编程 provider 用量 / 额度 / 重置时间", nil)
	})
	a.arrival.hold(c, a.now(), func() {
		card(c, p, func() {
			keyValue(c, p, "版本", "leeway "+version)
			keyValue(c, p, "界面", "go "+goVersion())
			keyValue(c, p, "配置", configPathText())
			keyValue(c, p, "缓存", cacheDirText())
			keyValue(c, p, "会话", sessionDirText())
			keyValue(c, p, "历史", historyPathText())
		})
	})
	a.arrival.hold(c, a.now(), func() {
		card(c, p, func() {
			label(c, p, "命令行")
			unconfiguredLine(c, p, "窗口和命令行共用同一份配置与历史：命令行里的 leeway 照常可用。")
			ui.Text(c, "leeway usage --json | leeway status | leeway help login").
				Font(p.mono).
				FontSize(sizeSmall).
				TextColor(p.t.TextMuted).
				Selectable()
		})
	})
	a.arrival.hold(c, a.now(), func() {
		card(c, p, func() {
			label(c, p, "探测方式")
			unconfiguredLine(c, p, "leeway 只读各 provider 的用量接口：用你自己的 key、cookie，或官方 CLI 已经登录好的会话。")
			unconfiguredLine(c, p, "凭据不进日志、不进 JSON 输出；密钥只写进你自己的 config.json。")
		})
	})
}

func configPathText() string {
	if path, err := config.Path(); err == nil {
		return path
	}
	return "未知"
}

func cacheDirText() string {
	if dir, err := auth.CacheDir(); err == nil {
		return dir
	}
	return "未知"
}

func historyPathText() string {
	path, err := history.Path()
	if err != nil {
		return "未知"
	}
	return path
}
