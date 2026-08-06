package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOutputJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	pct := 62.5
	used := 12.34
	windowMinutes := 7 * 24 * 60
	out := Output{
		Version:     1,
		GeneratedAt: now,
		Providers: []ProviderResult{{
			ID: "deepseek", DisplayName: "DeepSeek", Status: "ok", UpdatedAt: now,
			Accounts: []AccountResult{{
				Name: "default", Status: "ok",
				Snapshot: &Snapshot{
					UpdatedAt: now,
					Windows: []RateWindow{{
						ID: "primary", Title: "Weekly", UsedPercent: &pct,
						ResetsAt: &now, WindowMinutes: &windowMinutes,
					}, {
						ID: "balance", Title: "Balance", Used: &used, Currency: "CNY",
					}},
					Cost: &Cost{Period: "Balance", Currency: "CNY", Amount: &used},
				},
			}},
		}},
	}
	raw, err := json.Marshal(&out)
	if err != nil {
		t.Fatal(err)
	}
	var back Output
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Version != 1 || len(back.Providers) != 1 || len(back.Providers[0].Accounts) != 1 {
		t.Fatalf("round trip mismatch: %+v", back)
	}
	win := back.Providers[0].Accounts[0].Snapshot.Windows[0]
	if *win.UsedPercent != pct || *win.WindowMinutes != windowMinutes {
		t.Fatalf("window mismatch: %+v", win)
	}
	if !strings.Contains(string(raw), `"usedPercent":62.5`) {
		t.Fatalf("JSON contract field missing: %s", raw)
	}
}

func TestDurationShort(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{45 * time.Minute, "45m"},
		{75 * time.Minute, "1h 15m"},
		{3*24*time.Hour + 4*time.Hour, "3d 04h"},
		{3 * 24 * time.Hour, "3d"},
		{1 * time.Hour, "1h"},
		{-5 * time.Minute, "overdue"},
		// Exact boundaries the table renderer hits.
		{24 * time.Hour, "1d"},
		{25 * time.Hour, "1d 01h"},
		{59*time.Minute + 59*time.Second, "59m"},
		{1 * time.Minute, "1m"},
		{0, "now"},
		{90 * time.Minute, "1h 30m"},
		{1*time.Hour + 5*time.Minute, "1h 05m"},
	}
	for _, c := range cases {
		if got := DurationShort(c.in); got != c.want {
			t.Errorf("DurationShort(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWindowStatus(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "ok"},
		{-1, "ok"}, // negatives (e.g. unknown/absent usage) are not critical
		{49.9, "ok"},
		{50, "warn"}, // threshold: >= 50 is warn, not ok
		{79.9, "warn"},
		{80, "critical"}, // threshold: >= 80 is critical, not warn
		{95, "critical"},
		{100, "critical"},
		{150, "critical"},
	}
	for _, c := range cases {
		if got := WindowStatus(c.in); got != c.want {
			t.Errorf("WindowStatus(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
