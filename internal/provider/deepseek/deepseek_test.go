package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

var fixedNow = time.Date(2026, time.August, 6, 10, 30, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func assertKind(t *testing.T, err error, want ErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error of kind %v, got nil", want)
	}
	var de *Error
	if !errors.As(err, &de) {
		t.Fatalf("expected *deepseek.Error, got %T: %v", err, err)
	}
	if de.Kind != want {
		t.Fatalf("error kind = %v (%s), want %v", de.Kind, de.Msg, want)
	}
}

func assertFloat(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseBalance(t *testing.T) {
	b, err := parseBalance(fixture(t, "balance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !b.isAvailable {
		t.Error("isAvailable = false, want true")
	}
	// CNY row is funded, USD row is empty: CNY must win.
	if b.currency != "CNY" {
		t.Errorf("currency = %q, want CNY", b.currency)
	}
	assertFloat(t, b.totalBalance, 12.34)
	assertFloat(t, b.grantedBalance, 0)
	assertFloat(t, b.toppedUpBalance, 12.34)
}

func TestParseBalancePlatform(t *testing.T) {
	b, err := parseBalance(fixture(t, "balance_platform.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !b.isAvailable {
		t.Error("isAvailable = false, want true")
	}
	if b.currency != "USD" {
		t.Errorf("currency = %q, want USD", b.currency)
	}
	// normal_wallets 5.5 + 1.25 (string-typed), bonus_wallets 2.0.
	assertFloat(t, b.toppedUpBalance, 6.75)
	assertFloat(t, b.grantedBalance, 2.0)
	assertFloat(t, b.totalBalance, 8.75)
}

func TestParseBalanceAuthError(t *testing.T) {
	_, err := parseBalance(fixture(t, "balance_auth_error.json"))
	assertKind(t, err, KindInvalidPlatformToken)
}

func TestParseBalanceEnvelopeError(t *testing.T) {
	_, err := parseBalance([]byte(`{"code": 42, "data": {"biz_code": 0}}`))
	assertKind(t, err, KindAPI)
	if !strings.Contains(err.Error(), "用户概览返回 code 42") {
		t.Errorf("message = %q, want it to mention the code", err.Error())
	}
}

func TestParseBalanceNonNumeric(t *testing.T) {
	raw := `{"is_available": true, "balance_infos": [
		{"currency": "CNY", "total_balance": "abc", "granted_balance": "0", "topped_up_balance": "0"}
	]}`
	_, err := parseBalance([]byte(raw))
	assertKind(t, err, KindParse)
	if !strings.Contains(err.Error(), "返回里的余额值不是数字") {
		t.Errorf("message = %q, want the Swift failure message", err.Error())
	}
}

func TestParseBalanceEmpty(t *testing.T) {
	b, err := parseBalance([]byte(`{"is_available": false, "balance_infos": []}`))
	if err != nil {
		t.Fatal(err)
	}
	if b.currency != "USD" || b.totalBalance != 0 || b.isAvailable {
		t.Errorf("empty balance parsed as %+v", b)
	}
}

func TestParseUsageSummary(t *testing.T) {
	u, err := parseUsageSummary(fixture(t, "usage_amount.json"), fixture(t, "usage_cost.json"), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if u.currency != "CNY" {
		t.Errorf("currency = %q, want CNY", u.currency)
	}
	// No daily rows for 2026-08-06 (today): zeros.
	if u.todayTokens != 0 || u.todayCost != nil || u.currentMonthRequestCount != 9 {
		t.Errorf("today = %d tokens/%v cost, month requests = %d, want 0/nil/9",
			u.todayTokens, u.todayCost, u.currentMonthRequestCount)
	}
	if u.currentMonthTokens != 88000 {
		t.Errorf("month tokens = %d, want 88000", u.currentMonthTokens)
	}
	if u.currentMonthCost == nil || math.Abs(*u.currentMonthCost-0.266) > 1e-9 {
		t.Errorf("month cost = %v, want 0.266", u.currentMonthCost)
	}
	if u.topModel != "deepseek-chat" {
		t.Errorf("topModel = %q, want deepseek-chat", u.topModel)
	}
	wantCats := []struct {
		category string
		tokens   int64
		cost     float64
		hasCost  bool
	}{
		{"PROMPT_CACHE_HIT_TOKEN", 120000, 0.024, true},
		{"PROMPT_CACHE_MISS_TOKEN", 50000, 0.10, true},
		{"RESPONSE_TOKEN", 23000, 0.276, true},
	}
	if len(u.categoryBreakdown) != len(wantCats) {
		t.Fatalf("breakdown length = %d, want %d", len(u.categoryBreakdown), len(wantCats))
	}
	for i, want := range wantCats {
		got := u.categoryBreakdown[i]
		if got.category != want.category || got.tokens != want.tokens {
			t.Errorf("breakdown[%d] = %s:%d, want %s:%d",
				i, got.category, got.tokens, want.category, want.tokens)
		}
		if got.cost == nil && want.hasCost {
			t.Errorf("breakdown[%d] cost = nil, want %v", i, want.cost)
		} else if got.cost != nil && math.Abs(*got.cost-want.cost) > 1e-9 {
			t.Errorf("breakdown[%d] cost = %v, want %v", i, *got.cost, want.cost)
		}
	}
	if len(u.daily) != 2 {
		t.Fatalf("daily length = %d, want 2", len(u.daily))
	}
	if u.daily[0].date != "2026-08-01" || u.daily[0].totalTokens != 60000 ||
		u.daily[0].requestCount != 5 || u.daily[0].cost == nil || math.Abs(*u.daily[0].cost-0.13) > 1e-9 {
		t.Errorf("daily[0] = %+v", u.daily[0])
	}
	if u.daily[1].date != "2026-08-05" || u.daily[1].totalTokens != 28000 ||
		u.daily[1].requestCount != 4 || u.daily[1].cost == nil || math.Abs(*u.daily[1].cost-0.136) > 1e-9 {
		t.Errorf("daily[1] = %+v", u.daily[1])
	}
}

// TestParseUsageSummaryTopModelTie mirrors Swift's max(by:) tie-break: the
// lexicographically smaller model name wins among equal token totals.
func TestParseUsageSummaryTopModelTie(t *testing.T) {
	amount := `{"code": 0, "data": {"biz_code": 0, "biz_data": {"total": [
		{"model": "deepseek-reasoner", "usage": [{"type": "RESPONSE_TOKEN", "amount": "100"}]},
		{"model": "deepseek-chat", "usage": [{"type": "RESPONSE_TOKEN", "amount": "100"}]}
	], "days": []}}}`
	cost := `{"code": 0, "data": {"biz_code": 0, "biz_data": [{"currency": "USD"}]}}`
	u, err := parseUsageSummary([]byte(amount), []byte(cost), fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if u.topModel != "deepseek-chat" {
		t.Errorf("topModel = %q, want deepseek-chat", u.topModel)
	}
}

func TestParseUsageSummaryAuthCode(t *testing.T) {
	amount := `{"code": 40002, "data": {"biz_code": 40002, "biz_data": {"total": [], "days": []}}}`
	cost := `{"code": 0, "data": {"biz_code": 0, "biz_data": [{"currency": "USD"}]}}`
	_, err := parseUsageSummary([]byte(amount), []byte(cost), fixedNow)
	assertKind(t, err, KindInvalidPlatformToken)
}

func TestParseUsageSummaryBizCodeError(t *testing.T) {
	amount := `{"code": 0, "data": {"biz_code": 12345, "biz_data": null}}`
	cost := `{"code": 0, "data": {"biz_code": 0, "biz_data": [{"currency": "USD"}]}}`
	_, err := parseUsageSummary([]byte(amount), []byte(cost), fixedNow)
	assertKind(t, err, KindAPI)
	if !strings.Contains(err.Error(), "用量返回 biz_code 12345") {
		t.Errorf("message = %q, want it to mention the biz_code", err.Error())
	}
}

func TestParseUsageSummaryMissingBizData(t *testing.T) {
	amount := `{"code": 0, "data": {"biz_code": 0, "biz_data": null}}`
	cost := `{"code": 0, "data": {"biz_code": 0, "biz_data": [{"currency": "USD"}]}}`
	_, err := parseUsageSummary([]byte(amount), []byte(cost), fixedNow)
	assertKind(t, err, KindParse)
	if !strings.Contains(err.Error(), "用量里没有 biz_data") {
		t.Errorf("message = %q", err.Error())
	}
}

// TestFetch exercises the full pipeline against a local test server.
func TestFetch(t *testing.T) {
	var gotBalanceAuth, gotAmountAuth, gotCostAuth string
	var gotAmountQuery, gotCostQuery string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v0/users/get_user_summary", func(w http.ResponseWriter, r *http.Request) {
		gotBalanceAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture(t, "balance.json"))
	})
	mux.HandleFunc("/api/v0/usage/amount", func(w http.ResponseWriter, r *http.Request) {
		gotAmountAuth = r.Header.Get("Authorization")
		gotAmountQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture(t, "usage_amount.json"))
	})
	mux.HandleFunc("/api/v0/usage/cost", func(w http.ResponseWriter, r *http.Request) {
		gotCostAuth = r.Header.Get("Authorization")
		gotCostQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture(t, "usage_cost.json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("DEEPSEEK_PLATFORM_TOKEN", "plat-token")
	p := newProvider(srv.Client(), func() time.Time { return fixedNow })
	p.balanceURL = srv.URL + "/api/v0/users/get_user_summary"
	p.usageAmountURL = srv.URL + "/api/v0/usage/amount"
	p.usageCostURL = srv.URL + "/api/v0/usage/cost"

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "test-api-key"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotBalanceAuth != "Bearer test-api-key" {
		t.Errorf("balance Authorization = %q", gotBalanceAuth)
	}
	if gotAmountAuth != "Bearer plat-token" || gotCostAuth != "Bearer plat-token" {
		t.Errorf("usage Authorization = %q / %q", gotAmountAuth, gotCostAuth)
	}
	if gotAmountQuery != "month=8&year=2026" || gotCostQuery != "month=8&year=2026" {
		t.Errorf("usage query = %q / %q, want month=8&year=2026", gotAmountQuery, gotCostQuery)
	}

	if snap.Identity == nil || snap.Identity.Account != "default" {
		t.Errorf("identity = %+v", snap.Identity)
	}
	if snap.Cost == nil || snap.Cost.Period != "Balance" || snap.Cost.Currency != "CNY" ||
		snap.Cost.Amount == nil || math.Abs(*snap.Cost.Amount-12.34) > 1e-9 {
		t.Errorf("cost = %+v", snap.Cost)
	}

	wantWindows := []struct {
		id   string
		used float64
	}{
		{"primary", 12.34},
		{"day-2026-08-01", 60000},
		{"day-2026-08-05", 28000},
	}
	if len(snap.Windows) != len(wantWindows) {
		t.Fatalf("windows = %d, want %d", len(snap.Windows), len(wantWindows))
	}
	for i, want := range wantWindows {
		w := snap.Windows[i]
		if w.ID != want.id {
			t.Errorf("windows[%d].id = %q, want %q", i, w.ID, want.id)
		}
		if w.Used == nil || math.Abs(*w.Used-want.used) > 1e-9 {
			t.Errorf("windows[%d].used = %v, want %v", i, w.Used, want.used)
		}
		if w.Currency != "CNY" {
			t.Errorf("windows[%d].currency = %q, want CNY", i, w.Currency)
		}
	}
	if snap.Windows[0].Title != "Balance" {
		t.Errorf("primary title = %q, want Balance", snap.Windows[0].Title)
	}
}

// TestFetchBalanceOnly: without a platform token the API-key path still
// works and no usage endpoints are hit.
func TestFetchBalanceOnly(t *testing.T) {
	t.Setenv("DEEPSEEK_PLATFORM_TOKEN", "")
	t.Setenv("DEEPSEEK_USER_TOKEN", "")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v0/users/get_user_summary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture(t, "balance.json"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to %s", r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newProvider(srv.Client(), func() time.Time { return fixedNow })
	p.balanceURL = srv.URL + "/api/v0/users/get_user_summary"

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Windows) != 1 || snap.Windows[0].ID != "primary" {
		t.Errorf("windows = %+v, want only the primary balance window", snap.Windows)
	}
	if snap.Cost == nil || snap.Cost.Period != "Balance" {
		t.Errorf("cost = %+v", snap.Cost)
	}
}

func TestFetchMissingAPIKey(t *testing.T) {
	p := newProvider(nil, func() time.Time { return fixedNow })
	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{},
	})
	assertKind(t, err, KindMissingCredentials)
}

func TestFetchBalance401(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newProvider(srv.Client(), func() time.Time { return fixedNow })
	p.balanceURL = srv.URL + "/balance"

	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "bad"},
	})
	assertKind(t, err, KindInvalidAPIKey)
	if !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "API 密钥不合法") {
		t.Errorf("message = %q, want status and classification", err.Error())
	}
}

