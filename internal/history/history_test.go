package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"leeway/internal/model"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("LEEWAY_CACHE", t.TempDir())
}

func f64(v float64) *float64 { return &v }

func tptr(tm time.Time) *time.Time { return &tm }

func TestProjectOmitsSecretsAndErrors(t *testing.T) {
	isolate(t)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	out := &model.Output{
		Version:     1,
		GeneratedAt: now,
		Providers: []model.ProviderResult{{
			ID: "cursor", DisplayName: "Cursor", Status: "error",
			Error: "Bearer sk-secret Cookie: a=b",
			Accounts: []model.AccountResult{
				{Name: "bad", Status: "error", Error: "Authorization: Bearer sk-secret"},
				{Name: "work", Status: "ok", Snapshot: &model.Snapshot{
					UpdatedAt: now,
					Identity:  &model.Identity{Account: "alice@example.com", Plan: "Ultra"},
					Windows: []model.RateWindow{{
						ID: "primary", Title: "Monthly", UsedPercent: f64(95),
					}},
				}},
			},
		}},
	}
	rec := project(out, Meta{})
	if rec == nil {
		t.Fatal("expected a record")
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, ban := range []string{"Bearer", "sk-secret", "Cookie", "Authorization", "alice@example.com", "Ultra", "error"} {
		if strings.Contains(s, ban) {
			t.Fatalf("projection leaked %q:\n%s", ban, s)
		}
	}
	if !strings.Contains(s, `"account":"work"`) {
		t.Fatalf("ok account missing:\n%s", s)
	}
}

func TestProjectDropsDuplicateDeepSeekCost(t *testing.T) {
	isolate(t)
	amt := 396.68
	out := &model.Output{GeneratedAt: time.Now(), Providers: []model.ProviderResult{{
		ID: "deepseek", Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{
				Windows: []model.RateWindow{{ID: "primary", Title: "Balance", Used: &amt, Currency: "CNY"}},
				Cost:    &model.Cost{Period: "Balance", Currency: "CNY", Amount: &amt},
			},
		}},
	}}}
	rec := project(out, Meta{})
	if rec == nil || len(rec.Entries) != 1 {
		t.Fatalf("entries = %+v", rec)
	}
	for _, w := range rec.Entries[0].Windows {
		if w.ID == costWindow {
			t.Fatal("duplicate cost window must not be projected")
		}
	}
}

func TestRecordDedupByGeneratedAtNotTail(t *testing.T) {
	isolate(t)
	t1 := time.Date(2026, 9, 9, 1, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	full := func(ts time.Time, pct float64) *model.Output {
		return &model.Output{GeneratedAt: ts, Providers: []model.ProviderResult{
			{ID: "cursor", Accounts: []model.AccountResult{{
				Name: "work", Status: "ok",
				Snapshot: &model.Snapshot{Windows: []model.RateWindow{{
					ID: "primary", UsedPercent: f64(pct),
				}}},
			}}},
			{ID: "grok", Accounts: []model.AccountResult{{
				Name: "home", Status: "ok",
				Snapshot: &model.Snapshot{Windows: []model.RateWindow{{
					ID: "primary", UsedPercent: f64(10),
				}}},
			}}},
		}}
	}
	cursorOnly := func(ts time.Time, pct float64) *model.Output {
		return &model.Output{GeneratedAt: ts, Providers: []model.ProviderResult{
			{ID: "cursor", Accounts: []model.AccountResult{{
				Name: "work", Status: "ok",
				Snapshot: &model.Snapshot{Windows: []model.RateWindow{{
					ID: "primary", UsedPercent: f64(pct),
				}}},
			}}},
		}}
	}
	if err := Record(full(t1, 95), Meta{}); err != nil {
		t.Fatal(err)
	}
	if err := Record(cursorOnly(t2, 96), Meta{ProviderFilter: "cursor"}); err != nil {
		t.Fatal(err)
	}
	if err := Record(full(t1, 95), Meta{}); err != nil {
		t.Fatal(err)
	}
	recs, err := loadRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2 (T1 must not be rewritten)", len(recs))
	}
	seen, err := LoadLastSeenBefore(t1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := seen[Key{"cursor", "work", "primary"}]; ok {
		t.Fatal("last-seen before T1 must be empty")
	}
}

func TestRecordSkipAndEmpty(t *testing.T) {
	isolate(t)
	out := &model.Output{GeneratedAt: time.Now(), Providers: []model.ProviderResult{{
		ID: "cursor", Accounts: []model.AccountResult{{Name: "work", Status: "error", Error: "nope"}},
	}}}
	if err := Record(out, Meta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("LEEWAY_CACHE"), fileName)); !os.IsNotExist(err) {
		t.Fatal("empty projection must not create history.jsonl")
	}
	ok := &model.Output{GeneratedAt: time.Now(), Providers: []model.ProviderResult{{
		ID: "cursor", Accounts: []model.AccountResult{{
			Name: "work", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{{ID: "primary", UsedPercent: f64(1)}}},
		}},
	}}}
	if err := Record(ok, Meta{Skip: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("LEEWAY_CACHE"), fileName)); !os.IsNotExist(err) {
		t.Fatal("Skip must not write")
	}
}

