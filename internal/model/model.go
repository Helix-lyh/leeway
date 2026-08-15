// Package model defines the core data model of leeway.
//
// This is the JSON contract for agent integration: field names and JSON tags
// are part of the public interface and must not change without a version bump.
package model

import (
	"fmt"
	"time"
)

// RateWindow describes one quota/usage lane (e.g. "Weekly", "5-hour",
// "On-demand"). Either UsedPercent or Used/Limit is populated, never both
// meaningfully — percent windows come from provider APIs, raw windows from
// balance-style sources.
type RateWindow struct {
	ID            string     `json:"id"` // "primary"/"secondary"/"tertiary"/custom
	Title         string     `json:"title"`
	UsedPercent   *float64   `json:"usedPercent,omitempty"` // 0-100
	Used          *float64   `json:"used,omitempty"`        // raw value
	Limit         *float64   `json:"limit,omitempty"`
	Currency      string     `json:"currency,omitempty"` // "CNY"/"USD"
	ResetsAt      *time.Time `json:"resetsAt,omitempty"`
	WindowMinutes *int       `json:"windowMinutes,omitempty"`
	RateLimited   bool       `json:"rateLimited,omitempty"` // provider reports this lane as rate-limited
}

// Cost is a monetary snapshot (balance, Zen balance, daily cost).
type Cost struct {
	Period   string   `json:"period"`
	Currency string   `json:"currency,omitempty"`
	Amount   *float64 `json:"amount,omitempty"`
	Limit    *float64 `json:"limit,omitempty"`
}

// Identity identifies the account a snapshot belongs to.
type Identity struct {
	Account string `json:"account,omitempty"`
}

// Snapshot is one provider's usage state at a point in time.
type Snapshot struct {
	UpdatedAt time.Time    `json:"updatedAt"`
	Identity  *Identity    `json:"identity,omitempty"`
	Windows   []RateWindow `json:"windows"` // ordered: primary, secondary, tertiary, extras
	Cost      *Cost        `json:"cost,omitempty"`
}

// AccountResult is the fetch result for one account of a provider.
type AccountResult struct {
	Name     string    `json:"name"`
	Status   string    `json:"status"` // "ok" | "error" | "not_configured"
	Error    string    `json:"error,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}

// ProviderResult is the aggregate result for one provider.
type ProviderResult struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"displayName"`
	Status      string          `json:"status"` // "ok" | "error" | "not_configured" | "disabled"
	Error       string          `json:"error,omitempty"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	Accounts    []AccountResult `json:"accounts"` // single-account providers get one "default" account
}

// Output is the top-level JSON contract emitted by `leeway usage --json`.
type Output struct {
	Version     int              `json:"version"` // 1
	GeneratedAt time.Time        `json:"generatedAt"`
	Providers   []ProviderResult `json:"providers"`
}

// DurationShort renders a duration as "3d 04h", "1h 02m", "45m" or "now",
// omitting zero units (days omit hours when 0 etc.).
func DurationShort(d time.Duration) string {
	if d < 0 {
		return "overdue"
	}
	if d < time.Minute {
		return "now"
	}
	days := int(d / (24 * time.Hour))
	hours := int(d/time.Hour) % 24
	mins := int(d/time.Minute) % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %02dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%dh %02dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// WindowStatus derives a window's ok/warn/critical tone based on usage.
func WindowStatus(percent float64) string {
	switch {
	case percent < 50:
		return "ok"
	case percent < 80:
		return "warn"
	default:
		return "critical"
	}
}