func TestFetchBalance403(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newProvider(srv.Client(), func() time.Time { return fixedNow })
	p.balanceURL = srv.URL + "/balance"

	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "bad"},
	})
	assertKind(t, err, KindAPI)
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("message = %q, want the status", err.Error())
	}
}

// TestFetchUsageStatus mirrors the Swift amount/cost classification: 401/403
// on the platform endpoints means the session is missing or expired.
func TestFetchUsageStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newProvider(srv.Client(), func() time.Time { return fixedNow })
	p.usageAmountURL = srv.URL + "/amount"

	_, err := p.fetchUsageSummary(context.Background(), "expired", 8, 2026, fixedNow)
	assertKind(t, err, KindInvalidPlatformToken)
}

func TestFetchBadJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{not json`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newProvider(srv.Client(), func() time.Time { return fixedNow })
	p.balanceURL = srv.URL + "/balance"

	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "key"},
	})
	assertKind(t, err, KindParse)
}

// TestRegistration checks the contract surface exposed to the orchestrator.
func TestRegistration(t *testing.T) {
	p, ok := provider.Get("deepseek")
	if !ok {
		t.Fatal("deepseek provider not registered")
	}
	if p.ID() != "deepseek" || p.DisplayName() != "DeepSeek" {
		t.Errorf("id/display = %q/%q", p.ID(), p.DisplayName())
	}
	spec := p.AuthSpec()
	if len(spec.EnvAPIKey) != 2 || spec.EnvAPIKey[0] != "DEEPSEEK_API_KEY" || spec.EnvAPIKey[1] != "DEEPSEEK_KEY" {
		t.Errorf("AuthSpec = %+v", spec)
	}
	if spec.UsesCookies || spec.SupportsAccounts || spec.UsesCodexCLI {
		t.Errorf("AuthSpec unexpectedly enables cookies/accounts/CLI: %+v", spec)
	}
	windows := p.DeclaredWindows()
	if len(windows) != 1 || windows[0].ID != "primary" || windows[0].Title != "Balance" ||
		windows[0].Kind != provider.BalanceWindow {
		t.Errorf("DeclaredWindows = %+v", windows)
	}
}

// TestSnapshotJSON round-trips the snapshot shape through the model's JSON
// contract to make sure nothing leaks in weird forms.
func TestSnapshotJSON(t *testing.T) {
	used := 12.34
	snap := &model.Snapshot{
		UpdatedAt: fixedNow,
		Windows:   []model.RateWindow{{ID: "primary", Title: "Balance", Used: &used, Currency: "CNY"}},
		Cost:      &model.Cost{Period: "Balance", Currency: "CNY", Amount: &used},
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":"primary"`, `"period":"Balance"`, `"currency":"CNY"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("snapshot JSON missing %s: %s", want, raw)
		}
	}
}
