package gui

import (
	"github.com/egoist/mygo/ui"

	"leeway/internal/history"
)

// sparkline draws the recorded observations of one window as a line with the
// area under it filled. The stroke is laid down a step at a time, because
// Painter.Line draws only horizontal and vertical lines.
func sparkline(c *ui.Context, p palette, points []history.Point, color ui.Color, height float32) ui.Element {
	vals := sparkValues(points)
	return ui.Box(c).FillWidth().Height(height).Draw(func(pt *ui.Painter, r ui.Rect) {
		base := r.Y + r.H - 1
		pt.Fill(ui.Rect{X: r.X, Y: base, W: r.W, H: 1}, p.track, 0)
		if len(vals) < 2 {
			return
		}
		lo, hi := vals[0], vals[0]
		for _, v := range vals[1:] {
			lo, hi = min(lo, v), max(hi, v)
		}
		span := hi - lo
		flat := span <= 0
		if flat {
			span = 1
		}
		top, bottom := r.Y+3, r.Y+r.H-3
		x := func(i int) float32 { return r.X + float32(i)*r.W/float32(len(vals)-1) }
		y := func(v float64) float32 {
			if flat {
				return (top + bottom) / 2
			}
			return bottom - float32((v-lo)/span)*(bottom-top)
		}
		// Painter.Line draws only horizontal and vertical lines, so the
		// stroke is a band one device pixel wide per column, each column
		// reaching back to the one before it: at that pitch it reads as one
		// smooth line, with no steps and no gaps.
		const thick = 2.0
		prev := y(vals[0])
		for i := 1; i < len(vals); i++ {
			x0, y0 := x(i-1), y(vals[i-1])
			x1, y1 := x(i), y(vals[i])
			width := x1 - x0
			if width <= 0 {
				continue
			}
			for xx := x0; xx < x1; xx++ {
				yy := y0 + (y1-y0)*((xx-x0)/width)
				hiY, loY := min(prev, yy), max(prev, yy)
				// One flat wash, light enough to be a tint rather than a
				// block: the shape is the line's job, not the fill's.
				pt.Fill(ui.Rect{X: xx, Y: hiY, W: 1, H: base - hiY}, color.Alpha(0.08), 0)
				pt.Fill(ui.Rect{X: xx, Y: hiY - thick/2, W: 1, H: loY - hiY + thick}, color, 0)
				prev = yy
			}
		}
		// A window that reset mid-series explains a drop in the line, so it
		// gets a tick of its own.
		for i, point := range points {
			if !point.Reset || i == 0 {
				continue
			}
			pt.Fill(ui.Rect{X: x(i), Y: r.Y + 2, W: 1, H: base - r.Y - 2}, p.t.TextMuted.Alpha(0.5), 0)
		}
		lx, ly := x(len(vals)-1), y(vals[len(vals)-1])
		pt.Fill(ui.Rect{X: lx - 2.5, Y: ly - 2.5, W: 5, H: 5}, color, 2.5)
	})
}

// sparkValues takes the number each observation carries: the percentage when
// there is one, the raw amount otherwise.
func sparkValues(points []history.Point) []float64 {
	vals := make([]float64, 0, len(points))
	for _, pt := range points {
		switch {
		case pt.UsedPercent != nil:
			vals = append(vals, *pt.UsedPercent)
		case pt.Used != nil:
			vals = append(vals, *pt.Used)
		case pt.Limit != nil:
			vals = append(vals, *pt.Limit)
		}
	}
	return vals
}
