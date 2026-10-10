package gui

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
	// "最近一次" is the last successful probe, which can be newer than the last
	// recorded observation: with nothing recorded yet the head said
	// "最近一次 刚刚" over a card that says 还没有记录.
	last := ""
	if len(a.series) > 0 {
		last = " · 最近一次 " + agoText(a.lastAt, a.now())
	}
	sub := fmt.Sprintf("每个额度周期最近 %d 次查询%s", a.settings.HistoryN, last)
	if path, err := history.Path(); err == nil {
		sub = fmt.Sprintf("每个额度周期最近 %d 次查询 · 记录在 %s%s",
			a.settings.HistoryN, filepath.Base(path), last)
	}
	pageHead(c, p, "历史", sub, func() {
		if btnLarge(ui.Button(c, "刷新")).Clicked() {
			a.loadHistory()
		}
	})

	if len(a.series) == 0 {
		historyEmpty(c, p, a)
		return
	}
	// What every chart on the page shares, said once: one axis and two
	// thresholds for every percentage series, and the record they are all read
	// from. Under each chart it was the same sentence again — four times over
	// on a page of four — and the caption it left room for is the one thing
	// that is about that chart alone.
	if legend := historyLegend(a.series); legend != "" {
		muted(c, p, legend)
	}
	byProvider := groupSeries(a.series)
	// One full-width tray per provider, stacked — same column as 总览. Two-up
	// halved the chart measure and made sparklines read like fat sliders
	// ("比例" broke); a single column keeps slope and bar width honest.
	ui.Column(c).FillWidth().Gap(space4).Children(func() {
		for _, id := range providerOrder(byProvider) {
			series := byProvider[id]
			name := providerName(id)
			tray := shell(c, p, func() {
				ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
					mark(c, p, id, name, 24, "")
					section(c, p, name)
					ui.Spacer(c)
					label(c, p, fmt.Sprintf("%d 条记录", len(series)))
				})
				ui.Column(c).FillWidth().Gap(space3).Children(func() {
					for _, s := range series {
						a.historyRow(c, p, s)
					}
				})
			})
			tray.Label(name + " 的历史")
		}
	})
}

// historyEmpty is the history page before anything has been recorded: it shows
// the shape the page will take, and the one thing that fills it.
func historyEmpty(c *ui.Context, p palette, a *App) ui.Element {
	return shell(c, p, func() {
		ui.Column(c).FillWidth().Gap(space4).Children(func() {
			ui.Box(c).FillWidth().Height(96).Radius(radiusControl).Background(chartWellColor(p)).Draw(func(pt *ui.Painter, r ui.Rect) {
				paintDotPreview(pt, r, p)
			})
			ui.Text(c, "还没有记录").
				FontSize(sizeValue).
				FontWeight(700).
				LetterSpacing(-0.2)
			ui.Text(c, "每次查询记一笔，记满后这里就有走势：涨得快说明用得多，掉下去说明重置了。").
				FontSize(sizeBody).
				TextColor(p.t.TextMuted).
				LineHeight(1.55).
				MaxWidth(560)
			ui.Row(c).Gap(space2).Padding(space1, 0, 0, 0).Children(func() {
				if btnLarge(ui.PrimaryButton(c, "去总览页刷新")).Clicked() {
					a.navigate("overview")
					a.Refresh(true)
				}
			})
		})
	})
}

// historyRow is one series inside its provider's tray: the name, the latest
// reading, then the dot field that shows how that reading got there.
func (a *App) historyRow(c *ui.Context, p palette, s history.Series) {
	last := s.Points[len(s.Points)-1]
	window := historyWindow(last, s.Window)
	// The series carries the window's ID, not the name the provider gives it:
	// the probe that saw it last is where the name comes from.
	live := a.liveWindow(s)
	title := ""
	if live != nil {
		title = live.Title
	}
	ui.Column(c).FillWidth().Gap(space2).Children(func() {
		line := ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center)
		line.Children(func() {
			ui.Text(c, windowTitle(s.Provider, s.Window, title)).FontSize(sizeBody).FontWeight(600).SingleLine()
			if s.Account != "" && s.Account != "default" {
				muted(c, p, displayAccount(s.Account))
			}
			ui.Spacer(c)
			if text, dir := changeChip(s.Points); text != "" {
				muted(c, p, chipText(text, dir))
			}
		})
		if last.UsedPercent != nil || last.Used != nil {
			historyHero(c, p, window, last)
		}
		_, over := a.dotfieldInWell(c, p, "history:"+s.Provider+"/"+s.Window+"/"+s.Account, "走势："+windowTitle(s.Provider, s.Window, title), s.Points, dotFieldH, nil)
		parts := []string{}
		// A money series is drawn against its own peak, so it still says what
		// its scale is; the percentage axis is the page's, said above.
		if !percentSeries(s.Points) {
			parts = append(parts, chartScale(s.Points))
		}
		if r := trendRange(s.Points); r != "" {
			parts = append(parts, r)
		}
		if n := countResets(s.Points); n > 0 {
			parts = append(parts, fmt.Sprintf("期间重置 %d 次（竖线就是重置）", n))
		}
		// A series that never moved has no range to report; how many
		// observations its flat line is drawn from is then the one thing left
		// that is about this chart and not about the page.
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("%d 次查询", len(s.Points)))
		}
		muted(c, p, chartCaption(s.Points, over, joinNonEmpty(parts, " · ")))
		resetNote(c, p, s.Points)
	})
}

