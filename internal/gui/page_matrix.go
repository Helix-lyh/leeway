package gui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"leeway/internal/model"
)

// providerMatrix reads several subscriptions of one provider side by side: one
// column per account, one block per window. Four accounts then answer "which
// one is nearly out" on one screen, which a full card per account cannot.
func (a *App) providerMatrix(c *ui.Context, p palette, v providerView) ui.Element {
	return card(c, p, func() {
		// One Gap(0) column, with the vertical rhythm carried by each row's own
		// padding: the rows then touch, so the rules between the columns meet
		// and read as one line down the whole matrix.
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			matrixRow(c, p, v, 0, space2, func(c *ui.Context, i int, av accountView) {
				a.matrixHead(c, p, v, av)
			})
			for _, w := range matrixWindows(v) {
				// One line across the whole card: a block boundary is a rule of
				// the table, not a dash inside each column.
				hairline(c, p)
				// The window title sits in the first column like a row header,
				// so the rules keep running instead of stopping at each label.
				matrixRow(c, p, v, space2, 0, func(c *ui.Context, i int, av accountView) {
					if i != 0 {
						return
					}
					ui.Text(c, windowTitle(v.ID, w.ID, w.Title)).FontSize(sizeBody).FontWeight(600).SingleLine()
					if detail := amountDetail(w); detail != "" {
						muted(c, p, detail)
					}
					ui.Spacer(c)
				})
				matrixRow(c, p, v, 0, 0, func(c *ui.Context, i int, av accountView) {
					a.matrixCell(c, p, av, w)
				})
				matrixRow(c, p, v, 0, space2, func(c *ui.Context, i int, av accountView) {
					cw, ok := accountWindow(av, w.ID)
					if !ok {
						return
					}
					points := a.historyFor(v.ID, av.Name, cw.ID)
					burnAlarmInColumn(c, p, displayAccount(av.Name), points, cw)
					resetNote(c, p, points)
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
		chrome(c, p, displayAccount(av.Name)).Tooltip(displayAccount(av.Name))
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
		pill(c, p, statusLabel(av.Status, av.Cred), statusTone(av.Status))
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
	return btnSmall(ui.ButtonBase(c).Label("账号操作").Cursor(ui.CursorPointer).Children(func() {
		ui.Icon(c, iconMore).FontSize(sizeChrome).TextColor(p.t.TextMuted)
	})).Menu(func(m *ui.Menu) {
		for _, method := range loginMethods(providerID) {
			if m.Item(method.label).Chosen() {
				method.open(a, providerID, account)
			}
		}
		m.Separator()
		if canSwitchTo(providerID, ready) {
			if m.Item("让本机用它").Chosen() {
				a.askUse(providerID, account)
			}
			m.Separator()
		}
		if m.Item("复制当前 JSON").Chosen() {
			mygo.Clipboard.WriteText(a.CopyJSON())
			a.toast = "已复制 JSON"
		}
		m.Separator()
		if m.Item(removeAccountLabel(session)).Chosen() {
			a.askLogout(providerID, account)
		}
	})
}

// matrixCell is one account's answer for one window: the number, how far it
// has come, and when it goes back to zero.
func (a *App) matrixCell(c *ui.Context, p palette, av accountView, w model.RateWindow) {
	cw, ok := accountWindow(av, w.ID)
	if !ok {
		muted(c, p, "—")
		return
	}
	if cw.UsedPercent == nil && cw.Used == nil {
		muted(c, p, "未报告")
		return
	}
	value(c, p, amountText(cw), p.amountColor(cw))
	if fraction, ok := windowFraction(cw); ok {
		bar(c, p, fraction, barColor(p, cw), 7)
	}
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
