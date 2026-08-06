package codex

import (
	"context"
	"strings"
	"time"
	"unicode"

	"leeway/internal/model"
)

// Window minutes defining a window's role, mirroring
// CodexRateWindowNormalizer (5h session, 7d weekly).
const (
	sessionWindowMinutes = 5 * 60
	weeklyWindowMinutes  = 7 * 24 * 60

	sparkWindowID          = "codex-spark"
	sparkWeeklyWindowID    = "codex-spark-weekly"
	sparkWindowTitle       = "Codex Spark 5-hour"
	sparkWeeklyWindowTitle = "Codex Spark Weekly"
)

// windowFromSnapshot mirrors CodexReconciledState.makeWindow: reset_at is
// used as-is (no zero guard), window minutes are limit_window_seconds / 60.
func windowFromSnapshot(s *windowSnapshot) *model.RateWindow {
	if s == nil {
		return nil
	}
	used := float64(*s.usedPercent)
	resetsAt := time.Unix(int64(*s.resetAt), 0)
	minutes := *s.limitWindowSeconds / 60
	return &model.RateWindow{
		UsedPercent:   &used,
		ResetsAt:      &resetsAt,
		WindowMinutes: &minutes,
	}
}

type windowRole int

const (
	roleUnknown windowRole = iota
	roleSession
	roleWeekly
)

func roleFor(w *model.RateWindow) windowRole {
	if w == nil || w.WindowMinutes == nil {
		return roleUnknown
	}
	switch *w.WindowMinutes {
	case sessionWindowMinutes:
		return roleSession
	case weeklyWindowMinutes:
		return roleWeekly
	default:
		return roleUnknown
	}
}

// normalizeWindows mirrors CodexRateWindowNormalizer.normalize plus the lane
// labeling of CodexReconciledState.toUsageSnapshot: the session lane is
// primary ("Session"), the weekly lane is secondary ("Weekly").
func normalizeWindows(primary, secondary *model.RateWindow) (*model.RateWindow, *model.RateWindow) {
	switch {
	case primary != nil && secondary != nil:
		if roleFor(primary) == roleWeekly &&
			(roleFor(secondary) == roleSession || roleFor(secondary) == roleUnknown) {
			primary, secondary = secondary, primary
		}
	case primary != nil:
		if roleFor(primary) == roleWeekly {
			primary, secondary = nil, primary
		}
	case secondary != nil:
		if roleFor(secondary) == roleSession || roleFor(secondary) == roleUnknown {
			primary, secondary = secondary, nil
		}
	}
	if primary != nil {
		primary.ID, primary.Title = "primary", "Session"
	}
	if secondary != nil {
		secondary.ID, secondary.Title = "secondary", "Weekly"
	}
	return primary, secondary
}

// extraRateWindows mirrors CodexAdditionalRateLimitMapper.extraRateWindows:
// model-specific limits (e.g. GPT-5.3-Codex-Spark) become named extra
// windows, deduplicated by ID, appended after the primary/secondary lanes.
func extraRateWindows(limits []additionalRateLimit) []model.RateWindow {
	if len(limits) == 0 {
		return nil
	}
	usedIDs := map[string]bool{}
	var out []model.RateWindow
	for _, entry := range limits {
		if isSparkEntry(entry) {
			out = append(out, sparkWindows(entry, usedIDs)...)
			continue
		}
		// Model-specific limits report utilization in the primary window;
		// fall back to the secondary window only when a primary is absent.
		snap := windowOf(entry, true)
		if snap == nil {
			snap = windowOf(entry, false)
		}
		if snap == nil {
			continue
		}
		id := windowID(entry)
		if id == "" || usedIDs[id] {
			continue
		}
		usedIDs[id] = true
		out = append(out, namedWindow(id, windowTitle(entry), snap))
	}
	return out
}

func windowOf(entry additionalRateLimit, primary bool) *windowSnapshot {
	if entry.rateLimit == nil {
		return nil
	}
	if primary {
		return entry.rateLimit.primaryWindow
	}
	return entry.rateLimit.secondaryWindow
}

type sparkWindowKind struct {
	id    string
	title string
}

