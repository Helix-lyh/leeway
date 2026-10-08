package gui

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"leeway/internal/history"
	"leeway/internal/model"
)

// seriesLeg is the part of a series that belongs to the quota the window is
// on now. A reset starts a new one, and the points before it describe a quota
// that is gone: read as one run, the drop a reset makes would look like the
// fastest fall in the series.
func seriesLeg(points []history.Point) []history.Point {
	for i := len(points) - 1; i >= 1; i-- {
		if points[i].Reset {
			return points[i:]
		}
	}
	return points
}

// changeChip is the last move a series made — what the row prints beside the
// value, and which way it went (-1, 0, +1).
func changeChip(points []history.Point) (text string, dir int) {
	leg := seriesLeg(points)
	if len(leg) < 2 {
		return "", 0
	}
	last, prev := leg[len(leg)-1], leg[len(leg)-2]
	switch {
	case last.UsedPercent != nil && prev.UsedPercent != nil:
		d := *last.UsedPercent - *prev.UsedPercent
		if d == 0 {
			return "持平", 0
		}
		return fmt.Sprintf("%+.0f%%", d), sign(d)
	case last.Used != nil && prev.Used != nil:
		d := *last.Used - *prev.Used
		if d == 0 {
			return "持平", 0
		}
		return fmt.Sprintf("%+.2f", d), sign(d)
	}
	return "", 0
}

// liveWindow finds the window a series belongs to as the last probe saw it.
// History knows what the value was; only the probe knows when the window
// resets next, and the two halves make the projection.
func (a *App) liveWindow(s history.Series) *model.RateWindow {
	if a.out == nil {
		return nil
	}
	for i := range a.out.Providers {
		pr := &a.out.Providers[i]
		if pr.ID != s.Provider {
			continue
		}
		for j := range pr.Accounts {
			ar := &pr.Accounts[j]
			if ar.Name != s.Account && !(ar.Name == "" && s.Account == "default") {
				continue
			}
			if ar.Snapshot == nil {
				continue
			}
			for k := range ar.Snapshot.Windows {
				if ar.Snapshot.Windows[k].ID == s.Window {
					return &ar.Snapshot.Windows[k]
				}
			}
		}
	}
	return nil
}

// chipText is the change chip with its arrow: the arrow carries the
// direction, and the color stays with the value it belongs to.
func chipText(text string, dir int) string {
	switch dir {
	case 1:
		return "▲ " + text
	case -1:
		return "▼ " + text
	}
	return text
}

// rateText is a rate in points per hour: one decimal until the number is big
// enough that the decimal is noise, and never "0" for a window that is
// climbing.
func rateText(rate float64) string {
	if rate < 10 {
		return fmt.Sprintf("%.1f%%", rate)
	}
	return fmt.Sprintf("%.0f%%", rate)
}

// burnAlarm draws the projection under a bar, when there is one to draw: the
// only line in the window that is allowed to be red.
func burnAlarm(c *ui.Context, p palette, points []history.Point, w model.RateWindow) {
	text, t := burnLine(points, &w)
	if text == "" {
		return
	}
	ui.Text(c, text).FontSize(sizeSmall).TextColor(p.toneColor(t)).MaxLines(2)
}

// burnAlarmInColumn draws the projection the way a matrix column can hold it:
// whose window it is and how long until it is full. The measured rate stays on
// the provider's own page, and the reset is already on the line above.
func burnAlarmInColumn(c *ui.Context, p palette, who string, points []history.Point, w model.RateWindow) {
	b, ok := burnOf(points, &w)
	if !ok {
		return
	}
	text := fmt.Sprintf("约 %s后用满", spanText(b.in))
	if who != "" {
		text = who + "：" + text
	}
	ui.Text(c, text).FontSize(sizeSmall).TextColor(p.toneColor(toneCritical)).MaxLines(2)
}

// resetDrop is what the newest reset in a series fell from and to. A reset can
// also be nothing but the provider moving the window's end, and then the two
// numbers tell no story: only a real drop is reported.
func resetDrop(points []history.Point) (from, to float64, ok bool) {
	for i := len(points) - 1; i >= 1; i-- {
		if !points[i].Reset {
			continue
		}
		prev, cur := points[i-1].UsedPercent, points[i].UsedPercent
		if prev == nil || cur == nil || *prev <= *cur {
			return 0, 0, false
		}
		return *prev, *cur, true
	}
	return 0, 0, false
}

