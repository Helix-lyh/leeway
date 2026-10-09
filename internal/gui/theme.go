package gui

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"leeway/internal/model"
)

// palette is leeway's own set of colors and metrics, derived from the theme
// MyGo follows (the system's, or the one the settings window asks for).
//
// Colors are written in Oklch so they stay perceptually even across the
// light and dark themes, and so the accent can use the wide gamut of a
// recent Mac's display.
type palette struct {
	t         *ui.Theme
	dark      bool
	canvas    ui.Color // the window's floor
	canvasTop ui.Color // and its ceiling, where the accent tint sits
	card      ui.Color // a panel's surface
	cardHi    ui.Color // that surface, under the pointer
	sheet     ui.Color // a modal, which has to stay legible over the page
	bezel     ui.Color // the tray a panel sits in
	bezelHi   ui.Color // that tray, under the pointer
	border    ui.Color // hairlines
	divider   ui.Color // the rule between two account columns, which has to read as one line
	track     ui.Color // a bar's empty part
	markTile  ui.Color // the tile under a mark the provider publishes alone
	tileEdge  ui.Color // and the hairline that keeps it off a dark card
	markShade ui.Color // the shadow a provider mark sits on
	accent    ui.Color
	mono      string
}

func paletteOf(t *ui.Theme) palette {
	p := palette{t: t, dark: t.Dark, mono: "monospace"}
	// The providers that publish a tile draw theirs in near black, so the
	// window's own is near black too, in either theme: the marks are read as
	// one row, not as a row and an exception.
	p.markTile = ui.Hex("#0b0b0e")
	p.tileEdge = ui.RGBA(255, 255, 255, 0.12)
	if t.Dark {
		p.canvas = ui.Hex("#0d0d11")
		p.canvasTop = ui.Oklch(0.20, 0.035, 285)
		p.card = ui.RGBA(255, 255, 255, 0.055)
		p.cardHi = ui.RGBA(255, 255, 255, 0.095)
		p.sheet = ui.RGBA(28, 28, 34, 0.98)
		p.bezel = ui.RGBA(255, 255, 255, 0.035)
		p.bezelHi = ui.RGBA(255, 255, 255, 0.06)
		p.border = ui.RGBA(255, 255, 255, 0.10)
		p.divider = ui.RGBA(255, 255, 255, 0.26)
		p.track = ui.RGBA(255, 255, 255, 0.11)
		p.markShade = ui.RGBA(0, 0, 0, 0.45)
		p.accent = ui.Oklch(0.72, 0.17, 285)
	} else {
		p.canvas = ui.Hex("#f2f2f6")
		p.canvasTop = ui.Oklch(0.975, 0.016, 285)
		p.card = ui.RGBA(255, 255, 255, 0.82)
		p.cardHi = ui.RGBA(255, 255, 255, 0.98)
		p.sheet = ui.RGBA(255, 255, 255, 0.98)
		p.bezel = ui.RGBA(20, 18, 40, 0.035)
		p.bezelHi = ui.RGBA(20, 18, 40, 0.055)
		p.border = ui.RGBA(0, 0, 0, 0.07)
		p.divider = ui.RGBA(0, 0, 0, 0.18)
		p.track = ui.RGBA(0, 0, 0, 0.075)
		p.markShade = ui.RGBA(20, 18, 40, 0.16)
		p.accent = ui.Oklch(0.55, 0.20, 285)
	}
	return p
}

// theme applies leeway's colors to the frame's theme and returns them.
func (a *App) theme(c *ui.Context) palette {
	t := *c.Theme()
	p := paletteOf(&t)
	t.Background = p.canvas
	t.Accent = p.accent
	t.AccentHover = p.accent.Mix(ui.RGB(255, 255, 255), 0.18)
	t.AccentPressed = p.accent.Mix(ui.RGB(0, 0, 0), 0.18)
	t.Focus = p.accent.Alpha(0.6)
	t.Radius = 9
	c.SetTheme(&t)
	return p
}

// --- type scale ------------------------------------------------------------
//
// Six sizes, three weights: the numbers carry the design, so they get the
// room and the tabular figures, and everything else stays quiet.

