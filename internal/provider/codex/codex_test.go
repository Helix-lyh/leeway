package codex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// --- parsing ---------------------------------------------------------------

func TestParseUsageResponseFixture(t *testing.T) {
	resp, err := parseUsageResponse(readFixture(t, "usage.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.planType != "plus" {
		t.Errorf("planType = %q, want plus", resp.planType)
	}
	if resp.rateLimit == nil {
		t.Fatal("rateLimit missing")
	}
	if resp.rateLimit.primaryWindow == nil || resp.rateLimit.secondaryWindow == nil {
		t.Fatal("windows missing")
	}
	p := resp.rateLimit.primaryWindow
	if *p.usedPercent != 32 || *p.resetAt != 1785985200 || *p.limitWindowSeconds != 18000 {
		t.Errorf("primary window = %d/%d/%d", *p.usedPercent, *p.resetAt, *p.limitWindowSeconds)
	}
	if resp.credits == nil || resp.credits.balance == nil || *resp.credits.balance != 34.2 {
		t.Errorf("credits balance = %v", resp.credits)
	}
	if resp.rateLimit.individualLimit == nil || resp.rateLimit.individualLimit.limit == nil ||
		*resp.rateLimit.individualLimit.limit != 120 {
		t.Errorf("individual limit = %+v", resp.rateLimit.individualLimit)
	}
	if len(resp.additionalRateLimits) != 3 {
		t.Fatalf("additionalRateLimits = %d, want 3", len(resp.additionalRateLimits))
	}
	if resp.additionalRateLimits[0].limitName != "GPT-5.3-Codex-Spark" {
		t.Errorf("first extra = %+v", resp.additionalRateLimits[0])
	}
}

func TestParseUsageResponseTopLevelFailure(t *testing.T) {
	if _, err := parseUsageResponse([]byte("not json")); err == nil {
		t.Fatal("want error for invalid JSON")
	}
}

func TestParseUsageResponseLossy(t *testing.T) {
	// Malformed rate_limit (string) must not fail the whole parse; a window
	// with a missing required field must be dropped like Swift's strict decode.
	resp, err := parseUsageResponse(readFixture(t, "usage_malformed.json"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.rateLimit != nil {
		t.Errorf("rateLimit = %+v, want nil", resp.rateLimit)
	}

	resp, err = parseUsageResponse([]byte(`{
		"rate_limit": {
			"primary_window": {"used_percent": 1, "reset_at": 1785985200},
			"secondary_window": {"used_percent": "two", "reset_at": 1785985200, "limit_window_seconds": 18000}
		}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.rateLimit.primaryWindow != nil {
		t.Error("primary window with missing field should be nil")
	}
	if resp.rateLimit.secondaryWindow != nil {
		t.Error("secondary window with string used_percent should be nil")
	}
}

func TestFlexibleScalars(t *testing.T) {
	// limit/used as ints, remaining_percent as snake_case, resets_at as string.
	resp, err := parseUsageResponse([]byte(`{
		"rate_limit": {
			"individualLimit": {"limit": 100, "used": 25, "remaining_percent": 75, "resets_at": "1786233600"}
		}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	il := resp.rateLimit.individualLimit
	if il == nil || il.limit == nil || *il.limit != 100 || il.used == nil || *il.used != 25 ||
		il.remainingPercent == nil || *il.remainingPercent != 75 ||
		il.resetsAt == nil || *il.resetsAt != 1786233600 {
		t.Fatalf("individualLimit = %+v", il)
	}
}

// --- window normalization --------------------------------------------------

func windowFor(minutes int) *model.RateWindow {
	p := 10.0
	now := time.Unix(1785985200, 0)
	return &model.RateWindow{UsedPercent: &p, ResetsAt: &now, WindowMinutes: &minutes}
}

func TestNormalizeWindows(t *testing.T) {
	const none = -1
	cases := []struct {
		name          string
		primaryMin    int
		secondaryMin  int
		wantPrimary   int
		wantSecondary int
	}{
		{"session+weekly keep", 300, 10080, 300, 10080},
		{"session+unknown keep", 300, 1234, 300, 1234},
		{"unknown+weekly keep", 1234, 10080, 1234, 10080},
		{"weekly+session swap", 10080, 300, 300, 10080},
		{"weekly+unknown swap", 10080, 1234, 1234, 10080},
		{"weekly+weekly keep", 10080, 10080, 10080, 10080},
		{"primary session only", 300, none, 300, none},
		{"primary weekly only -> secondary", 10080, none, none, 10080},
		{"primary unknown only", 1234, none, 1234, none},
		{"secondary session only -> primary", none, 300, 300, none},
		{"secondary weekly only", none, 10080, none, 10080},
		{"secondary unknown only -> primary", none, 1234, 1234, none},
		{"none", none, none, none, none},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p, s *model.RateWindow
			if tc.primaryMin != none {
				p = windowFor(tc.primaryMin)
			}
			if tc.secondaryMin != none {
				s = windowFor(tc.secondaryMin)
			}
			gotP, gotS := normalizeWindows(p, s)
			gotPMin, gotSMin := none, none
			if gotP != nil {
				gotPMin = *gotP.WindowMinutes
				if gotP.ID != "primary" || gotP.Title != "Session" {
					t.Errorf("primary lane = %s/%s, want primary/Session", gotP.ID, gotP.Title)
				}
			}
			if gotS != nil {
				gotSMin = *gotS.WindowMinutes
				if gotS.ID != "secondary" || gotS.Title != "Weekly" {
					t.Errorf("secondary lane = %s/%s, want secondary/Weekly", gotS.ID, gotS.Title)
				}
			}
			if gotPMin != tc.wantPrimary || gotSMin != tc.wantSecondary {
				t.Errorf("lanes = %d/%d, want %d/%d", gotPMin, gotSMin, tc.wantPrimary, tc.wantSecondary)
			}
		})
	}
}

func TestWindowFromSnapshotZeroReset(t *testing.T) {
	z := 0
	w := windowFromSnapshot(&windowSnapshot{usedPercent: &z, resetAt: &z, limitWindowSeconds: &z})
	if w == nil || !w.ResetsAt.Equal(time.Unix(0, 0)) || w.WindowMinutes == nil || *w.WindowMinutes != 0 {
		t.Fatalf("zero window = %+v", w)
	}
}

// --- extra rate limits -----------------------------------------------------

func entryWith(primary, secondary *windowSnapshot, limitName, metered string) additionalRateLimit {
	if primary == nil && secondary == nil {
		return additionalRateLimit{limitName: limitName, meteredFeature: metered}
	}
	return additionalRateLimit{
		limitName:      limitName,
		meteredFeature: metered,
		rateLimit:      &rateLimitDetails{primaryWindow: primary, secondaryWindow: secondary},
	}
}

func snap(percent, resetAt, seconds int) *windowSnapshot {
	return &windowSnapshot{usedPercent: &percent, resetAt: &resetAt, limitWindowSeconds: &seconds}
}

func TestExtraRateWindows(t *testing.T) {
	extras := extraRateWindows([]additionalRateLimit{
		entryWith(snap(18, 1785985200, 18000), snap(42, 1786233600, 604800), "GPT-5.3-Codex-Spark", "gpt-5.3-codex-spark"),
		entryWith(snap(7, 1785985200, 18000), nil, "GPT-5.3-Codex", "gpt-5.3-codex"),
		entryWith(snap(3, 1785985200, 18000), nil, "", "gpt-4-mini-codex"),
		entryWith(nil, snap(9, 1785985200, 300), "Limit", ""), // generic falls back to secondary
		entryWith(nil, nil, "", "no-limit"),                   // no windows -> skipped
	})
	if len(extras) != 5 {
		t.Fatalf("extras = %d, want 5: %+v", len(extras), extras)
	}
	got := map[string]model.RateWindow{}
	for _, w := range extras {
		got[w.ID] = w
	}
	if w, ok := got["codex-spark"]; !ok || w.Title != "Codex Spark 5-hour" || *w.UsedPercent != 18 || *w.WindowMinutes != 300 {
		t.Errorf("spark 5h = %+v", w)
	}
	if w, ok := got["codex-spark-weekly"]; !ok || w.Title != "Codex Spark Weekly" || *w.UsedPercent != 42 || *w.WindowMinutes != 10080 {
		t.Errorf("spark weekly = %+v", w)
	}
	if w, ok := got["codex-gpt-5-3-codex"]; !ok || w.Title != "GPT-5.3-Codex" {
		t.Errorf("gpt-5.3 = %+v", w)
	}
	if w, ok := got["codex-gpt-4-mini-codex"]; !ok || w.Title != "gpt-4-mini-codex" {
		t.Errorf("gpt-4-mini = %+v", w)
	}
	if w, ok := got["codex-limit"]; !ok || *w.UsedPercent != 9 {
		t.Errorf("secondary fallback = %+v", w)
	}
	if _, ok := got["codex-no-limit"]; ok {
		t.Error("entry without windows should be skipped")
	}
}

func TestExtraRateWindowsDedupesByID(t *testing.T) {
	extras := extraRateWindows([]additionalRateLimit{
		entryWith(snap(18, 1785985200, 18000), snap(42, 1786233600, 604800), "GPT-5.3-Codex-Spark", "gpt-5.3-codex-spark"),
		entryWith(snap(90, 1785985200, 18000), snap(91, 1786233600, 604800), "GPT-5.3-Codex-Spark2", "gpt-5.3-codex-spark"),
	})
	if len(extras) != 2 {
		t.Fatalf("duplicate spark entries should collapse to 2 windows, got %d", len(extras))
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"gpt-5.3-codex":  "gpt-5-3-codex",
		"Hello World!":   "hello-world",
		"a.b.c":          "a-b-c",
		"---":            "",
		"  spaced  out ": "spaced-out",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- URL resolution --------------------------------------------------------

func TestResolveUsageURL(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		got := resolveUsageURL(t.TempDir())
		want := "https://chatgpt.com/backend-api/wham/usage"
		if got != want {
			t.Errorf("resolveUsageURL = %q, want %q", got, want)
		}
	})
	writeConfig := func(t *testing.T, contents string) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	cases := []struct {
		name, config, want string
	}{
		{"default base", `chatgpt_base_url = "https://chatgpt.com/backend-api/"`, "https://chatgpt.com/backend-api/wham/usage"},
		{"host only", `chatgpt_base_url = "https://chatgpt.com"`, "https://chatgpt.com/backend-api/wham/usage"},
		{"chat.openai.com", `chatgpt_base_url = 'https://chat.openai.com'`, "https://chat.openai.com/backend-api/wham/usage"},
		{"custom base", `chatgpt_base_url = "https://api.example.com"`, "https://api.example.com/api/codex/usage"},
		{"custom with backend-api", `chatgpt_base_url = "https://example.com/backend-api"`, "https://example.com/backend-api/wham/usage"},
		{"commented out", `# chatgpt_base_url = "https://x.example"`, "https://chatgpt.com/backend-api/wham/usage"},
		{"different key", `other_url = "https://x.example"`, "https://chatgpt.com/backend-api/wham/usage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveUsageURL(writeConfig(t, tc.config)); got != tc.want {
				t.Errorf("resolveUsageURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseChatGPTBaseURL(t *testing.T) {
	if v, ok := parseChatGPTBaseURL("chatgpt_base_url = \"https://chatgpt.com/backend-api/\""); !ok || v != "https://chatgpt.com/backend-api/" {
		t.Errorf("quoted parse = %q, %v", v, ok)
	}
	// An empty value parses to "" and the caller falls back to the default.
	if v, ok := parseChatGPTBaseURL("chatgpt_base_url="); !ok || v != "" {
		t.Errorf("empty value parse = %q, %v", v, ok)
	}
	if _, ok := parseChatGPTBaseURL("other_key = \"x\""); ok {
		t.Error("other keys should not parse")
	}
}

// --- HTTP fetch ------------------------------------------------------------

func clientFor(t *testing.T, usageBody []byte, usageStatus int) *apiClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			w.WriteHeader(usageStatus)
			if usageBody != nil {
				_, _ = w.Write(usageBody)
			}
		case "/backend-api/accounts":
			_, _ = w.Write(readFixture(t, "accounts.json"))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	c := newAPIClient()
	c.usageURL = srv.URL + "/backend-api/wham/usage"
	c.accountsURL = srv.URL + "/backend-api/accounts"
	return c
}

func TestFetchUsageStatusClassification(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		fixture string
		status  int
		wantErr string
	}{
		{"ok", "usage_partial.json", 200, ""},
		{"unauthorized", "usage_partial.json", 401, "登录已过期 — 请跑 leeway login codex"},
		{"forbidden", "usage_partial.json", 403, "登录已过期 — 请跑 leeway login codex"},
		{"server error with body", "usage_partial.json", 500, "Codex 接口出错 500：boom"},
		{"server error empty", "", 502, "Codex 接口出错 502。"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body []byte
			if tc.fixture != "" {
				body = readFixture(t, tc.fixture)
			}
			if tc.name == "server error with body" {
				body = []byte("boom")
			}
			c := clientFor(t, body, tc.status)
			_, err := c.fetchUsage(ctx, "tok", "acct-personal")
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("fetchUsage: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("fetchUsage err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestFetchUsageInvalidJSON(t *testing.T) {
	c := clientFor(t, []byte("<html>not json</html>"), 200)
	_, err := c.fetchUsage(context.Background(), "tok", "")
	if err == nil || !strings.Contains(err.Error(), "Codex 用量接口返回的内容不对。") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchUsageSendsHeaders(t *testing.T) {
	var gotAuth, gotUA, gotAccount, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAccount = r.Header.Get("ChatGPT-Account-Id")
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write(readFixture(t, "usage_partial.json"))
	}))
	t.Cleanup(srv.Close)
	c := newAPIClient()
	c.usageURL = srv.URL + "/wham/usage"
	if _, err := c.fetchUsage(context.Background(), "secret-token", "acct-work"); err != nil {
		t.Fatalf("fetchUsage: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotUA != "leeway" {
		t.Errorf("User-Agent = %q, want leeway", gotUA)
	}
	if gotAccount != "acct-work" {
		t.Errorf("ChatGPT-Account-Id = %q", gotAccount)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestFetchAccountsSendsHeaders(t *testing.T) {
	var gotUA, gotAccount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccount = r.Header.Get("ChatGPT-Account-Id")
		_, _ = w.Write(readFixture(t, "accounts.json"))
	}))
	t.Cleanup(srv.Close)
	c := newAPIClient()
	c.accountsURL = srv.URL + "/accounts"
	if _, err := c.fetchAccounts(context.Background(), "secret-token", "acct-work"); err != nil {
		t.Fatalf("fetchAccounts: %v", err)
	}
	if gotUA != "codex-cli" {
		t.Errorf("User-Agent = %q, want codex-cli", gotUA)
	}
	if gotAccount != "acct-work" {
		t.Errorf("ChatGPT-Account-Id = %q", gotAccount)
	}
}

// --- identity --------------------------------------------------------------

func TestResolveIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "accounts.json"))
	}))
	t.Cleanup(srv.Close)
	c := newAPIClient()
	c.accountsURL = srv.URL + "/accounts"
	ctx := context.Background()

	if id := c.resolveIdentity(ctx, "tok", "ACCT-Personal"); id == nil || id.Account != "acct-personal" {
		t.Errorf("match identity = %+v", id)
	}
	if id := c.resolveIdentity(ctx, "tok", "acct-other"); id == nil || id.Account != "acct-other" {
		t.Errorf("no-match identity = %+v", id)
	}
	if id := c.resolveIdentity(ctx, "tok", "  "); id != nil {
		t.Errorf("empty account id identity = %+v, want nil", id)
	}
}

func TestResolveIdentityBestEffortOnAccountsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", 500)
	}))
	t.Cleanup(srv.Close)
	c := newAPIClient()
	c.accountsURL = srv.URL + "/accounts"
	if id := c.resolveIdentity(context.Background(), "tok", "acct-fallback"); id == nil || id.Account != "acct-fallback" {
		t.Errorf("fallback identity = %+v", id)
	}
}

