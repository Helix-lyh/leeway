package gui

import (
	"math"

	"github.com/egoist/mygo/ui"

	"leeway/internal/history"
)

// A chart's own numbers, in DIPs. The width is a cap rather than a size: the
// same series drawn across a wider card reads as a shallower climb, so a chart
// that is stretched is a chart that lies about its slope.
const (
	chartStroke    = 2
	chartInset     = 7 // room at each end for the endpoint's halo
	chartPadY      = 4 // headroom above the peak and below the baseline
	chartDotR      = 4
	chartHaloR     = 7
	chartHalo      = 0.25
	chartFillTop   = 0.30
	chartFillBase  = 0.02
	chartHeadroom  = 1.1 // the peak never touches the top of a money chart
	chartGridSteps = 2   // the scale is halved: its baseline, its middle and its top
	// The zones behind a percentage series, where the number turns warn and
	// then critical. They are tints rather than fills, because the line has to
	// stay the strongest thing in the chart — and faint enough that the axis
	// label drawn over them keeps its 4.5:1 (contrast_test.go measures the
	// label against both).
	chartBandWarn     = 0.12
	chartBandCritical = 0.18
)

// chartPoint is one observation as the line draws it: the number it carries,
// and the place it holds in the series, so the rules a reset draws line up
// with the observations they mark.
type chartPoint struct {
	at int
	v  float64
}

// chartSeries takes the number each observation carries — the percentage when
// there is one, the raw amount otherwise — with its place in the series. An
// observation that carries no number is left out of the line.
func chartSeries(points []history.Point) []chartPoint {
	series := make([]chartPoint, 0, len(points))
	for i, pt := range points {
		switch {
		case pt.UsedPercent != nil:
			series = append(series, chartPoint{at: i, v: *pt.UsedPercent})
		case pt.Used != nil:
			series = append(series, chartPoint{at: i, v: *pt.Used})
		case pt.Limit != nil:
			series = append(series, chartPoint{at: i, v: *pt.Limit})
		}
	}
	return series
}

// chartRuns splits a series into the stretches it really has. An observation
// that carries no number is a gap in the record, and a line drawn across it
// would state a value the provider never reported, so each stretch is drawn on
// its own: two observations a step apart are one run, and a jump in the index
// starts the next. A run of one point is dropped — a single point is a dot,
// which the endpoint already draws.
func chartRuns(series []chartPoint) [][]chartPoint {
	var runs [][]chartPoint
	run := make([]chartPoint, 0, len(series))
	flush := func() {
		if len(run) >= 2 {
			runs = append(runs, run)
		}
		run = make([]chartPoint, 0, len(series))
	}
	for _, s := range series {
		if len(run) > 0 && s.at != run[len(run)-1].at+1 {
			flush()
		}
		run = append(run, s)
	}
	flush()
	return runs
}

// chartAxis is the scale a series is drawn against, and the words that state
// it: a chart scaled to itself has to say what it is scaled to.
type chartAxis struct {
	lo, hi float64
	text   string
	bands  bool // a percentage series has levels behind it; money has none
}

// chartScale is the scale a series is drawn against, stated in words for the
// caption under the chart. It is said there and not inside the plot because
// the plot is where the line is: a chart at 96% runs along the top edge and one
// hovering at 5% runs along the foot, so there is no corner of it a label can
// sit in that some series will not run through. The scale still travels with
// the chart — one line below it, where every other caption is.
func chartScale(points []history.Point) string {
	axis := chartAxisFor(chartSeries(points), points)
	if !axis.bands {
		return axis.text
	}
	// The warn and critical bands differ from the card by 1.19:1 and 1.32:1 —
	// a tint, not a boundary a reader can find by eye — so the words say where
	// the band starts. The colour is the reminder; this is the fact.
	return axis.text + "，" + percentText(percentWarnAt) + " 提醒、" + percentText(percentCriticalAt) + " 警告"
}

// chartAxisFor reads a series' scale from the series itself. A percentage is a
// share of a whole, so every percentage series shares one 0–100 axis and two
// rows in one card can be compared at a glance. Money has no natural top, so a
// money series is drawn against its own peak with headroom, and the axis says
// so in the series' own currency.
func chartAxisFor(series []chartPoint, points []history.Point) chartAxis {
	if percentSeries(points) {
		return chartAxis{lo: 0, hi: 100, text: "0–100%", bands: true}
	}
	hi := 0.0
	for _, s := range series {
		hi = math.Max(hi, s.v)
	}
	hi *= chartHeadroom
	if hi <= 0 {
		hi = 1
	}
	return chartAxis{lo: 0, hi: hi, text: "0–" + money(hi, seriesCurrency(points))}
}

// percentSeries reports whether a series carries percentages, which is the one
// kind of series with a natural top.
func percentSeries(points []history.Point) bool {
	for i := len(points) - 1; i >= 0; i-- {
		switch {
		case points[i].UsedPercent != nil:
			return true
		case points[i].Used != nil, points[i].Limit != nil:
			return false
		}
	}
	return false
}

