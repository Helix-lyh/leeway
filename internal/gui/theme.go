package gui

import (
	"math"
	"strings"
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
//
// Accent vs nav (Pulse fork): accent stays brand violet for primary buttons
// and the focus ring. Sidebar selection is a separate system-blue pill
// (navSelected / navSelectedText) — not the violet, and not MyGo's gray
// SurfacePressed wash. Chart wells use chartWell, a recessed fill under
// card for other pages to seat sparklines without a border.
type palette struct {
	t               *ui.Theme
	dark            bool
	canvas          ui.Color // the window's floor
	canvasTop       ui.Color // and its ceiling, where the accent tint sits
	card            ui.Color // a panel's surface
	cardHi          ui.Color // that surface, with the hover state layer over it
	cardPress       ui.Color // and with the stronger press layer — under the text, not over it
	chartWell       ui.Color // recessed seat for charts/dot grids inside a card (slightly under card)
	sheet           ui.Color // a modal, which has to stay legible over the page
	border          ui.Color // hairlines
	divider         ui.Color // the rule between two account columns, which has to read as one line
	hairline        ui.Color // a rule that only groups: a whisper, not a boundary
	track           ui.Color // a bar's empty part
	rule            ui.Color // a line the caption points at, which has to hold over every band it crosses
	grid            ui.Color // the scale behind a chart: quiet on the card, but never as loud as the data
	markTile        ui.Color // the tile under a mark the provider publishes alone
	tileEdge        ui.Color // and the hairline that keeps it off a dark card
	markShade       ui.Color // the shadow a provider mark sits on
	money           ui.Color // a window with no level: money, which is data and not an action
	accent          ui.Color // brand violet — buttons / focus; not sidebar selection
	navSelected     ui.Color // Pulse blue pill fill for the current sidebar row
	navSelectedText ui.Color // label/icon on that pill (white; ≥4.5:1)
	mono            string
}

func paletteOf(t *ui.Theme) palette {
	p := palette{t: t, dark: t.Dark, mono: "monospace"}
	// The providers that publish a tile draw theirs in near black, so the
	// window's own is near black too, in either theme: the marks are read as
	// one row, not as a row and an exception.
	p.markTile = ui.Hex("#0b0b0e")
	p.tileEdge = ui.RGBA(255, 255, 255, 0.12)
	// System blue deepened one step from #007aff / #0a84ff so white labels
	// clear 4.5:1 on the pill (stock #007aff measures ~4.02:1).
	p.navSelected = ui.Hex("#0071e3")
	p.navSelectedText = ui.RGB(255, 255, 255)
	if t.Dark {
		// Pulse dark spirit: canvas ≈ #141416, card ≈ #1f1f23 (~1.12:1 lift).
		p.canvas = ui.Hex("#141416")
		p.canvasTop = ui.Oklch(0.22, 0.028, 285)
		p.card = ui.Hex("#1f1f23")
		p.cardHi = p.card.Mix(t.Text, stateHover)
		p.cardPress = p.card.Mix(t.Text, statePress)
		p.chartWell = ui.Hex("#1a1a1d") // recessed vs card; for spark/dot seats
		p.sheet = ui.RGBA(28, 28, 34, 0.98)
		p.border = ui.RGBA(255, 255, 255, 0.18)
		p.divider = ui.RGBA(255, 255, 255, 0.36)
		p.hairline = ui.RGBA(255, 255, 255, 0.10)
		p.track = ui.RGBA(255, 255, 255, 0.08)
		p.markShade = ui.RGBA(0, 0, 0, 0.45)
		p.money = ui.Oklch(0.72, 0.09, 255)
		p.accent = ui.Oklch(0.72, 0.17, 285)
		// The dark accent is a light violet: white on it is 2.64:1, so the
		// label on a primary button goes dark instead, which is the choice
		// MyGo's own theme following makes for a light accent.
		t.AccentText = ui.RGB(24, 24, 27)
	} else {
		p.canvas = ui.Hex("#f2f2f6")
		p.canvasTop = ui.Oklch(0.975, 0.016, 285)
		p.card = ui.RGBA(255, 255, 255, 0.82)
		p.cardHi = p.card.Mix(t.Text, stateHover)
		p.cardPress = p.card.Mix(t.Text, statePress)
		p.chartWell = ui.RGBA(0, 0, 0, 0.04) // slight recess under the light card
		p.sheet = ui.RGBA(255, 255, 255, 0.98)
		p.border = ui.RGBA(0, 0, 0, 0.16)
		p.divider = ui.RGBA(0, 0, 0, 0.44)
		p.hairline = ui.RGBA(20, 18, 40, 0.10)
		p.track = ui.RGBA(0, 0, 0, 0.06)
		p.markShade = ui.RGBA(20, 18, 40, 0.16)
		p.money = ui.Oklch(0.52, 0.09, 255)
		p.accent = ui.Oklch(0.55, 0.20, 285)
		// MyGo's own muted grey is 4.33:1 on this window's canvas, just under
		// the 4.5:1 ordinary text needs; this one is 4.72:1 there and 5.10:1
		// on a card. The dark theme's is 6.12:1 and stays.
		t.TextMuted = ui.Hex("#6b6b74")
	}
	// The rules on a chart are read, not decoration: the caption counts them
	// ("图上的竖线"), and they cross the band tints, so they keep the full
	// muted strength that clears 3:1 on the card and on every band.
	p.rule = t.TextMuted
	// The scale is the quietest line the window draws, and it is a line under
	// the data: on the dark card the card border is under 2:1, louder than
	// the rubric's 1.5:1 ceiling for a gridline, so it gets its own ink.
	p.grid = p.border
	if t.Dark {
		p.grid = ui.RGBA(255, 255, 255, 0.12)
	}
	// The focus ring is the one line a keyboard user has to find, so it is the
	// accent at full strength: at 60% over a card it measured 2.4:1, under the
	// 3:1 a state indicator needs. It lives here rather than in theme() so the
	// palette a test can build is the palette the window draws.
	t.Focus = p.accent
	return p
}

// theme applies leeway's colors to the frame's theme and returns them.
func (a *App) theme(c *ui.Context) palette {
	t := *c.Theme()
	scaleType(c.Preferences().TextScale)
	p := paletteOf(&t)
	// Increase Contrast, which is a request for exactly the lines this window
	// draws faintest: the rule between two accounts, the empty part of a bar,
	// the edge of a card. They are decoration at the default setting, so they
	// are drawn lightly; asked for more, they are what has to carry the
	// structure, so they take on the text color until they do.
	if c.Preferences().HighContrast {
		p.border = p.border.Mix(t.Text, 0.45)
		p.divider = p.divider.Mix(t.Text, 0.40)
		p.hairline = p.hairline.Mix(t.Text, 0.55)
		p.track = p.track.Mix(t.Text, 0.50)
	}
	t.Background = p.canvas
	t.Accent = p.accent
	t.AccentHover = p.accent.Mix(ui.RGB(255, 255, 255), 0.18)
	t.AccentPressed = p.accent.Mix(ui.RGB(0, 0, 0), 0.18)
	t.Radius = radiusControl
	c.SetTheme(&t)
	return p
}

// --- type scale ------------------------------------------------------------
//
// Six sizes, two weights: the numbers carry the design, so they get the room
// and the tabular figures, and everything else stays quiet. There is no size
// between these; a page that wants one is a page that wants a different rank.

// The six are variables rather than constants because the system's text size
// multiplies them: a reader who has asked for larger text gets larger text in
// every place the window sets a size, and no call site has to know about the
// setting. MyGo's own theme scales its default size the same way; a window
// that sets its sizes explicitly opts out of that, so it has to do it here.
// Every step is at least 1.2x the one below it, so no two sizes read as the
// same size with a mistake in it.
const (
	baseHero   = 40   // the one number the page is about
	baseTitle  = 25   // a page or a provider's name
	baseValue  = 20   // a window's number on a card
	baseChrome = 16   // the wordmark, a card's title, a column head
	baseBody   = 13   //
	baseLabel  = 10.5 // captions and the smallest print
)

var (
	sizeHero   float32 = baseHero
	sizeTitle  float32 = baseTitle
	sizeValue  float32 = baseValue
	sizeChrome float32 = baseChrome
	sizeBody   float32 = baseBody
	sizeLabel  float32 = baseLabel
	// The caption column is a measure of text like the sizes are, so it scales
	// with them: at 1.5x the caption 登录文件目录 is 94px wide, and a column
	// pinned at 76 cut it to 登录文….
	labelColumn float32 = labelColumnBase
	// So is the width at which a matrix cell's name fits on the number's line.
	matrixInlineMin float32 = matrixInlineBase
)

const trackLabel = 0.7

// The state layers a surface is filled with, one number per state rather than
// a colour per component: meiro's table (m3/theme.go:147-153) is the only one
// in the corpus, and it is why nothing there needs a hover colour of its own.
// Pressed is stronger than hovered, so a click reads as one.
const (
	stateHover = 0.08
	statePress = 0.10
)

// typeScale is the six sizes at a system text size, and scaleType puts them
// back. A scale of zero is a window that never asked, which is the default.
func typeScale(scale float32) (hero, title, value, chrome, body, label float32) {
	if scale <= 0 {
		scale = 1
	}
	return baseHero * scale, baseTitle * scale, baseValue * scale, baseChrome * scale, baseBody * scale, baseLabel * scale
}

func scaleType(scale float32) {
	sizeHero, sizeTitle, sizeValue, sizeChrome, sizeBody, sizeLabel = typeScale(scale)
	// The two measures that are made of text take the factor the sizes took:
	// a reader who asks for bigger type gets a wider caption column and a wider
	// row, not a word cut to fit the one measured at 1x.
	f := sizeBody / baseBody
	labelColumn = labelColumnBase * f
	matrixInlineMin = matrixInlineBase * f
}

// --- spacing and corners ---------------------------------------------------
//
// One 4 DIP rhythm, and four corners: a control, a card's surface, the tray
// it sits in, and a capsule. Everything in the window is one of them.

const (
	space1 = 4
	space2 = 8
	space3 = 12
	space4 = 16
	space5 = 24
	space6 = 32

	radiusControl   = 10
	labelColumnBase = 76 // the caption column every details list shares, at 1x
	radiusTray      = 18
	radiusCapsule   = 999
)

// icon sizes: MyGo sizes an icon by FontSize, so these are sizes, not type.
const (
	iconSmall = 13
	iconBody  = 15
)

// heroUnitScale is how much smaller a number's unit is than its digits. The
// number is what the page leads with; at the digits' own size the % competed
// with it for the same glance.
const heroUnitScale = 0.45

// hero is the number the page leads with, with its unit stepped down. The two
// are one text, so the number stays one string to read, to copy and to
// announce, and the spans sit on one baseline without being told to.
func hero(c *ui.Context, p palette, text string, color ui.Color) ui.Element {
	if digits, ok := strings.CutSuffix(text, "%"); ok {
		return ui.RichText(c,
			ui.Span{Text: digits, Size: sizeHero, Weight: 600},
			ui.Span{Text: "%", Size: sizeHero * heroUnitScale, Weight: 600, Color: p.t.TextMuted},
		).
			FontFeatures("tnum").
			LetterSpacing(-1.1).
			TextColor(color).
			SingleLine()
	}
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
		FontWeight(700).
		LetterSpacing(-0.5).
		SingleLine().
		Role(ui.RoleHeading).
		Level(1)
}

// section is a card's title: a rank under the page's title, so it is set in
// the text color at the chrome size rather than as another muted caption —
// a card's name is something to scan for, not small print.
func section(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).
		FontSize(sizeChrome).
		FontWeight(600).
		TextColor(p.t.Text).
		SingleLine().
		Role(ui.RoleHeading).
		Level(2)
}

