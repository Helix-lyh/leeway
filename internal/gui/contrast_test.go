package gui

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The palette has to stay legible. A window's number is text as much as its
// bar is a graphic, so every tone color is held to WCAG's 4.5:1 for ordinary
// text on the surface it is drawn on — stricter than the 3:1 a bar needs —
// and so is the muted text, which is the smallest text the window has.
func TestThePaletteStaysLegible(t *testing.T) {
	for _, appearance := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		theme := ui.LightTheme()
		if appearance.dark {
			theme = ui.DarkTheme()
		}
		p := paletteOf(theme)
		// A card is its surface over the tray it sits in, over the canvas.
		card := over(p.card, over(p.bezel, p.canvas))
		for _, check := range []struct {
			what   string
			fg, bg ui.Color
		}{
			{"toneOK on a card", p.toneColor(toneOK), card},
			{"toneWarn on a card", p.toneColor(toneWarn), card},
			{"toneCritical on a card", p.toneColor(toneCritical), card},
			{"body text on a card", p.t.Text, card},
			{"muted text on a card", p.t.TextMuted, card},
			{"muted text on the canvas", p.t.TextMuted, p.canvas},
			// A pill sets its label on a tint of its own tone, which costs
			// contrast the tone does not pay on a plain card.
			{"an OK pill's label", p.toneInk(toneOK), over(p.toneColor(toneOK).Alpha(0.14), card)},
			{"a warn pill's label", p.toneInk(toneWarn), over(p.toneColor(toneWarn).Alpha(0.14), card)},
			{"a critical pill's label", p.toneInk(toneCritical), over(p.toneColor(toneCritical).Alpha(0.14), card)},
			{"a neutral pill's label", p.toneInk(toneNeutral), over(p.toneColor(toneNeutral).Alpha(0.14), card)},
			// The label on a primary button.
			{"text on the accent", p.t.AccentText, p.accent},
			// The state counts in the title bar. They used to be a muted at
			// half alpha, which measured 2.02:1 — a count is text.
			{"a state count in the title bar", p.t.TextMuted, p.canvas},
			// The error detail under a failed provider is text too: at 0.75
			// alpha of the muted it measured 3.11:1 on a card.
			{"a provider's error detail", p.t.TextMuted, card},
			// The money bar is a graphic, so 3:1 is its floor, but it is
			// checked here with everything else.
			{"the money bar on a card", p.money, card},
		} {
			if r := contrast(check.fg, check.bg); r < 4.5 {
				t.Errorf("%s: %s is %.2f:1, under the 4.5:1 ordinary text needs", appearance.name, check.what, r)
			}
		}
	}
}

// The palette is what the window asked for; the frame is what it drew. A page
// that fades an ink at its own call site — `p.t.TextMuted.Alpha(0.75)` — is
// invisible to every check above, because they read the palette. This one
// reads the pixels: for every label the window publishes, the strongest ink in
// its box has to clear 4.5:1 against that box's own surface.
func TestTheInkAPageDrawsClearsItsSurface(t *testing.T) {
	for _, state := range []struct {
		name  string
		setup func(*App)
	}{
		{"the answer", func(a *App) {}},
		{"a probe that failed", func(a *App) {
			a.fetchErr = "unexpected EOF"
			a.lastAt = pinned.Add(-12 * time.Minute)
		}},
		{"a provider's own page", func(a *App) { a.navigate("provider:deepseek") }},
	} {
		a := testApp(t)
		state.setup(a)
		checkInk(t, state.name, a)
	}
}

func checkInk(t *testing.T, state string, a *App) {
	t.Helper()
	tester := ui.NewTester(a.view, mainWidth, mainHeight)
	tester.Frame()
	img := tester.Image()
	checked := 0
	for _, text := range tester.Texts() {
		r, ok := tester.Find(text)
		if !ok || r.W < 4 || r.H < 6 || len([]rune(text)) < 2 {
			continue
		}
		checked++
		surface := modalColor(img, r)
		strongest := 1.0
		for y := int(r.Y); y < int(r.Y+r.H); y++ {
			for x := int(r.X); x < int(r.X+r.W); x++ {
				if c := colorAt(img, x, y); contrast(c, surface) > strongest {
					strongest = contrast(c, surface)
				}
			}
		}
		if strongest < 4.5 {
			t.Errorf("%s: %q is drawn at %.2f:1 on the surface under it", state, text, strongest)
		}
	}
	if checked < 20 {
		// A guard that looks at nothing passes: this is the floor that makes
		// the silence mean something.
		t.Errorf("%s: only %d labels were checked", state, checked)
	}
}

// modalColor is the color a box is mostly made of: the surface the text in it
// is set on.
func modalColor(img *image.RGBA, r ui.Rect) ui.Color {
	counts := map[ui.Color]int{}
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			counts[colorAt(img, x, y)]++
		}
	}
	best, bestN := ui.Color{}, -1
	for c, n := range counts {
		// Ties are broken by the colour itself so the answer does not depend on
		// the order a map happens to iterate in.
		if n > bestN || (n == bestN && colorKey(c) < colorKey(best)) {
			best, bestN = c, n
		}
	}
	return best
}

// colorKey orders colours for a deterministic tie-break.
func colorKey(c ui.Color) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }

func colorAt(img *image.RGBA, x, y int) ui.Color {
	o := img.PixOffset(x, y)
	return ui.RGB(img.Pix[o], img.Pix[o+1], img.Pix[o+2])
}

// over flattens a translucent color onto an opaque one.
func over(fg, bg ui.Color) ui.Color {
	a := float64(fg.A) / 255
	blend := func(f, b uint8) uint8 { return uint8(math.Round(float64(f)*a + float64(b)*(1-a))) }
	return ui.RGB(blend(fg.R, bg.R), blend(fg.G, bg.G), blend(fg.B, bg.B))
}

