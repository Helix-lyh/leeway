package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"leeway/internal/model"
)

func TestRenderJSON(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	out := &model.Output{Version: 1, GeneratedAt: now}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"version": 1`) {
		t.Fatalf("json missing version: %s", buf.String())
	}
}

func TestRenderTablePercentWindow(t *testing.T) {
	pct := 62.0
	weekly := 7 * 24 * 60
	now := time.Now().Add(3 * 24 * time.Hour)
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "kimi", DisplayName: "Kimi Code", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows: []model.RateWindow{{
					ID: "primary", Title: "Weekly", UsedPercent: &pct, ResetsAt: &now, WindowMinutes: &weekly,
				}},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "Kimi Code") {
		t.Fatalf("provider head missing:\n%s", s)
	}
	for _, col := range []string{"PROVIDER", "5-HOUR", "WEEKLY", "MONTHLY", "CASH"} {
		if !strings.Contains(s, col) {
			t.Fatalf("header column %q missing:\n%s", col, s)
		}
	}
	if !strings.Contains(s, "62%") {
		t.Fatalf("window segment missing:\n%s", s)
	}
	if strings.Contains(s, "0%") {
		t.Fatalf("missing quota slots must not be rendered as synthetic 0%%:\n%s", s)
	}
	if strings.Count(s, "—") != 2 {
		t.Fatalf("missing 5-hour/monthly slots must be unavailable markers:\n%s", s)
	}
	want := model.DurationShort(time.Until(now))
	if !strings.Contains(s, want) {
		t.Fatalf("reset countdown %q missing:\n%s", want, s)
	}
	// An all-ok, default-account provider omits status/account noise.
	if strings.Contains(s, "ok") || strings.Contains(s, "default") {
		t.Fatalf("status/account noise should be omitted:\n%s", s)
	}
}

func TestRenderTableErrorRow(t *testing.T) {
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "codex", DisplayName: "Codex", Status: "error", Error: "session expired — run: codex login",
		Accounts: []model.AccountResult{{Name: "default", Status: "error", Error: "session expired — run: codex login"}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "✗ session expired") {
		t.Fatalf("error segment missing:\n%s", s)
	}
	if !strings.Contains(s, "error") {
		t.Fatalf("status badge missing from head:\n%s", s)
	}
}

func TestRenderTableCostRows(t *testing.T) {
	amount := 12.34
	now := time.Now()
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "deepseek", DisplayName: "DeepSeek", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: now,
				Windows:   []model.RateWindow{{ID: "primary", Title: "Balance", Used: &amount, Currency: "USD"}},
				Cost:      &model.Cost{Period: "Balance", Currency: "CNY", Amount: &amount},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "¥12.34 balance") {
		t.Fatalf("snapshot cost footnote missing:\n%s", s)
	}
}

func TestRenderTableCostDeduplicated(t *testing.T) {
	amount := 12.34
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "deepseek", DisplayName: "DeepSeek", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows:   []model.RateWindow{{ID: "primary", Title: "Balance", Used: &amount, Currency: "CNY"}},
				Cost:      &model.Cost{Period: "Balance", Currency: "CNY", Amount: &amount},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if strings.Count(s, "¥12.34") != 1 {
		t.Fatalf("amount should appear exactly once:\n%s", s)
	}
}

func TestRenderTableEmptyOutput(t *testing.T) {
	var buf bytes.Buffer
	RenderTable(&buf, &model.Output{Version: 1}, false)
	if !strings.Contains(buf.String(), "(no providers)") {
		t.Fatalf("empty output marker missing:\n%s", buf.String())
	}
}

func TestRenderTableUsedWithLimit(t *testing.T) {
	used := 8.5
	limit := 10.0
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "deepseek", DisplayName: "DeepSeek", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows:   []model.RateWindow{{ID: "primary", Title: "Balance", Used: &used, Limit: &limit, Currency: "USD"}},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "$8.50 / $10.00") {
		t.Fatalf("used/limit value missing:\n%s", s)
	}
	if strings.Contains(s, "bal") { // single money window: no title
		t.Fatalf("single balance window should render bare:\n%s", s)
	}
}

func TestRenderTableMultiAccountHeads(t *testing.T) {
	pct := 10.0
	weekly := 7 * 24 * 60
	snap := func() *model.Snapshot {
		return &model.Snapshot{UpdatedAt: time.Now(), Windows: []model.RateWindow{{
			ID: "primary", Title: "Weekly", UsedPercent: &pct, WindowMinutes: &weekly,
		}}}
	}
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "opencode-go", DisplayName: "OpenCode Go", Status: "ok",
		Accounts: []model.AccountResult{
			{Name: "github", Status: "ok", Snapshot: snap()},
			{Name: "work", Status: "ok", Snapshot: snap()},
		},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "github") || !strings.Contains(s, "work") {
		t.Fatalf("account names should be shown for multi-account providers:\n%s", s)
	}
}

func TestRenderTableOverflowWindowOmittedFromCash(t *testing.T) {
	main, spark := 79.0, 0.0
	weekly := 7 * 24 * 60
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "codex", DisplayName: "Codex", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows: []model.RateWindow{
					{ID: "primary", Title: "Weekly", UsedPercent: &main, WindowMinutes: &weekly},
					{ID: "secondary", Title: "Codex Spark Weekly", UsedPercent: &spark, WindowMinutes: &weekly},
				},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "79%") {
		t.Fatalf("primary window missing:\n%s", s)
	}
	if strings.Contains(s, "Codex Spark Weekly") {
		t.Fatalf("overflow quota must not appear in cash/credits:\n%s", s)
	}
	if strings.Count(s, "0%") != 0 {
		t.Fatalf("the duplicate Spark zero must not leak into the table:\n%s", s)
	}
}

func TestRenderTableUnclassifiedPercentOmittedFromCash(t *testing.T) {
	pct := 100.0
	reset := time.Now().Add(2*24*time.Hour + 7*time.Hour)
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "custom", DisplayName: "Custom", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows: []model.RateWindow{{
					ID: "primary", Title: "Credits", UsedPercent: &pct, ResetsAt: &reset,
				}},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if strings.Contains(s, "Credits") || strings.Contains(s, "100%") {
		t.Fatalf("unclassified quota must not appear in cash/credits:\n%s", s)
	}
	if strings.Count(s, "—") != 3 {
		t.Fatalf("unclassified window must not occupy a fixed-period slot:\n%s", s)
	}
}

func TestRenderTableExplicitZeroRemainsZero(t *testing.T) {
	zero := 0.0
	fiveHours := 5 * 60
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "kimi", DisplayName: "Kimi Code", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows: []model.RateWindow{{
					ID: "primary", Title: "5-hour", UsedPercent: &zero, WindowMinutes: &fiveHours,
				}},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if strings.Count(s, "0%") != 1 {
		t.Fatalf("an explicit provider zero must render exactly once:\n%s", s)
	}
	if strings.Count(s, "—") != 2 {
		t.Fatalf("only absent weekly/monthly slots should be unavailable:\n%s", s)
	}
}

func TestQuotaSlotUsesExplicitDurationNotTitle(t *testing.T) {
	fiveHours, weekly, monthly, unknown := 300, 10080, 43200, 1234
	cases := []struct {
		name string
		win  model.RateWindow
		want int
	}{
		{"five-hour", model.RateWindow{Title: "Monthly", WindowMinutes: &fiveHours}, slot5h},
		{"weekly", model.RateWindow{Title: "Credits", WindowMinutes: &weekly}, slotWeekly},
		{"monthly", model.RateWindow{Title: "Weekly", WindowMinutes: &monthly}, slotMonthly},
		{"title without duration", model.RateWindow{Title: "Weekly"}, -1},
		{"unknown duration", model.RateWindow{Title: "5-hour", WindowMinutes: &unknown}, -1},
	}
	for _, tc := range cases {
		if got := quotaSlot(tc.win); got != tc.want {
			t.Errorf("%s: quotaSlot = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestRenderTableTimeOnlyMarkerOmittedFromCash(t *testing.T) {
	renews := time.Now().Add(28*24*time.Hour + 3*time.Hour)
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "opencode-go", DisplayName: "OpenCode Go", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows: []model.RateWindow{{
					ID: "renewal", Title: "Renews", ResetsAt: &renews,
				}},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if strings.Contains(s, "Renews") {
		t.Fatalf("time-only marker must not appear in cash/credits:\n%s", s)
	}
	if strings.Contains(s, "0%") {
		t.Fatalf("time-only marker must not manufacture zero usage:\n%s", s)
	}
}

func TestRenderTableZeroCostFootnoteDropped(t *testing.T) {
	zero, balance := 0.0, 395.06
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "codex", DisplayName: "Codex", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows:   []model.RateWindow{{ID: "primary", Title: "Balance", Used: &balance, Currency: "CNY"}},
				Cost:      &model.Cost{Period: "Credits", Currency: "USD", Amount: &zero},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if strings.Contains(s, "$0.00") {
		t.Fatalf("zero cost footnote should be dropped:\n%s", s)
	}
	if !strings.Contains(s, "¥395.06") {
		t.Fatalf("real balance missing:\n%s", s)
	}
}

func TestMoney(t *testing.T) {
	cases := []struct {
		amount   float64
		currency string
		want     string
	}{
		{12.34, "CNY", "¥12.34"},
		{12.34, "¥", "¥12.34"},
		{12.34, "USD", "$12.34"},
		{12.34, "$", "$12.34"},
		{12.34, "EUR", "€12.34"},
		{12.34, "GBP", "£12.34"},
		{12.34, "JPY", "JPY 12.34"},
		{12.34, "", "12.34"},
	}
	for _, c := range cases {
		if got := money(c.amount, c.currency); got != c.want {
			t.Errorf("money(%v, %q) = %q, want %q", c.amount, c.currency, got, c.want)
		}
	}
}

func TestBar(t *testing.T) {
	if got := bar(100); got != strings.Repeat("⣿", barWidth) {
		t.Errorf("bar(100) = %q, want %d full cells", got, barWidth)
	}
	if got := bar(150); got != strings.Repeat("⣿", barWidth) {
		t.Errorf("bar(150) = %q, want clamped to full", got)
	}
	if got := bar(-5); got != strings.Repeat("⣀", barWidth) {
		t.Errorf("bar(-5) = %q, want clamped to empty", got)
	}
	if got := bar(50); got != strings.Repeat("⣿", 4)+strings.Repeat("⣀", 4) {
		t.Errorf("bar(50) = %q, want 4 full 4 track", got)
	}
	// 56% of 16 dot columns = 8.96 → 9 columns: 4 full cells + a half edge.
	if got := bar(56); got != strings.Repeat("⣿", 4)+"⡇"+strings.Repeat("⣀", 3) {
		t.Errorf("bar(56) = %q, want 4 full + half edge + 3 track", got)
	}
}

func TestUsageBarColor(t *testing.T) {
	got := usageBar(62, true)
	if !strings.Contains(got, "\x1b[33m") { // warn band (50-79)
		t.Errorf("usageBar(62, color) = %q, want yellow warn color", got)
	}
	if !strings.Contains(got, "⣿") {
		t.Errorf("usageBar(62, color) = %q, want filled braille cells", got)
	}
	if !strings.Contains(got, "\x1b[90m") {
		t.Errorf("usageBar(62, color) = %q, want dimmed track", got)
	}
	if green := usageBar(30, true); !strings.Contains(green, "\x1b[32m") {
		t.Errorf("usageBar(30, color) = %q, want green", green)
	}
	if red := usageBar(95, true); !strings.Contains(red, "\x1b[31m") {
		t.Errorf("usageBar(95, color) = %q, want red", red)
	}
	if green := usageBar(30, true); !strings.Contains(green, "\x1b[32m") {
		t.Errorf("usageBar(30, color) = %q, want green", green)
	}
	if red := usageBar(95, true); !strings.Contains(red, "\x1b[31m") {
		t.Errorf("usageBar(95, color) = %q, want red", red)
	}
	if plain := usageBar(62, false); strings.Contains(plain, "\x1b[") {
		t.Errorf("usageBar(62, no color) = %q, want no escapes", plain)
	}
}

func TestVisibleLen(t *testing.T) {
	if got := visibleLen("\x1b[32mok\x1b[0m"); got != 2 {
		t.Errorf("visibleLen(colored ok) = %d, want 2", got)
	}
	if got := visibleLen("¥12.34"); got != 6 {
		t.Errorf("visibleLen(¥12.34) = %d, want 6", got)
	}
	if got := visibleLen(strings.Repeat("━", barWidth)); got != barWidth {
		t.Errorf("visibleLen(bar) = %d, want %d", got, barWidth)
	}
}

func TestStatusAndPercentColorBoundaries(t *testing.T) {
	if got := statusColor("ok", true); got != cGreen {
		t.Errorf("statusColor(ok) = %q, want green", got)
	}
	if got := statusColor("error", true); got != cRed {
		t.Errorf("statusColor(error) = %q, want red", got)
	}
	if got := statusColor("not_configured", true); got != cYellow {
		t.Errorf("statusColor(not_configured) = %q, want yellow", got)
	}
	if got := statusColor("ok", false); got != "" {
		t.Errorf("statusColor(ok, no color) = %q, want empty", got)
	}
	// percentColor: <50 green, <80 yellow, >=80 red.
	if got := percentColor(49); got != cGreen {
		t.Errorf("percentColor(49) = %q, want green", got)
	}
	if got := percentColor(50); got != cYellow {
		t.Errorf("percentColor(50) = %q, want yellow", got)
	}
	if got := percentColor(80); got != cRed {
		t.Errorf("percentColor(80) = %q, want red", got)
	}
}
