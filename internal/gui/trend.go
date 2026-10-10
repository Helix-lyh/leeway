package gui

import (
	"fmt"

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

// liveWindow finds the window a series belongs to as the last probe saw it:
// history records the window's ID, and the name the provider gives that window
// only the probe knows.
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

// resetNote draws a reset under a bar, in the place and the weight the value
// above it uses.
func resetNote(c *ui.Context, p palette, points []history.Point) {
	text, t := resetLine(points)
	if text == "" {
		return
	}
	ui.Text(c, text).FontSize(sizeBody).TextColor(p.toneColor(t)).MaxLines(2).Ellipsis("…").Focusable().Tooltip(text)
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
