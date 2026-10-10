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
	chartStroke    = 2.25 // data ink a step above the quiet grid (Pulse spirit)
	chartInset     = 7    // room at each end for the endpoint's halo
	chartPadY      = 4    // headroom above the peak and below the baseline
	chartDotR      = 4
	chartHaloR     = 7
	chartHalo      = 0.28
	chartFillTop   = 0.42 // stronger area wash so the series, not the well, leads
	chartFillBase  = 0.03
	chartHeadroom  = 1.1 // the peak never touches the top of a money chart
	chartGridSteps = 2   // the scale is halved: its baseline, its middle and its top
	// The zones behind a percentage series, where the number turns warn and
	// then critical. They are tints rather than fills, because the line has to
	// stay the strongest thing in the chart — and faint enough that the axis
	// label drawn over them keeps its 4.5:1 (contrast_test.go measures the
	// label against both).
	chartBandWarn     = 0.10
	chartBandCritical = 0.15
	chartWellPad      = space1 // inset of the recessed well; keep trays short enough for two-up tests
	// The history page draws a dot field instead of a line: a fixed lattice,
	// like the network graph in Activity Monitor. Pitch and radius are chosen
	// so a dot never touches its neighbour — the gap is what makes it a field
	// rather than a textured fill.
	dotPitch  = 3.75
	dotR      = 1.25
	dotFieldH = 128
	dotLabelH = 16
	// A reset is not a slope. The previous reading holds this far across the
	// gap, then drops; the rule is drawn at the same place.
	dotResetHold = 0.72
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

// chartWellColor is the recessed pad a plot sits in: palette.chartWell (Pulse's
// third grey), no stroke.
func chartWellColor(p palette) ui.Color {
	return p.chartWell
}

// chartGridInk is quieter than p.grid so the series stroke/fill lead. It is
// not p.hairline: history's decorative-hairline test matches that ink across
// the tray, and a full-width baseline in hairline colour would count as a rule.
func chartGridInk(p palette) ui.Color {
	return p.track
}

// sparklineInWell is sparkline seated in a rounded recessed pad (Pulse chart
// well): fill difference only, no border stroke.
func (a *App) sparklineInWell(c *ui.Context, p palette, key, label string, points []history.Point, color ui.Color, height float32, onOpen func()) (ui.Element, int) {
	over := -1
	well := ui.Column(c).
		FillWidth().
		Padding(chartWellPad).
		Radius(radiusControl).
		Background(chartWellColor(p))
	well.Children(func() {
		_, over = a.sparkline(c, p, key, label, points, color, height, onOpen)
	})
	return well, over
}