// contrast is the WCAG contrast ratio of two colors, as drawn: a translucent
// ink is composited onto the surface it is drawn on first, so a pair cannot
// pass by measuring the color the alpha was applied to.
func contrast(a, b ui.Color) float64 {
	a = over(a, b)
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c ui.Color) float64 {
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

func channel(v uint8) float64 {
	s := float64(v) / 255
	if s <= 0.04045 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}

// Increase Contrast has to reach the lines this window draws faintest, or the
// setting does nothing: the rule between two accounts, the empty part of a
// bar and the edge of a card are what separate things when a reader has asked
// for them to separate. 3:1 is WCAG's floor for a graphic that carries
// meaning, which is what these become once they are asked for.
func TestIncreaseContrastStrengthensTheLines(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := &App{}
		var plain, strong palette
		view := func(c *ui.Context) {
			p := a.theme(c)
			if c.Preferences().HighContrast {
				strong = p
				return
			}
			plain = p
		}
		plainTester := ui.NewTester(view, 200, 100)
		if dark {
			plainTester.SetDark(true)
		}
		plainTester.Frame()
		strongTester := ui.NewTester(view, 200, 100)
		if dark {
			strongTester.SetDark(true)
		}
		strongTester.SetPreferences(ui.Preferences{HighContrast: true})
		strongTester.Frame()
		card := over(plain.card, over(plain.bezel, plain.canvas))
		for _, check := range []struct {
			what   string
			plain  ui.Color
			strong ui.Color
		}{
			{"the rule between two accounts", plain.divider, strong.divider},
			{"the empty part of a bar", plain.track, strong.track},
			{"the edge of a card", plain.border, strong.border},
		} {
			before, after := contrast(over(check.plain, card), card), contrast(over(check.strong, card), card)
			if after <= before {
				t.Errorf("dark=%v: %s did not get stronger: %.2f:1 → %.2f:1", dark, check.what, before, after)
			}
			if after < 3 {
				t.Errorf("dark=%v: %s is %.2f:1 with Increase Contrast on, under the 3:1 a graphic needs", dark, check.what, after)
			}
		}
	}
}

// The scale behind a chart is the one line that has a ceiling rather than a
// floor: it is a line under the data, and the rubric wants it quieter than
// 1.5:1 so the data never competes with its own grid. The dark theme's card
// border is under 2:1 there, which is why the chart has its own ink.
func TestTheChartScaleStaysUnderItsData(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := ui.LightTheme()
		if dark {
			theme = ui.DarkTheme()
		}
		p := paletteOf(theme)
		card := over(p.card, over(p.bezel, p.canvas))
		if r := contrast(p.grid, card); r > 1.5 {
			t.Errorf("dark=%v: the chart's scale is %.2f:1 on the card, louder than the 1.5:1 a gridline is allowed", dark, r)
		}
	}
}

// The lines that carry structure are held to 3:1 in the default palette, not
// only once a reader asks for more: the edge of a bar is what says how much of
// it is empty, the rule between two accounts is what makes the matrix a table,
// and the focus ring is the only mark a keyboard user has to find. A card's
// own border is not in this list — a card is a container, its edge is
// decoration, and the shadow under it is what separates it from the canvas.
func TestTheStructuralLinesCarryTheirOwnContrast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := ui.LightTheme()
		if dark {
			theme = ui.DarkTheme()
		}
		p := paletteOf(theme)
		card := over(p.card, over(p.bezel, p.canvas))
		for _, check := range []struct {
			what   string
			fg, bg ui.Color
		}{
			{"the edge of a bar", p.trackEdge, card},
			{"the rule between two accounts", p.divider, card},
			{"the focus ring", p.t.Focus, card},
			// The rules the chart's caption counts: they cross the band tints,
			// so 3:1 on the card is not enough. At 0.75 alpha of the muted they
			// measured 2.82:1 and 2.59:1 over the critical band.
			{"a rule on a chart", p.rule, card},
			{"that rule over the warn band", p.rule, over(p.toneColor(toneWarn).Alpha(chartBandWarn), card)},
			{"that rule over the critical band", p.rule, over(p.toneColor(toneCritical).Alpha(chartBandCritical), card)},
			{"that rule where it crosses the scale", p.rule, over(p.border, card)},
			// The dot beside a provider's name is a state indicator, which is
			// a graphic: at 0.45 alpha of the muted it measured 1.87:1.
			{"the not-configured dot", p.statusColor("not_configured"), p.canvas},
			// The level a percentage chart turns warn and critical at is drawn
			// as a line over its own band: the 1.1:1 wash cannot carry it, so
			// the line does. At 0.55 alpha it measured 2.12:1.
			{"the warn level on its band", p.toneColor(toneWarn).Alpha(chartBandRule), over(p.toneColor(toneWarn).Alpha(chartBandWarn), card)},
			{"the critical level on its band", p.toneColor(toneCritical).Alpha(chartBandRule), over(p.toneColor(toneCritical).Alpha(chartBandCritical), card)},
			// The quarter marks on a quota bar and the run that says how much
			// is left before it runs out. Both are drawn over the empty part of
			// the bar; at 0.38 and 0.5 alpha they measured 1.58:1 and 2.08:1.
			{"the quarter marks on a bar", p.t.TextMuted.Alpha(barTickAlpha), over(p.track, card)},
			{"the projection dashes on a bar", p.toneColor(toneCritical).Alpha(barRunAlpha), over(p.track, card)},
		} {
			if r := contrast(check.fg, check.bg); r < 3 {
				t.Errorf("dark=%v: %s is %.2f:1, under the 3:1 a graphic needs", dark, check.what, r)
			}
		}
	}
}