func TestAttachHappyPathAndOmitZero(t *testing.T) {
	prev := map[Key]Obs{
		{"cursor", "work", "primary"}:      {ID: "primary", UsedPercent: f64(93)},
		{"deepseek", "default", "primary"}: {ID: "primary", Used: f64(400), Currency: "CNY"},
	}
	out := &model.Output{Providers: []model.ProviderResult{
		{ID: "cursor", Accounts: []model.AccountResult{{
			Name: "work", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{{ID: "primary", Title: "Monthly", UsedPercent: f64(95)}}},
		}}},
		{ID: "deepseek", Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{{ID: "primary", Title: "Balance", Used: f64(400), Currency: "CNY"}}},
		}}},
		{ID: "grok", Accounts: []model.AccountResult{{
			Name: "home", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{{ID: "primary", UsedPercent: f64(10)}}},
		}}},
	}}
	Attach(out, prev)
	d := out.Providers[0].Accounts[0].Snapshot.Windows[0].Delta
	if d == nil || d.UsedPercent == nil || *d.UsedPercent != 2 {
		t.Fatalf("cursor delta = %+v", d)
	}
	if out.Providers[1].Accounts[0].Snapshot.Windows[0].Delta != nil {
		t.Fatal("sub-threshold money delta must omit")
	}
	if out.Providers[2].Accounts[0].Snapshot.Windows[0].Delta != nil {
		t.Fatal("missing previous must omit, not 0")
	}
}

