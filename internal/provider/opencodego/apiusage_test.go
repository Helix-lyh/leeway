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
	if sub.rolling.percent != 15 || sub.weekly.percent != 90 || sub.monthly.percent != 55 {
		t.Fatalf("percents wrong: %+v", sub)
	}
	if sub.weekly == nil || sub.monthly == nil {
		t.Fatalf("window flags wrong: %+v", sub)
	}
	if sub.rolling.resetInSec <= 0 || sub.weekly.resetInSec <= 0 || sub.monthly.resetInSec <= 0 {
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
	if sub.rolling.percent != 7 {
		t.Fatalf("rolling wrong: %+v", sub)
	}
	if sub.weekly != nil || sub.monthly != nil {
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
	if sub.rolling.percent != 1 {
		t.Fatalf("rolling percent = %v, want 1 (integer percent must not be scaled)", sub.rolling.percent)
	}
	if sub.weekly.percent != 100 {
		t.Fatalf("weekly percent = %v, want 100", sub.weekly.percent)
	}
	if sub.monthly.percent != 0 {
		t.Fatalf("monthly percent = %v, want 0", sub.monthly.percent)
	}
	if sub.rolling.rateLimited || !sub.weekly.rateLimited || sub.monthly.rateLimited {
		t.Fatalf("rate-limited flags wrong: rolling=%v weekly=%v monthly=%v",
			sub.rolling.rateLimited, sub.weekly.rateLimited, sub.monthly.rateLimited)
	}
}

func TestParseAPIWindowRateLimitedWithoutPercent(t *testing.T) {
	// A rate-limited lane without a percent means the quota is exhausted:
	// report 100% rather than dropping the lane.
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	w, ok := parseAPIWindow(map[string]any{
		"status":   "rate-limited",
		"resetsAt": "2026-08-17T00:00:00Z",
	}, now)
	if !ok {
		t.Fatal("rate-limited lane without percent must not be dropped")
	}
	if w.percent != 100 || !w.rateLimited {
		t.Fatalf("want percent 100 + rateLimited, got %+v", w)
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
		} else if !strings.Contains(err.Error(), "用量字段缺失") && text != "not json" {
			t.Fatalf("expected missing-usage error, got %v", err)
		}
	}
}
