package gui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"leeway/internal/model"
)

// providerMatrix reads several subscriptions of one provider side by side: one
// column per account, one block per window. Four accounts then answer "which
// one is nearly out" on one screen, which a full card per account cannot.
// Dense but calm: column dividers mark the grid; no zebra, no hairline between
// window blocks, no outer table frame beyond the card surface.
func (a *App) providerMatrix(c *ui.Context, p palette, v providerView) ui.Element {
	return card(c, p, func() {
		// One Gap(0) column, with the vertical rhythm carried by each row's own
		// padding: the rows then touch, so the rules between the columns meet
		// and read as one line down the whole matrix.
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			matrixRow(c, p, v, 0, space3, func(c *ui.Context, i int, av accountView) {
				a.matrixHead(c, p, v, av)
			})
			for _, w := range matrixWindows(v) {
				// Block boundary is padding alone — a full-width hairline would
				// be a table rule Pulse does not draw between dense lanes.
				inline := matrixColumnWidth(c, len(v.Accounts)) >= matrixInlineMin
				matrixRow(c, p, v, space3, 0, func(c *ui.Context, i int, av accountView) {
					a.matrixCell(c, p, v.ID, av, w, i == 0, inline)
				})
				matrixRow(c, p, v, 0, space3, func(c *ui.Context, i int, av accountView) {
					if _, ok := accountWindow(av, w.ID); !ok {
						return
					}
					resetNote(c, p, a.historyFor(v.ID, av.Name, w.ID))
				})
			}
		})
	})
}

// matrixRow is one grid row of the matrix. The rule between two columns is
// drawn inside the row's own gap, which it does not take: the columns keep
// their full width, and the rule still spans the row's whole height, so the
// rules of touching rows read as one line down the matrix.
func matrixRow(c *ui.Context, p palette, v providerView, top, bottom float32, cell func(c *ui.Context, i int, av accountView)) {
	ui.Row(c).FillWidth().Gap(space4).AlignItems(ui.Start).
		Dividers(1, p.divider).
		Children(func() {
			for i := range v.Accounts {
				av := v.Accounts[i]
				// The row's padding is carried by the cells, so every column
				// starts and ends on the same line and the rule spans the lot.
				ui.Column(c).Grow(1).MinWidth(0).Gap(space2).Padding(top, 0, bottom).Children(func() { cell(c, i, av) })
			}
		})
}

// matrixHead is one account's column head: who it is, how it stands, and the
// one control that acts on it.
func (a *App) matrixHead(c *ui.Context, p palette, v providerView, av accountView) {
	// The control sits on the name's line, where a card's actions sit
	// everywhere else in the window; the pills keep a line of their own,
	// because a pill sharing a line with a control in a column this narrow is
	// squeezed down to a dot.
	ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.Center).Children(func() {
		// The head is quieter than the numbers under it: it names the column,
		// it does not compete with what the column says.
		// The column is named by the account; a narrow column cuts it, so the
		// whole name hangs off the label the way every other cut string does.
		chrome(c, p, displayAccount(av.Name)).MinWidth(20).Focusable().Tooltip(displayAccount(av.Name))
		ui.Spacer(c)
		matrixActions(c, a, p, v.ID, av.Name)
	})
	if av.Email != "" && av.Email != av.Name {
		// One line, cut at the end, with the whole address on the tooltip. An
		// address has no space in it, so a second line does not wrap it — it
		// breaks it in the middle of a word ("alpha@example.c" / "om"), which
		// reads as a broken column rather than a shortened one. The column is
		// named by the account name above, which carries its own tooltip for
		// the same reason this line does.
		ui.Text(c, av.Email).
			FontSize(sizeLabel).
			TextColor(p.t.TextMuted).
			SingleLine().
			Ellipsis("…").
			Tooltip(av.Email)
	}
	ui.Row(c).Gap(space2).AlignItems(ui.Center).Children(func() {
		if av.Plan != "" {
			pill(c, p, av.Plan, toneNeutral)
		}
		// At the window's minimum width with seven accounts a pill is squeezed
		// to "…": it still says which state it is, on the tooltip.
		pill(c, p, statusLabel(av.Status, av.Cred), statusTone(av.Status)).
			Tooltip(statusLabel(av.Status, av.Cred))
	})
}