func TestIsResetRules(t *testing.T) {
	t1 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(30 * 24 * time.Hour)
	tBack := t1.Add(-time.Hour)

	monthlyJump := map[Key]Obs{
		{"cursor", "a", "primary"}:  {ID: "primary", UsedPercent: f64(90), ResetsAt: tptr(t1)},
		{"cursor", "a", "included"}: {ID: "included", Used: f64(50), Limit: f64(400)},
		{"cursor", "a", "auto"}:     {ID: "auto", UsedPercent: f64(40)},
	}
	out := &model.Output{Providers: []model.ProviderResult{{
		ID: "cursor", Accounts: []model.AccountResult{{
			Name: "a", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{
				{ID: "primary", UsedPercent: f64(3), ResetsAt: tptr(t2)},
				{ID: "included", Used: f64(0), Limit: f64(400)},
				{ID: "auto", UsedPercent: f64(5)},
			}},
		}},
	}}}
	Attach(out, monthlyJump)
	wins := out.Providers[0].Accounts[0].Snapshot.Windows
	if wins[0].Delta == nil || !wins[0].Delta.Reset {
		t.Fatalf("monthly ResetsAt jump: %+v", wins[0].Delta)
	}
	if wins[1].Delta == nil || !wins[1].Delta.Reset {
		t.Fatalf("included $50→$0 with monthly jump must reset, got %+v", wins[1].Delta)
	}
	if wins[2].Delta == nil || !wins[2].Delta.Reset {
		t.Fatalf("auto drop with monthly jump must reset, got %+v", wins[2].Delta)
	}

	orphan := map[Key]Obs{{"cursor", "a", "included"}: {ID: "included", Used: f64(50), Limit: f64(400)}}
	out2 := &model.Output{Providers: []model.ProviderResult{{
		ID: "cursor", Accounts: []model.AccountResult{{
			Name: "a", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{
				{ID: "included", Used: f64(0), Limit: f64(400)},
			}},
		}},
	}}}
	Attach(out2, orphan)
	d := out2.Providers[0].Accounts[0].Snapshot.Windows[0].Delta
	if d == nil || !d.Reset {
		t.Fatalf("orphan included near-zero must reset via rule 4: %+v", d)
	}

	grokPrev := map[Key]Obs{
		{"grok", "a", "primary"}:  {ID: "primary", UsedPercent: f64(80), ResetsAt: tptr(t1)},
		{"grok", "a", "ondemand"}: {ID: "ondemand", Used: f64(2500)},
	}
	grok := &model.Output{Providers: []model.ProviderResult{{
		ID: "grok", Accounts: []model.AccountResult{{
			Name: "a", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{
				{ID: "primary", UsedPercent: f64(5), ResetsAt: tptr(t2)},
				{ID: "ondemand", Used: f64(2400)},
			}},
		}},
	}}}
	Attach(grok, grokPrev)
	od := grok.Providers[0].Accounts[0].Snapshot.Windows[1].Delta
	if od == nil || od.Reset || od.Used == nil || *od.Used != -100 {
		t.Fatalf("grok ondemand must be spend, not reset: %+v", od)
	}

	ccPrev := map[Key]Obs{
		{"commandcode", "a", "primary"}:  {ID: "primary", UsedPercent: f64(90), ResetsAt: tptr(t1)},
		{"commandcode", "a", "tertiary"}: {ID: "tertiary", Used: f64(12)},
	}
	cc := &model.Output{Providers: []model.ProviderResult{{
		ID: "commandcode", Accounts: []model.AccountResult{{
			Name: "a", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{
				{ID: "primary", UsedPercent: f64(1), ResetsAt: tptr(t1.Add(5 * time.Hour))},
				{ID: "tertiary", Used: f64(10)},
			}},
		}},
	}}}
	Attach(cc, ccPrev)
	ter := cc.Providers[0].Accounts[0].Snapshot.Windows[1].Delta
	if ter == nil || ter.Reset || ter.Used == nil || *ter.Used != -2 {
		t.Fatalf("commandcode remaining must not reset: %+v", ter)
	}

	zenPrev := map[Key]Obs{
		{"opencode-go", "a", "primary"}:  {ID: "primary", UsedPercent: f64(40), ResetsAt: tptr(t1)},
		{"opencode-go", "a", costWindow}: {ID: costWindow, Used: f64(18.5)},
	}
	zen := &model.Output{Providers: []model.ProviderResult{{
		ID: "opencode-go", Accounts: []model.AccountResult{{
			Name: "a", Status: "ok",
			Snapshot: &model.Snapshot{
				Windows: []model.RateWindow{{ID: "primary", UsedPercent: f64(2), ResetsAt: tptr(t1.Add(5 * time.Hour))}},
				Cost:    &model.Cost{Period: "Zen balance", Currency: "USD", Amount: f64(17.5)},
			},
		}},
	}}}
	Attach(zen, zenPrev)
	if z := zen.Providers[0].Accounts[0].Snapshot.Cost.Delta; z == nil || z.Reset || z.Amount == nil || *z.Amount != -1 {
		t.Fatalf("zen cost must be spend, not reset: %+v", zen.Providers[0].Accounts[0].Snapshot.Cost.Delta)
	}

	backPrev := map[Key]Obs{{"opencode-go", "a", "primary"}: {ID: "primary", UsedPercent: f64(40), ResetsAt: tptr(t1)}}
	back := &model.Output{Providers: []model.ProviderResult{{
		ID: "opencode-go", Accounts: []model.AccountResult{{
			Name: "a", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{{
				ID: "primary", UsedPercent: f64(41), ResetsAt: tptr(tBack),
			}}},
		}},
	}}}
	Attach(back, backPrev)
	bd := back.Providers[0].Accounts[0].Snapshot.Windows[0].Delta
	if bd == nil || bd.Reset || bd.UsedPercent == nil || *bd.UsedPercent != 1 {
		t.Fatalf("ResetsAt moving backward is countdown, not reset: %+v", bd)
	}

	dayPrev := map[Key]Obs{{"deepseek", "default", "day-2026-09-09"}: {ID: "day-2026-09-09", Used: f64(1e6), Currency: "CNY"}}
	day := &model.Output{Providers: []model.ProviderResult{{
		ID: "deepseek", Accounts: []model.AccountResult{{
			Name: "default", Status: "ok",
			Snapshot: &model.Snapshot{Windows: []model.RateWindow{{
				ID: "day-2026-09-09", Used: f64(1.2e6), Currency: "CNY",
			}}},
		}},
	}}}
	Attach(day, dayPrev)
	dd := day.Providers[0].Accounts[0].Snapshot.Windows[0].Delta
	if dd == nil || dd.Reset || dd.Used == nil || *dd.Used != 200000 {
		t.Fatalf("day-* must be spend delta: %+v", dd)
	}
}

func TestCollectLastNAndFilter(t *testing.T) {
	isolate(t)
	base := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		out := &model.Output{GeneratedAt: base.Add(time.Duration(i) * time.Hour), Providers: []model.ProviderResult{
			{ID: "cursor", Accounts: []model.AccountResult{{
				Name: "work", Status: "ok",
				Snapshot: &model.Snapshot{Windows: []model.RateWindow{{ID: "primary", UsedPercent: f64(float64(10 + i))}}},
			}}},
			{ID: "grok", Accounts: []model.AccountResult{{
				Name: "home", Status: "ok",
				Snapshot: &model.Snapshot{Windows: []model.RateWindow{{ID: "primary", UsedPercent: f64(1)}}},
			}}},
		}}
		if err := Record(out, Meta{}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := Collect(3, "cursor", "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || len(all[0].Points) != 3 {
		t.Fatalf("series = %+v", all)
	}
	if *all[0].Points[0].UsedPercent != 12 || *all[0].Points[2].UsedPercent != 14 {
		t.Fatalf("last 3 points = %+v", all[0].Points)
	}
}