// historyWindow is the RateWindow a recorded point implies for tone, meter and
// hero colour — the same fields barColor / windowTone already understand.
func historyWindow(last history.Point, id string) model.RateWindow {
	return model.RateWindow{
		ID:          id,
		UsedPercent: last.UsedPercent,
		Used:        last.Used,
		Limit:       last.Limit,
		Currency:    last.Currency,
	}
}

// historyHero is the series' lead reading: used as the number, remaining
// beside it when the point can say how much is left. Both numbers are the
// value size. A smaller remainder sits on a different line box, and the
// column then reads as two heights. The remainder stays in the text colour
// so the heat colour still belongs to the amount used.
func historyHero(c *ui.Context, p palette, w model.RateWindow, last history.Point) {
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Start).Gap(space4).Children(func() {
		ui.Column(c).Gap(space1).Children(func() {
			if detail := amountDetail(w); detail != "" {
				label(c, p, detail)
			}
			value(c, p, pointText(last), p.amountColor(w))
		})
		if rem, ok := historyRemaining(last); ok {
			ui.Column(c).Gap(space1).Children(func() {
				label(c, p, "剩余")
				value(c, p, rem, p.t.Text)
			})
		}
	})
}

// historyRemaining is what is left of a quota point, when used and a whole
// are both known. Percentages are out of 100; money needs a limit.
func historyRemaining(last history.Point) (string, bool) {
	switch {
	case last.UsedPercent != nil:
		left := math.Max(0, 100-*last.UsedPercent)
		return percentText(left), true
	case last.Used != nil && last.Limit != nil:
		left := math.Max(0, *last.Limit-*last.Used)
		return money(left, last.Currency), true
	}
	return "", false
}

// historyLegend is the line under the page's head: the axis and the thresholds
// every percentage chart is drawn against, and the queries the series were
// recorded from. Both are properties of the page, not of a chart — and the
// queries are counted across all of the series, because a provider that missed
// one is not a reason for the page to say the record is shorter than it is.
func historyLegend(series []history.Series) string {
	var parts []string
	queries := map[int64]bool{}
	var first, last time.Time
	for _, s := range series {
		if len(parts) == 0 && percentSeries(s.Points) {
			parts = append(parts, chartScale(s.Points))
		}
		for _, pt := range s.Points {
			queries[pt.T.Unix()] = true
			if first.IsZero() || pt.T.Before(first) {
				first = pt.T
			}
			if pt.T.After(last) {
				last = pt.T
			}
		}
	}
	if len(queries) > 1 {
		parts = append(parts, fmt.Sprintf("记录里最近 %d 次查询：%s → %s", len(queries),
			first.Local().Format("01-02 15:04"), last.Local().Format("01-02 15:04")))
	}
	return strings.Join(parts, " · ")
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
	pageHead(c, p, "关于", "leeway · AI 编程服务的用量、额度和重置时间", nil)
	card(c, p, func() {
		keyValue(c, p, "版本", "leeway "+version)
		keyValue(c, p, "运行环境", "go "+goVersion())
		keyValue(c, p, "配置", configPathText())
		keyValue(c, p, "缓存", cacheDirText())
		keyValue(c, p, "登录文件", sessionDirText())
		keyValue(c, p, "历史", historyPathText())
	})
	card(c, p, func() {
		section(c, p, "命令行")
		unconfiguredLine(c, p, "窗口和命令行共用同一份配置和历史，命令行的 leeway 照常可用。")
		ui.Text(c, "leeway usage --json | leeway status | leeway help login").
			Font(p.mono).
			FontSize(sizeBody).
			TextColor(p.t.TextMuted).
			Selectable()
	})
	card(c, p, func() {
		section(c, p, "数据来源")
		unconfiguredLine(c, p, "leeway 只看不改：用你已有的登录信息，去问服务商还剩多少额度。")
		unconfiguredLine(c, p, "登录信息不进日志、不进 JSON 输出；API 密钥只写进你自己的 config.json。")
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
