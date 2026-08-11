package opencodego

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Parsing of the workspace usage page mirrors OpenCodeGoUsageFetcher's
// parseSubscription* family: JSON bodies are walked through several layered
// heuristics (exact keys first, then path-name matching, then any-window
// candidates), HTML bodies fall back to regex extraction of the serialized
// Next.js payload, and window values are normalized the same way
// (fraction <= 1 becomes percent, used/limit ratio, clamped 0...100).

// subscription is the parsed usage state, mirroring OpenCodeGoUsageSnapshot.
type subscription struct {
	isBalanceOnly   bool
	hasWeeklyUsage  bool
	hasMonthlyUsage bool
	rollingPercent  float64
	weeklyPercent   float64
	monthlyPercent  float64
	rollingResetSec int
	weeklyResetSec  int
	monthlyResetSec int
	renewsAt        *time.Time
}

func balanceOnly() *subscription { return &subscription{isBalanceOnly: true} }

// parsedWindow is the normalized (percent, resetInSec) of one window dict.
type parsedWindow struct {
	percent    float64
	resetInSec int
}

var (
	percentKeys = []string{
		"usagePercent", "usedPercent", "percentUsed", "percent",
		"usage_percent", "used_percent", "utilization", "utilizationPercent",
		"utilization_percent", "usage",
	}
	resetInKeys = []string{
		"resetInSec", "resetInSeconds", "resetSeconds", "reset_sec",
		"reset_in_sec", "resetsInSec", "resetsInSeconds", "resetIn", "resetSec",
	}
	resetAtKeys = []string{
		"resetAt", "resetsAt", "reset_at", "resets_at",
		"nextReset", "next_reset", "renewAt", "renew_at",
	}
	renewAtKeys = []string{"renewAt", "renew_at"}

	rollingKeys = []string{"rollingUsage", "rolling", "rolling_usage", "rollingWindow", "rolling_window"}
	weeklyKeys  = []string{"weeklyUsage", "weekly", "weekly_usage", "weeklyWindow", "weekly_window"}
	monthlyKeys = []string{"monthlyUsage", "monthly", "monthly_usage", "monthlyWindow", "monthly_window"}

	usedKeys  = []string{"used", "usage", "consumed", "count", "usedTokens"}
	limitKeys = []string{"limit", "total", "quota", "max", "cap", "tokenLimit"}

	workspaceIDPattern     = regexp.MustCompile(`id\s*:\s*"(wrk_[^"]+)"`)
	workspaceIDToken       = regexp.MustCompile(`wrk_[A-Za-z0-9]+`)
	rollingUsageRegex      = regexp.MustCompile(`rollingUsage[^}]*?usagePercent\s*:\s*([0-9]+(?:\.[0-9]+)?)`)
	rollingResetRegex      = regexp.MustCompile(`rollingUsage[^}]*?resetInSec\s*:\s*([0-9]+)`)
	weeklyUsageRegex       = regexp.MustCompile(`weeklyUsage[^}]*?usagePercent\s*:\s*([0-9]+(?:\.[0-9]+)?)`)
	weeklyResetRegex       = regexp.MustCompile(`weeklyUsage[^}]*?resetInSec\s*:\s*([0-9]+)`)
	monthlyUsageRegex      = regexp.MustCompile(`monthlyUsage[^}]*?usagePercent\s*:\s*([0-9]+(?:\.[0-9]+)?)`)
	monthlyResetRegex      = regexp.MustCompile(`monthlyUsage[^}]*?resetInSec\s*:\s*([0-9]+)`)
	pageTitlePattern       = regexp.MustCompile(`(?i)<title>([^<]+)</title>`)
	numberWithOptionalFrac = `([0-9]+(?:\.[0-9]+)?)`
)

// ---------------------------------------------------------------------------
// Subscription parsing
// ---------------------------------------------------------------------------

// parseSubscription mirrors OpenCodeGoUsageFetcher.parseSubscription.
func parseSubscription(text string, now time.Time) (*subscription, error) {
	if snap, ok := parseSubscriptionJSON(text, now); ok {
		return snap, nil
	}
	return parseSubscriptionRegex(text, now)
}

