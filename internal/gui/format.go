// Package gui is the native-UI desktop frontend of leeway: the same probes,
// config and history as the CLI, shown in a window (github.com/egoist/mygo).
package gui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

// money renders an amount with its currency symbol, as the CLI table does.
func money(v float64, currency string) string {
	switch strings.ToUpper(currency) {
	case "CNY", "¥", "RMB":
		return fmt.Sprintf("¥%.2f", v)
	case "USD", "$":
		return fmt.Sprintf("$%.2f", v)
	default:
		if currency != "" {
			return fmt.Sprintf("%s %.2f", currency, v)
		}
		return fmt.Sprintf("%.2f", v)
	}
}

// percentText renders a 0-100 value as the table does: no decimals.
func percentText(v float64) string { return fmt.Sprintf("%.0f%%", v) }

// amountText is a window's headline value: a percentage, or money.
func amountText(w model.RateWindow) string {
	switch {
	case w.UsedPercent != nil:
		return percentText(*w.UsedPercent)
	case w.Used != nil && w.Limit != nil:
		return money(*w.Used, w.Currency)
	case w.Used != nil:
		return money(*w.Used, w.Currency)
	default:
		return "—"
	}
}

// amountDetail is the secondary line of a window: the limit it is out of, or
// what the money value means.
func amountDetail(w model.RateWindow) string {
	switch {
	case w.UsedPercent != nil:
		return "已用"
	case w.Used != nil && w.Limit != nil:
		return "共 " + money(*w.Limit, w.Currency)
	case w.Used != nil:
		return "余额"
	default:
		return ""
	}
}

// resetLabel is the sentence for a reset countdown, in the same words as
// every other span of time in the window.
func resetLabel(w model.RateWindow, now time.Time) string {
	if w.ResetsAt == nil {
		return ""
	}
	left := w.ResetsAt.Sub(now)
	switch {
	case left < 0:
		return "等待重置"
	case left < time.Minute:
		return "即将重置"
	default:
		return spanText(left) + "后重置"
	}
}

// resetIn is the countdown alone, for the places that name it with a caption
// above it — "重置" over "2 天 4 小时" — rather than in a sentence.
func resetIn(w model.RateWindow, now time.Time) string {
	if w.ResetsAt == nil {
		return ""
	}
	left := w.ResetsAt.Sub(now)
	switch {
	case left < 0:
		return "等待中"
	case left < time.Minute:
		return "即将"
	default:
		return spanText(left)
	}
}

// credSentence says where an account's credential comes from. Chinese and
// Latin take a space where they meet, so a source that starts with a Latin
// letter is set off and one that starts with a Chinese word is not.
func credSentence(cred string) string {
	gap := ""
	if cred != "" && !isCJK(rune(cred[0])) {
		gap = " "
	}
	return "登录信息来自" + gap + cred + "；刷新一次就能看到用量。"
}

// isCJK reports whether r is written in Chinese, Japanese or Korean.
func isCJK(r rune) bool {
	return r >= 0x3000 && r <= 0x9fff || r >= 0xf900 && r <= 0xfaff || r >= 0xff00 && r <= 0xffef
}

// shortenPath makes a path readable in a narrow row without hiding which
// file it names: the home directory becomes ~, and a path that is still too
// long keeps its last one or two components, so the name always survives.
func shortenPath(path string) string {
	if path == "" || path == "未知" {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home+"/") {
		path = "~" + strings.TrimPrefix(path, home)
	}
	if len([]rune(path)) <= 46 {
		return path
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return path
	}
	name, parent := parts[len(parts)-1], parts[len(parts)-2]
	for _, candidate := range []string{parent + "/" + name, name} {
		if len([]rune(candidate)) <= 40 {
			return "…/" + candidate
		}
	}
	return "…/" + name
}

// spanText says how long a stretch of time was, in the UI's own words.
func spanText(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	days := int(d / (24 * time.Hour))
	hours := int(d/time.Hour) % 24
	mins := int(d/time.Minute) % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	case days > 0:
		return fmt.Sprintf("%d 天", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%d 小时 %d 分", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%d 小时", hours)
	default:
		return fmt.Sprintf("%d 分钟", max(mins, 1))
	}
}

// agoText renders how long ago something happened, for "更新于" lines.
func agoText(t, now time.Time) string {
	if t.IsZero() {
		return "还没有查询过"
	}
	d := now.Sub(t)
	switch {
	case d < 5*time.Second:
		return "刚刚"
	case d < time.Minute:
		return fmt.Sprintf("%d 秒前", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
}

// statusText names a provider or account status in the UI's language.
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
	default:
		return status
	}
}

// statusLabel names a provider's or an account's state, and knows the one
// state a bare status cannot express: an account whose key is already on this
// machine has not been probed yet, which is not the same as not being set up.
func statusLabel(status, cred string) string {
	if status == "not_configured" && cred != "" {
		return "待查询"
	}
	return statusText(status)
}

// tone is how a value reads: fine, worth watching, or bad.
type tone int

const (
	toneNeutral tone = iota
	toneOK
	toneWarn
	toneCritical
)

// The levels a percentage window is judged by, in one place: the number's
// color, its bar and the bands behind its chart all read the same two numbers.
// They mirror the CLI's percentColor thresholds.
const (
	percentWarnAt     = 50
	percentCriticalAt = 80
)

// percentTone mirrors the CLI's percentColor thresholds.
func percentTone(pct float64) tone {
	switch {
	case pct < percentWarnAt:
		return toneOK
	case pct < percentCriticalAt:
		return toneWarn
	default:
		return toneCritical
	}
}

// windowTone reads a window's level: percent windows by their use, money
// windows neutral (a balance is not a percentage of anything by itself).
func windowTone(w model.RateWindow) tone {
	if w.RateLimited {
		return toneCritical
	}
	if w.UsedPercent != nil {
		return percentTone(*w.UsedPercent)
	}
	return toneNeutral
}

// statusTone colors a provider or account status.
func statusTone(status string) tone {
	switch status {
	case "ok":
		return toneOK
	case "warn":
		return toneWarn
	case "error":
		return toneCritical
	default:
		return toneNeutral
	}
}

// windowTitle is the display name of a window lane, falling back to the
// provider's declaration and then to well-known ids (same as the CLI).
func windowTitle(providerID, windowID, title string) string {
	if title != "" {
		return title
	}
	if p, ok := provider.Get(providerID); ok {
		for _, w := range p.DeclaredWindows() {
			if w.ID == windowID && w.Title != "" {
				return w.Title
			}
		}
	}
	switch windowID {
	case "included":
		return "Included"
	case "credits":
		return "Credits"
	case "cost":
		return "Balance"
	case "ondemand":
		return "On-demand"
	case "renewal":
		return "Renewal"
	}
	return windowID
}
