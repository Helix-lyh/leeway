package opencodego

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseSubscriptionJSONNested(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{
		"data": {
			"usage": {
				"rollingUsage": {"usagePercent": 41, "resetInSec": 3600},
				"weeklyUsage": {"usagePercent": 60, "resetInSec": 700000},
				"monthlyUsage": {"usagePercent": 10, "resetInSec": 2000000}
			},
			"renewAt": "2026-08-13T12:00:00Z"
		}
	}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingPercent != 41 || sub.weeklyPercent != 60 || sub.monthlyPercent != 10 {
		t.Fatalf("percents wrong: %+v", sub)
	}
	if sub.rollingResetSec != 3600 || sub.weeklyResetSec != 700000 || sub.monthlyResetSec != 2000000 {
		t.Fatalf("resets wrong: %+v", sub)
	}
	if !sub.hasWeeklyUsage || !sub.hasMonthlyUsage {
		t.Fatalf("window flags wrong: %+v", sub)
	}
	if sub.renewsAt == nil || !sub.renewsAt.Equal(time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("renewsAt wrong: %v", sub.renewsAt)
	}
}

func TestParseSubscriptionJSONDirectFraction(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{"rollingUsage": {"percent": 0.41, "resetInSeconds": 300}}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingPercent != 41 {
		t.Fatalf("direct fraction should scale to 41, got %v", sub.rollingPercent)
	}
	if sub.rollingResetSec != 300 {
		t.Fatalf("reset wrong: %v", sub.rollingResetSec)
	}
}

func TestParseSubscriptionJSONUsedLimit(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{"rollingUsage": {"used": 25, "limit": 100, "resetInSec": 0}}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingPercent != 25 {
		t.Fatalf("used/limit ratio should be 25, got %v", sub.rollingPercent)
	}
	if sub.rollingResetSec != 0 {
		t.Fatalf("reset should be 0 (explicit), got %v", sub.rollingResetSec)
	}
}

func TestParseSubscriptionJSONResetAtEpochMillis(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	future := now.Add(90 * time.Second)
	text := `{"rollingUsage": {"usagePercent": 5, "resetAt": ` + itoa(future.UnixMilli()) + `}}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingResetSec < 89 || sub.rollingResetSec > 90 {
		t.Fatalf("epoch-ms resetAt wrong: %v", sub.rollingResetSec)
	}
}

func TestParseSubscriptionJSONResetAtISOTime(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	future := now.Add(120 * time.Second)
	text := `{"rollingUsage": {"usagePercent": 5, "nextReset": "` + future.Format(time.RFC3339) + `"}}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingResetSec < 119 || sub.rollingResetSec > 120 {
		t.Fatalf("ISO resetAt wrong: %v", sub.rollingResetSec)
	}
}

func TestParseSubscriptionJSONPastResetIsZero(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	past := now.Add(-60 * time.Second)
	text := `{"rollingUsage": {"usagePercent": 5, "resetAt": ` + itoa(past.UnixMilli()) + `}}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingResetSec != 0 {
		t.Fatalf("past reset should clamp to 0, got %v", sub.rollingResetSec)
	}
}

func TestParseSubscriptionJSONPercentClamped(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{"rollingUsage": {"usagePercent": 150, "resetInSec": 100}}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingPercent != 100 {
		t.Fatalf("percent should clamp to 100, got %v", sub.rollingPercent)
	}
}

func TestParseSubscriptionJSONWeeklyMissing(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{"rollingUsage": {"usagePercent": 41, "resetInSec": 3600}}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.hasWeeklyUsage || sub.hasMonthlyUsage {
		t.Fatalf("missing windows must not be flagged: %+v", sub)
	}
}

func TestParseSubscriptionJSONNestedKeyNames(t *testing.T) {
	// Keys recognized by parseUsageNested via name matching.
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{
		"payload": {
			"usage_5h": {"used_percent": 33, "reset_sec": 900},
			"usage_week": {"used_percent": 55, "reset_sec": 90000},
			"usage_month": {"used_percent": 77, "reset_sec": 900000}
		}
	}`
	sub, ok := parseSubscriptionJSON(text, now)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if sub.rollingPercent != 33 || sub.weeklyPercent != 55 || sub.monthlyPercent != 77 {
		t.Fatalf("nested-key percents wrong: %+v", sub)
	}
}