// seriesCurrency is the currency a money series is written in.
func seriesCurrency(points []history.Point) string {
	for i := len(points) - 1; i >= 0; i-- {
		if points[i].Currency != "" {
			return points[i].Currency
		}
	}
	return ""
}

// sparkline draws the recorded observations of one window: a line with the
// area under it filled, a dot where the series ended, and the faint rules of
// the scale it is drawn against.
//
// The scale is the point of it. Two percentage series in one card are drawn
// against the same 0–100 axis, so their heights can be compared; a money
// series is drawn against its own peak, and the axis states which of the two a
// chart is. It spans the width it is given, like the bar above it, because the
// scale is what makes two series comparable and two charts of different
// measures in one card read as a mistake.
//
// It returns the observation the pointer is over, or -1. The chart carries no
// tooltip: the caption under it already describes the series, so while the
// pointer is on a point that caption keeps the series and appends that point.
// Nothing above the reading moves — measured pixel-identical up to the chart's
// own focus ring — but a long caption can wrap to a second line while the
// pointer is on it, which moves what is below it by one line (20 px in the
// overview's hero at the default width). (uix's linechart reads the same way —
// the row of cells does the hit-testing and the painter only draws.)
func (a *App) sparkline(c *ui.Context, p palette, key, label string, points []history.Point, color ui.Color, height float32, onOpen func()) (ui.Element, int) {
	series := chartSeries(points)
	axis := chartAxisFor(series, points)
	over := -1
	// The line is a picture of numbers, so it carries the name of what it
	// shows for assistive technology; the numbers themselves are in the
	// caption below it and in `leeway usage --last`.
	// Radius so the focus ring's corners match every other control's: MyGo adds
	// 3 to the element's own radius, and a plot at 0 drew a near-square ring.
	plot := ui.Row(c).FillWidth().Height(height).Radius(radiusControl).Focusable().Label(label).DrawOver(func(pt *ui.Painter, r ui.Rect) {
		plot := ui.Rect{X: r.X + chartInset, Y: r.Y + chartPadY, W: r.W - 2*chartInset, H: r.H - 2*chartPadY}
		if plot.W <= 0 || plot.H <= 0 {
			return
		}
		base := plot.Y + plot.H
		last := len(points) - 1
		x := func(i int) float32 {
			if last <= 0 {
				return plot.X + plot.W/2
			}
			return plot.X + float32(i)*plot.W/float32(last)
		}
		y := func(v float64) float32 {
			f := (v - axis.lo) / (axis.hi - axis.lo)
			return plot.Y + plot.H*(1-float32(math.Max(0, math.Min(1, f))))
		}

		// The levels come first of all: a percentage series is read against
		// where its window turns warn and then critical, so those two zones
		// are behind the line. Money has no such level and gets no zone —
		// inventing one would be a threshold the provider never set.
		if axis.bands {
			for _, band := range []struct {
				from, to float64
				color    ui.Color
				rule     ui.Color
			}{
				{percentWarnAt, percentCriticalAt, p.toneColor(toneWarn).Alpha(chartBandWarn), p.rule},
				{percentCriticalAt, 100, p.toneColor(toneCritical).Alpha(chartBandCritical), p.rule},
			} {
				top, bottom := y(band.to), y(band.from)
				pt.Fill(ui.Rect{X: plot.X, Y: top, W: plot.W, H: bottom - top}, band.color, 0)
				// The wash is a zone, not a boundary: a tint of 1.18:1
				// (warn) or 1.33:1 (critical) says nothing on its own, so the
				// level itself is drawn as a line the caption can point at
				// ("图上的横线"). That line is the neutral rule ink, not the
				// series' own tone: in the tone it disappeared into a series
				// sitting in that band — measured as one 3px amber smear on the
				// overview trend and one 5px red band on Codex's history.
				pt.Fill(ui.Rect{X: plot.X, Y: y(band.from), W: plot.W, H: 1}, band.rule, 0)
			}
		}
		// The scale is laid down next and the data over it, so a line never
		// loses a pixel to the rule it crosses.
		for i := 0; i <= chartGridSteps; i++ {
			if axis.bands && i > 0 {
				// A percentage chart's scale is its bands. The 50% step would
				// land on the warn rule — one border drawn twice — and the
				// 100% step is the top edge of the critical band, so a step
				// there is ink laid inside a tint, saying nothing the band's
				// own edge does not already say. Only the baseline is left,
				// and the caption still says 0–100%.
				continue
			}
			gy := plot.Y + plot.H*float32(chartGridSteps-i)/chartGridSteps
			pt.Fill(ui.Rect{X: plot.X, Y: gy, W: plot.W, H: 1}, p.grid, 0)
		}
		if len(series) > 1 {
			line := &ui.Path{}
			area := &ui.Path{}
			for _, run := range chartRuns(series) {
				line.MoveTo(x(run[0].at), y(run[0].v))
				area.MoveTo(x(run[0].at), base).LineTo(x(run[0].at), y(run[0].v))
				for _, s := range run[1:] {
					line.LineTo(x(s.at), y(s.v))
					area.LineTo(x(s.at), y(s.v))
				}
				area.LineTo(x(run[len(run)-1].at), base).Close()
			}
			// The area fades from the peak down to the baseline, so the shape
			// reads as a quantity rather than as a block of color.
			pt.FillPathGradient(area, ui.LinearGradient{
				From:  color.Alpha(chartFillTop),
				To:    color.Alpha(chartFillBase),
				Angle: 180,
			})
			// A window that reset mid-series explains a drop in the line, so
			// it gets a rule of its own: under the line, over the fill.
			for i, point := range points {
				if !point.Reset || i == 0 {
					continue
				}
				// The caption counts these rules and points at them ("图上
				// 的竖线"), so they are a graphic the words refer to: 3:1.
				pt.Fill(ui.Rect{X: x(i), Y: plot.Y, W: 1, H: plot.H}, p.rule, 0)
			}
			pt.StrokePath(line, chartStroke, color)
		}
		// The last observation is where the reader's eye lands, so it is the
		// one point that is marked.
		if len(series) > 0 {
			ex, ey := x(series[len(series)-1].at), y(series[len(series)-1].v)
			pt.FillPath((&ui.Path{}).Circle(ex, ey, chartHaloR), color.Alpha(chartHalo))
			pt.FillPath((&ui.Path{}).Circle(ex, ey, chartDotR), color)
		}
		// The observation under the pointer gets a rule down to the baseline
		// and a ring, so the caption below is visibly about one point.
		if v, ok := chartValueAt(points, over); ok {
			hx, hy := x(over), y(v)
			// The pointer's own mark, a state indicator while it is showing.
			pt.Fill(ui.Rect{X: hx, Y: plot.Y, W: 1, H: plot.H}, p.rule, 0)
			pt.FillPath((&ui.Path{}).Circle(hx, hy, chartHaloR), color.Alpha(chartHalo))
			pt.FillPath((&ui.Path{}).Circle(hx, hy, chartDotR), color)
		}
	})
	// A chart inside a card that opens something takes the pointer for its own
	// keys, so it hands the press back to the card it lives in: the whole card
	// is the target, and the strip the chart covers is not an exception.
	if onOpen != nil {
		plot.OnClick(onOpen)
	}
	// The keyboard reads the same observations the pointer does: one stop per
	// chart and the arrows to walk it, so a reader who cannot see the line
	// still gets every number in it, one sentence at a time. The focus lands
	// on the newest observation, which is the one the chart marks.
	if plot.Focused() {
		before := a.scrub
		last := len(points) - 1
		if a.scrub.key == key && a.scrub.at <= last {
			over = a.scrub.at
		} else {
			over = last
		}
		switch {
		case plot.Shortcut(0, ui.KeyLeft) && over > 0:
			over--
		case plot.Shortcut(0, ui.KeyRight) && over < last:
			over++
		}
		a.scrub = chartScrub{key: key, at: over}
		if a.scrub != before {
			if _, ok := chartValueAt(points, over); ok {
				c.Announce(scrubText(points[over]))
			}
		}
	} else if a.scrub.key == key {
		// The cursor belongs to the keyboard: when the focus leaves, so does it.
		a.scrub = chartScrub{}
	}
	// One invisible cell per observation: the layout does the hit-testing, and
	// the pointer wins over the keyboard while it is on the chart.
	plot.Children(func() {
		for i := range points {
			if ui.Box(c).Grow(1).FillHeight().Hovered() {
				over = i
			}
		}
	})
	return plot, over
}

// chartValueAt is the number an observation carries, if it carries one.
func chartValueAt(points []history.Point, i int) (float64, bool) {
	if i < 0 || i >= len(points) {
		return 0, false
	}
	switch {
	case points[i].UsedPercent != nil:
		return *points[i].UsedPercent, true
	case points[i].Used != nil:
		return *points[i].Used, true
	}
	return 0, false
}

// scrubText is what a chart's caption says while the pointer is on it: the one
// observation being pointed at, in place of the summary of all of them.
func scrubText(pt history.Point) string {
	return pt.T.Local().Format("01-02 15:04") + " · " + pointText(pt)
}

// chartCaption is the line under a chart: the scale it is drawn to and what
// the series did, and — while the pointer or the keyboard is on it — the
// single observation it is on. The summary stays: reading one point is not a
// reason to take the scale, the band levels, the count and the range off the
// screen, and the reading is appended so nothing above it moves.
func chartCaption(points []history.Point, over int, summary string) string {
	if _, ok := chartValueAt(points, over); ok {
		// The scale, the band thresholds, how many observations there are and
		// where the series ends are what the summary is for: reading one point
		// must not take them off the screen.
		return joinNonEmpty([]string{summary, scrubText(points[over])}, " · ")
	}
	return summary
}