// parseSubscriptionJSON walks a JSON usage payload through the same layered
// heuristics as parseSubscriptionJSON in the Swift client.
func parseSubscriptionJSON(text string, now time.Time) (*subscription, bool) {
	object, ok := decodeJSON(text)
	if !ok {
		return nil, false
	}
	dict, ok := object.(map[string]any)
	if !ok {
		return nil, false
	}
	renewsAt := dateValue(value(dict, renewAtKeys))

	if snap := parseUsageDictionary(dict, now, renewsAt); snap != nil {
		return snap, true
	}
	for _, key := range []string{"data", "result", "usage", "billing", "payload"} {
		if nested, ok := dict[key].(map[string]any); ok {
			if snap := parseUsageDictionary(nested, now, renewsAt); snap != nil {
				return snap, true
			}
		}
	}
	if snap := parseUsageNested(dict, now, 0, renewsAt); snap != nil {
		return snap, true
	}
	if snap := parseUsageFromCandidates(object, now, renewsAt); snap != nil {
		return snap, true
	}
	return nil, false
}

func parseUsageDictionary(dict map[string]any, now time.Time, inherited *time.Time) *subscription {
	renewsAt := dateValue(value(dict, renewAtKeys))
	if renewsAt == nil {
		renewsAt = inherited
	}
	if usage, ok := dict["usage"].(map[string]any); ok {
		if snap := parseUsageDictionary(usage, now, renewsAt); snap != nil {
			return snap
		}
	}
	rolling := firstDict(dict, rollingKeys)
	weekly := firstDict(dict, weeklyKeys)
	monthly := firstDict(dict, monthlyKeys)
	if rolling == nil {
		return nil
	}
	return buildSnapshot(rolling, weekly, monthly, now, renewsAt)
}

func parseUsageNested(dict map[string]any, now time.Time, depth int, inherited *time.Time) *subscription {
	if depth > 3 {
		return nil
	}
	renewsAt := dateValue(value(dict, renewAtKeys))
	if renewsAt == nil {
		renewsAt = inherited
	}
	var rolling, weekly, monthly map[string]any
	for key, v := range dict {
		sub, ok := v.(map[string]any)
		if !ok {
			continue
		}
		lower := strings.ToLower(key)
		switch {
		case strings.Contains(lower, "rolling") || strings.Contains(lower, "hour") ||
			strings.Contains(lower, "5h") || strings.Contains(lower, "5-hour"):
			rolling = sub
		case strings.Contains(lower, "weekly") || strings.Contains(lower, "week"):
			weekly = sub
		case strings.Contains(lower, "monthly") || strings.Contains(lower, "month"):
			monthly = sub
		}
	}
	if rolling != nil {
		if snap := buildSnapshot(rolling, weekly, monthly, now, renewsAt); snap != nil {
			return snap
		}
	}
	for _, v := range dict {
		if sub, ok := v.(map[string]any); ok {
			if snap := parseUsageNested(sub, now, depth+1, renewsAt); snap != nil {
				return snap
			}
		}
	}
	return nil
}

type windowCandidate struct {
	id         uint64
	percent    float64
	resetInSec int
	pathLower  string
}

var candidateCounter atomic.Uint64

func parseUsageFromCandidates(object any, now time.Time, inherited *time.Time) *subscription {
	candidates := collectWindowCandidates(object, now)
	if len(candidates) == 0 {
		return nil
	}
	var rollingC, weeklyC, monthlyC []windowCandidate
	for _, c := range candidates {
		// Independent filters, like the Swift client: a candidate can match
		// more than one lane.
		if strings.Contains(c.pathLower, "rolling") || strings.Contains(c.pathLower, "hour") ||
			strings.Contains(c.pathLower, "5h") || strings.Contains(c.pathLower, "5-hour") {
			rollingC = append(rollingC, c)
		}
		if strings.Contains(c.pathLower, "weekly") || strings.Contains(c.pathLower, "week") {
			weeklyC = append(weeklyC, c)
		}
		if strings.Contains(c.pathLower, "monthly") || strings.Contains(c.pathLower, "month") {
			monthlyC = append(monthlyC, c)
		}
	}
	// A short reset countdown does not prove that a generic window is the
	// 5-hour lane. Require an explicit period name in the response path.
	rolling := pickCandidateList(rollingC, true)
	if rolling == nil {
		return nil
	}
	weekly := pickCandidateList(filterByID(weeklyC, rolling.id), false)
	monthly := pickCandidateList(filterByID(filterByID(monthlyC, rolling.id), idOrZero(weekly)), false)

	var renewsAt *time.Time
	if d, ok := object.(map[string]any); ok {
		renewsAt = dateValue(value(d, renewAtKeys))
	}
	if renewsAt == nil {
		renewsAt = inherited
	}

	sub := &subscription{
		hasWeeklyUsage:  weekly != nil,
		hasMonthlyUsage: monthly != nil,
		rollingPercent:  rolling.percent,
		rollingResetSec: rolling.resetInSec,
		renewsAt:        renewsAt,
	}
	if weekly != nil {
		sub.weeklyPercent = weekly.percent
		sub.weeklyResetSec = weekly.resetInSec
	}
	if monthly != nil {
		sub.monthlyPercent = monthly.percent
		sub.monthlyResetSec = monthly.resetInSec
	}
	return sub
}

