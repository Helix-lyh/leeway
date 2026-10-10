// Package render formats model.Output as JSON or a human-readable view.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
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
	cCyan   = "\x1b[36m"
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

// tableLine is one account: a name, then that account's own quota rows.
// One window stays on the same line; two or more stack so the terminal
// does not wrap a long " · " soup into an unreadable second line.
type tableLine struct {
	head string
	err  string
	rows []string
}

// RenderTable writes each account's native windows. No shared 5h/weekly/
// monthly grid and no synthetic "—" for fields a provider does not have.
// Several accounts under one provider become a group: provider once, then
// each tag on its own line so the tag is what you scan.
func RenderTable(w io.Writer, out *model.Output, color bool) {
	renderTable(w, out, color, true)
}

// RenderTableNoDelta is RenderTable without adjacent-run arrows.
func RenderTableNoDelta(w io.Writer, out *model.Output, color bool) {
	renderTable(w, out, color, false)
}

func renderTable(w io.Writer, out *model.Output, color, showDelta bool) {
	if len(out.Providers) == 0 {
		fmt.Fprintln(w, "（没有服务商）")
		return
	}
	var shown []model.ProviderResult
	for _, pr := range out.Providers {
		if len(pr.Accounts) > 0 {
			shown = append(shown, pr)
		}
	}
	sort.SliceStable(shown, func(i, j int) bool {
		li, lj := providerPrintLines(shown[i]), providerPrintLines(shown[j])
		if li != lj {
			return li < lj
		}
		return shown[i].ID < shown[j].ID
	})
	n := 0
	for _, pr := range shown {
		if n > 0 {
			fmt.Fprintln(w)
		}
		n++
		grouped := len(pr.Accounts) > 1
		if grouped {
			fmt.Fprintln(w, colored(pr.DisplayName, cBold+statusColor(pr.Status, color), color))
		}
		for _, ar := range pr.Accounts {
			printAccount(w, accountLine(pr, ar, grouped, color, showDelta), grouped)
		}
	}
	if n == 0 {
		fmt.Fprintln(w, "（没有服务商）")
		return
	}
	if !out.GeneratedAt.IsZero() {
		fmt.Fprintln(w, colored("截至 "+out.GeneratedAt.Local().Format("15:04"), cGray, color))
	}
}

func providerPrintLines(pr model.ProviderResult) int {
	grouped := len(pr.Accounts) > 1
	n := 0
	if grouped {
		n++
	}
	for _, ar := range pr.Accounts {
		n += accountPrintLines(accountLine(pr, ar, grouped, false, false), grouped)
	}
	return n
}

func accountPrintLines(l tableLine, grouped bool) int {
	if l.err != "" || (!grouped && len(l.rows) <= 1) {
		return 1
	}
	return 1 + len(l.rows)
}

func printAccount(w io.Writer, l tableLine, grouped bool) {
	headPad, rowPad := "", "  "
	if grouped {
		headPad, rowPad = "  ", "    "
	}
	if l.err != "" {
		fmt.Fprintln(w, headPad+l.head+"  "+l.err)
		return
	}
	if !grouped && len(l.rows) == 1 {
		fmt.Fprintln(w, l.head+"  "+l.rows[0])
		return
	}
	fmt.Fprintln(w, headPad+l.head)
	for _, row := range l.rows {
		fmt.Fprintln(w, rowPad+row)
	}
}

// accountHead is the scan target: tag in bold cyan. A lone account still
// prefixes the provider; a grouped account does not — the group header did.
func accountHead(pr model.ProviderResult, ar model.AccountResult, grouped bool, color bool) string {
	tag, extra := accountParts(ar)
	painted := colored(tag, cBold+cCyan, color)
	if extra != "" {
		painted += "  " + colored(extra, cGray, color)
	}
	if plan := identityPlan(ar); plan != "" && !strings.EqualFold(plan, tag) {
		painted += "  " + colored(plan, cGray, color)
	}
	var head string
	if grouped {
		head = painted
	} else {
		head = colored(pr.DisplayName, cBold+statusColor(pr.Status, color), color)
		if tag != "default" {
			head += " " + painted
		} else if plan := identityPlan(ar); plan != "" {
			head += " " + colored(plan, cGray, color)
		}
	}
	if ar.Status != "ok" {
		head += " " + colored(statusText(ar.Status), statusColor(ar.Status, color), color)
	}
	return head
}

// statusText names a status in the table's language. The tokens themselves stay
// English in the JSON contract; only the table spells them out.
func statusText(status string) string {
	switch status {
	case "ok":
		return "正常"
	case "error":
		return "失败"
	case "not_configured":
		return "未配置"
	case "disabled":
		return "已移除"
	case "warn":
		return "注意"
	}
	return status
}

// accountParts splits the tag from a differing email. Identity.Account is
// the email when the provider resolved it.
func accountParts(ar model.AccountResult) (tag, extra string) {
	real := ""
	if ar.Snapshot != nil && ar.Snapshot.Identity != nil {
		real = strings.TrimSpace(ar.Snapshot.Identity.Account)
	}
	name := ar.Name
	if real == "" || strings.EqualFold(real, name) {
		return name, ""
	}
	if name == "" || name == "default" {
		return real, ""
	}
	if local, _, ok := strings.Cut(real, "@"); ok && strings.EqualFold(local, name) {
		return name, ""
	}
	return name, real
}