// dotfieldInWell is the history page's chart: the same recessed pad, with a
// dot field in place of the line.
func (a *App) dotfieldInWell(c *ui.Context, p palette, key, label string, points []history.Point, height float32, onOpen func()) (ui.Element, int) {
	over := -1
	well := ui.Column(c).
		FillWidth().
		Padding(chartWellPad).
		Radius(radiusControl).
		Background(chartWellColor(p))
	well.Children(func() {
		_, over = a.dotfield(c, p, key, label, points, height, onOpen)
	})
	return well, over
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
	return a.chartPlot(c, key, label, points, height, onOpen, func(pt *ui.Painter, r ui.Rect, over int) {
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
		// loses a pixel to the rule it crosses. Quiet track ink keeps the
		// grid under the series (Pulse: texture serves data, not the frame).
		// Percentage charts already have warn/critical band edges; a baseline
		// in the well composites to nearly hairline-on-card and reads as a
		// tray rule — skip the grid there. Money charts keep the steps.
		if !axis.bands {
			grid := chartGridInk(p)
			for i := 0; i <= chartGridSteps; i++ {
				gy := plot.Y + plot.H*float32(chartGridSteps-i)/chartGridSteps
				pt.Fill(ui.Rect{X: plot.X, Y: gy, W: plot.W, H: 1}, grid, 0)
			}
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
}

// chartPlot is the hit target, the keyboard cursor and the focus ring shared
// by the line chart and the history dot field. The painter only draws; the
// row of cells does the hit-testing.
func (a *App) chartPlot(c *ui.Context, key, label string, points []history.Point, height float32, onOpen func(), draw func(pt *ui.Painter, r ui.Rect, over int)) (ui.Element, int) {
	over := -1
	// The picture carries the name of what it shows; the numbers themselves
	// are in the caption below it and in `leeway usage --last`.
	// Radius so the focus ring's corners match every other control's: MyGo adds
	// 3 to the element's own radius, and a plot at 0 drew a near-square ring.
	plot := ui.Row(c).FillWidth().Height(height).Radius(radiusControl).Focusable().Label(label).DrawOver(func(pt *ui.Painter, r ui.Rect) {
		draw(pt, r, over)
	})
	// A chart inside a card that opens something takes the pointer for its own
	// keys, so it hands the press back to the card it lives in: the whole card
	// is the target, and the strip the chart covers is not an exception.
	if onOpen != nil {
		plot.OnClick(onOpen)
	}
	// The keyboard reads the same observations the pointer does: one stop per
	// chart and the arrows to walk it. The focus lands on the newest
	// observation, which is the one a line chart marks and the bright end of
	// a dot field.
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

// dotfield draws one history series as a lattice of dots. Height is the
// reading, left to right is time, and the colour is the level that reading
// had reached: fine, then warn, then critical. Dots brighten toward the
// right, so the newest sample is the one the eye lands on. A column with no
// number stays lattice — a gap is not a zero and not a line invented across it.
func (a *App) dotfield(c *ui.Context, p palette, key, label string, points []history.Point, height float32, onOpen func()) (ui.Element, int) {
	strong := c.Preferences().HighContrast
	return a.chartPlot(c, key, label, points, height, onOpen, func(pt *ui.Painter, r ui.Rect, over int) {
		paintDotField(pt, r, p, points, over, strong)
	})
}

// dotGrid is the lattice a dot field is drawn on. Row 0 is the baseline.
type dotGrid struct {
	cols, rows int
	x0, y0     float32 // centre of the bottom-left dot
	pitch      float32
}

func layoutDots(area ui.Rect) (dotGrid, bool) {
	var g dotGrid
	if area.W < dotPitch*2 || area.H < dotPitch*3 {
		return g, false
	}
	g.pitch = dotPitch
	g.cols = int(area.W / dotPitch)
	g.rows = int(area.H / dotPitch)
	if g.cols < 2 || g.rows < 3 {
		return g, false
	}
	gridW := float32(g.cols-1) * g.pitch
	gridH := float32(g.rows-1) * g.pitch
	g.x0 = area.X + (area.W-gridW)/2
	g.y0 = area.Y + area.H - (area.H-gridH)/2
	return g, true
}

func (g dotGrid) center(col, row int) (float32, float32) {
	return g.x0 + float32(col)*g.pitch, g.y0 - float32(row)*g.pitch
}

// colPoint is the observation a column's hit target belongs to. The pointer
// cells are equal widths, one per observation; the field itself lerps between
// those observations.
func colPoint(col, cols, n int) int {
	if n <= 1 || cols <= 1 {
		return 0
	}
	i := col * n / cols
	if i >= n {
		return n - 1
	}
	return i
}

func dotFraction(v float64, axis chartAxis) float64 {
	span := axis.hi - axis.lo
	if span <= 0 {
		return 0
	}
	f := (v - axis.lo) / span
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// dotSeriesInk is the colour of one observation's dots: the level, when the
// series has one, and the money ink when it does not.
func dotSeriesInk(p palette, pt history.Point) ui.Color {
	if pt.UsedPercent != nil {
		return p.toneColor(percentTone(*pt.UsedPercent))
	}
	return p.money
}

// dotLatticeColor is the unlit field. It is tinted, not grey, so the empty
// part of the chart still reads as the same material as the data.
func dotLatticeColor(p palette, highContrast bool) ui.Color {
	a := float32(0.22)
	if p.dark {
		a = 0.32
	}
	if highContrast {
		a = 0.55
	}
	if p.dark {
		return ui.Oklch(0.74, 0.045, 175).Alpha(a)
	}
	return ui.Oklch(0.42, 0.03, 175).Alpha(a)
}

// dotLitAlpha is how strong a lit dot is. depth is 0 on the crest and 1 on
// the baseline; columns brighten toward the right, which is now. On a light
// well the tone inks are darker, so the floor is higher or the field washes out.
func dotLitAlpha(col, cols int, depth float64, light, highContrast bool) float64 {
	if depth < 0 {
		depth = 0
	}
	if depth > 1 {
		depth = 1
	}
	t := 0.0
	if cols > 1 {
		t = float64(col) / float64(cols-1)
	}
	recency := 0.72 + 0.28*t
	if light {
		recency = 0.84 + 0.16*t
	}
	body := 0.8 + 0.2*math.Pow(1-depth, 0.8)
	a := body * recency
	if a > 1 {
		a = 1
	}
	if highContrast && a < 0.9 {
		a = 0.9
	}
	return a
}

// dotSample is the reading a column draws. Adjacent observations are lerped,
// the way the line chart already joins them; a reset is not a slope, it is a
// cliff, so the previous reading holds until that observation.
func dotSample(points []history.Point, has []bool, val []float64, p palette, col, cols int, axis chartAxis) (frac float64, ink ui.Color, ok bool) {
	n := len(points)
	if n == 0 || cols == 0 {
		return 0, ui.Color{}, false
	}
	idx := 0.0
	if n > 1 && cols > 1 {
		idx = float64(col) / float64(cols-1) * float64(n-1)
	}
	i0 := int(math.Floor(idx))
	if i0 >= n {
		i0 = n - 1
	}
	if i0 < 0 {
		i0 = 0
	}
	i1 := i0
	t := 0.0
	if i0+1 < n {
		i1 = i0 + 1
		t = idx - float64(i0)
	}
	switch {
	case has[i0] && has[i1] && i1 != i0 && points[i1].Reset:
		v := val[i0]
		at := i0
		if t >= dotResetHold {
			v = val[i1]
			at = i1
		}
		return dotFraction(v, axis), dotInkValue(p, points[at], v), true
	case has[i0] && has[i1] && i1 != i0:
		v := val[i0]*(1-t) + val[i1]*t
		return dotFraction(v, axis), dotInkValue(p, points[i0], v), true
	case has[i0] && (i1 == i0 || t < 0.5):
		return dotFraction(val[i0], axis), dotSeriesInk(p, points[i0]), true
	case has[i1] && t >= 0.5:
		return dotFraction(val[i1], axis), dotSeriesInk(p, points[i1]), true
	default:
		return 0, ui.Color{}, false
	}
}

// dotInkValue colours a column from the value it is showing, so a climb
// through 50% and 80% changes colour where the reading does, not only on
// the sample that happened to land past the line.
func dotInkValue(p palette, pt history.Point, v float64) ui.Color {
	if pt.UsedPercent != nil {
		return p.toneColor(percentTone(v))
	}
	return p.money
}

// dotOn reports whether this row of the column is filled. A reading that
// falls between two rows snaps to the nearer one. A fraction of a dot is
// not drawn: below a small cover it is skipped entirely, and above that it
// is still darker than the lattice, so along a slope the fringe reads as a
// black shadow. A reading at the floor still marks the baseline, so a flat
// empty series is a trace and not a blank field.
func dotOn(row int, level float64, lit bool) bool {
	if !lit {
		return false
	}
	if level >= float64(row)+0.5 {
		return true
	}
	return row == 0 && level < 0.5
}

func quantAlpha(a float64) float32 {
	if a <= 0 {
		return 0
	}
	q := math.Round(a*12) / 12
	if q < 0.08 {
		return 0
	}
	if q > 1 {
		return 1
	}
	return float32(q)
}

type dotBatch struct {
	c    ui.Color
	path *ui.Path
}

func addDot(batches *[]dotBatch, index map[ui.Color]int, x, y float32, c ui.Color) {
	if c.A == 0 {
		return
	}
	i, ok := index[c]
	if !ok {
		i = len(*batches)
		index[c] = i
		*batches = append(*batches, dotBatch{c: c, path: &ui.Path{}})
	}
	(*batches)[i].path.Circle(x, y, dotR)
}

// paintDotField fills r with the lattice and the series. The bottom strip is
// the time axis, kept clear of dots so the labels sit on the well.
func paintDotField(pt *ui.Painter, r ui.Rect, p palette, points []history.Point, over int, highContrast bool) {
	labelH := float32(0)
	if len(points) > 1 && points[0].T != points[len(points)-1].T {
		labelH = dotLabelH
	}
	area := ui.Rect{X: r.X + 4, Y: r.Y + 4, W: r.W - 8, H: r.H - 8 - labelH}
	g, ok := layoutDots(area)
	if !ok {
		return
	}
	series := chartSeries(points)
	axis := chartAxisFor(series, points)
	has := make([]bool, len(points))
	val := make([]float64, len(points))
	for _, s := range series {
		has[s.at] = true
		val[s.at] = s.v
	}
	lattice := dotLatticeColor(p, highContrast)
	var batches []dotBatch
	index := map[ui.Color]int{}
	n := len(points)
	light := !p.dark
	for col := 0; col < g.cols; col++ {
		frac, ink, lit := dotSample(points, has, val, p, col, g.cols, axis)
		slot := 0
		if n > 0 {
			slot = colPoint(col, g.cols, n)
		}
		level := frac * float64(g.rows)
		hover := lit && over >= 0 && slot == over
		for row := 0; row < g.rows; row++ {
			x, y := g.center(col, row)
			if !dotOn(row, level, lit) {
				addDot(&batches, index, x, y, lattice)
				continue
			}
			depth := 1.0
			if level > 0 {
				depth = (level - (float64(row) + 0.5)) / level
			}
			a := dotLitAlpha(col, g.cols, depth, light, highContrast)
			if hover {
				a = 1
			}
			addDot(&batches, index, x, y, ink.Alpha(quantAlpha(a)))
		}
	}
	for _, b := range batches {
		pt.FillPath(b.path, b.c)
	}
	// A reset is a cliff in the field. The caption points at the rule.
	// The pointer's column gets one too, so the caption is about a place.
	ruleAt := func(col float32) {
		if col <= 0 {
			return
		}
		x := g.x0 + col*g.pitch
		pt.Fill(ui.Rect{X: x, Y: area.Y, W: 1, H: area.H}, p.rule, 0)
	}
	if n > 1 {
		for i := 1; i < n; i++ {
			if !points[i].Reset {
				continue
			}
			// The cliff is where dotSample stops holding the previous reading.
			drop := (float32(i) - (1 - dotResetHold)) / float32(n-1) * float32(g.cols-1)
			ruleAt(drop)
		}
	}
	if over >= 0 && n > 0 {
		first, last := -1, -1
		for c := 0; c < g.cols; c++ {
			if colPoint(c, g.cols, n) == over {
				if first < 0 {
					first = c
				}
				last = c
			}
		}
		if first >= 0 {
			ruleAt(float32(first+last) / 2)
		}
	}
	if labelH > 0 {
		paintDotLabels(pt, ui.Rect{X: area.X, Y: r.Y + r.H - dotLabelH, W: area.W, H: dotLabelH}, p, points)
	}
}

func paintDotLabels(pt *ui.Painter, r ui.Rect, p palette, points []history.Point) {
	left := points[0].T.Local().Format("01-02 15:04")
	right := points[len(points)-1].T.Local().Format("01-02 15:04")
	ink := p.t.TextMuted
	y := r.Y + 1
	wLeft, _ := pt.MeasureText(1000, ui.Span{Text: left, Size: sizeLabel})
	pt.Text(r.X, y, left, sizeLabel, ink)
	wRight, _ := pt.MeasureText(1000, ui.Span{Text: right, Size: sizeLabel})
	pt.Text(r.X+r.W-wRight, y, right, sizeLabel, ink)
	if len(points) < 3 {
		return
	}
	mid := points[len(points)/2].T.Local().Format("01-02 15:04")
	if mid == left || mid == right {
		return
	}
	wMid, _ := pt.MeasureText(1000, ui.Span{Text: mid, Size: sizeLabel})
	x := r.X + (r.W-wMid)/2
	if x > r.X+wLeft+12 && x+wMid < r.X+r.W-wRight-12 {
		pt.Text(x, y, mid, sizeLabel, ink)
	}
}

// paintDotPreview is the empty history card: the field the records will fill,
// and a short rise where the first of them lands.
func paintDotPreview(pt *ui.Painter, r ui.Rect, p palette) {
	area := ui.Rect{X: r.X + 8, Y: r.Y + 8, W: r.W - 16, H: r.H - 16}
	g, ok := layoutDots(area)
	if !ok {
		return
	}
	lattice := dotLatticeColor(p, false)
	ink := p.toneColor(toneOK)
	var batches []dotBatch
	index := map[ui.Color]int{}
	rise := g.cols / 5
	if rise < 3 {
		rise = 3
	}
	for col := 0; col < g.cols; col++ {
		frac := 0.0
		if col < rise {
			frac = 0.18 + 0.22*float64(col)/float64(rise)
		}
		level := frac * float64(g.rows)
		for row := 0; row < g.rows; row++ {
			x, y := g.center(col, row)
			if col >= rise || float64(row) >= level {
				addDot(&batches, index, x, y, lattice)
				continue
			}
			a := quantAlpha(0.55 + 0.35*float64(col)/float64(rise))
			addDot(&batches, index, x, y, ink.Alpha(a))
		}
	}
	for _, b := range batches {
		pt.FillPath(b.path, b.c)
	}
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