// chrome is the wordmark, a column head, a dialog's title: between a card's
// number and its captions.
func chrome(c *ui.Context, p palette, text string) ui.Element {
	// Column heads are words, but the same style carries the lead card's
	// footer money — a number that changes under the reader — so the digits
	// are one width: without it five equal-length amounts measured 45.97 to
	// 57.55 px and the row moved whenever a digit changed shape.
	return ui.Text(c, text).FontSize(sizeChrome).FontWeight(700).SingleLine().FontFeatures("tnum")
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

// label is a caption above or beside a group: small, muted, and spaced only
// when it is Latin — CJK is already set on its own square, and tracking it
// only pulls the characters apart.
func label(c *ui.Context, p palette, text string) ui.Element {
	e := ui.Text(c, text).
		FontSize(sizeLabel).
		FontWeight(600).
		TextColor(p.t.TextMuted).
		SingleLine()
	if !hasCJK(text) {
		e.LetterSpacing(trackLabel)
	}
	return e
}

// hasCJK reports whether a string is written in Chinese, Japanese or Korean,
// which is set without the tracking Latin captions want.
func hasCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x2E80 && r <= 0x9FFF, // radicals through unified ideographs
			r >= 0xAC00 && r <= 0xD7AF, // Hangul syllables
			r >= 0xF900 && r <= 0xFAFF, // compatibility ideographs
			r >= 0xFF00 && r <= 0xFF60: // full-width forms
			return true
		}
	}
	return false
}

