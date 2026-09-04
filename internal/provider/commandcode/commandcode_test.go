package commandcode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

var fixedNow = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func newTestServer(t *testing.T, status int, credits []byte) *httptest.Server {
	t.Helper()
	return newTestServerFull(t, status, credits, []byte(`{"org":null}`), []byte(`{"data":{"planId":"individual-goat"}}`), "")
}

func newTestServerFull(t *testing.T, status int, credits, whoami, subs []byte, wantOrg string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer user_testkey" {
			t.Errorf("Authorization = %q", got)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if wantOrg != "" && r.URL.Path != "/alpha/whoami" && r.URL.Query().Get("orgId") != wantOrg {
			t.Errorf("path %s orgId = %q, want %s", r.URL.Path, r.URL.Query().Get("orgId"), wantOrg)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/alpha/whoami" {
			w.WriteHeader(http.StatusOK)
			w.Write(whoami)
			return
		}
		w.WriteHeader(status)
		switch r.URL.Path {
		case "/alpha/billing/credits":
			w.Write(credits)
		case "/alpha/billing/subscriptions":
			w.Write(subs)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestFetchOKParsesThreeWindows(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, fixture(t, "credits.json"))
	defer srv.Close()
	p := newProvider(srv.Client(), srv.URL, func() time.Time { return fixedNow })

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{APIKey: "user_testkey"},
		Account: "work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Identity == nil || snap.Identity.Account != "work" || snap.Identity.Plan != "GOAT" {
		t.Fatalf("identity = %+v", snap.Identity)
	}
	if len(snap.Windows) != 3 {
		t.Fatalf("windows = %d, want 3: %+v", len(snap.Windows), snap.Windows)
	}

	primary := snap.Windows[0]
	if primary.ID != "primary" || primary.Title != "5-hour" {
		t.Fatalf("primary = %+v", primary)
	}
	if primary.Used == nil || primary.Limit == nil || *primary.Used != 4.48 || *primary.Limit != 14 {
		t.Fatalf("5-hour spend = %+v, want $4.48 / $14", primary)
	}
	if primary.Currency != "USD" || primary.UsedPercent != nil {
		t.Fatalf("5-hour should be dollar spend, not percent: %+v", primary)
	}
	if primary.WindowMinutes == nil || *primary.WindowMinutes != 5*60 {
		t.Fatalf("5-hour minutes = %v", primary.WindowMinutes)
	}
	wantReset := time.Date(2026, 9, 2, 18, 0, 0, 0, time.UTC)
	if primary.ResetsAt == nil || !primary.ResetsAt.Equal(wantReset) {
		t.Fatalf("5-hour resetsAt = %v, want %v", primary.ResetsAt, wantReset)
	}

	weekly := snap.Windows[1]
	if weekly.ID != "secondary" || weekly.Title != "Weekly" {
		t.Fatalf("weekly = %+v", weekly)
	}
	if weekly.Used == nil || weekly.Limit == nil || *weekly.Used != 14.35 || *weekly.Limit != 35 {
		t.Fatalf("weekly spend = %+v, want $14.35 / $35", weekly)
	}
	if weekly.WindowMinutes == nil || *weekly.WindowMinutes != 7*24*60 {
		t.Fatalf("weekly minutes = %v", weekly.WindowMinutes)
	}

	monthly := snap.Windows[2]
	if monthly.ID != "tertiary" || monthly.Title != "Monthly" {
		t.Fatalf("monthly = %+v", monthly)
	}
	if monthly.Used == nil || monthly.Limit == nil || *monthly.Used != 21.5 || *monthly.Limit != 70 {
		t.Fatalf("monthly spend = %+v, want $21.50 / $70", monthly)
	}
	if monthly.WindowMinutes == nil || *monthly.WindowMinutes != 30*24*60 {
		t.Fatalf("monthly minutes = %v", monthly.WindowMinutes)
	}
}

