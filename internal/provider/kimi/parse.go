package kimi

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"leeway/internal/model"
)

// Response types mirror KimiModels.swift. Field names are part of the API
// contract and must tolerate both camelCase and snake_case reset keys plus
// string/number coercion.

// kimiUsageDetail is one usage lane (weekly quota or a rate limit window).
// Values are strings in practice but the API sometimes sends numbers, so all
// fields coerce like KimiModels.KimiUsageDetail.stringValue.
type kimiUsageDetail struct {
	Limit     string
	Used      string
	Remaining string
	ResetTime string
}

func (d *kimiUsageDetail) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	limit, ok := stringValue(raw, "limit")
	if !ok {
		return fmt.Errorf("Kimi usage limit is missing")
	}
	d.Limit = limit
	d.Used, _ = stringValue(raw, "used")
	d.Remaining, _ = stringValue(raw, "remaining")
	for _, key := range []string{"resetTime", "resetAt", "reset_time", "reset_at"} {
		if v, ok := stringValue(raw, key); ok {
			d.ResetTime = v
			break
		}
	}
	return nil
}

// stringValue mirrors KimiModels.KimiUsageDetail.stringValue: strings pass
// through; integral numbers render without a decimal; doubles render with
// Go's shortest representation.
func stringValue(raw map[string]json.RawMessage, key string) (string, bool) {
	v, ok := raw[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s, true
	}
	var i int64
	if err := json.Unmarshal(v, &i); err == nil {
		return strconv.FormatInt(i, 10), true
	}
	var f float64
	if err := json.Unmarshal(v, &f); err == nil {
		if math.Trunc(f) == f && f >= math.MinInt64 && f <= float64(math.MaxInt64) {
			return strconv.FormatInt(int64(f), 10), true
		}
		return strconv.FormatFloat(f, 'g', -1, 64), true
	}
	return "", false
}

type kimiUsage struct {
	Scope  string          `json:"scope"`
	Detail kimiUsageDetail `json:"detail"`
	Limits []kimiRateLimit `json:"limits"`
}

type kimiUsageResponse struct {
	Usages []kimiUsage `json:"usages"`
}

// kimiCodeAPIUsageResponse is the Kimi Code API (api.kimi.com/coding/v1/usages)
// response shape.
type kimiCodeAPIUsageResponse struct {
	Usage  kimiUsageDetail `json:"usage"`
	Limits []kimiRateLimit `json:"limits"`
}

type kimiRateLimit struct {
	Window kimiWindow      `json:"window"`
	Detail kimiUsageDetail `json:"detail"`
}

type kimiWindow struct {
	Duration int    `json:"duration"`
	TimeUnit string `json:"timeUnit"`
}

// durationMinutes mirrors KimiModels.KimiWindow.durationMinutes.
func (w kimiWindow) durationMinutes() *int {
	if w.Duration <= 0 {
		return nil
	}
	var multiplier int
	switch w.TimeUnit {
	case "TIME_UNIT_MINUTE":
		multiplier = 1
	case "TIME_UNIT_HOUR":
		multiplier = 60
	case "TIME_UNIT_DAY":
		multiplier = 24 * 60
	default:
		return nil
	}
	if w.Duration > math.MaxInt/multiplier {
		return nil
	}
	m := w.Duration * multiplier
	return &m
}

type kimiSubscriptionStatsResponse struct {
	SubscriptionBalance *kimiSubscriptionBalance   `json:"subscriptionBalance"`
	RatelimitCode7d     *kimiSubscriptionRateLimit `json:"ratelimitCode7d"`
}

type kimiSubscriptionBalance struct {
	Feature         *string  `json:"feature"`
	Type            *string  `json:"type"`
	AmountUsedRatio *float64 `json:"amountUsedRatio"`
	ExpireTime      string   `json:"expireTime"`
}

type kimiSubscriptionRateLimit struct {
	Ratio     *float64 `json:"ratio"`
	Enabled   *bool    `json:"enabled"`
	ResetTime string   `json:"resetTime"`
}

// usageCounts mirrors KimiUsageSnapshot.usageCounts: used is authoritative,
// remaining is a fallback, and an invalid counter yields a reliable=false
// legacy 0% gauge. ok=false means the limit itself is unusable.
type usageCounts struct {
	used     int
	limit    int
	reliable bool
}

