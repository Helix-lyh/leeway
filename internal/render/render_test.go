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

func TestRenderTableColumns(t *testing.T) {
	pct := 62.0
	now := time.Now().Add(3 * 24 * time.Hour)
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "kimi", DisplayName: "Kimi Code", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: time.Now(),
				Windows:   []model.RateWindow{{ID: "primary", Title: "Weekly", UsedPercent: &pct, ResetsAt: &now}},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	for _, col := range []string{"PROVIDER", "STATUS", "ACCOUNT", "WINDOW", "USED", "LIMIT", "%", "RESET IN"} {
		if !strings.Contains(s, col) {
			t.Fatalf("table missing column %q:\n%s", col, s)
		}
	}
	if !strings.Contains(s, "62%") {
		t.Fatalf("table missing percent:\n%s", s)
	}
	want := model.DurationShort(time.Until(now))
	if !strings.Contains(s, want) {
		t.Fatalf("table missing reset countdown %q:\n%s", want, s)
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
	if !strings.Contains(s, "└ session expired") {
		t.Fatalf("error continuation row missing:\n%s", s)
	}
	if !strings.Contains(s, "error") {
		t.Fatalf("status column missing error:\n%s", s)
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
		LocalCost: &model.Cost{Period: "today", Currency: "USD", Amount: &amount},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "cost:Balance") || !strings.Contains(s, "¥12.34") {
		t.Fatalf("snapshot cost row missing:\n%s", s)
	}
	if !strings.Contains(s, "local:today") || !strings.Contains(s, "$12.34") {
		t.Fatalf("local cost row missing:\n%s", s)
	}
}

func TestRenderTableEmptyOutput(t *testing.T) {
	var buf bytes.Buffer
	RenderTable(&buf, &model.Output{Version: 1}, false)
	if !strings.Contains(buf.String(), "(no providers)") {
		t.Fatalf("empty output marker missing:\n%s", buf.String())
	}
}

func TestRenderTableUsedWithoutPercent(t *testing.T) {
	used := 8.5
	limit := 10.0
	now := time.Now()
	out := &model.Output{Version: 1, Providers: []model.ProviderResult{{
		ID: "deepseek", DisplayName: "DeepSeek", Status: "ok",
		Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				UpdatedAt: now,
				Windows:   []model.RateWindow{{ID: "primary", Title: "Balance", Used: &used, Limit: &limit, Currency: "USD"}},
			},
		}},
	}}}
	var buf bytes.Buffer
	RenderTable(&buf, out, false)
	s := buf.String()
	if !strings.Contains(s, "$8.50") || !strings.Contains(s, "$10.00") {
		t.Fatalf("used/limit columns missing:\n%s", s)
	}
	// No UsedPercent and no ResetsAt: the percent and reset columns render
	// "—" (tabwriter pads cells with spaces, so check for the two dashes).
	if strings.Count(s, "—") != 2 {
		t.Fatalf("percent/reset columns should render —:\n%s", s)
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

func TestBar8Clamping(t *testing.T) {
	if got := bar8(100); got != strings.Repeat("█", 8) {
		t.Errorf("bar8(100) = %q, want 8 full cells", got)
	}
	if got := bar8(150); got != strings.Repeat("█", 8) {
		t.Errorf("bar8(150) = %q, want clamped to 8 full cells", got)
	}
	if got := bar8(-5); got != strings.Repeat("░", 8) {
		t.Errorf("bar8(-5) = %q, want all empty cells", got)
	}
	if got := bar8(50); got != strings.Repeat("█", 4)+strings.Repeat("░", 4) {
		t.Errorf("bar8(50) = %q, want 4 full 4 empty", got)
	}
}

func TestPercentCellColor(t *testing.T) {
	pct := func(v float64) *float64 { return &v }
	w := model.RateWindow{UsedPercent: pct(62)}
	got := percentCell(w, true)
	if !strings.Contains(got, "\x1b[33m") { // warn band (50-79)
		t.Errorf("percentCell(62, color) = %q, want yellow warn color", got)
	}
	if !strings.Contains(got, "█") {
		t.Errorf("percentCell(62, color) = %q, want usage bar", got)
	}
	green := percentCell(model.RateWindow{UsedPercent: pct(30)}, true)
	if !strings.Contains(green, "\x1b[32m") {
		t.Errorf("percentCell(30, color) = %q, want green", green)
	}
	red := percentCell(model.RateWindow{UsedPercent: pct(95)}, true)
	if !strings.Contains(red, "\x1b[31m") {
		t.Errorf("percentCell(95, color) = %q, want red", red)
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