func TestFetchScopesOrgAndUnknownPlanOmitsCeiling(t *testing.T) {
	credits := []byte(`{"credits":{"monthlyCredits":12},"windowLimits":{"fiveHour":{"used":1,"cap":14,"resetAt":"2026-09-02T18:00:00Z"}}}`)
	whoami := []byte(`{"org":{"id":"org_9"}}`)
	subs := []byte(`{"data":{"planId":"individual-future"}}`)
	srv := newTestServerFull(t, http.StatusOK, credits, whoami, subs, "org_9")
	defer srv.Close()
	p := newProvider(srv.Client(), srv.URL, func() time.Time { return fixedNow })
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{APIKey: "user_testkey"},
		Account: "goat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Identity == nil || snap.Identity.Plan != "individual-future" {
		t.Fatalf("unknown plan should keep the id: %+v", snap.Identity)
	}
	var monthly *model.RateWindow
	for i := range snap.Windows {
		if snap.Windows[i].ID == "tertiary" {
			monthly = &snap.Windows[i]
		}
	}
	if monthly == nil || monthly.Used == nil || *monthly.Used != 12 || monthly.Limit != nil {
		t.Fatalf("unknown plan must not invent a monthly cap: %+v", monthly)
	}
}

func TestMonthlyAllowanceFromPlanTable(t *testing.T) {
	credits := []byte(`{"credits":{"monthlyCredits":20}}`)
	whoami := []byte(`{"org":null}`)
	subs := []byte(`{"data":{"planId":"individual-goat"}}`)
	srv := newTestServerFull(t, http.StatusOK, credits, whoami, subs, "")
	defer srv.Close()
	p := newProvider(srv.Client(), srv.URL, func() time.Time { return fixedNow })
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds: provider.Credentials{APIKey: "user_testkey"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Windows) != 1 || snap.Windows[0].Used == nil || snap.Windows[0].Limit == nil {
		t.Fatalf("windows = %+v", snap.Windows)
	}
	if *snap.Windows[0].Used != 50 || *snap.Windows[0].Limit != 70 {
		t.Fatalf("GOAT monthly = %+v, want $50 / $70", snap.Windows[0])
	}
}

func TestFetchUnauthorized(t *testing.T) {
	srv := newTestServer(t, http.StatusUnauthorized, []byte(`{"success":false,"error":{"code":"UNAUTHORIZED","status":401}}`))
	defer srv.Close()
	p := newProvider(srv.Client(), srv.URL, func() time.Time { return fixedNow })

	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds: provider.Credentials{APIKey: "user_testkey"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid Command Code API key") {
		t.Fatalf("expected invalid-key error, got %v", err)
	}
}

func TestFetchMissingKey(t *testing.T) {
	p := newProvider(nil, "http://127.0.0.1:1", func() time.Time { return fixedNow })
	_, err := p.Fetch(context.Background(), provider.FetchRequest{})
	if err == nil || !strings.Contains(err.Error(), "missing Command Code API key") {
		t.Fatalf("expected missing-key error, got %v", err)
	}
}

func TestParseSnakeCaseWindows(t *testing.T) {
	body := []byte(`{
		"window_limits": {
			"five_hour": {"used": 1, "cap": 4, "resets_at": "2026-09-02T18:00:00Z"},
			"weekly": {"used_percent": 10, "resets_at": "2026-09-07T00:00:00Z"}
		},
		"credits": {"monthly_credits": 12}
	}`)
	u := parseUsage(body)
	if !u.fiveHour.ok || !u.fiveHour.hasSpend || u.fiveHour.used != 1 || u.fiveHour.cap != 4 {
		t.Fatalf("fiveHour = %+v", u.fiveHour)
	}
	if !u.weekly.ok || u.weekly.percent != 10 {
		t.Fatalf("weekly = %+v", u.weekly)
	}
	if u.remaining == nil || *u.remaining != 12 {
		t.Fatalf("remaining = %v", u.remaining)
	}
}

func TestAuthSpec(t *testing.T) {
	p := New()
	if p.ID() != "commandcode" || p.DisplayName() != "Command Code" {
		t.Fatalf("id/name = %s %s", p.ID(), p.DisplayName())
	}
	spec := p.AuthSpec()
	if len(spec.EnvAPIKey) != 2 || spec.EnvAPIKey[0] != "COMMAND_CODE_API_KEY" || spec.EnvAPIKey[1] != "COMMANDCODE_API_KEY" {
		t.Fatalf("EnvAPIKey = %v", spec.EnvAPIKey)
	}
	if !spec.SupportsAccounts || spec.LoginHint == "" {
		t.Fatalf("AuthSpec = %+v", spec)
	}
	if n := len(p.DeclaredWindows()); n != 3 {
		t.Fatalf("DeclaredWindows = %d", n)
	}
}