func idOrZero(w *windowCandidate) uint64 {
	if w == nil {
		return 0
	}
	return w.id
}

func filterByID(cs []windowCandidate, id uint64) []windowCandidate {
	if id == 0 {
		return cs
	}
	out := cs[:0:0]
	for _, c := range cs {
		if c.id != id {
			out = append(out, c)
		}
	}
	return out
}

func collectWindowCandidates(object any, now time.Time) []windowCandidate {
	var out []windowCandidate
	var walk func(any, []string)
	walk = func(v any, path []string) {
		switch t := v.(type) {
		case map[string]any:
			if w, ok := parseWindow(t, now); ok {
				out = append(out, windowCandidate{
					id:         candidateCounter.Add(1),
					percent:    w.percent,
					resetInSec: w.resetInSec,
					pathLower:  strings.ToLower(strings.Join(path, ".")),
				})
			}
			for key, sub := range t {
				walk(sub, append(path, key))
			}
		case []any:
			for i, sub := range t {
				walk(sub, append(path, "["+strconv.Itoa(i)+"]"))
			}
		}
	}
	walk(object, nil)
	return out
}

func pickCandidateList(cs []windowCandidate, pickShorter bool) *windowCandidate {
	if len(cs) == 0 {
		return nil
	}
	best := cs[0]
	for _, c := range cs[1:] {
		if lessThan(c, best, pickShorter) {
			best = c
		}
	}
	return &best
}

// lessThan mirrors Swift's pickCandidate comparator: shorter-reset windows
// pick the minimum reset (longest-reset picks the maximum); ties break on
// higher percent.
func lessThan(lhs, rhs windowCandidate, pickShorter bool) bool {
	if lhs.resetInSec == rhs.resetInSec {
		return lhs.percent > rhs.percent
	}
	if pickShorter {
		return lhs.resetInSec < rhs.resetInSec
	}
	return lhs.resetInSec > rhs.resetInSec
}

func firstDict(dict map[string]any, keys []string) map[string]any {
	for _, key := range keys {
		if v, ok := dict[key].(map[string]any); ok {
			return v
		}
	}
	return nil
}

func buildSnapshot(rolling, weekly, monthly map[string]any, now time.Time, renewsAt *time.Time) *subscription {
	rw, ok := parseWindow(rolling, now)
	if !ok {
		return nil
	}
	var weeklyWindow *parsedWindow
	if weekly != nil {
		w, ok := parseWindow(weekly, now)
		if !ok {
			return nil
		}
		weeklyWindow = &w
	}
	var monthlyWindow *parsedWindow
	if monthly != nil {
		if w, ok := parseWindow(monthly, now); ok {
			monthlyWindow = &w
		}
	}
	sub := &subscription{
		hasWeeklyUsage:  weeklyWindow != nil,
		hasMonthlyUsage: monthlyWindow != nil,
		rollingPercent:  rw.percent,
		rollingResetSec: rw.resetInSec,
		renewsAt:        renewsAt,
	}
	if weeklyWindow != nil {
		sub.weeklyPercent = weeklyWindow.percent
		sub.weeklyResetSec = weeklyWindow.resetInSec
	}
	if monthlyWindow != nil {
		sub.monthlyPercent = monthlyWindow.percent
		sub.monthlyResetSec = monthlyWindow.resetInSec
	}
	return sub
}

