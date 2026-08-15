package opencodego

import (
	"strings"
	"testing"
	"time"
)

func TestParseAPIUsage(t *testing.T) {
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	text := `{"usage":{
		"rolling":{"status":"ok","percent":15,"resetsAt":"2026-08-16T13:00:00Z"},
		"weekly":{"status":"ok","percent":90,"resetsAt":"2026-08-17T00:00:00Z"},
		"monthly":{"status":"ok","percent":55,"resetsAt":"2026-09-08T13:31:22Z"}
	}}`
	sub, err := parseAPIUsage(text, now)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if sub.rollingPercent != 15 || sub.weeklyPercent != 90 || sub.monthlyPercent != 55 {
		t.Fatalf("percents wrong: %+v", sub)
	}
	if !sub.hasWeeklyUsage || !sub.hasMonthlyUsage {
		t.Fatalf("window flags wrong: %+v", sub)
	}
	if sub.rollingResetSec <= 0 || sub.weeklyResetSec <= 0 || sub.monthlyResetSec <= 0 {
		t.Fatalf("resets must be positive intervals: %+v", sub)
	}
}

func TestParseAPIUsageToleratesMissingLane(t *testing.T) {
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	// weekly lane missing entirely, monthly unparseable — only rolling survives.
	text := `{"usage":{
		"rolling":{"status":"ok","percent":7,"resetsAt":"2026-08-16T13:00:00Z"},
		"monthly":{"status":"error"}
	}}`
	sub, err := parseAPIUsage(text, now)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if sub.rollingPercent != 7 {
		t.Fatalf("rolling wrong: %+v", sub)
	}
	if sub.hasWeeklyUsage || sub.hasMonthlyUsage {
		t.Fatalf("broken lanes must be dropped: %+v", sub)
	}
}

func TestParseAPIUsagePercentIsDirect(t *testing.T) {
	// The gateway reports percent as an integer 0-100. A value of 1 must stay
	// 1% — the workspace-page fraction heuristic must not scale it to 100.
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	text := `{"usage":{
		"rolling":{"status":"ok","percent":1,"resetsAt":"2026-08-16T13:00:00Z"},
		"weekly":{"status":"rate-limited","percent":100,"resetsAt":"2026-08-17T00:00:00Z"},
		"monthly":{"status":"ok","percent":0,"resetsAt":"2026-09-08T13:31:22Z"}
	}}`
	sub, err := parseAPIUsage(text, now)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if sub.rollingPercent != 1 {
		t.Fatalf("rolling percent = %v, want 1 (integer percent must not be scaled)", sub.rollingPercent)
	}
	if sub.weeklyPercent != 100 {
		t.Fatalf("weekly percent = %v, want 100", sub.weeklyPercent)
	}
	if sub.monthlyPercent != 0 {
		t.Fatalf("monthly percent = %v, want 0", sub.monthlyPercent)
	}
}

func TestParseAPIUsageRequiresRolling(t *testing.T) {
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	for _, text := range []string{
		`{"usage":{}}`,
		`{"weekly":{"percent":90,"resetsAt":"2026-08-17T00:00:00Z"}}`, // no usage object
		`not json`,
	} {
		if _, err := parseAPIUsage(text, now); err == nil {
			t.Fatalf("expected error for %q", text)
		} else if !strings.Contains(err.Error(), "Missing usage fields") && text != "not json" {
			t.Fatalf("expected missing-usage error, got %v", err)
		}
	}
}