// --- end-to-end Fetch ------------------------------------------------------

func TestFetchEndToEnd(t *testing.T) {
	var usageAuth, usageUA, usageAccount, accountsAuth, accountsUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			usageAuth = r.Header.Get("Authorization")
			usageUA = r.Header.Get("User-Agent")
			usageAccount = r.Header.Get("ChatGPT-Account-Id")
			_, _ = w.Write(readFixture(t, "usage.json"))
		case "/backend-api/accounts":
			accountsAuth = r.Header.Get("Authorization")
			accountsUA = r.Header.Get("User-Agent")
			_, _ = w.Write(readFixture(t, "accounts.json"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := newAPIClient()
	c.usageURL = srv.URL + "/backend-api/wham/usage"
	c.accountsURL = srv.URL + "/backend-api/accounts"

	snap, err := fetchCodex(context.Background(), c, "tok123", "ACCT-Personal")
	if err != nil {
		t.Fatalf("fetchCodex: %v", err)
	}
	if usageAuth != "Bearer tok123" || accountsAuth != "Bearer tok123" {
		t.Errorf("auth headers: usage=%q accounts=%q", usageAuth, accountsAuth)
	}
	if usageUA != "leeway" || accountsUA != "codex-cli" {
		t.Errorf("user agents: usage=%q accounts=%q", usageUA, accountsUA)
	}
	if usageAccount != "ACCT-Personal" {
		t.Errorf("usage ChatGPT-Account-Id = %q", usageAccount)
	}
	if snap.Identity == nil || snap.Identity.Account != "acct-personal" {
		t.Errorf("identity = %+v", snap.Identity)
	}
	if snap.Cost == nil || snap.Cost.Period != "Credits" || snap.Cost.Currency != "USD" || snap.Cost.Amount == nil || *snap.Cost.Amount != 34.2 {
		t.Errorf("cost = %+v", snap.Cost)
	}
	if len(snap.Windows) != 6 {
		t.Fatalf("windows = %d, want 6", len(snap.Windows))
	}
	want := []struct {
		id, title string
		percent   float64
		minutes   int
	}{
		{"primary", "Session", 32, 300},
		{"secondary", "Weekly", 65, 10080},
		{"codex-spark", "Codex Spark 5-hour", 18, 300},
		{"codex-spark-weekly", "Codex Spark Weekly", 42, 10080},
		{"codex-gpt-5-3-codex", "GPT-5.3-Codex", 7, 300},
		{"codex-gpt-4-mini-codex", "gpt-4-mini-codex", 3, 300},
	}
	for i, w := range want {
		got := snap.Windows[i]
		if got.ID != w.id || got.Title != w.title || got.UsedPercent == nil || *got.UsedPercent != w.percent ||
			got.WindowMinutes == nil || *got.WindowMinutes != w.minutes {
			t.Errorf("window[%d] = %+v, want %+v", i, got, w)
		}
	}
	if !snap.Windows[0].ResetsAt.Equal(time.Unix(1785985200, 0)) {
		t.Errorf("primary resetsAt = %v", snap.Windows[0].ResetsAt)
	}
	if time.Since(snap.UpdatedAt) > time.Minute {
		t.Errorf("UpdatedAt too old: %v", snap.UpdatedAt)
	}
}

func TestFetchSwappedWindows(t *testing.T) {
	c := clientFor(t, readFixture(t, "usage_swapped.json"), 200)
	snap, err := fetchCodex(context.Background(), c, "tok", "")
	if err != nil {
		t.Fatalf("fetchCodex: %v", err)
	}
	if len(snap.Windows) != 2 {
		t.Fatalf("windows = %d", len(snap.Windows))
	}
	if snap.Windows[0].ID != "primary" || *snap.Windows[0].WindowMinutes != 300 || *snap.Windows[0].UsedPercent != 20 {
		t.Errorf("primary = %+v", snap.Windows[0])
	}
	if snap.Windows[1].ID != "secondary" || *snap.Windows[1].WindowMinutes != 10080 || *snap.Windows[1].UsedPercent != 55 {
		t.Errorf("secondary = %+v", snap.Windows[1])
	}
}

func TestFetchWeeklyOnlyBecomesSecondary(t *testing.T) {
	c := clientFor(t, readFixture(t, "usage_weekly_only.json"), 200)
	snap, err := fetchCodex(context.Background(), c, "tok", "")
	if err != nil {
		t.Fatalf("fetchCodex: %v", err)
	}
	if len(snap.Windows) != 1 || snap.Windows[0].ID != "secondary" || *snap.Windows[0].WindowMinutes != 10080 {
		t.Fatalf("windows = %+v", snap.Windows)
	}
}

func TestFetchNoRateLimits(t *testing.T) {
	for _, fixture := range []string{"usage_bare.json", "usage_malformed.json"} {
		t.Run(fixture, func(t *testing.T) {
			c := clientFor(t, readFixture(t, fixture), 200)
			_, err := fetchCodex(context.Background(), c, "tok", "")
			if err == nil || !strings.Contains(err.Error(), "还没有额度记录") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestFetchCreditsOnlyPartialSnapshot(t *testing.T) {
	c := clientFor(t, readFixture(t, "usage_credits_only.json"), 200)
	snap, err := fetchCodex(context.Background(), c, "tok", "")
	if err != nil {
		t.Fatalf("fetchCodex: %v", err)
	}
	if len(snap.Windows) != 0 {
		t.Errorf("windows = %+v, want none", snap.Windows)
	}
	if snap.Cost == nil || snap.Cost.Amount == nil || *snap.Cost.Amount != 12.5 {
		t.Errorf("cost = %+v", snap.Cost)
	}
}

func TestProviderFetchMissingSession(t *testing.T) {
	p := codexProvider{}
	_, err := p.Fetch(context.Background(), provider.FetchRequest{})
	if err == nil || !strings.Contains(err.Error(), "leeway login codex") {
		t.Fatalf("err = %v, want leeway login codex hint", err)
	}
}

func TestProviderContract(t *testing.T) {
	p := codexProvider{}
	if p.ID() != "codex" || p.DisplayName() != "Codex" {
		t.Errorf("id/name = %q/%q", p.ID(), p.DisplayName())
	}
	spec := p.AuthSpec()
	if !spec.UsesCodexCLI {
		t.Error("AuthSpec must declare UsesCodexCLI")
	}
	windows := p.DeclaredWindows()
	if len(windows) != 2 {
		t.Fatalf("DeclaredWindows = %d, want 2", len(windows))
	}
	if windows[0].ID != "primary" || windows[0].Title != "Session" ||
		windows[0].WindowMinutes == nil || *windows[0].WindowMinutes != 300 ||
		windows[0].Kind != provider.PercentWindow {
		t.Errorf("primary spec = %+v", windows[0])
	}
	if windows[1].ID != "secondary" || windows[1].Title != "Weekly" ||
		windows[1].WindowMinutes == nil || *windows[1].WindowMinutes != 10080 {
		t.Errorf("secondary spec = %+v", windows[1])
	}
}
