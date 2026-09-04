package commandcode

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"leeway/internal/model"
)

type lane struct {
	ok       bool
	percent  float64
	used     float64
	cap      float64
	hasSpend bool
	resetsAt *time.Time
	limited  bool
}

type usage struct {
	fiveHour, weekly, monthly lane
	remaining                 *float64
}

func parseUsage(data []byte) usage {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return usage{}
	}
	var u usage
	credits := asObject(root["credits"])
	u.remaining = remainingCredits(credits)
	if u.remaining == nil {
		u.remaining = remainingCredits(root)
	}
	limits := lookup(root, "windowLimits", "window_limits")
	if limits == nil {
		limits = lookup(credits, "windowLimits", "window_limits")
	}
	parseWindowLimits(limits, &u)
	assignNamedLanes(asObject(root["usage"]), &u)
	assignNamedLanes(root, &u)
	return u
}

func remainingCredits(m map[string]any) *float64 {
	monthly, ok := asFloat(lookup(m, "monthlyCredits", "monthly_credits", "remainingCredits", "remaining_credits"))
	if !ok {
		return nil
	}
	purchased, _ := asFloat(lookup(m, "purchasedCredits", "purchased_credits"))
	free, _ := asFloat(lookup(m, "freeCredits", "free_credits"))
	total := monthly + purchased + free
	return &total
}

func parseWindowLimits(v any, u *usage) {
	if m := asObject(v); m != nil {
		assignNamedLanes(m, u)
		return
	}
	arr, ok := v.([]any)
	if !ok {
		return
	}
	for _, item := range arr {
		dict := asObject(item)
		ln := parseLane(dict)
		if dict == nil || !ln.ok {
			continue
		}
		name := strings.ToLower(asString(lookup(dict, "window", "name", "id")))
		switch {
		case !u.fiveHour.ok && (strings.Contains(name, "five") || strings.Contains(name, "5h") || strings.Contains(name, "hour") || strings.Contains(name, "rolling")):
			u.fiveHour = ln
		case !u.weekly.ok && strings.Contains(name, "week"):
			u.weekly = ln
		case !u.monthly.ok && strings.Contains(name, "month"):
			u.monthly = ln
		}
	}
}

func assignNamedLanes(obj map[string]any, u *usage) {
	if obj == nil {
		return
	}
	if !u.fiveHour.ok {
		u.fiveHour = parseLane(lookup(obj, "fiveHour", "five_hour", "five-hour", "rolling", "5h"))
	}
	if !u.weekly.ok {
		u.weekly = parseLane(lookup(obj, "weekly", "weeklyUsage", "weekly_usage"))
	}
	if !u.monthly.ok {
		u.monthly = parseLane(lookup(obj, "monthly", "monthlyUsage", "monthly_usage"))
	}
}

func parseLane(v any) lane {
	dict := asObject(v)
	if dict == nil {
		return lane{}
	}
	used, hasUsed := asFloat(lookup(dict, "used", "usage", "consumed"))
	cap, hasCap := asFloat(lookup(dict, "cap", "limit", "total", "quota"))
	percent := 0.0
	hasPercent := false
	hasSpend := hasUsed && hasCap && cap > 0
	if hasSpend {
		percent = used / cap * 100
		hasPercent = true
	} else if n, ok := asFloat(lookup(dict, "usedPercent", "used_percent", "usagePercent", "percent")); ok {
		percent = n
		if percent >= 0 && percent <= 1 {
			percent *= 100
		}
		hasPercent = true
	}
	if !hasPercent {
		return lane{}
	}
	return lane{
		ok:       true,
		percent:  math.Max(0, math.Min(100, percent)),
		used:     used,
		cap:      cap,
		hasSpend: hasSpend,
		resetsAt: asTime(lookup(dict, "resetAt", "resetsAt", "reset_at", "resets_at")),
		limited:  hasSpend && used >= cap,
	}
}