const (
	sizeHero   = 40 // the one number the page is about
	sizeTitle  = 25 // a page or a provider's name
	sizeValue  = 21 // a window's number on a card
	sizeBody   = 13
	sizeSmall  = 12
	sizeLabel  = 11 // captions and section labels
	trackLabel = 0.7
)

// hero is the number the page leads with.
func hero(c *ui.Context, p palette, text string, color ui.Color) ui.Element {
	return ui.Text(c, text).
		FontSize(sizeHero).
		FontWeight(600).
		FontFeatures("tnum").
		LetterSpacing(-1.1).
		TextColor(color).
		SingleLine()
}

// title is a page's or a provider's name, and the top of the page for
// assistive technology, which navigates by headings.
func title(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).
		FontSize(sizeTitle).
		FontWeight(650).
		LetterSpacing(-0.5).
		SingleLine().
		Role(ui.RoleHeading).
		Level(1)
}

// section is a card's title: the same look as a label, one rank under the
// page's title.
func section(c *ui.Context, p palette, text string) ui.Element {
	return label(c, p, text).Role(ui.RoleHeading).Level(2)
}

// value is a window's number on a card.
func value(c *ui.Context, p palette, text string, color ui.Color) ui.Element {
	return ui.Text(c, text).
		FontSize(sizeValue).
		FontWeight(600).
		FontFeatures("tnum").
		LetterSpacing(-0.4).
		TextColor(color).
		SingleLine()
}

// label is a caption above or beside a group: small, spaced, muted.
func label(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).
		FontSize(sizeLabel).
		FontWeight(600).
		LetterSpacing(trackLabel).
		TextColor(p.t.TextMuted).
		SingleLine()
}

// body is ordinary running text.
func body(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).FontSize(sizeBody).TextColor(p.t.Text)
}

// muted is ordinary text that should not shout.
func muted(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).FontSize(sizeSmall).TextColor(p.t.TextMuted)
}

// --- surfaces --------------------------------------------------------------

// shadowSoft is the two-layer shadow panels cast: a tight one for the edge
// and a wide one for the lift.
func shadowSoft(e ui.Element, p palette) ui.Element {
	if p.dark {
		return e.Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.5)).
			Shadow(0, 6, 18, -6, ui.RGBA(0, 0, 0, 0.45))
	}
	return e.Shadow(0, 1, 2, 0, ui.RGBA(20, 18, 40, 0.05)).
		Shadow(0, 8, 22, -10, ui.RGBA(20, 18, 40, 0.13))
}

// card is the surface every panel sits on: the tray, without the hover.
func card(c *ui.Context, p palette, fn func()) ui.Element {
	return shell(c, p, fn)
}

// shell is a card in a tray: an outer bezel with a hairline, the surface
// inside it, and one soft shadow under the whole thing, so a panel reads as
// a plate set into the window rather than as a rectangle drawn on it.
func shell(c *ui.Context, p palette, fn func()) ui.Element {
	outer := ui.Column(c).
		FillWidth().
		Padding(3).
		Radius(19).
		Background(p.bezel).
		Border(1, p.border)
	hovered := outer.Hovered()
	if hovered {
		outer.Background(p.bezelHi)
	}
	shadowSoft(outer, p)
	outer.Children(func() {
		inner := ui.Column(c).
			FillWidth().
			Padding(18).
			Gap(14).
			Radius(16).
			Background(p.card).
			Children(fn)
		if hovered {
			inner.Background(p.cardHi)
		}
		inner.Transition(panelMotion)
	})
	return outer.Transition(panelMotion)
}

// cardMotion eases a surface between its resting and hovered colors.
var cardMotion = ui.ElementTransition{
	Colors:   true,
	Duration: 160 * time.Millisecond,
	Ease:     easeGlide,
}

// panelMotion is cardMotion plus the way a panel arrives: the app opening, a
// probe adding a provider, a tab switching. It is what the pages use, so a
// card that was already there does not move when another one appears.
var panelMotion = ui.ElementTransition{
	Colors:   true,
	Enter:    &ui.Motion{Y: 8, Opacity: 1},
	Exit:     &ui.Motion{Opacity: 0},
	Duration: 240 * time.Millisecond,
	Ease:     easeGlide,
}