func TestParseSubscriptionJSONCandidates(t *testing.T) {
	// Generic windows without period-bearing paths must not be guessed from
	// reset distance.
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{
		"usageStats": {
			"windows": {
				"w0": {"usagePercent": 80, "resetInSec": 100},
				"w1": {"usagePercent": 30, "resetInSec": 2000000}
			}
		}
	}`
	sub, ok := parseSubscriptionJSON(text, now)
	if ok || sub != nil {
		t.Fatalf("generic reset-distance candidates must be rejected: %+v", sub)
	}
}

func TestParseSubscriptionJSONRequiresReset(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `{"rollingUsage":{"usagePercent":41}}`
	if sub, ok := parseSubscriptionJSON(text, now); ok || sub != nil {
		t.Fatalf("usage without an explicit reset must be rejected: %+v", sub)
	}
}

func TestParseSubscriptionRegexFallback(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `window.__next_f.push([1,"... rollingUsage usagePercent:41.5 resetInSec:1234 weeklyUsage usagePercent:60 resetInSec:999999 ..."])`
	sub, err := parseSubscription(text, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.rollingPercent != 41.5 || sub.rollingResetSec != 1234 {
		t.Fatalf("rolling wrong: %+v", sub)
	}
	if !sub.hasWeeklyUsage || sub.weeklyPercent != 60 || sub.weeklyResetSec != 999999 {
		t.Fatalf("weekly wrong: %+v", sub)
	}
	if sub.hasMonthlyUsage {
		t.Fatalf("monthly must be absent: %+v", sub)
	}
}

func TestParseSubscriptionRegexMissingFields(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	_, err := parseSubscription(`<html><body>nothing useful</body></html>`, now)
	if err == nil || !strings.Contains(err.Error(), "Missing usage fields") {
		t.Fatalf("expected Missing usage fields error, got %v", err)
	}
	if !isMissingUsageFields(err) {
		t.Fatalf("must classify as the balance-only fallback trigger")
	}
}

func TestParseSubscriptionRegexRequiresCompleteMonthlyWindow(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	text := `rollingUsage usagePercent:41 resetInSec:1234 monthlyUsage resetInSec:999999`
	sub, err := parseSubscriptionRegex(text, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.hasMonthlyUsage {
		t.Fatalf("partial monthly window must not manufacture 0%%: %+v", sub)
	}
}

func TestParseWorkspaceIDs(t *testing.T) {
	text := `self.__next_f.push([1,"... id:"wrk_abc123" id: "wrk_def456" ..."])`
	ids := parseWorkspaceIDs(text)
	if len(ids) != 2 || ids[0] != "wrk_abc123" || ids[1] != "wrk_def456" {
		t.Fatalf("regex workspace ids wrong: %v", ids)
	}
}

func TestParseWorkspaceIDsFromJSON(t *testing.T) {
	text := `{"data": [{"workspaceID": "wrk_json111"}, {"nested": {"id": "wrk_json222"}}, {"workspaceID": "wrk_json111"}]}`
	ids := parseWorkspaceIDsFromJSON(text)
	if len(ids) != 2 || ids[0] != "wrk_json111" || ids[1] != "wrk_json222" {
		t.Fatalf("json workspace ids wrong: %v", ids)
	}
}

func TestNormalizeWorkspaceID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"wrk_abc123", "wrk_abc123"},
		{" wrk_abc123 ", "wrk_abc123"},
		{"https://opencode.ai/workspace/wrk_url99/go", "wrk_url99"},
		{"https://opencode.ai/workspace/wrk_url99", "wrk_url99"},
		{"prefix wrk_regex1 suffix", "wrk_regex1"},
		{"", ""},
		{"other_thing", ""},
		{"wrk_", ""},
	}
	for _, c := range cases {
		if got := normalizeWorkspaceID(c.in); got != c.want {
			t.Fatalf("normalizeWorkspaceID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDateValue(t *testing.T) {
	sec := time.Unix(1_700_000_000, 0)
	ms := time.UnixMilli(1_700_000_000_123)
	cases := []struct {
		in   any
		want *time.Time
	}{
		{float64(1_700_000_000), &sec},
		{float64(1_700_000_000_123), &ms},
		{"1700000000", &sec},
		{"2026-08-06T12:00:00Z", tptr(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC))},
		{"2026-08-06T12:00:00.123Z", tptr(time.Date(2026, 8, 6, 12, 0, 0, 123000000, time.UTC))},
		{float64(500), nil},
		{"garbage", nil},
		{nil, nil},
	}
	for _, c := range cases {
		got := dateValue(c.in)
		if c.want == nil {
			if got != nil {
				t.Fatalf("dateValue(%v) = %v, want nil", c.in, got)
			}
			continue
		}
		if got == nil || !got.Equal(*c.want) {
			t.Fatalf("dateValue(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLooksSignedOut(t *testing.T) {
	signed := []string{
		"please login to continue",
		"<title>Sign in to OpenCode</title>",
		`window.location = "/auth/authorize"`,
		"not associated with an account",
		`actor of type "public"`,
	}
	for _, s := range signed {
		if !looksSignedOut(s) {
			t.Fatalf("looksSignedOut(%q) = false, want true", s)
		}
	}
	// The Swift check is a blunt substring match: "login" anywhere counts.
	if !looksSignedOut(`{"usagePercent": 41, "loginUrl": null}`) {
		t.Fatalf("Swift's substring check must flag any 'login' occurrence")
	}
	if looksSignedOut(`{"usagePercent": 41, "resetInSec": 60}`) {
		t.Fatalf("innocent payload must not be signed out")
	}
}

func TestExtractServerErrorMessage(t *testing.T) {
	if got := extractServerErrorMessage(`{"message": "boom"}`); got != "boom" {
		t.Fatalf("message extraction wrong: %q", got)
	}
	if got := extractServerErrorMessage(`{"error": "no"}`); got != "no" {
		t.Fatalf("error extraction wrong: %q", got)
	}
	if got := extractServerErrorMessage(`<html><head><title>Oops</title></head></html>`); got != "Oops" {
		t.Fatalf("title extraction wrong: %q", got)
	}
	if got := extractServerErrorMessage(`plain text`); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestSnapshotFromSubscription(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	renew := now.Add(7 * 24 * time.Hour)
	sub := &subscription{
		hasWeeklyUsage:  true,
		hasMonthlyUsage: true,
		rollingPercent:  41,
		weeklyPercent:   60,
		monthlyPercent:  10,
		rollingResetSec: 3600,
		weeklyResetSec:  700000,
		monthlyResetSec: 2000000,
		renewsAt:        &renew,
	}
	balance := 18.5
	p := newProvider()
	snap := p.buildSnapshot(sub, now, &balance)

	if len(snap.Windows) != 4 {
		t.Fatalf("expected 4 windows (3 + renewal), got %d", len(snap.Windows))
	}
	primary := snap.Windows[0]
	if primary.ID != "primary" || primary.Title != "5-hour" || primary.WindowMinutes == nil || *primary.WindowMinutes != 300 {
		t.Fatalf("primary wrong: %+v", primary)
	}
	if primary.UsedPercent == nil || *primary.UsedPercent != 41 {
		t.Fatalf("primary percent wrong: %+v", primary)
	}
	if primary.ResetsAt == nil || !primary.ResetsAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("primary resetsAt wrong: %v", primary.ResetsAt)
	}
	secondary := snap.Windows[1]
	if secondary.ID != "secondary" || secondary.Title != "Weekly" || secondary.WindowMinutes == nil || *secondary.WindowMinutes != 7*24*60 {
		t.Fatalf("secondary wrong: %+v", secondary)
	}
	tertiary := snap.Windows[2]
	if tertiary.ID != "tertiary" || tertiary.Title != "Monthly" || tertiary.WindowMinutes == nil || *tertiary.WindowMinutes != 30*24*60 {
		t.Fatalf("tertiary wrong: %+v", tertiary)
	}
	renewal := snap.Windows[3]
	if renewal.ID != "renewal" || renewal.Title != "Renews" || renewal.UsedPercent != nil || renewal.ResetsAt == nil {
		t.Fatalf("renewal wrong: %+v", renewal)
	}
	if snap.Cost == nil || snap.Cost.Period != "Zen balance" || snap.Cost.Currency != "USD" || snap.Cost.Amount == nil || *snap.Cost.Amount != 18.5 {
		t.Fatalf("cost wrong: %+v", snap.Cost)
	}
}

func TestSnapshotBalanceOnly(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	balance := 3.0
	p := newProvider()
	snap := p.buildSnapshot(balanceOnly(), now, &balance)
	if len(snap.Windows) != 0 {
		t.Fatalf("balance-only snapshot must have no windows, got %+v", snap.Windows)
	}
	if snap.Cost == nil || *snap.Cost.Amount != 3 {
		t.Fatalf("balance-only cost wrong: %+v", snap.Cost)
	}
	if snap.Cost.Period != "Zen balance" || snap.Cost.Currency != "USD" {
		t.Fatalf("balance-only cost metadata wrong: %+v", snap.Cost)
	}
}

func TestDeclaredWindowsAndAuthSpec(t *testing.T) {
	p := New()
	spec := p.AuthSpec()
	if !spec.UsesCookies || !spec.SupportsAccounts {
		t.Fatalf("auth spec wrong: %+v", spec)
	}
	if len(spec.CookieDomains) != 1 || spec.CookieDomains[0] != "opencode.ai" {
		t.Fatalf("cookie domains wrong: %v", spec.CookieDomains)
	}
	windows := p.DeclaredWindows()
	if len(windows) != 3 {
		t.Fatalf("declared windows wrong: %v", windows)
	}
	if windows[0].ID != "primary" || windows[1].ID != "secondary" || windows[2].ID != "tertiary" {
		t.Fatalf("window ids wrong: %v", windows)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func tptr(t time.Time) *time.Time { return &t }