var (
	sparkKindFiveHour = sparkWindowKind{id: sparkWindowID, title: sparkWindowTitle}
	sparkKindWeekly   = sparkWindowKind{id: sparkWeeklyWindowID, title: sparkWeeklyWindowTitle}
)

func sparkWindows(entry additionalRateLimit, usedIDs map[string]bool) []model.RateWindow {
	candidates := []struct {
		snap *windowSnapshot
		kind sparkWindowKind
	}{
		{snap: windowOf(entry, true), kind: sparkKindFiveHour},
		{snap: windowOf(entry, false), kind: sparkKindWeekly},
	}
	var out []model.RateWindow
	for _, c := range candidates {
		if c.snap == nil {
			continue
		}
		kind := sparkWindowKindFor(c.snap, c.kind)
		if usedIDs[kind.id] {
			continue
		}
		usedIDs[kind.id] = true
		out = append(out, namedWindow(kind.id, kind.title, c.snap))
	}
	return out
}

// sparkWindowKindFor mirrors the Swift heuristic: minutes <= 6h → 5-hour
// lane, minutes >= 6d → weekly lane, otherwise the slot's fallback.
func sparkWindowKindFor(s *windowSnapshot, fallback sparkWindowKind) sparkWindowKind {
	minutes := 0
	if s.limitWindowSeconds != nil && *s.limitWindowSeconds > 0 {
		minutes = *s.limitWindowSeconds / 60
	}
	switch {
	case minutes > 0 && minutes <= 6*60:
		return sparkKindFiveHour
	case minutes >= 6*24*60:
		return sparkKindWeekly
	default:
		return fallback
	}
}

// namedWindow mirrors CodexAdditionalRateLimitMapper.namedWindow: reset_at
// only when > 0, window minutes only when limit_window_seconds > 0.
func namedWindow(id, title string, s *windowSnapshot) model.RateWindow {
	used := float64(*s.usedPercent)
	var resetsAt *time.Time
	if *s.resetAt > 0 {
		t := time.Unix(int64(*s.resetAt), 0)
		resetsAt = &t
	}
	var minutes *int
	if *s.limitWindowSeconds > 0 {
		m := *s.limitWindowSeconds / 60
		minutes = &m
	}
	return model.RateWindow{
		ID:            id,
		Title:         title,
		UsedPercent:   &used,
		ResetsAt:      resetsAt,
		WindowMinutes: minutes,
	}
}

func isSparkEntry(entry additionalRateLimit) bool {
	for _, v := range []string{entry.limitName, entry.meteredFeature} {
		if strings.Contains(strings.ToLower(v), "spark") {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

func windowID(entry additionalRateLimit) string {
	source := firstNonEmpty(entry.meteredFeature, entry.limitName)
	sl := slug(source)
	if sl == "" {
		return ""
	}
	return "codex-" + sl
}

func windowTitle(entry additionalRateLimit) string {
	return firstNonEmpty(entry.limitName, entry.meteredFeature, "Codex extra limit")
}

// slug mirrors the Swift slug(): lowercase, alphanumerics kept, runs of other
// characters collapse into a single dash, dashes trimmed at both ends.
func slug(value string) string {
	var b strings.Builder
	lastWasDash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastWasDash = false
		} else if !lastWasDash {
			b.WriteByte('-')
			lastWasDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// --- identity ---

// normalizeAccountID mirrors CodexOpenAIWorkspaceResolver.normalizeWorkspaceAccountID.
func normalizeAccountID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// resolveIdentity mirrors CodexOpenAIWorkspaceResolver.resolve: match the
// session's account id against the accounts list. Best-effort: when the
// lookup fails the account id from the session is kept (identity must never
// fail the whole fetch).
func (c *apiClient) resolveIdentity(ctx context.Context, accessToken, accountID string) *model.Identity {
	id := normalizeAccountID(accountID)
	if id == "" {
		return nil
	}
	items, err := c.fetchAccounts(ctx, accessToken, accountID)
	if err != nil {
		return &model.Identity{Account: id}
	}
	for _, item := range items {
		if normalizeAccountID(item.ID) == id {
			return &model.Identity{Account: normalizeAccountID(item.ID)}
		}
	}
	return &model.Identity{Account: id}
}
