// Package render formats model.Output as JSON or a human-readable view.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"leeway/internal/model"
)

// Colors (ANSI); applied only when the output is a TTY.
const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cRed    = "\x1b[31m"
	cGray   = "\x1b[90m"
)

// barWidth is the number of cells in a usage bar.
const barWidth = 8

// IsTTY reports whether stdout is a terminal, honoring the COLOR env var.
func IsTTY() bool {
	switch strings.ToLower(os.Getenv("COLOR")) {
	case "0", "false", "never":
		return false
	case "1", "true", "always":
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// RenderJSON writes the machine contract to w.
func RenderJSON(w io.Writer, out *model.Output) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Quota column slots, in display order. Every account line renders all three
// quota columns, so bars align vertically across providers. A slot with no
// matching window shows an unavailable marker, never a synthetic 0%.
const (
	slot5h = iota
	slotWeekly
	slotMonthly
	numSlots
)

// tableLine is one provider account: head, then fixed quota columns and a
// cash/credits column (balances and monetary cost footnotes). A note
// (error/no data) replaces the columns entirely.
type tableLine struct {
	head string
	note string
	cols [numSlots]string
	cash string
}

// RenderTable writes one aligned line per provider account: a bold status-
// colored name, then fixed columns — 5-hour, weekly, monthly quota (braille
// bar + percent + reset) — then cash/credits. Column widths are computed on
// visible text (ANSI escapes ignored); all-ok status noise is omitted.
func RenderTable(w io.Writer, out *model.Output, color bool) {
	if len(out.Providers) == 0 {
		fmt.Fprintln(w, "(no providers)")
		return
	}
	var lines []tableLine
	for _, pr := range out.Providers {
		accs := pr.Accounts
		if len(accs) == 0 {
			accs = []model.AccountResult{{Name: "default", Status: pr.Status, Error: pr.Error}}
		}
		multi := len(accs) > 1
		for _, ar := range accs {
			lines = append(lines, accountLine(pr, ar, multi, color))
		}
	}
	headW := visibleLen("PROVIDER")
	var colW [numSlots]int
	header := [numSlots]string{"5-HOUR", "WEEKLY", "MONTHLY"}
	phW := visibleLen(placeholderCell(false))
	for i := range colW {
		colW[i] = max(phW, visibleLen(header[i]))
	}
	for _, l := range lines {
		if n := visibleLen(l.head); n > headW {
			headW = n
		}
		if l.note != "" {
			continue
		}
		for i, c := range l.cols {
			if n := visibleLen(c); n > colW[i] {
				colW[i] = n
			}
		}
	}
	pad := func(s string, width int) string {
		if n := width - visibleLen(s); n > 0 {
			return s + strings.Repeat(" ", n)
		}
		return s
	}
	// Header row so humans and agents can name the fixed columns.
	var hb strings.Builder
	hb.WriteString(pad("PROVIDER", headW) + "  ")
	for i, h := range header {
		hb.WriteString(pad(h, colW[i]) + "  ")
	}
	hb.WriteString("CASH/CREDITS")
	fmt.Fprintln(w, colored(hb.String(), cGray, color))
	for _, l := range lines {
		head := pad(l.head, headW) + "  "
		if l.note != "" {
			fmt.Fprintln(w, head+l.note)
			continue
		}
		var sb strings.Builder
		sb.WriteString(head)
		for i, c := range l.cols {
			if c == "" {
				c = placeholderCell(color)
			}
			sb.WriteString(pad(c, colW[i]) + "  ")
		}
		sb.WriteString(l.cash)
		fmt.Fprintln(w, strings.TrimRight(sb.String(), " "))
	}
}

// accountHead is the provider name (bold, status-colored) plus an optional
// dim account name and a status badge when anything is not ok.
func accountHead(pr model.ProviderResult, ar model.AccountResult, multi bool, color bool) string {
	head := colored(pr.DisplayName, cBold+statusColor(pr.Status, color), color)
	if multi || ar.Name != "default" {
		head += " " + colored(ar.Name, cGray, color)
	}
	if ar.Status != "ok" {
		head += " " + colored(ar.Status, statusColor(ar.Status, color), color)
	}
	return head
}

// accountLine maps one account's snapshot into the fixed columns.
func accountLine(pr model.ProviderResult, ar model.AccountResult, multi bool, color bool) tableLine {
	dim := func(s string) string { return colored(s, cGray, color) }
	l := tableLine{head: accountHead(pr, ar, multi, color)}
	if ar.Snapshot == nil {
		if ar.Error != "" {
			l.note = colored("✗ "+ar.Error, cRed, color)
		} else {
			l.note = dim("no data")
		}
		return l
	}
	wins := ar.Snapshot.Windows
	var taken [numSlots]bool
	var cash []string
	for _, win := range wins {
		switch {
		case win.UsedPercent != nil:
			slot := quotaSlot(win)
			if slot < 0 || taken[slot] {
				// The table is a compact projection with one canonical cell per
				// period. Additional percent windows remain available in JSON.
				continue
			}
			taken[slot] = true
			l.cols[slot] = quotaCell(win, color)
		case win.Used != nil:
			cash = append(cash, cashSegment(win, color))
		}
	}
	// Zero-amount cost footnotes ("$0.00 credits") are noise; real balances stay.
	if c := ar.Snapshot.Cost; c != nil && c.Amount != nil && *c.Amount != 0 && !costDuplicatesWindow(*c, wins) {
		cash = append(cash, dim(money(*c.Amount, c.Currency)+" "+strings.ToLower(c.Period)))
	}
	l.cash = strings.Join(cash, dim(" · "))
	return l
}

// quotaSlot classifies only explicit canonical durations. Titles are display
// text and reset countdowns are remaining time; neither is a period contract.
func quotaSlot(win model.RateWindow) int {
	if win.WindowMinutes == nil {
		return -1
	}
	switch *win.WindowMinutes {
	case 5 * 60:
		return slot5h
	case 7 * 24 * 60:
		return slotWeekly
	case 30 * 24 * 60:
		return slotMonthly
	default:
		return -1
	}
}

// quotaCell renders a percent window: "⣿⣿⣿⣿⡇⣀⣀⣀  55% 4d 01h". The fixed
// columns imply the window kind, so no title is shown; a rate-limited lane
// appends a red marker.
func quotaCell(win model.RateWindow, color bool) string {
	cell := usageBar(*win.UsedPercent, color)
	if r := resetText(win); r != "" {
		cell += " " + colored(r, cGray, color)
	}
	if win.RateLimited {
		cell += " " + colored("rate-limited", cRed, color)
	}
	return cell
}

// placeholderCell marks a slot that the provider did not return. Zero is a
// valid usage value, so absence must never be rendered as 0%.
func placeholderCell(color bool) string {
	return colored("—", cGray, color)
}

// cashSegment renders a balance window: "¥406.39", optionally " / limit".
// Windows without a currency symbol carry a plain unit from their Title
// (e.g. Cursor credits → "17649.00 / 40000.00 credits") so the column is
// self-describing.
func cashSegment(win model.RateWindow, color bool) string {
	seg := money(*win.Used, win.Currency)
	if win.Limit != nil {
		seg += colored(" / "+money(*win.Limit, win.Currency), cGray, color)
	}
	if win.Currency == "" && win.Title != "" {
		seg += colored(" "+strings.ToLower(win.Title), cGray, color)
	}
	return seg
}

// resetText renders the reset countdown ("1d 12h"), empty when unknown.
func resetText(w model.RateWindow) string {
	if w.ResetsAt == nil {
		return ""
	}
	return model.DurationShort(time.Until(*w.ResetsAt))
}

// costDuplicatesWindow reports whether a cost footnote would repeat a window
// that already shows the same amount (e.g. DeepSeek "Balance" window + cost).
func costDuplicatesWindow(c model.Cost, windows []model.RateWindow) bool {
	for _, win := range windows {
		if win.Used != nil && win.Title == c.Period &&
			money(*win.Used, win.Currency) == money(*c.Amount, c.Currency) {
			return true
		}
	}
	return false
}

// Braille bar glyphs: each cell is a 2×4 dot matrix, so a horizontal bar gets
// two columns of resolution per cell. bTrack is the bottom dot row, used as
// the empty track.
const (
	bFull  = "⣿" // both columns filled
	bHalf  = "⡇" // left column only — the smooth edge
	bTrack = "⣀" // bottom row only
)

// usageBar renders the quota bar plus percent, e.g. "⣿⣿⣿⣿⡇⣀⣀⣀  54%".
// The fill is colored by the usage band, the track dimmed.
func usageBar(pct float64, color bool) string {
	full, half := barCols(pct)
	fill := strings.Repeat(bFull, full)
	if half {
		fill += bHalf
	}
	track := strings.Repeat(bTrack, barWidth-full-bool2int(half))
	num := fmt.Sprintf(" %3.0f%%", pct)
	if !color {
		return fill + track + num
	}
	return colored(fill, percentColor(pct), true) + colored(track, cGray, true) +
		colored(num, percentColor(pct), true)
}

// bar renders the braille usage bar without color or percent text.
func bar(pct float64) string {
	full, half := barCols(pct)
	s := strings.Repeat(bFull, full)
	if half {
		s += bHalf
	}
	return s + strings.Repeat(bTrack, barWidth-full-bool2int(half))
}

// barCols splits a percentage into full and half braille cells; each cell
// holds two dot columns, so the bar resolves to 1/(2*barWidth).
func barCols(pct float64) (full int, half bool) {
	clamped := pct
	if clamped > 100 {
		clamped = 100
	}
	if clamped < 0 {
		clamped = 0
	}
	cols := int(clamped/100*(barWidth*2) + 0.5)
	return cols / 2, cols%2 == 1
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

// visibleLen counts printable runes, skipping ANSI CSI escape sequences.
func visibleLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return n
}

func money(v float64, currency string) string {
	switch currency {
	case "CNY", "¥", "RMB":
		return fmt.Sprintf("¥%.2f", v)
	case "USD", "$":
		return fmt.Sprintf("$%.2f", v)
	case "EUR":
		return fmt.Sprintf("€%.2f", v)
	case "GBP":
		return fmt.Sprintf("£%.2f", v)
	default:
		if currency != "" {
			return fmt.Sprintf("%s %.2f", currency, v)
		}
		return fmt.Sprintf("%.2f", v)
	}
}

func statusColor(status string, color bool) string {
	if !color {
		return ""
	}
	switch status {
	case "ok":
		return cGreen
	case "warn":
		return cYellow
	case "error":
		return cRed
	default:
		return cYellow
	}
}

func percentColor(pct float64) string {
	switch {
	case pct < 50:
		return cGreen
	case pct < 80:
		return cYellow
	default:
		return cRed
	}
}

func colored(s, code string, color bool) string {
	if !color || code == "" || s == "" {
		return s
	}
	return code + s + cReset
}
