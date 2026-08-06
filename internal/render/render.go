// Package render formats model.Output as JSON or a human-readable table.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"leeway/internal/model"
)

// Colors (ANSI); applied only when the output is a TTY.
const (
	cReset  = "\x1b[0m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cRed    = "\x1b[31m"
	cGray   = "\x1b[90m"
)

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

// RenderTable writes the human-readable view to w.
func RenderTable(w io.Writer, out *model.Output, color bool) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PROVIDER\tSTATUS\tACCOUNT\tWINDOW\tUSED\tLIMIT\t%\tRESET IN")
	any := false
	for _, pr := range out.Providers {
		any = true
		provColor := statusColor(pr.Status, color)
		accs := pr.Accounts
		if len(accs) == 0 {
			accs = []model.AccountResult{{Name: "default", Status: pr.Status, Error: pr.Error}}
		}
		for _, ar := range accs {
			status := colored(ar.Status, statusColor(ar.Status, color), color)
			if ar.Snapshot == nil {
				fmt.Fprintf(tw, "%s\t%s\t%s\t—\t—\t—\t—\t—\n",
					colored(pr.DisplayName, provColor, color), status, ar.Name)
				if ar.Error != "" {
					fmt.Fprintf(tw, "%s\t%s\t%s\t—\t—\t—\t—\t—\n",
						"", cGrayNote(colored("└ "+ar.Error, cGray, color), color), "")
				}
				continue
			}
			windows := ar.Snapshot.Windows
			if len(windows) == 0 {
				fmt.Fprintf(tw, "%s\t%s\t%s\t—\t—\t—\t—\t—\n",
					colored(pr.DisplayName, provColor, color), status, ar.Name)
				continue
			}
			for i, win := range windows {
				name, account := colored(pr.DisplayName, provColor, color), ar.Name
				if i > 0 {
					name, account = "", ""
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					name, status, account, win.Title, usedCell(win), limitCell(win),
					percentCell(win, color), resetCell(win))
			}
			if ar.Snapshot.Cost != nil && ar.Snapshot.Cost.Amount != nil {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t—\t—\t—\n",
					"", "", "", "cost:"+ar.Snapshot.Cost.Period, money(*ar.Snapshot.Cost.Amount, ar.Snapshot.Cost.Currency))
			}
		}
		if pr.LocalCost != nil && pr.LocalCost.Amount != nil {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t—\t—\t—\n",
				"", "", "", "local:"+pr.LocalCost.Period, money(*pr.LocalCost.Amount, pr.LocalCost.Currency))
		}
	}
	if !any {
		fmt.Fprintln(tw, "(no providers)")
	}
	tw.Flush()
}

func usedCell(w model.RateWindow) string {
	if w.Used != nil {
		return money(*w.Used, w.Currency)
	}
	return "—"
}

func limitCell(w model.RateWindow) string {
	if w.Limit != nil {
		return money(*w.Limit, w.Currency)
	}
	return "—"
}

func percentCell(w model.RateWindow, color bool) string {
	if w.UsedPercent == nil {
		return "—"
	}
	pct := *w.UsedPercent
	if !color {
		return fmt.Sprintf("%.0f%%", pct)
	}
	bar := bar8(pct)
	return colored(fmt.Sprintf("%s %.0f%%", bar, pct), percentColor(pct), true)
}

func resetCell(w model.RateWindow) string {
	if w.ResetsAt == nil {
		return "—"
	}
	return model.DurationShort(time.Until(*w.ResetsAt))
}

// bar8 renders an 8-cell usage bar.
func bar8(pct float64) string {
	clamped := pct
	if clamped > 100 {
		clamped = 100
	}
	full := int(clamped / 12.5)
	return strings.Repeat("█", full) + strings.Repeat("░", 8-full)
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

func cGrayNote(s string, _ bool) string { return s }