// parseWindow mirrors parseWindow: a direct percent field (fraction <= 1
// scales to 0...100), a used/limit ratio, clamped, plus a reset countdown
// from resetInSec or resetAt.
func parseWindow(dict map[string]any, now time.Time) (parsedWindow, bool) {
	var percent *float64
	percentIsDirect := false
	for _, key := range percentKeys {
		if v, ok := doubleValue(dict[key]); ok {
			percent = &v
			percentIsDirect = true
			break
		}
	}
	if percent == nil {
		var used, limit *float64
		for _, key := range usedKeys {
			if v, ok := doubleValue(dict[key]); ok {
				used = &v
				break
			}
		}
		for _, key := range limitKeys {
			if v, ok := doubleValue(dict[key]); ok {
				limit = &v
				break
			}
		}
		if used != nil && limit != nil && *limit > 0 {
			p := (*used / *limit) * 100
			percent = &p
		}
	}
	if percent == nil {
		return parsedWindow{}, false
	}
	resolvedPercent := *percent
	if percentIsDirect && resolvedPercent >= 0 && resolvedPercent <= 1.0 {
		resolvedPercent *= 100
	}
	resolvedPercent = math.Max(0, math.Min(100, resolvedPercent))

	resetInSec := 0
	foundReset := false
	for _, key := range resetInKeys {
		if v, ok := intValue(dict[key]); ok {
			resetInSec = v
			foundReset = true
			break
		}
	}
	if !foundReset {
		for _, key := range resetAtKeys {
			if resetAt := dateValue(dict[key]); resetAt != nil {
				if interval, ok := resetInterval(*resetAt, now); ok {
					resetInSec = interval
					foundReset = true
					break
				}
			}
		}
	}
	if !foundReset {
		return parsedWindow{}, false
	}
	return parsedWindow{percent: resolvedPercent, resetInSec: resetInSec}, true
}

// parseSubscriptionRegex mirrors the HTML fallback path of parseSubscription.
func parseSubscriptionRegex(text string, now time.Time) (*subscription, error) {
	rollingPercent, ok1 := extractDouble(rollingUsageRegex, text)
	rollingReset, ok2 := extractInt(rollingResetRegex, text)
	if !ok1 || !ok2 {
		return nil, parseErrorf("Missing usage fields.")
	}
	weeklyPercent, okW1 := extractDouble(weeklyUsageRegex, text)
	weeklyReset, okW2 := extractInt(weeklyResetRegex, text)
	hasWeekly := okW1 && okW2
	monthlyPercent, okM1 := extractDouble(monthlyUsageRegex, text)
	monthlyReset, okM2 := extractInt(monthlyResetRegex, text)
	hasMonthly := okM1 && okM2

	sub := &subscription{
		hasWeeklyUsage:  hasWeekly,
		hasMonthlyUsage: hasMonthly,
		rollingPercent:  rollingPercent,
		rollingResetSec: rollingReset,
	}
	if hasWeekly {
		sub.weeklyPercent = weeklyPercent
		sub.weeklyResetSec = weeklyReset
	}
	if hasMonthly {
		sub.monthlyPercent = monthlyPercent
		sub.monthlyResetSec = monthlyReset
	}
	return sub, nil
}

// usageFieldsPresent mirrors the guard in fetchUsagePage.
func usageFieldsPresent(text string) bool {
	if _, ok := parseSubscriptionJSON(text, time.Now()); ok {
		return true
	}
	_, ok := extractDouble(rollingUsageRegex, text)
	return ok
}

// ---------------------------------------------------------------------------
// Workspace ID parsing
// ---------------------------------------------------------------------------

// normalizeWorkspaceID mirrors OpenCodeGoUsageFetcher.normalizeWorkspaceID:
// "wrk_..." tokens are accepted as-is, from a workspace URL, or found by
// scanning the string; anything else is ignored.
func normalizeWorkspaceID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "wrk_") && len(trimmed) > 4 {
		return trimmed
	}
	// Accept a workspace dashboard URL (…/workspace/<id>[/…]).
	if scheme := strings.Index(trimmed, "://"); scheme > 0 {
		rest := trimmed[scheme+3:]
		slash := strings.IndexByte(rest, '/')
		if slash >= 0 {
			parts := strings.Split(rest[slash+1:], "/")
			for i, part := range parts {
				if part == "workspace" && i+1 < len(parts) {
					candidate := parts[i+1]
					if strings.HasPrefix(candidate, "wrk_") && len(candidate) > 4 {
						return candidate
					}
				}
			}
		}
	}
	if m := workspaceIDToken.FindString(trimmed); m != "" {
		return m
	}
	return ""
}