// resetLine is the good news, in the same words wherever it is read: the
// window that just came back. A quota reset is the other half of a quota
// running out, and the value it fell back to is the whole point of it.
func resetLine(points []history.Point) (string, tone) {
	if len(points) == 0 || !points[len(points)-1].Reset {
		return "", toneNeutral
	}
	if from, to, ok := resetDrop(points); ok {
		return fmt.Sprintf("刚重置：%s → %s，额度已恢复", percentText(from), percentText(to)), toneOK
	}
	return "刚重置，额度已恢复", toneOK
}

// resetNote draws a reset under a bar, in the place and the weight the alarm
// uses. The two never appear together: a window that just reset has one point
// on its new quota, and one point is not a rate.
func resetNote(c *ui.Context, p palette, points []history.Point) {
	text, t := resetLine(points)
	if text == "" {
		return
	}
	ui.Text(c, text).FontSize(sizeSmall).TextColor(p.toneColor(t)).MaxLines(2)
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// burn is what a window's own rate says about its limit: how fast it is
// filling, when it would be full, and when the provider gives it back.
type burn struct {
	span time.Duration // the run the rate was read over
	rate float64       // points an hour
	in   time.Duration // until the window is full at that rate
	left time.Duration // until the window resets
}

// burnOf reads a series against the window it records, and reports whether
// that window is on course to fill before it resets. It needs both halves to
// be honest — how fast the window is filling and when the reset comes — so it
// says no when it does not know, and when the reset wins.
func burnOf(points []history.Point, w *model.RateWindow) (burn, bool) {
	if w == nil || w.UsedPercent == nil || w.ResetsAt == nil {
		return burn{}, false
	}
	leg := seriesLeg(points)
	// Two points a minute apart can be a burst or a glitch; the reading
	// waits for a leg with enough of a run in it to be worth believing.
	if len(leg) < 3 {
		return burn{}, false
	}
	first, last := leg[0], leg[len(leg)-1]
	if first.UsedPercent == nil || last.UsedPercent == nil {
		return burn{}, false
	}
	span := last.T.Sub(first.T)
	if span < 10*time.Minute {
		return burn{}, false
	}
	moved := *last.UsedPercent - *first.UsedPercent
	remaining := 100 - *last.UsedPercent
	if moved <= 0 || remaining <= 0 {
		return burn{}, false
	}
	b := burn{
		span: span,
		rate: moved / span.Hours(),
		left: w.ResetsAt.Sub(last.T),
	}
	b.in = time.Duration(remaining / b.rate * float64(time.Hour))
	if b.left <= 0 || b.in >= b.left {
		return burn{}, false
	}
	return b, true
}

// burnLine is the sentence no other tool says: this window runs out before
// the provider gives it back.
func burnLine(points []history.Point, w *model.RateWindow) (text string, t tone) {
	b, ok := burnOf(points, w)
	if !ok {
		return "", toneNeutral
	}
	return fmt.Sprintf("最近 %s每小时涨 %s，约 %s后用满；重置还有 %s",
		spanText(b.span), rateText(b.rate), spanText(b.in), spanText(b.left)), toneCritical
}

// burnWarning is the projection worth telling the user about without being
// asked, and the key naming the window it is about: the most urgent one across
// everything the app is showing.
func (a *App) burnWarning() (key, text string) {
	var worst burn
	for _, s := range a.series {
		w := a.liveWindow(s)
		if w == nil {
			continue
		}
		b, ok := burnOf(s.Points, w)
		if !ok {
			continue
		}
		if key != "" && b.in >= worst.in {
			continue
		}
		worst = b
		key = s.Provider + "/" + s.Account + "/" + s.Window
		text = fmt.Sprintf("%s %s 按现在的速度 %s后用满",
			providerName(s.Provider), windowTitle(s.Provider, s.Window, w.Title), spanText(b.in))
	}
	return key, text
}

// burnToast tells the user about the projection as it appears, wherever they
// are looking. A probe that repeats the same warning is not news, so the
// window it named is remembered until some other window takes over.
func (a *App) burnToast(c *ui.Context) {
	key, text := a.burnWarning()
	if key == a.burned {
		return
	}
	a.burned = key
	if text != "" {
		c.AddToast(ui.Toast{ID: "burn", Title: text})
	}
}