// matrixActions is the one control a matrix column carries: the ways to give
// the account credentials and everything that can be done with it, in a single
// menu. A pair of buttons per column is eight controls on the densest page in
// the window, and the column head already says whose they are.
//
// It mirrors accountActions with the login methods folded in, because a matrix
// column has room for one control and view.go's two menus cannot be merged
// from here.
func matrixActions(c *ui.Context, a *App, p palette, providerID, account string) ui.Element {
	ready := accountSwitchReady(a, providerID, account)
	session := accountUsesSession(a, providerID, account)
	btn := btnSmall(ui.ButtonBase(c).Label("账号").Cursor(ui.CursorPointer).Children(func() {
		ui.Icon(c, iconMore).FontSize(sizeChrome).TextColor(p.t.TextMuted)
	}))
	ghostFill(btn, p)
	return btn.Menu(func(m *ui.Menu) {
		for _, method := range loginMethods(providerID) {
			if m.Item(method.label).Chosen() {
				method.open(a, providerID, account)
			}
		}
		m.Separator()
		if canSwitchTo(providerID, ready) {
			if m.Item("启用").Chosen() {
				a.askUse(providerID, account)
			}
			m.Separator()
		}
		if m.Item("复制数据").Chosen() {
			mygo.Clipboard.WriteText(a.CopyJSON())
			a.toast = "已复制"
		}
		m.Separator()
		if m.Item(removeAccountLabel(session)).Chosen() {
			a.askLogout(providerID, account)
		}
	})
}

// matrixInlineBase is how wide one account's column has to be, at 1x, for the
// window's name to ride on the number's line. The widest thing that line holds
// is "Monthly 已用 100%": 51.2 + 25.8 + 54.9 and the three space2 gaps between
// them make 155.9, so below that the name takes a line of its own — the number
// is the one thing in the cell that must not be cut. It is a measure of text,
// so it scales with the type the way the caption column does.
const matrixInlineBase = 156

// matrixColumnWidth is how wide one account's column is. Every cell in a row
// has to make the same choice about where the window's name goes, and the
// layout engine has not run when the cells are built, so the width is worked
// out here from the window: the page's measure, less the card's tray and
// padding, shared between the columns and the gaps between them.
func matrixColumnWidth(c *ui.Context, accounts int) float32 {
	winW, _ := c.Size()
	inner := contentWidth(winW)
	inner -= 2 * space4
	return (inner - space4*float32(accounts-1)) / float32(accounts)
}

