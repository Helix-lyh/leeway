package gui

import (
	"github.com/egoist/mygo/ui"

	"leeway/internal/model"
)

// providerMatrix reads several subscriptions of one provider side by side: one
// column per account, one block per window. Four accounts then answer "which
// one is nearly out" on one screen, which a full card per account cannot.
func (a *App) providerMatrix(c *ui.Context, p palette, v providerView) *ui.Element {
	return card(c, p, func() {
		// One Gap(0) column, with the vertical rhythm carried by each row's own
		// padding: the rows then touch, so the rules between the columns meet
		// and read as one line down the whole matrix. With the gaps the card
		// would otherwise add, every rule would be a short dash.
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			matrixRow(c, p, v, 0, 14, func(c *ui.Context, i int, av accountView) {
				a.matrixHead(c, p, v, av)
			})
			for _, w := range matrixWindows(v) {
				matrixSeparator(c, p, v)
				// The window title sits in the first column like a row header,
				// so the rules keep running instead of stopping at each label.
				matrixRow(c, p, v, 12, 0, func(c *ui.Context, i int, av accountView) {
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
				matrixRow(c, p, v, 0, 12, func(c *ui.Context, i int, av accountView) {
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
// part of every row, which is what makes the dividers continuous.
func matrixRow(c *ui.Context, p palette, v providerView, top, bottom float32, cell func(c *ui.Context, i int, av accountView)) {
	ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
		for i := range v.Accounts {
			if i > 0 {
				columnRule(c, p)
			}
			av := v.Accounts[i]
			// The padding belongs to the cells, not to the row: a rule only
			// stretches over the row's content, so padding outside the cells
			// would leave the same gaps the card's own gap does.
			ui.Column(c).Grow(1).MinWidth(0).Gap(6).Padding(top, 0, bottom).Children(func() { cell(c, i, av) })
		}
	})
}

// matrixSeparator is the hairline between two blocks, drawn as one segment per
// column so the rules cross it instead of stopping at its edge.
func matrixSeparator(c *ui.Context, p palette, v providerView) {
	ui.Row(c).FillWidth().Gap(14).AlignItems(ui.Start).Children(func() {
		for i := range v.Accounts {
			if i > 0 {
				columnRule(c, p)
			}
			ui.Column(c).Grow(1).MinWidth(0).Children(func() { hairline(c, p) })
		}
	})
}

// matrixHead is one account's column head: who it is, how it stands, and what
// can be done about it.
func (a *App) matrixHead(c *ui.Context, p palette, v providerView, av accountView) {
	ui.Text(c, displayAccount(av.Name)).FontSize(15).FontWeight(650).LetterSpacing(-0.2).SingleLine()
	if av.Email != "" && av.Email != av.Name {
		// An address is the one thing that tells two accounts apart, so it is
		// allowed a second line instead of an ellipsis.
		ui.Text(c, av.Email).FontSize(sizeLabel).TextColor(p.t.TextMuted).MaxLines(2)
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if av.Plan != "" {
			pill(c, p, av.Plan, toneNeutral)
		}
		pill(c, p, statusLabel(av.Status, av.Cred), statusTone(av.Status))
	})
	// The buttons keep their own line: in a column this narrow a pill and a
	// button on one line squeeze the pill down to a dot.
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		loginActions(c, a, v.ID, av.Name)
		accountActions(c, a, p, v.ID, av.Name)
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