// easeGlide decelerates hard at the end, the way a sheet settles.
func easeGlide(t float32) float32 {
	return 1 - float32(math.Pow(1-float64(t), 3.4))
}

// iconButton is a compact round action with one icon in it.
func iconButton(c *ui.Context, p palette, label string, ic *ui.SVG, on func()) ui.Element {
	b := ui.Button(c, "").Label(label).Padding(7).Radius(999).Children(func() {
		ui.Icon(c, ic).FontSize(15).TextColor(p.t.TextMuted)
	})
	if on != nil && b.Clicked() {
		on()
	}
	return b
}

// columnRule separates two accounts in the matrix. AlignSelf(Stretch) makes it
// as tall as the row's other children, so the columns read as columns down the
// whole matrix and not only inside one cell.
func columnRule(c *ui.Context, p palette) ui.Element {
	return ui.Box(c).Width(1).Background(p.divider).AlignSelf(ui.Stretch)
}

// hairline separates blocks inside a card.
func hairline(c *ui.Context, p palette) ui.Element {
	return ui.Box(c).FillWidth().Height(1).Background(p.border)
}

// dot shows a small level indicator.
func dot(c *ui.Context, color ui.Color) ui.Element {
	return ui.Box(c).Size(8, 8).Radius(4).Background(color)
}

// pill is a short status label in a tinted capsule.
func pill(c *ui.Context, p palette, text string, t tone) ui.Element {
	color := p.toneColor(t)
	return ui.Box(c).
		Padding(3, 8).
		Radius(999).
		Background(color.Alpha(0.14)).
		Children(func() {
			ui.Text(c, text).FontSize(sizeLabel).FontWeight(600).LetterSpacing(0.2).TextColor(color).SingleLine()
		})
}

// --- identity --------------------------------------------------------------

// providerHue is the hue each provider's mark keeps, chosen to spread the
// seven of them around the wheel at one lightness, so the set reads as a
// family rather than as a rainbow.
func providerHue(id string) float32 {
	switch id {
	case "deepseek":
		return 262
	case "kimi":
		return 320
	case "grok":
		return 215
	case "codex":
		return 158
	case "cursor":
		return 285
	case "opencode-go":
		return 32
	case "commandcode":
		return 345
	}
	var h uint32 = 2166136261
	for i := 0; i < len(id); i++ {
		h = (h ^ uint32(id[i])) * 16777619
	}
	return float32(h % 360)
}

// mark is a provider's identity: the provider's own icon, with the state of
// its last probe as a small dot at the corner. A provider we have no icon for
// falls back to its initial on a tint of its own hue, which is at least ours
// rather than someone else's mark drawn wrong.
func mark(c *ui.Context, p palette, id, name string, size float32, status string) ui.Element {
	e := ui.Column(c).
		Size(size, size).
		Radius(size*0.26).
		Justify(ui.Center).
		AlignItems(ui.Center).
		Shadow(0, 1, size*0.16, 0, p.markShade)
	if b := providerMark(id); b != nil {
		if glyphMarks[id] {
			e.Background(p.markTile).Border(1, p.tileEdge)
		}
		e.Children(func() { ui.Image(c, b).Size(size, size) })
	} else {
		l := float32(0.62)
		if p.dark {
			l = 0.58
		}
		base := ui.Oklch(l, 0.15, providerHue(id))
		e.LinearGradient(ui.LinearGradient{
			From:  base.Mix(ui.RGB(255, 255, 255), 0.14),
			To:    base.Mix(ui.RGB(0, 0, 0), 0.10),
			Angle: 145,
			Oklab: true,
		}).Children(func() {
			// The letter is decoration: the provider's name is text of its
			// own next to the mark, and reading both says it twice.
			ui.Text(c, initial(name)).
				FontSize(size * 0.44).
				FontWeight(700).
				TextColor(ui.RGBA(255, 255, 255, 0.96)).
				SingleLine().
				Role(ui.RoleNone)
		})
	}
	if status != "" && status != "ok" {
		e.Children(func() {
			ui.Box(c).
				Absolute().
				Bottom(-1).
				Right(-1).
				Size(size*0.34, size*0.34).
				Radius(size*0.17).
				Background(p.statusColor(status)).
				Border(1.5, p.canvas)
		})
	}
	return e
}