// body is ordinary running text.
func body(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).FontSize(sizeBody).LineHeight(1.5).TextColor(p.t.Text)
}

// muted is ordinary text that should not shout: the same size as the body, so
// a sentence keeps its shape whichever of the two it is set in.
func muted(c *ui.Context, p palette, text string) ui.Element {
	return ui.Text(c, text).FontSize(sizeBody).LineHeight(1.5).TextColor(p.t.TextMuted)
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

// card is the surface a panel that only holds content sits on: the tray,
// without the hover. A surface that lightens under the pointer is promising a
// click it does not have, so only the cards that answer the pointer are
// shell() or openCard().
func card(c *ui.Context, p palette, fn func()) ui.Element {
	return shellSurface(c, p, shellOpts{}, fn)
}

// shell is a card that answers the pointer with a hover fill, but is not
// itself a button. History cards use this; cards that open a provider go
// through openCard, which owns the press as well.
func shell(c *ui.Context, p palette, fn func()) ui.Element {
	return shellSurface(c, p, shellOpts{hover: true}, fn)
}

// openCard is a shell that is the target: label, click, hover, and press all
// live on the same surface. Pressed() marks an element clickable, so it is
// only asked on a surface that already has OnClick — never on a child, and
// never on a history shell that only hovers. The press fill is the state
// layer under the text (meiro StateFill / MyGo Button), not a DrawOver wash
// on top of it: DrawOver is paintAfterFn and would darken the words by 10%.
func openCard(c *ui.Context, p palette, label string, open func(), fn func()) ui.Element {
	return shellSurface(c, p, shellOpts{hover: true, label: label, open: open}, fn)
}

type shellOpts struct {
	hover bool
	label string
	open  func()
}

func shellSurface(c *ui.Context, p palette, opt shellOpts, fn func()) ui.Element {
	// One surface, not a plate in a tray. The tray used to be a second rounded
	// rectangle three DIP inside the card's edge, filled with a darkening tint
	// and outlined with a hairline: at 1040 that is a 1px line plus a 3px
	// groove around every panel, and the window drew eight such lines a screen
	// against motrix's zero and codex-monitor's one. What separates a panel is
	// its fill over the canvas — the card is eight units lighter, and the
	// shadow under it does the rest. A line around it only says the panel is
	// a drawing of a panel.
	plate := ui.Column(c).FillWidth()
	plate.Children(func() {
		surface := ui.Column(c).
			FillWidth().
			Padding(space4).
			Gap(space3).
			Radius(radiusTray).
			Background(p.card)
		if opt.open != nil {
			surface.Label(opt.label).
				Role(ui.RoleButton).
				Cursor(ui.CursorPointer).
				Focusable().
				OnClick(opt.open)
			// Pressed before hovered: meiro StateFill (m3/button.go:100-108).
			switch {
			case surface.Pressed():
				surface.Background(p.cardPress)
			case surface.Hovered():
				surface.Background(p.cardHi)
			}
		} else if opt.hover && surface.Hovered() {
			// The pointer lays the state layer on the surface, as meiro's
			// cards do. It used to lift the fill from card to cardHi, which
			// over the canvas is two units: the tray's hairline was what a
			// reader actually saw, and when that went the hover went with it.
			surface.Background(p.cardHi)
		}
		shadowSoft(surface, p)
		surface.Children(fn)
		surface.Transition(stateMotion)
	})
	return plate.Transition(panelMotion)
}

// stateMotion eases a control between its resting and hovered colors: fast
// enough that the pointer feels answered rather than animated at.
var stateMotion = ui.ElementTransition{
	Colors:   true,
	Duration: 140 * time.Millisecond,
	Ease:     easeGlide,
}

// panelMotion is how a panel arrives and how its colors move: the app
// opening, a probe adding a provider, a tab switching, the pointer arriving
// on a card. It is one preset because it is one element — a card can only
// carry one transition, and the pages want both halves of it. A card that was
// already there does not move when another one appears.
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

// btnSmall gives a control the padding and corner of a row's action; btnLarge,
// of a page's. Both are the same shape at two ranks, so a row of controls in a
// card reads as one set rather than three unrelated widgets.
func btnSmall(e ui.Element) ui.Element {
	return e.Padding(5, 10).Radius(radiusControl).FontSize(sizeBody)
}

// ghostFill lays the state layer under a quiet control's ink (meiro StateFill
// on a transparent container → content.Alpha(op)). DrawOver of the same tint
// is paintAfterFn and would sit on the words.
func ghostFill(e ui.Element, p palette) {
	switch {
	case e.Pressed():
		e.Background(p.t.Text.Alpha(statePress))
	case e.Hovered():
		e.Background(p.t.Text.Alpha(stateHover))
	}
}

func btnLarge(e ui.Element) ui.Element {
	return e.Padding(7, 14).Radius(radiusControl).FontSize(sizeBody)
}

// chromeRule is the short vertical hairline between two groups in a title bar.
func chromeRule(c *ui.Context, p palette) ui.Element {
	// The same weight as every other separator: it is what tells the app's
	// name from the state beside it, not an ornament.
	return ui.Box(c).Width(1).Height(14).Shrink(0).Background(p.hairline)
}

// sidebarItem is a navigation row. Unselected glyphs stay muted (MyGo paints
// every SidebarItem icon as t.Accent). Selected rows are a Pulse blue pill:
// MyGo only fills Accent when the row is chosen *and* focused, and otherwise
// uses gray SurfacePressed — so the current row temporarily swaps Accent /
// AccentText / SurfacePressed to the nav pill colors, then forces the fill
// and label ink so focus never falls back to the gray wash.
func sidebarItem(c *ui.Context, p palette, id string, icon *ui.SVG, label string, current bool) ui.Element {
	return paintSidebarNav(c, p, id, icon, label, current)
}

// paintSidebarNav builds a SidebarItem with Pulse nav selection (or quiet
// unselected glyphs). Provider rows call this too so a selected account page
// gets the same blue pill.
func paintSidebarNav(c *ui.Context, p palette, id string, icon *ui.SVG, label string, current bool) ui.Element {
	full := c.Theme()
	if current {
		nav := *full
		// Unfocused chosen uses SurfacePressed + iconColor=Accent; focused
		// chosen uses Accent fill + AccentText ink/icon. White Accent makes
		// the unfocused icon readable; Background() after forces the blue
		// fill even if MyGo painted Accent (white) for the focused case.
		nav.Accent = p.navSelectedText
		nav.AccentText = p.navSelectedText
		nav.SurfacePressed = p.navSelected
		c.SetTheme(&nav)
		item := ui.SidebarItem(c, id, icon, label).
			Background(p.navSelected).
			TextColor(p.navSelectedText)
		c.SetTheme(full)
		return item
	}
	muted := *full
	muted.Accent = full.TextMuted
	c.SetTheme(&muted)
	item := ui.SidebarItem(c, id, icon, label)
	c.SetTheme(full)
	return item
}

// sidebarLive is the right-aligned remaining-quota (or count) on a sidebar
// row: muted Pulse typography, tabular figures. On the blue pill the ink
// flips to navSelectedText so it stays ≥4.5:1.
func sidebarLive(c *ui.Context, p palette, text string, selected bool) ui.Element {
	ink := p.t.TextMuted
	if selected {
		ink = p.navSelectedText
	}
	return ui.Text(c, text).
		FontSize(sizeBody).
		FontFeatures("tnum").
		TextColor(ink).
		SingleLine()
}

// hairline separates blocks inside a card. It is the divider and not the
// border: a rule between two rows is what makes a list a table, and the border
// is a surface's own edge, which sits back. One is read, the other is only
// noticed, so they are two weights and not one.
func hairline(c *ui.Context, p palette) ui.Element {
	return ui.Box(c).FillWidth().Height(1).Background(p.hairline)
}

// dot shows a small level indicator.
func dot(c *ui.Context, color ui.Color) ui.Element {
	return ui.Box(c).Size(8, 8).Radius(4).Background(color)
}

// statusDot is the sidebar's state mark. A colour on its own is not allowed to
// carry a state, so a provider that has not answered yet gets a hollow ring:
// the shape says "nothing to read yet", the colour says how bad it is.
func statusDot(c *ui.Context, p palette, status string) ui.Element {
	if status == "not_configured" {
		return ui.Box(c).Size(8, 8).Radius(4).Border(1.5, p.statusColor(status))
	}
	return dot(c, p.statusColor(status))
}

// pill is a short status label in a tinted capsule.
func pill(c *ui.Context, p palette, text string, t tone) ui.Element {
	color := p.toneColor(t)
	ink := p.toneInk(t)
	return ui.Box(c).
		Padding(3, 8).
		Radius(999).
		Background(color.Alpha(0.14)).
		Children(func() {
			ui.Text(c, text).FontSize(sizeLabel).FontWeight(600).LetterSpacing(0.2).TextColor(ink).SingleLine()
		})
}

// toneInk is the tone as a label: a capsule sets it on a tint of the tone
// itself, which costs contrast the tone does not pay on a plain card, so in
// the light theme it is darkened until the label clears 4.5:1 there too.
// The dark tones already clear it (the weakest is the critical pill at 4.71:1).
func (p palette) toneInk(t tone) ui.Color {
	c := p.toneColor(t)
	if p.dark {
		return c
	}
	return c.Mix(ui.RGB(0, 0, 0), 0.10)
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

// meter is a window's quota bar in the Pulse style: a soft full-width track
// (p.track, no stroke) plus the coloured fill. At 0% the track still marks the
// lane — fill-only left an empty hole. Level marks stay 1px gaps in the fill
// (track showing through). No rim: a stroked capsule read as a grey border.
//
// It carries its value to assistive technology as a meter with a range rather
// than as a picture, so the bar is readable without seeing it. It is not given
// a label: the window's name and its number are already published as text in
// the same row, and a label would put this box in the frame's text registry,
// where every ink inside it is held to 4.5:1 — which no saturated fill can
// meet as text, because the two are a graphic pair at 3:1.
func meter(c *ui.Context, p palette, w model.RateWindow, height float32) ui.Element {
	fraction, ok := windowFraction(w)
	if !ok {
		return ui.Element{}
	}
	f := float32(math.Max(0, math.Min(fraction, 1)))
	e := ui.Box(c).
		FillWidth().
		Height(height).
		Radius(height / 2).
		Role(ui.RoleMeter)
	switch {
	// A provider can report past its own limit (codex and cursor pass the API
	// value through) and the fill is clamped to the bar's width, so the
	// published value is clamped with it: a bar that announces 140 in a 0-100
	// range contradicts the range it declares and the bar it draws.
	case w.UsedPercent != nil:
		e.Range(0, 100, float64(f)*100)
	case w.Used != nil && w.Limit != nil:
		e.Range(0, *w.Limit, float64(f)*(*w.Limit))
	}
	fill := e.Animate("fill", f, numberMotion)
	marks := meterMarks(w)
	ink := barColor(p, w)
	e.Draw(func(pt *ui.Painter, r ui.Rect) {
		if r.W <= 0 || r.H <= 0 {
			return
		}
		pt.Clip(r, r.H/2, func() {
			pt.Fill(r, p.track, 0)
			full := r.W * fill
			if full <= 0 {
				return
			}
			from := float32(0)
			for _, edge := range append(append([]float32{}, marks...), 1) {
				to := edge * r.W
				if from >= full {
					break
				}
				end := to
				switch {
				case full < to:
					end = full // the fill stops inside this stretch
				case edge < 1:
					end = to - 1 // a level is a gap, not ink
				}
				if end > from {
					pt.Fill(ui.Rect{X: r.X + from, Y: r.Y, W: end - from, H: r.H}, ink, 0)
				}
				from = to
			}
		})
	})
	return e
}

// meterMarks is the scale a window's bar carries: a percentage window is read
// against the levels that decide its color, and money has no level, so it gets
// no marks.
func meterMarks(w model.RateWindow) []float32 {
	if w.UsedPercent == nil {
		return nil
	}
	return []float32{float32(percentWarnAt) / 100, float32(percentCriticalAt) / 100}
}

// keyValue is one line of a details list: the caption, then the value, which
// stays on one line, keeps its whole self in a tooltip, and reads as code
// when it is a path.
func keyValue(c *ui.Context, p palette, key, text string) ui.Element {
	return ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Start).Children(func() {
		label(c, p, key).Width(labelColumn).Padding(2, 0, 0, 0)
		v := ui.Text(c, shortenPath(text)).FontSize(sizeBody).Grow(1).SingleLine().Ellipsis("…").Tooltip(text).Selectable()
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

// toneColor maps a level to the color that shows it. These colors are text
// (a window's number) as well as graphics (its bar and its dot), so each one
// is chosen to clear WCAG's 4.5:1 for ordinary text on the surface it sits
// on, which is stricter than the 3:1 graphics need; contrast_test.go holds
// them to it.
func (p palette) toneColor(t tone) ui.Color {
	switch t {
	case toneOK:
		if p.dark {
			return ui.Oklch(0.78, 0.16, 152)
		}
		return ui.Oklch(0.52, 0.15, 152)
	case toneWarn:
		if p.dark {
			return ui.Oklch(0.82, 0.14, 78)
		}
		return ui.Oklch(0.54, 0.15, 62)
	case toneCritical:
		if p.dark {
			return ui.Oklch(0.72, 0.17, 25)
		}
		return ui.Oklch(0.56, 0.20, 25)
	default:
		return p.t.TextMuted
	}
}