func parseCounts(d kimiUsageDetail) (usageCounts, bool) {
	limit, err := strconv.Atoi(d.Limit)
	if err != nil || limit <= 0 {
		return usageCounts{}, false
	}
	if d.Used != "" {
		if used, err := strconv.Atoi(d.Used); err == nil && used >= 0 {
			return usageCounts{used: used, limit: limit, reliable: true}, true
		}
	}
	if d.Remaining != "" {
		if remaining, err := strconv.Atoi(d.Remaining); err == nil && remaining >= 0 && remaining <= limit {
			return usageCounts{used: limit - remaining, limit: limit, reliable: true}, true
		}
	}
	return usageCounts{used: 0, limit: limit, reliable: false}, true
}

// clampPercent mirrors KimiUsageSnapshot.clampedPercent.
func clampPercent(v float64) float64 {
	return math.Min(100, math.Max(0, v))
}

// parseDate mirrors KimiUsageSnapshot.parseDate: ISO8601 with fractional
// seconds first, then without.
func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// buildSnapshot mirrors KimiUsageSnapshot.toUsageSnapshot.
func buildSnapshot(weekly kimiUsageDetail, rateLimit *kimiUsageDetail, rateLimitWindow *kimiWindow, stats *kimiSubscriptionStatsResponse, now time.Time, account string) *model.Snapshot {
	snap := &model.Snapshot{
		UpdatedAt: now,
		Identity:  &model.Identity{Account: account},
		Windows:   []model.RateWindow{},
	}

	// Primary: weekly quota (7d).
	if c, ok := parseCounts(weekly); ok {
		pct := clampPercent(float64(c.used) / float64(c.limit) * 100)
		var minutes *int
		if c.reliable {
			m := weeklyWindowMinutes
			minutes = &m
		}
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID:            "primary",
			Title:         "Weekly",
			UsedPercent:   &pct,
			ResetsAt:      parseDate(weekly.ResetTime),
			WindowMinutes: minutes,
		})
	}

	// Secondary: rate limit (5-hour session by default; the API window wins
	// when present — an invalid API window yields nil, mirroring Swift).
	if rateLimit != nil {
		if c, ok := parseCounts(*rateLimit); ok {
			pct := clampPercent(float64(c.used) / float64(c.limit) * 100)
			var minutes *int
			if c.reliable {
				if rateLimitWindow == nil {
					m := sessionWindowMinutes
					minutes = &m
				} else {
					minutes = rateLimitWindow.durationMinutes()
				}
			}
			snap.Windows = append(snap.Windows, model.RateWindow{
				ID:            "secondary",
				Title:         "5-hour",
				UsedPercent:   &pct,
				ResetsAt:      parseDate(rateLimit.ResetTime),
				WindowMinutes: minutes,
			})
		}
	}

	// Extras from the best-effort subscription stats call.
	if stats != nil {
		if w := monthlyWindow(stats.SubscriptionBalance); w != nil {
			snap.Windows = append(snap.Windows, *w)
		}
		if w := code7dWindow(stats.RatelimitCode7d); w != nil {
			snap.Windows = append(snap.Windows, *w)
		}
	}
	return snap
}

// monthlyWindow mirrors KimiUsageSnapshot's monthly window: the shared
// subscription pool (feature FEATURE_OMNI, type SUBSCRIPTION).
func monthlyWindow(b *kimiSubscriptionBalance) *model.RateWindow {
	if b == nil {
		return nil
	}
	if b.Feature != nil && *b.Feature != "FEATURE_OMNI" {
		return nil
	}
	if b.Type != nil && *b.Type != "SUBSCRIPTION" {
		return nil
	}
	if b.AmountUsedRatio == nil || !finite(*b.AmountUsedRatio) {
		return nil
	}
	pct := clampPercent(*b.AmountUsedRatio * 100)
	m := monthlySentinel
	return &model.RateWindow{
		ID:            "kimi-monthly",
		Title:         "Monthly",
		UsedPercent:   &pct,
		WindowMinutes: &m,
		ResetsAt:      parseDate(b.ExpireTime),
	}
}

// code7dWindow mirrors KimiUsageSnapshot's subscription code weekly window.
func code7dWindow(l *kimiSubscriptionRateLimit) *model.RateWindow {
	if l == nil {
		return nil
	}
	if l.Enabled != nil && !*l.Enabled {
		return nil
	}
	if l.Ratio == nil || !finite(*l.Ratio) {
		return nil
	}
	pct := clampPercent(*l.Ratio * 100)
	m := weeklyWindowMinutes
	return &model.RateWindow{
		ID:            "kimi-code-7d",
		Title:         "Code 7-day",
		UsedPercent:   &pct,
		WindowMinutes: &m,
		ResetsAt:      parseDate(l.ResetTime),
	}
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