// initial is the letter a provider's mark shows.
func initial(name string) string {
	for _, r := range name {
		if r == ' ' {
			continue
		}
		return string([]rune{r})
	}
	return "?"
}

// --- data marks ------------------------------------------------------------

// bar draws a proportion: a track, a fill that eases in when it first
// appears, and nothing else.
// numberMotion is how a number and the bar under it move to a new value. The
// two share it so a refresh reads as one movement, not two.
const numberMotion = 520 * time.Millisecond

// windowValue is a window's headline number, the one the card shows big.
func windowValue(w model.RateWindow) float64 {
	switch {
	case w.UsedPercent != nil:
		return *w.UsedPercent
	case w.Used != nil:
		return *w.Used
	}
	return 0
}

// tweenValue eases a value the way its bar does. host is the element the
// animation state hangs on, key tells two numbers on one host apart.
func tweenValue(host ui.Element, key string, v float64) float64 {
	return float64(host.Animate(key, float32(v), numberMotion))
}

// tweenText formats a window's value while it moves.
func tweenText(w model.RateWindow, v float64) string {
	switch {
	case w.UsedPercent != nil:
		return percentText(v)
	case w.Used != nil:
		return money(v, w.Currency)
	}
	return amountText(w)
}

func bar(c *ui.Context, p palette, fraction float64, color ui.Color, height float32) ui.Element {
	f := float32(math.Max(0, math.Min(fraction, 1)))
	return ui.Box(c).
		FillWidth().
		Height(height).
		Radius(height / 2).
		Background(p.track).
		Clip().
		Children(func() {
			w := float32(0)
			if f > 0 {
				w = ui.Box(c).Animate("fill", f, numberMotion) * 100
			}
			if w <= 0 {
				return
			}
			ui.Box(c).
				Height(height).
				WidthPercent(w).
				Radius(height / 2).
				LinearGradient(ui.LinearGradient{
					From:  color,
					To:    color.Mix(ui.RGB(255, 255, 255), 0.22),
					Angle: 90,
					Oklab: true,
				})
		})
}

// labeledRow is a caption on the left and a control on the right, as
// settings rows read. The control is built inside the row, so give it as a
// function.
func labeledRow(c *ui.Context, p palette, text string, control func()) ui.Element {
	return ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
		body(c, p, text)
		ui.Box(c).Grow(1)
		control()
	})
}

// keyValue is one line of a details list: the caption, then the value, which
// stays on one line, keeps its whole self in a tooltip, and reads as code
// when it is a path.
func keyValue(c *ui.Context, p palette, key, text string) ui.Element {
	return ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
		label(c, p, key).Width(76).Padding(2, 0, 0, 0)
		v := ui.Text(c, shortenPath(text)).FontSize(sizeSmall).Grow(1).SingleLine().Ellipsis("…").Tooltip(text).Selectable()
		if looksLikePath(text) {
			v.Font(p.mono).TextColor(p.t.TextMuted)
		}
	})
}

// looksLikePath reports whether a value is a file path or a URL, which reads
// better in a monospaced face.
func looksLikePath(s string) bool {
	if len(s) < 2 {
		return false
	}
	return s[0] == '/' || s[0] == '~' || (len(s) > 8 && (s[:8] == "https://" || s[:7] == "http://"))
}

// amountColor colors a window's headline number: the level when the window
// is a percentage, plain text when it is money.
func (p palette) amountColor(w model.RateWindow) ui.Color {
	if windowTone(w) == toneNeutral {
		return p.t.Text
	}
	return p.toneColor(windowTone(w))
}

// toneColor maps a level to the color that shows it.
func (p palette) toneColor(t tone) ui.Color {
	switch t {
	case toneOK:
		if p.dark {
			return ui.Oklch(0.78, 0.16, 152)
		}
		return ui.Oklch(0.62, 0.15, 152)
	case toneWarn:
		if p.dark {
			return ui.Oklch(0.82, 0.14, 78)
		}
		return ui.Oklch(0.68, 0.15, 62)
	case toneCritical:
		if p.dark {
			return ui.Oklch(0.72, 0.17, 25)
		}
		return ui.Oklch(0.58, 0.20, 25)
	default:
		return p.t.TextMuted
	}
}