// parseWorkspaceIDs mirrors parseWorkspaceIDs (serialized JS payloads).
func parseWorkspaceIDs(text string) []string {
	var out []string
	for _, m := range workspaceIDPattern.FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	return out
}

// parseWorkspaceIDsFromJSON mirrors parseWorkspaceIDsFromJSON: any string
// starting with "wrk_" anywhere in the JSON, deduplicated.
func parseWorkspaceIDsFromJSON(text string) []string {
	object, ok := decodeJSON(text)
	if !ok {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for _, sub := range t {
				walk(sub)
			}
		case []any:
			for _, sub := range t {
				walk(sub)
			}
		case string:
			if strings.HasPrefix(t, "wrk_") && !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	walk(object)
	return out
}

// ---------------------------------------------------------------------------
// Value helpers
// ---------------------------------------------------------------------------

// decodeJSON decodes with UseNumber so integer/float distinctions survive
// (mirrors NSNumber semantics in the Swift client).
func decodeJSON(text string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var object any
	if err := dec.Decode(&object); err != nil {
		return nil, false
	}
	return object, true
}

func value(dict map[string]any, keys []string) any {
	for _, key := range keys {
		if v, ok := dict[key]; ok {
			return v
		}
	}
	return nil
}

// doubleValue mirrors doubleValue: numbers (truncating nothing), numeric
// strings; booleans are rejected.
func doubleValue(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		return f, true
	case float64:
		if math.IsInf(t, 0) || math.IsNaN(t) {
			return 0, false
		}
		return t, true
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

// intValue mirrors intValue: JSON integers stay integers, floats truncate
// (NSNumber.intValue), and strings must be plain integer text.
func intValue(v any) (int, bool) {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i), true
		}
		if f, err := t.Float64(); err == nil {
			return int(f), true // truncation toward zero, like NSNumber.intValue
		}
		return 0, false
	case float64:
		return int(t), true
	case int:
		return t, true
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// dateValue mirrors dateValue: epoch seconds/milliseconds (numeric or numeric
// string) or ISO8601 with fractional seconds.
func dateValue(v any) *time.Time {
	if v == nil {
		return nil
	}
	if n, ok := doubleValue(v); ok {
		var t time.Time
		switch {
		case n > 1_000_000_000_000:
			t = time.UnixMilli(int64(n))
		case n > 1_000_000_000:
			t = time.Unix(int64(n), 0)
		default:
			return nil
		}
		return &t
	}
	if s, ok := v.(string); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return dateValue(n)
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05Z07:00"} {
			if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
				return &t
			}
		}
	}
	return nil
}

func resetInterval(resetAt, now time.Time) (int, bool) {
	interval := resetAt.Sub(now)
	if interval <= 0 {
		return 0, true
	}
	return int(interval / time.Second), true
}

func extractDouble(re *regexp.Regexp, text string) (float64, bool) {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func extractInt(re *regexp.Regexp, text string) (int, bool) {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	i, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return i, true
}

// looksSignedOut mirrors looksSignedOut — deliberately broad (Swift checks
// for the word "login" anywhere in the payload).
func looksSignedOut(text string) bool {
	lower := strings.ToLower(text)
	for _, needle := range []string{
		"login",
		"sign in",
		"auth/authorize",
		"not associated with an account",
		`actor of type "public"`,
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// extractServerErrorMessage mirrors extractServerErrorMessage: JSON
// message/error/detail fields, then the HTML <title>.
func extractServerErrorMessage(text string) string {
	if object, ok := decodeJSON(text); ok {
		if dict, ok := object.(map[string]any); ok {
			for _, key := range []string{"message", "error", "detail"} {
				if s, ok := dict[key].(string); ok && strings.TrimSpace(s) != "" {
					return s
				}
			}
		}
	}
	if m := pageTitlePattern.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}