func identityPlan(ar model.AccountResult) string {
	if ar.Snapshot != nil && ar.Snapshot.Identity != nil {
		return strings.TrimSpace(ar.Snapshot.Identity.Plan)
	}
	return ""
}

// accountLabel is the tag, the real email, or "tag  email" when both exist
// and differ.
func accountLabel(ar model.AccountResult) string {
	tag, extra := accountParts(ar)
	if extra == "" {
		return tag
	}
	return tag + "  " + extra
}

func accountLine(pr model.ProviderResult, ar model.AccountResult, grouped bool, color bool, showDelta bool) tableLine {
	dim := func(s string) string { return colored(s, cGray, color) }
	l := tableLine{head: accountHead(pr, ar, grouped, color)}
	if ar.Snapshot == nil {
		if ar.Error != "" {
			l.err = colored("✗ "+ar.Error, cRed, color)
		} else {
			l.err = dim("没有数据")
		}
		return l
	}
	type item struct{ title, body, delta string }
	var items []item
	for _, win := range ar.Snapshot.Windows {
		body := windowBody(win, color)
		if body == "" {
			continue
		}
		items = append(items, item{win.Title, body, windowDeltaText(win, color, showDelta)})
	}
	if c := ar.Snapshot.Cost; c != nil && c.Amount != nil && *c.Amount != 0 && !costDuplicatesWindow(*c, ar.Snapshot.Windows) {
		items = append(items, item{c.Period, dim(money(*c.Amount, c.Currency)), costDeltaText(c, color, showDelta)})
	}
	titleW, bodyW := 0, 0
	for _, it := range items {
		if n := visibleLen(it.title); n > titleW {
			titleW = n
		}
		if n := visibleLen(it.body); n > bodyW {
			bodyW = n
		}
	}
	for _, it := range items {
		title := it.title
		if n := titleW - visibleLen(it.title); n > 0 {
			title += strings.Repeat(" ", n)
		}
		body := it.body
		if it.delta != "" {
			if n := bodyW - visibleLen(it.body); n > 0 {
				body += strings.Repeat(" ", n)
			}
			body += it.delta
		}
		if title == "" {
			l.rows = append(l.rows, body)
			continue
		}
		l.rows = append(l.rows, title+"  "+body)
	}
	return l
}

func windowDeltaText(win model.RateWindow, color, show bool) string {
	if !show || win.Delta == nil {
		return ""
	}
	return formatDelta(win.Delta.Reset, win.Delta.UsedPercent, win.Delta.Used, win.Currency, remainingWindow(win), color)
}

func costDeltaText(c *model.Cost, color, show bool) string {
	if !show || c == nil || c.Delta == nil {
		return ""
	}
	return formatDelta(c.Delta.Reset, nil, c.Delta.Amount, c.Currency, true, color)
}

func remainingWindow(win model.RateWindow) bool {
	if win.UsedPercent != nil {
		return false
	}
	return win.Used != nil && win.Limit == nil && !strings.HasPrefix(win.ID, "day-")
}

func formatDelta(reset bool, pp, used *float64, currency string, remaining, color bool) string {
	if reset {
		return "  " + colored("重置", cGray, color)
	}
	var v float64
	moneyish := false
	switch {
	case pp != nil:
		v = *pp
	case used != nil:
		v = *used
		moneyish = true
	default:
		return ""
	}
	up := v > 0
	var tone string
	if remaining {
		if up {
			tone = cGreen
		} else {
			tone = cRed
		}
	} else {
		if up {
			tone = cRed
		} else {
			tone = cGreen
		}
	}
	var text string
	if moneyish {
		text = moneyDelta(v, currency)
	} else if color {
		text = ppArrow(v)
	} else {
		text = ppASCII(v)
	}
	return "  " + colored(text, tone, color)
}

func ppArrow(v float64) string {
	sign := "↑"
	if v < 0 {
		sign = "↓"
	}
	return sign + ppAbs(v)
}

func ppASCII(v float64) string {
	sign := "+"
	if v < 0 {
		sign = "-"
	}
	return sign + ppAbs(v)
}

func ppAbs(v float64) string {
	a := math.Abs(v)
	if a >= 10 || math.Abs(a-math.Round(a)) < 0.05 {
		return fmt.Sprintf("%.0f", a)
	}
	return fmt.Sprintf("%.1f", a)
}

func moneyDelta(v float64, currency string) string {
	sign := "+"
	if v < 0 {
		sign = "-"
	}
	return sign + money(math.Abs(v), currency)
}

func windowBody(win model.RateWindow, color bool) string {
	switch {
	case win.UsedPercent != nil:
		body := usageBar(*win.UsedPercent, color)
		if r := resetText(win); r != "" {
			body += " " + colored(r, cGray, color)
		}
		if win.RateLimited {
			body += " " + colored("请求太频繁", cRed, color)
		}
		return body
	case win.Used != nil:
		body := money(*win.Used, win.Currency)
		if win.Limit != nil {
			body += colored(" / "+money(*win.Limit, win.Currency), cGray, color)
		}
		return body
	case win.ResetsAt != nil:
		return colored(resetText(win), cGray, color)
	default:
		return ""
	}
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