// matrixCell is one account's answer for one window: the number, how far it
// has come, and when it goes back to zero. While the column is wide enough,
// every column carries the window's name on the left of the number's line and
// the caption ("已用", or the money's) against the number, so a window is
// three lines in every column — the number, the bar, and when it resets. When
// it is not wide enough the name takes a line of its own, which keeps the
// numbers and so the bars on one line across the table.
func (a *App) matrixCell(c *ui.Context, p palette, providerID string, av accountView, w model.RateWindow, header, inline bool) {
	cw, ok := accountWindow(av, w.ID)
	// The 13px name sits on the 20px number's baseline. End alignment matches
	// box bottoms, and the number descends further. The caption is that same
	// face: muted() carries a 1.5 line height, which is a taller box than the
	// name and does not share the number's baseline.
	title := windowTitle(providerID, w.ID, w.Title)
	// The caption carries this account's own limit ("共 $…"). The row's
	// prototype is whichever account reported the window first, and another
	// column's pool is not that number.
	detail := ""
	if ok && (cw.UsedPercent != nil || cw.Used != nil) {
		detail = amountDetail(cw)
	}
	namePad := ui.Font{Size: sizeValue, Weight: 600}.Metrics().Descent -
		ui.Font{Size: sizeBody, Weight: 600}.Metrics().Descent
	padOf := func(onNumber bool) float32 {
		if onNumber {
			return namePad
		}
		return 0
	}
	titleText := func(onNumber bool) {
		// The name is cut, never broken, and carries itself whole on the
		// tooltip the way every other cut string in the window does. Only the
		// row header takes a tab stop: the other columns repeat the same word.
		e := ui.Text(c, title).
			FontSize(sizeBody).
			FontWeight(600).
			SingleLine().
			Ellipsis("…").
			Tooltip(title).
			Padding(0, 0, padOf(onNumber), 0)
		if header {
			e.Focusable()
		}
	}
	detailText := func(onNumber bool) {
		if detail == "" {
			return
		}
		// The number keeps its width. Past that, this word gives way and
		// shortens: a money pair that refuses to is drawn into the next column.
		ui.Text(c, detail).
			FontSize(sizeBody).
			FontWeight(600).
			TextColor(p.t.TextMuted).
			SingleLine().
			Ellipsis("…").
			Tooltip(detail).
			Padding(0, 0, padOf(onNumber), 0)
	}
	if !inline {
		// The name's line is kept in every column, empty where the name is not:
		// the cells have to be the same height in every column or the numbers
		// and the bars stop reading across the table.
		ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.End).Children(func() {
			if header {
				titleText(false)
				detailText(false)
				return
			}
			// A space in the name's own style, so the line box is the one the
			// name would have had: a spacer of a guessed height is off by the
			// font's leading, which is four pixels at this size.
			ui.Text(c, " ").FontSize(sizeBody).FontWeight(600)
		})
	}
	ui.Row(c).FillWidth().Gap(space2).AlignItems(ui.End).Children(func() {
		if inline {
			titleText(true)
		}
		ui.Spacer(c)
		if inline {
			detailText(true)
		}
		switch {
		case !ok:
			muted(c, p, "—")
		case cw.UsedPercent == nil && cw.Used == nil:
			muted(c, p, "无数据")
		default:
			// The number is the one thing in the cell that must not be cut, and
			// a provider's window title ("Codex Spark Weekly") can eat the whole
			// line: the value keeps its width and the name is what ellipsizes.
			// It is capped at the cell all the same — past that it would be drawn
			// over its neighbour, and two numbers the eye runs together are worse
			// than one that says it is short.
			value(c, p, amountText(cw), p.amountColor(cw)).
				Shrink(0).
				MaxWidthPercent(100).
				Ellipsis("…")
		}
	})
	if !ok || (cw.UsedPercent == nil && cw.Used == nil) {
		return
	}
	meter(c, p, cw, 7)
	if meta := resetMeta(cw, a.now()); meta != "" {
		muted(c, p, meta)
	}
}

// matrixWindows is every window the accounts report, in the order they first
// appear, so the rows are the same ones for every column.
func matrixWindows(v providerView) []model.RateWindow {
	var out []model.RateWindow
	seen := map[string]bool{}
	for i := range v.Accounts {
		snap := v.Accounts[i].Snapshot
		if snap == nil {
			continue
		}
		for _, w := range snap.Windows {
			if seen[w.ID] {
				continue
			}
			seen[w.ID] = true
			out = append(out, w)
		}
	}
	return out
}

// accountWindow finds one window of one account, which an account may not
// report at all.
func accountWindow(av accountView, id string) (model.RateWindow, bool) {
	if av.Snapshot == nil {
		return model.RateWindow{}, false
	}
	for _, w := range av.Snapshot.Windows {
		if w.ID == id {
			return w, true
		}
	}
	return model.RateWindow{}, false
}
