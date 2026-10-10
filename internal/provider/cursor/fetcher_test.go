package cursor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"leeway/internal/auth"
	"leeway/internal/model"
	"leeway/internal/provider"
)

func patchMeURL(t *testing.T, u string) {
	t.Helper()
	orig := auth.CursorMeURL
	auth.CursorMeURL = u
	t.Cleanup(func() { auth.CursorMeURL = orig })
}

func patchConnectURLs(t *testing.T, base string) {
	t.Helper()
	origU, origP, origG := usageURL, planURL, grantsURL
	usageURL = base + "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	planURL = base + "/aiserver.v1.DashboardService/GetPlanInfo"
	grantsURL = base + "/aiserver.v1.DashboardService/GetCreditGrantsBalance"
	t.Cleanup(func() {
		usageURL, planURL, grantsURL = origU, origP, origG
	})
}

const samplePayload = `{
  "billingCycleStart": "1786893641000",
  "billingCycleEnd": "1789572041000",
  "planUsage": {
    "totalSpend": 21921,
    "includedSpend": 21921,
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
		if strings.Contains(r.URL.Path, "GetMe") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(r.URL.Path, "GetPlanInfo") {
			w.Write([]byte(`{"planInfo":{"planName":"Ultra"}}`))
			return
		}
		if strings.Contains(r.URL.Path, "GetCreditGrantsBalance") {
			w.Write([]byte(`{"hasCreditGrants":false}`))
			return
		}
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

	patchConnectURLs(t, srv.URL)
	patchMeURL(t, srv.URL+"/aiserver.v1.DashboardService/GetMe")

	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{Account: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Identity == nil || snap.Identity.Plan != "Ultra" {
		t.Fatalf("plan = %+v, want Ultra", snap.Identity)
	}

	if len(snap.Windows) != 2 {
		t.Fatalf("expected 2 windows (monthly + included; auto is 0), got %d", len(snap.Windows))
	}
	primary := snap.Windows[0]
	if primary.UsedPercent == nil || *primary.UsedPercent != 43.842 {
		t.Fatalf("primary (API bucket) percent = %v, want 43.842", primary.UsedPercent)
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

	inc := snap.Windows[1]
	if inc.ID != "included" || inc.Used == nil || inc.Limit == nil {
		t.Fatalf("included window = %+v", inc)
	}
	if *inc.Used != 219.21 || *inc.Limit != 400 || inc.Currency != "USD" {
		t.Fatalf("included = %v / %v %s, want 219.21 / 400 USD", *inc.Used, *inc.Limit, inc.Currency)
	}
}

func TestFetchCreditsWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "GetMe"):
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(r.URL.Path, "GetPlanInfo"):
			w.Write([]byte(`{"planInfo":{"planName":"Pro"}}`))
		case strings.Contains(r.URL.Path, "GetCreditGrantsBalance"):
			w.Write([]byte(`{"hasCreditGrants":true,"totalCents":10000,"usedCents":2500}`))
		default:
			w.Write([]byte(samplePayload))
		}
	}))
	defer srv.Close()
	patchConnectURLs(t, srv.URL)
	patchMeURL(t, srv.URL+"/aiserver.v1.DashboardService/GetMe")
	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	p := newProvider(srv.Client(), time.Now)
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{Account: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Identity == nil || snap.Identity.Plan != "Pro" {
		t.Fatalf("plan = %+v", snap.Identity)
	}
	var credits *model.RateWindow
	for i := range snap.Windows {
		if snap.Windows[i].ID == "credits" {
			credits = &snap.Windows[i]
		}
	}
	if credits == nil || credits.Used == nil || credits.Limit == nil {
		t.Fatalf("missing credits window: %+v", snap.Windows)
	}
	if *credits.Used != 25 || *credits.Limit != 100 || credits.Currency != "USD" {
		t.Fatalf("credits = %+v, want $25 / $100", credits)
	}
}

func TestFetchIdentityFromGetMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "GetMe") {
			w.Write([]byte(`{"email":"alice@example.com","firstName":"A"}`))
			return
		}
		w.Write([]byte(samplePayload))
	}))
	defer srv.Close()
	patchConnectURLs(t, srv.URL)
	patchMeURL(t, srv.URL+"/aiserver.v1.DashboardService/GetMe")

	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	p := newProvider(srv.Client(), time.Now)
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{Account: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Identity == nil || snap.Identity.Account != "alice@example.com" {
		t.Fatalf("identity = %+v, want alice@example.com", snap.Identity)
	}
}

func TestFetchIdentityPrefersExtraEmail(t *testing.T) {
	p, srv := newTestProvider(t, 200, samplePayload)
	defer srv.Close()
	patchConnectURLs(t, srv.URL)
	patchMeURL(t, srv.URL+"/aiserver.v1.DashboardService/GetMe")

	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "work",
		Email:   "tag@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Identity == nil || snap.Identity.Account != "tag@example.com" {
		t.Fatalf("identity = %+v, want extra email", snap.Identity)
	}
}

func TestFetchUnauthorized(t *testing.T) {
	p, srv := newTestProvider(t, 401, `{"error":"unauthorized"}`)
	defer srv.Close()
	patchConnectURLs(t, srv.URL)
	patchMeURL(t, srv.URL+"/aiserver.v1.DashboardService/GetMe")

	t.Setenv("CURSOR_ACCESS_TOKEN", "test-token")
	_, err := p.Fetch(context.Background(), provider.FetchRequest{Account: "default"})
	if err == nil || !strings.Contains(err.Error(), "登录信息被拒") {
		t.Fatalf("expected token-rejected error, got %v", err)
	}
}

func TestFetchServerError(t *testing.T) {
	p, srv := newTestProvider(t, 500, `oops`)
	defer srv.Close()
	patchConnectURLs(t, srv.URL)
	patchMeURL(t, srv.URL+"/aiserver.v1.DashboardService/GetMe")

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
	tok, err := resolveToken(context.Background(), req)
	if err != nil || tok != "from-env" {
		t.Fatalf("env precedence failed: %q, %v", tok, err)
	}

	// account apiKey wins over keychain (keychain not consulted on non-mac)
	t.Setenv("CURSOR_ACCESS_TOKEN", "")
	req = provider.FetchRequest{Creds: provider.Credentials{APIKey: "from-account"}}
	tok, err = resolveToken(context.Background(), req)
	if err != nil || tok != "from-account" {
		t.Fatalf("account apiKey precedence failed: %q, %v", tok, err)
	}

	// AuthToken is what ResolveCredentials writes for cursor's provider-level key.
	req = provider.FetchRequest{Creds: provider.Credentials{AuthToken: "from-auth-token"}}
	tok, err = resolveToken(context.Background(), req)
	if err != nil || tok != "from-auth-token" {
		t.Fatalf("AuthToken precedence failed: %q, %v", tok, err)
	}
}

// writeCursorAuthFile drops an official-format auth.json into the account's
// private HOME (as cursor-agent login would).
func writeCursorAuthFile(t *testing.T, account, accessToken string) {
	t.Helper()
	p, err := auth.CursorAuthFile(account)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(auth.CursorAuth{AccessToken: accessToken})
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeExpiredJWT builds a 3-segment JWT whose exp claim is in the past, so
// resolveToken's expiry check trips without a real token.
func fakeExpiredJWT() string {
	payload, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(-time.Hour).Unix()})
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
}

func TestResolveTokenFromCursorAuth(t *testing.T) {
	t.Setenv("CURSOR_ACCESS_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	writeCursorAuthFile(t, "work", "from-auth-json")

	tok, err := resolveToken(context.Background(), provider.FetchRequest{Account: "work"})
	if err != nil || tok != "from-auth-json" {
		t.Fatalf("cursor auth source failed: %q, %v", tok, err)
	}
}

func TestResolveTokenExpiredCursorAuth(t *testing.T) {
	t.Setenv("CURSOR_ACCESS_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	writeCursorAuthFile(t, "work", fakeExpiredJWT())

	if _, err := resolveToken(context.Background(), provider.FetchRequest{Account: "work"}); err == nil {
		t.Fatal("expected expired-session error")
	}
}

// An account that holds a private auth.json must never fall through to the
// keychain: the keychain tracks the official CLI's own account, and silently
// mixing them is exactly the multi-account bug this isolation prevents. The
// empty-token file therefore yields an error even on machines whose keychain
// holds a live official token.
func TestCursorAuthBlocksKeychainFallback(t *testing.T) {
	t.Setenv("CURSOR_ACCESS_TOKEN", "")
	dir := t.TempDir()
	t.Setenv("LEEWAY_SESSIONS", dir)
	writeCursorAuthFile(t, "work", "")

	if _, err := resolveToken(context.Background(), provider.FetchRequest{Account: "work"}); err == nil {
		t.Fatal("expected error for empty auth file, not keychain fallback")
	}
}

func TestIncludedSpendInDollars(t *testing.T) {
	d := usageData{}
	d.PlanUsage.IncludedSpend = 21921
	d.PlanUsage.Limit = 40000
	snap := d.toModel("default", "", time.Now())
	var spend, limit float64
	found := false
	for _, w := range snap.Windows {
		if w.ID != "included" {
			continue
		}
		found = true
		if w.Used == nil || w.Limit == nil || w.Currency != "USD" {
			t.Fatalf("included window %+v", w)
		}
		spend, limit = *w.Used, *w.Limit
	}
	if !found {
		t.Fatal("missing included window")
	}
	if spend != 219.21 || limit != 400 {
		t.Fatalf("included = %v / %v, want 219.21 / 400", spend, limit)
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

func TestKeychainTokenHonorsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := keychainToken(ctx)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from canceled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("keychainToken hung on canceled context")
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