func buildSnapshot(u usage, now time.Time, account, planID string) *model.Snapshot {
	fiveHours, week, month := 5*60, 7*24*60, 30*24*60
	snap := &model.Snapshot{UpdatedAt: now}
	if account != "" || planID != "" {
		snap.Identity = &model.Identity{Account: account, Plan: planLabel(planID)}
	}
	if w := spendWindow("primary", "5-hour", u.fiveHour, &fiveHours); w != nil {
		snap.Windows = append(snap.Windows, *w)
	}
	if w := spendWindow("secondary", "Weekly", u.weekly, &week); w != nil {
		snap.Windows = append(snap.Windows, *w)
	}
	if w := monthlyWindow(u, planID, &month); w != nil {
		snap.Windows = append(snap.Windows, *w)
	}
	return snap
}

func spendWindow(id, title string, ln lane, minutes *int) *model.RateWindow {
	if !ln.ok {
		return nil
	}
	w := model.RateWindow{ID: id, Title: title, ResetsAt: ln.resetsAt, WindowMinutes: minutes, RateLimited: ln.limited}
	if ln.hasSpend {
		used, cap := ln.used, ln.cap
		w.Used, w.Limit, w.Currency = &used, &cap, "USD"
		return &w
	}
	p := ln.percent
	w.UsedPercent = &p
	return &w
}

func monthlyWindow(u usage, planID string, minutes *int) *model.RateWindow {
	if w := spendWindow("tertiary", "Monthly", u.monthly, minutes); w != nil {
		return w
	}
	if u.remaining == nil {
		return nil
	}
	w := model.RateWindow{ID: "tertiary", Title: "Monthly", Currency: "USD", WindowMinutes: minutes}
	if cap, ok := planCredits(planID); ok && cap > 0 {
		used := cap - *u.remaining
		if used < 0 {
			used = 0
		}
		w.Used, w.Limit = &used, &cap
		return &w
	}
	w.Used = u.remaining
	return &w
}

func parseOrgID(data []byte) string {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return ""
	}
	return strings.TrimSpace(asString(lookup(asObject(root["org"]), "id")))
}

func parsePlanID(data []byte) string {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return ""
	}
	if id := strings.TrimSpace(asString(lookup(asObject(root["data"]), "planId", "plan_id"))); id != "" {
		return id
	}
	return strings.TrimSpace(asString(lookup(root, "planId", "plan_id")))
}

var planCreditsTable = map[string]float64{
	"individual-go":       10,
	"individual-goat":     70,
	"individual-pro":      30,
	"individual-pro-v1":   80,
	"individual-provider": 15,
	"individual-max":      150,
	"individual-ultra":    300,
	"teams-pro":           40,
}

var planLabels = map[string]string{
	"individual-go":       "Go",
	"individual-goat":     "GOAT",
	"individual-pro":      "Pro",
	"individual-pro-v1":   "Pro",
	"individual-provider": "Provider",
	"individual-max":      "Max",
	"individual-ultra":    "Ultra",
	"teams-pro":           "Teams Pro",
}

func planCredits(id string) (float64, bool) {
	v, ok := planCreditsTable[strings.ToLower(strings.TrimSpace(id))]
	return v, ok
}

func planLabel(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if s, ok := planLabels[strings.ToLower(id)]; ok {
		return s
	}
	return id
}

func asObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func lookup(m map[string]any, keys ...string) any {
	if m == nil {
		return nil
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		if math.IsInf(t, 0) || math.IsNaN(t) {
			return 0, false
		}
		return t, true
	case json.Number:
		f, err := t.Float64()
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		return f, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func asTime(v any) *time.Time {
	if n, ok := asFloat(v); ok && n > 1_000_000_000 {
		var t time.Time
		if n > 1_000_000_000_000 {
			t = time.UnixMilli(int64(n)).UTC()
		} else {
			t = time.Unix(int64(n), 0).UTC()
		}
		return &t
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}
