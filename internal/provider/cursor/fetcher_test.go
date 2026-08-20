package cursor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"leeway/internal/provider"
)

const samplePayload = `{
  "billingCycleStart": "1786893641000",
  "billingCycleEnd": "1789572041000",
  "planUsage": {
    "totalSpend": 21921,
    "includedSpend": 21921,
    "remaining": 18079,
    "limit": 40000,
    "remainingBonus": false,
    "autoPercentUsed": 0,
    "apiPercentUsed": 43.842,
    "totalPercentUsed": 8.7684
  },
  "spendLimitUsage": {"limitType": "user"},
  "displayThreshold": 200,
  "enabled": true,
  "autoBucketModels": ["default", "composer-2.5"]
}`

func newTestProvider(t *testing.T, status int, body string) (*cursorProvider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/aiserver.v1.DashboardService/GetCurrentPeriodUsage" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("bad Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	p := newProvider(srv.Client(), time.Now)
	return p, srv
}

func TestFetchOK(t *testing.T) {
	p, srv := newTestProvider(t, 200, samplePayload)
	defer srv.Close()

	orig := usageURL
	usageURL = srv.URL + "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	defer func() { usageURL = orig }()

	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{Account: "default"})
	if err != nil {
		t.Fatal(err)
	}

	if len(snap.Windows) < 3 {
		t.Fatalf("expected >=3 windows, got %d", len(snap.Windows))
	}
	primary := snap.Windows[0]
	if primary.UsedPercent == nil || *primary.UsedPercent != 8.7684 {
		t.Fatalf("primary percent = %v, want 8.7684", primary.UsedPercent)
	}
	if primary.ResetsAt == nil {
		t.Fatal("expected billing cycle end as resetsAt")
	}
	wantReset := time.UnixMilli(1789572041000)
	if !primary.ResetsAt.Equal(wantReset) {
		t.Fatalf("resetsAt = %v, want %v", primary.ResetsAt, wantReset)
	}
	if primary.WindowMinutes == nil || *primary.WindowMinutes != monthlyWindowMinutes {
		t.Fatalf("primary windowMinutes = %v", primary.WindowMinutes)
	}

	api := snap.Windows[2]
	if api.UsedPercent == nil || *api.UsedPercent != 43.842 {
		t.Fatalf("api percent = %v, want 43.842", api.UsedPercent)
	}

	var credits *float64
	for _, w := range snap.Windows {
		if w.ID == "credits" {
			v := *w.Used
			credits = &v
			if w.Limit == nil || *w.Limit != 40000 {
				t.Fatalf("credits limit = %v, want 40000", w.Limit)
			}
		}
	}
	if credits == nil || *credits != 18079 {
		t.Fatalf("credits remaining = %v, want 18079", credits)
	}
}

func TestFetchUnauthorized(t *testing.T) {
	p, srv := newTestProvider(t, 401, `{"error":"unauthorized"}`)
	defer srv.Close()
	orig := usageURL
	usageURL = srv.URL + "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	defer func() { usageURL = orig }()

	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	_, err := p.Fetch(context.Background(), provider.FetchRequest{Account: "default"})
	if err == nil || !strings.Contains(err.Error(), "token rejected") {
		t.Fatalf("expected token-rejected error, got %v", err)
	}
}

func TestFetchServerError(t *testing.T) {
	p, srv := newTestProvider(t, 500, `oops`)
	defer srv.Close()
	orig := usageURL
	usageURL = srv.URL + "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	defer func() { usageURL = orig }()

	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	_, err := p.Fetch(context.Background(), provider.FetchRequest{Account: "default"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error, got %v", err)
	}
}

func TestResolveTokenPrecedence(t *testing.T) {
	// env wins over apiKey
	t.Setenv("CURSOR_ACCESS_TOKEN", "from-env")
	req := provider.FetchRequest{Creds: provider.Credentials{APIKey: "from-account"}}
	tok, err := resolveToken(req)
	if err != nil || tok != "from-env" {
		t.Fatalf("env precedence failed: %q, %v", tok, err)
	}

	// account apiKey wins over keychain (keychain not consulted on non-mac)
	t.Setenv("CURSOR_ACCESS_TOKEN", "")
	req = provider.FetchRequest{Creds: provider.Credentials{APIKey: "from-account"}}
	tok, err = resolveToken(req)
	if err != nil || tok != "from-account" {
		t.Fatalf("account apiKey precedence failed: %q, %v", tok, err)
	}
}

func TestAuthSpec(t *testing.T) {
	p := New()
	spec := p.AuthSpec()
	if len(spec.EnvAuthToken) != 1 || spec.EnvAuthToken[0] != "CURSOR_ACCESS_TOKEN" {
		t.Fatalf("bad AuthSpec env token: %v", spec.EnvAuthToken)
	}
	if !spec.SupportsAccounts {
		t.Fatal("expected SupportsAccounts=true")
	}
}

func TestMilliToTime(t *testing.T) {
	got := millisToTime("1789572041000")
	if got == nil {
		t.Fatal("expected time")
	}
	if !got.Equal(time.UnixMilli(1789572041000)) {
		t.Fatalf("got %v", got)
	}
	if millisToTime("") != nil || millisToTime("abc") != nil {
		t.Fatal("bad input should yield nil")
	}
}

func TestJSONContract(t *testing.T) {
	var d usageData
	if err := json.Unmarshal([]byte(samplePayload), &d); err != nil {
		t.Fatal(err)
	}
	if d.PlanUsage.TotalPercent != 8.7684 || d.PlanUsage.Limit != 40000 {
		t.Fatalf("bad parse: %+v", d.PlanUsage)
	}
}
