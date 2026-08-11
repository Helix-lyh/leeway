package kimi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

var fixedNow = time.Date(2026, 8, 6, 10, 30, 0, 0, time.UTC)

const (
	usagePath        = "/apiv2/kimi.gateway.billing.v1.BillingService/GetUsages"
	subscriptionPath = "/apiv2/kimi.gateway.membership.v2.MembershipService/GetSubscriptionStats"
	codeAPIUsagePath = "/coding/v1/usages"
)

// stubTransport answers requests in-process; no network is ever touched.
type stubTransport struct {
	mu       sync.Mutex
	requests []*http.Request
	handler  func(*http.Request) (*http.Response, error)
}

func (t *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	clone := req.Clone(req.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	t.requests = append(t.requests, clone)
	t.mu.Unlock()
	return t.handler(req)
}

func (t *stubTransport) lastRequest() *http.Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.requests) == 0 {
		return nil
	}
	return t.requests[len(t.requests)-1]
}

func (t *stubTransport) requestsLen() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.requests)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}

// defaultHandler serves the real Kimi endpoints with fixture payloads.
func defaultHandler(t *testing.T) func(*http.Request) (*http.Response, error) {
	t.Helper()
	return func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case usagePath:
			return jsonResponse(http.StatusOK, fixture(t, "usages_web.json")), nil
		case subscriptionPath:
			return jsonResponse(http.StatusOK, fixture(t, "subscription_stats.json")), nil
		case codeAPIUsagePath:
			return jsonResponse(http.StatusOK, fixture(t, "usages_api.json")), nil
		default:
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
	}
}

func newTestProvider(t *testing.T, handler func(*http.Request) (*http.Response, error)) (kimiProvider, *stubTransport) {
	// Isolation is unconditional: the local-session fallback must never read a
	// developer's real ~/.kimi-code/credentials/kimi-code.json (which would
	// switch Fetch onto the Code API path with real tokens). Tests that need
	// a fixture credential use newTestProviderHome with their own directory.
	return newTestProviderHome(t, handler, t.TempDir())
}

// newTestProviderHome builds a provider isolated onto the given KIMI_CODE_HOME
// directory (tests write their fixture session into it before Fetch).
func newTestProviderHome(t *testing.T, handler func(*http.Request) (*http.Response, error), home string) (kimiProvider, *stubTransport) {
	t.Setenv("KIMI_CODE_HOME", home)
	tr := &stubTransport{handler: handler}
	p := newProvider(&http.Client{Transport: tr})
	p.now = func() time.Time { return fixedNow }
	return p, tr
}

func testJWT() string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"device_id":"dev-123","ssid":"sess-456","sub":"traffic-789"}`))
	return header + "." + payload + ".sig"
}

func findWindow(t *testing.T, snap *model.Snapshot, id string) model.RateWindow {
	t.Helper()
	for _, w := range snap.Windows {
		if w.ID == id {
			return w
		}
	}
	t.Fatalf("window %q not found in %+v", id, snap.Windows)
	return model.RateWindow{}
}

func wantPercent(t *testing.T, w model.RateWindow, want float64) {
	t.Helper()
	if w.UsedPercent == nil {
		t.Fatalf("window %q: UsedPercent is nil", w.ID)
	}
	if *w.UsedPercent != want {
		t.Fatalf("window %q: UsedPercent = %v, want %v", w.ID, *w.UsedPercent, want)
	}
}

func wantMinutes(t *testing.T, w model.RateWindow, want int) {
	t.Helper()
	if w.WindowMinutes == nil {
		t.Fatalf("window %q: WindowMinutes is nil", w.ID)
	}
	if *w.WindowMinutes != want {
		t.Fatalf("window %q: WindowMinutes = %v, want %v", w.ID, *w.WindowMinutes, want)
	}
}

func wantResetsAt(t *testing.T, w model.RateWindow, want string) {
	t.Helper()
	if w.ResetsAt == nil {
		t.Fatalf("window %q: ResetsAt is nil", w.ID)
	}
	wantT, err := time.Parse(time.RFC3339, want)
	if err != nil {
		t.Fatalf("bad want time: %v", err)
	}
	if !w.ResetsAt.Equal(wantT) {
		t.Fatalf("window %q: ResetsAt = %v, want %v", w.ID, w.ResetsAt, wantT)
	}
}

func TestFetchWebEndToEnd(t *testing.T) {
	p, tr := newTestProvider(t, defaultHandler(t))
	jwt := testJWT()

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{AuthToken: jwt},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !snap.UpdatedAt.Equal(fixedNow) {
		t.Fatalf("UpdatedAt = %v, want %v", snap.UpdatedAt, fixedNow)
	}
	if snap.Identity == nil || snap.Identity.Account != "default" {
		t.Fatalf("Identity = %+v, want account default", snap.Identity)
	}
	if len(snap.Windows) != 4 {
		t.Fatalf("got %d windows, want 4: %+v", len(snap.Windows), snap.Windows)
	}

	primary := findWindow(t, snap, "primary")
	wantPercent(t, primary, 62)
	wantMinutes(t, primary, weeklyWindowMinutes)
	wantResetsAt(t, primary, "2026-08-13T00:00:00Z")
	if primary.Title != "Weekly" {
		t.Fatalf("primary Title = %q, want Weekly", primary.Title)
	}

	secondary := findWindow(t, snap, "secondary")
	wantPercent(t, secondary, 41)
	wantMinutes(t, secondary, 300) // 5h window from API
	wantResetsAt(t, secondary, "2026-08-06T15:30:00Z")
	if secondary.Title != "5-hour" {
		t.Fatalf("secondary Title = %q, want 5-hour", secondary.Title)
	}

	monthly := findWindow(t, snap, "kimi-monthly")
	wantPercent(t, monthly, 40)
	wantMinutes(t, monthly, monthlySentinel)
	wantResetsAt(t, monthly, "2026-09-05T00:00:00Z")
	if monthly.Title != "Monthly" {
		t.Fatalf("monthly Title = %q, want Monthly", monthly.Title)
	}

	code7d := findWindow(t, snap, "kimi-code-7d")
	wantPercent(t, code7d, 50)
	wantMinutes(t, code7d, weeklyWindowMinutes)
	wantResetsAt(t, code7d, "2026-08-12T00:00:00Z")
	if code7d.Title != "Code 7-day" {
		t.Fatalf("code7d Title = %q, want Code 7-day", code7d.Title)
	}

	// Request assertions: the usage and subscription calls run concurrently,
	// so order is not deterministic — both must be present with the web
	// headers.
	if got := tr.requestsLen(); got != 2 {
		t.Fatalf("made %d requests, want 2", got)
	}
	paths := map[string]*http.Request{}
	for _, r := range tr.requests {
		paths[r.URL.Path] = r
	}
	if _, ok := paths[usagePath]; !ok {
		t.Fatalf("missing usage request; paths = %v", paths)
	}
	req, ok := paths[subscriptionPath]
	if !ok {
		t.Fatalf("missing subscription request; paths = %v", paths)
	}
	_ = req

	// Usage request headers mirror webRequest.
	usageReq := paths[usagePath]
	if usageReq.Method != http.MethodPost {
		t.Fatalf("usage method = %s, want POST", usageReq.Method)
	}
	wantHeaders := map[string]string{
		"Content-Type":             "application/json",
		"Authorization":            "Bearer " + jwt,
		"Cookie":                   "kimi-auth=" + jwt,
		"Origin":                   "https://www.kimi.com",
		"Referer":                  "https://www.kimi.com/code/console",
		"Accept":                   "*/*",
		"Accept-Language":          "en-US,en;q=0.9",
		"Connect-Protocol-Version": "1",
		"X-Language":               "en-US",
		"X-Msh-Platform":           "web",
		"X-Msh-Device-Id":          "dev-123",
		"X-Msh-Session-Id":         "sess-456",
		"X-Traffic-Id":             "traffic-789",
	}
	for k, want := range wantHeaders {
		if got := usageReq.Header.Get(k); got != want {
			t.Fatalf("header %s = %q, want %q", k, got, want)
		}
	}
	if got := usageReq.Header.Get("User-Agent"); !strings.HasPrefix(got, "Mozilla/5.0") {
		t.Fatalf("User-Agent = %q, want Mozilla/5.0 prefix", got)
	}
	if got := usageReq.Header.Get("R-Timezone"); got == "" {
		t.Fatal("r-timezone header missing")
	}
	body, _ := io.ReadAll(usageReq.Body)
	if string(body) != `{"scope":["FEATURE_CODING"]}` {
		t.Fatalf("usage body = %s, want {\"scope\":[\"FEATURE_CODING\"]}", body)
	}
	// Look the subscription request up by path (never by index): the two calls
	// run concurrently, so which lands first in tr.requests is not
	// deterministic.
	subReq, ok := paths[subscriptionPath]
	if !ok {
		t.Fatalf("missing subscription request; paths = %v", paths)
	}
	subBody, _ := io.ReadAll(subReq.Body)
	if string(subBody) != "{}" {
		t.Fatalf("subscription body = %s, want {}", subBody)
	}
}

func TestFetchWebFromCookieHeader(t *testing.T) {
	p, _ := newTestProvider(t, defaultHandler(t))
	jwt := testJWT()

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds: provider.Credentials{
			CookieHeader: "kimi-auth=" + jwt + "; sessionid=abc123",
		},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	wantPercent(t, findWindow(t, snap, "primary"), 62)
	wantPercent(t, findWindow(t, snap, "kimi-monthly"), 40)
}

func TestFetchWebMissingScope(t *testing.T) {
	p, _ := newTestProvider(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"usages":[{"scope":"FEATURE_CHAT","detail":{"limit":"5"}}]}`), nil
	})
	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds: provider.Credentials{AuthToken: testJWT()},
	})
	if err == nil || !strings.Contains(err.Error(), "FEATURE_CODING") {
		t.Fatalf("err = %v, want FEATURE_CODING parse error", err)
	}
	var kerr *Error
	if !errors.As(err, &kerr) || kerr.Kind != KindParse {
		t.Fatalf("err = %v (%T), want KindParse", err, err)
	}
}

func TestFetchWebHTTPStatus(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{401, "Kimi auth token is invalid or expired"},
		{403, "Kimi auth token is invalid or expired"},
		{400, "Invalid request: Bad request"},
		{500, "Kimi API error: HTTP 500"},
	}
	for _, tc := range cases {
		p, _ := newTestProvider(t, func(*http.Request) (*http.Response, error) {
			return jsonResponse(tc.status, `{"error":"x"}`), nil
		})
		_, err := p.Fetch(context.Background(), provider.FetchRequest{
			Creds: provider.Credentials{AuthToken: testJWT()},
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("status %d: err = %v, want contains %q", tc.status, err, tc.want)
		}
	}
}

func TestFetchAPIAuthPath(t *testing.T) {
	p, tr := newTestProvider(t, defaultHandler(t))

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "k-123"},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if tr.requestsLen() != 1 {
		t.Fatalf("made %d requests, want 1 (no enrichment without web token)", tr.requestsLen())
	}
	req := tr.lastRequest()
	if req.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", req.Method)
	}
	if req.URL.Path != codeAPIUsagePath {
		t.Fatalf("path = %s, want %s", req.URL.Path, codeAPIUsagePath)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer k-123" {
		t.Fatalf("Authorization = %q, want Bearer k-123", got)
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}

	// Int/double/string coercion: limit 500 (int), used 150 (int),
	// remaining "350" (string), reset_time snake case.
	primary := findWindow(t, snap, "primary")
	wantPercent(t, primary, 30)
	wantMinutes(t, primary, weeklyWindowMinutes)
	wantResetsAt(t, primary, "2026-08-12T00:00:00Z")

	secondary := findWindow(t, snap, "secondary")
	wantPercent(t, secondary, 20)
	wantMinutes(t, secondary, 24*60) // 1-day window from API
	wantResetsAt(t, secondary, "2026-08-07T00:00:00Z")
}

// The Code API path classifies statuses distinctly from the web path
// (kimi.go fetchCodeAPI): 400 -> InvalidRequest, 401 -> InvalidAPIKey,
// 403 -> permission/quota hint, other -> HTTP n.
func TestFetchCodeAPIHTTPStatus(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{400, "Invalid request: Bad request"},
		{401, "Kimi Code API key is invalid or expired"},
		{403, "HTTP 403"},
		{500, "Kimi API error: HTTP 500"},
	}
	for _, tc := range cases {
		p, _ := newTestProvider(t, func(*http.Request) (*http.Response, error) {
			return jsonResponse(tc.status, `{"error":"x"}`), nil
		})
		_, err := p.Fetch(context.Background(), provider.FetchRequest{
			Creds: provider.Credentials{APIKey: "k-123"},
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("status %d: err = %v, want contains %q", tc.status, err, tc.want)
		}
	}
}

func TestFetchAPIEnrichedWithWebToken(t *testing.T) {
	p, tr := newTestProvider(t, defaultHandler(t))
	jwt := testJWT()

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "k-123", AuthToken: jwt},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if tr.requestsLen() != 2 {
		t.Fatalf("made %d requests, want 2 (API + enrichment)", tr.requestsLen())
	}
	sub := tr.lastRequest()
	if sub.URL.Path != subscriptionPath {
		t.Fatalf("enrichment path = %s, want %s", sub.URL.Path, subscriptionPath)
	}
	if got := sub.Header.Get("Authorization"); got != "Bearer "+jwt {
		t.Fatalf("enrichment Authorization = %q, want Bearer %s", got, jwt)
	}
	wantPercent(t, findWindow(t, snap, "primary"), 30)
	wantPercent(t, findWindow(t, snap, "kimi-monthly"), 40)
	wantPercent(t, findWindow(t, snap, "kimi-code-7d"), 50)
}

// A non-JWT AuthToken (e.g. a config API key surfaced via the orchestrator's
// AuthToken fallback) must not trigger enrichment, mirroring the JWT shape
// check in KimiCookieHeader.override.
func TestFetchAPINoEnrichmentForNonJWT(t *testing.T) {
	p, tr := newTestProvider(t, defaultHandler(t))
	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds: provider.Credentials{APIKey: "k-123", AuthToken: "k-123"},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if tr.requestsLen() != 1 {
		t.Fatalf("made %d requests, want 1", tr.requestsLen())
	}
}

func TestFetchAPIEnrichmentFailureIsBestEffort(t *testing.T) {
	p, _ := newTestProvider(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == subscriptionPath {
			return jsonResponse(http.StatusInternalServerError, `{"error":"boom"}`), nil
		}
		return jsonResponse(http.StatusOK, fixture(t, "usages_api.json")), nil
	})
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds: provider.Credentials{APIKey: "k-123", AuthToken: testJWT()},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	wantPercent(t, findWindow(t, snap, "primary"), 30)
	if _, found := findWindowOrNil(snap, "kimi-monthly"); found {
		t.Fatal("kimi-monthly present despite subscription stats failure")
	}
}

func TestFetchSubscriptionGraceTimeout(t *testing.T) {
	p, _ := newTestProvider(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == subscriptionPath {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(200 * time.Millisecond):
			}
			return jsonResponse(http.StatusOK, fixture(t, "subscription_stats.json")), nil
		}
		return jsonResponse(http.StatusOK, fixture(t, "usages_web.json")), nil
	})
	p.subscriptionGrace = 10 * time.Millisecond

	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds: provider.Credentials{AuthToken: testJWT()},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	wantPercent(t, findWindow(t, snap, "primary"), 62)
	wantPercent(t, findWindow(t, snap, "secondary"), 41)
	if _, found := findWindowOrNil(snap, "kimi-monthly"); found {
		t.Fatal("kimi-monthly present despite subscription stats timeout")
	}
}

func findWindowOrNil(snap *model.Snapshot, id string) (model.RateWindow, bool) {
	for _, w := range snap.Windows {
		if w.ID == id {
			return w, true
		}
	}
	return model.RateWindow{}, false
}

func TestFetchContextDeadline(t *testing.T) {
	p, _ := newTestProvider(t, func(req *http.Request) (*http.Response, error) {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(300 * time.Millisecond):
		}
		return jsonResponse(http.StatusOK, fixture(t, "usages_web.json")), nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	_, err := p.Fetch(ctx, provider.FetchRequest{
		Creds: provider.Credentials{AuthToken: testJWT()},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestFetchMissingCredentials(t *testing.T) {
	// newTestProvider isolates the local-session fallback onto an empty temp
	// dir: with no file present, no env token and no API key, the provider
	// must report the missing-token error.
	p, _ := newTestProvider(t, defaultHandler(t))
	_, err := p.Fetch(context.Background(), provider.FetchRequest{Creds: provider.Credentials{}})
	if err == nil || !strings.Contains(err.Error(), "auth token is missing") {
		t.Fatalf("err = %v, want missing token error", err)
	}
	var kerr *Error
	if !errors.As(err, &kerr) || kerr.Kind != KindMissingToken {
		t.Fatalf("err = %v (%T), want KindMissingToken", err, err)
	}
}

func TestFetchExpiredLocalSessionHint(t *testing.T) {
	// A present-but-expired CLI session must surface its own hint instead of
	// the generic missing-token error.
	dir := t.TempDir()
	credDir := dir + "/credentials"
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// expires_at = 2026-08-06 10:00 UTC, checked at the injected fixedNow
	// (10:30 UTC) — expired.
	content := `{"access_token":"tok","refresh_token":"rt","expires_at":1786010400}`
	if err := os.WriteFile(credDir+"/kimi-code.json", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	p, _ := newTestProviderHome(t, defaultHandler(t), dir)
	_, err := p.Fetch(context.Background(), provider.FetchRequest{Creds: provider.Credentials{}})
	if err == nil || !errors.Is(err, errSessionExpired) {
		t.Fatalf("err = %v, want errSessionExpired hint", err)
	}
}

func TestFetchCLITokenUsesCodeAPIPath(t *testing.T) {
	// A fresh Kimi CLI session must authenticate the Code API
	// (api.kimi.com/coding) — the same endpoint as the API key path — rather
	// than the web console API.
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/credentials", 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"access_token":"eyJhbGciOiJub25lIn0.eyJzdWIiOiJjbGktdG9rZW4ifQ.sig","refresh_token":"rt","expires_at":4102444800}`
	if err := os.WriteFile(dir+"/credentials/kimi-code.json", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	p, tr := newTestProviderHome(t, defaultHandler(t), dir)
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{Creds: provider.Credentials{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Windows) == 0 {
		t.Fatal("expected windows from the Code API path")
	}
	tr.mu.Lock()
	last := tr.requests[len(tr.requests)-1]
	tr.mu.Unlock()
	if last.URL.Path != codeAPIUsagePath {
		t.Fatalf("CLI token must hit Code API path %s, got %s", codeAPIUsagePath, last.URL.Path)
	}
}

func TestAuthSpec(t *testing.T) {
	spec := newProvider(nil).AuthSpec()
	if len(spec.EnvAPIKey) != 1 || spec.EnvAPIKey[0] != "KIMI_CODE_API_KEY" {
		t.Fatalf("EnvAPIKey = %v", spec.EnvAPIKey)
	}
	// KimiSettingsReader.authToken reads KIMI_AUTH_TOKEN / kimi_auth_token.
	if len(spec.EnvAuthToken) != 2 || spec.EnvAuthToken[0] != "KIMI_AUTH_TOKEN" {
		t.Fatalf("EnvAuthToken = %v", spec.EnvAuthToken)
	}
	if !spec.UsesCookies {
		t.Fatalf("UsesCookies = %v", spec.UsesCookies)
	}
	if len(spec.CookieDomains) != 2 || spec.CookieDomains[0] != "www.kimi.com" {
		t.Fatalf("CookieDomains = %v", spec.CookieDomains)
	}
	if len(spec.CookieFallbacks) != 1 || spec.CookieFallbacks[0] != "kimi-desktop" {
		t.Fatalf("CookieFallbacks = %v", spec.CookieFallbacks)
	}
}

func TestDeclaredWindows(t *testing.T) {
	specs := newProvider(nil).DeclaredWindows()
	if len(specs) != 2 {
		t.Fatalf("DeclaredWindows = %v, want 2", specs)
	}
	if specs[0].ID != "primary" || specs[0].Title != "Weekly" {
		t.Fatalf("primary spec = %+v", specs[0])
	}
	if specs[1].ID != "secondary" || specs[1].Title != "5-hour" {
		t.Fatalf("secondary spec = %+v", specs[1])
	}
	if specs[0].WindowMinutes == nil || *specs[0].WindowMinutes != weeklyWindowMinutes {
		t.Fatalf("primary WindowMinutes = %v", specs[0].WindowMinutes)
	}
	if specs[1].WindowMinutes == nil || *specs[1].WindowMinutes != sessionWindowMinutes {
		t.Fatalf("secondary WindowMinutes = %v", specs[1].WindowMinutes)
	}
}

func TestTokenExtraction(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"kimi-auth=eyJh.eyJi.c; foo=1", "eyJh.eyJi.c"},
		{"kimi-auth:eyJh.eyJi.c", "eyJh.eyJi.c"},
		{"kimi-auth: eyJh.eyJi.c; x=1", "eyJh.eyJi.c"},
		{"-H 'Cookie: kimi-auth=TOK1; a=b'", "TOK1"},
		{`-H "Cookie: kimi-auth=TOK2; a=b"`, "TOK2"},
		{"Cookie: a=1; kimi-auth=TOK3", "TOK3"},
		{"cookie: kimi-auth=TOK4", "TOK4"},
		{"kimi-auth=abc", "abc"},
		{"eyJh.eyJi.c", "eyJh.eyJi.c"},
		{"  eyJh.eyJi.c  ", "eyJh.eyJi.c"},
		{"garbage", ""},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := cookieToken(tc.raw); got != tc.want {
			t.Errorf("cookieToken(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestResolveWebToken(t *testing.T) {
	cases := []struct {
		name  string
		creds provider.Credentials
		want  string
	}{
		{"cookie wins", provider.Credentials{CookieHeader: "kimi-auth=CTOK", AuthToken: "ATOK"}, "CTOK"},
		{"env token", provider.Credentials{AuthToken: "ATOK"}, "ATOK"},
		{"quoted env token", provider.Credentials{AuthToken: `"ATOK"`}, "ATOK"},
		{"none", provider.Credentials{}, ""},
	}
	for _, tc := range cases {
		if got := ResolveWebToken(tc.creds); got != tc.want {
			t.Errorf("%s: ResolveWebToken = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEnrichmentTokenShape(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"eyJh.eyJi.c", "eyJh.eyJi.c"},
		{"k-123", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := enrichmentToken(tc.in); got != tc.want {
			t.Errorf("enrichmentToken(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDecodeSessionInfo(t *testing.T) {
	info := decodeSessionInfo(testJWT())
	if info == nil {
		t.Fatal("decodeSessionInfo returned nil for valid JWT")
	}
	if info.deviceID != "dev-123" || info.sessionID != "sess-456" || info.trafficID != "traffic-789" {
		t.Fatalf("sessionInfo = %+v", info)
	}
	for _, bad := range []string{"not-a-jwt", "a.b", "a.b.c", "eyJh.eyJub3RfYmFzZTY0ISEi.c"} {
		if info := decodeSessionInfo(bad); info != nil {
			t.Errorf("decodeSessionInfo(%q) = %+v, want nil", bad, info)
		}
	}
}

func TestParseCounts(t *testing.T) {
	cases := []struct {
		name         string
		detail       kimiUsageDetail
		wantUsed     int
		wantLimit    int
		wantReliable bool
		wantOK       bool
	}{
		{"used authoritative", kimiUsageDetail{Limit: "100", Used: "62"}, 62, 100, true, true},
		{"used zero", kimiUsageDetail{Limit: "100", Used: "0"}, 0, 100, true, true},
		{"used over limit", kimiUsageDetail{Limit: "100", Used: "150"}, 150, 100, true, true},
		{"remaining fallback", kimiUsageDetail{Limit: "100", Remaining: "38"}, 62, 100, true, true},
		{"remaining zero", kimiUsageDetail{Limit: "100", Remaining: "0"}, 100, 100, true, true},
		{"remaining over limit", kimiUsageDetail{Limit: "100", Remaining: "101"}, 0, 0, false, false},
		{"remaining negative", kimiUsageDetail{Limit: "100", Remaining: "-1"}, 0, 0, false, false},
		{"missing counters", kimiUsageDetail{Limit: "100"}, 0, 0, false, false},
		{"bad limit", kimiUsageDetail{Limit: "nope", Used: "5"}, 0, 0, false, false},
		{"zero limit", kimiUsageDetail{Limit: "0"}, 0, 0, false, false},
	}
	for _, tc := range cases {
		c, ok := parseCounts(tc.detail)
		if ok != tc.wantOK {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if c.used != tc.wantUsed || c.limit != tc.wantLimit || c.reliable != tc.wantReliable {
			t.Errorf("%s: counts = %+v, want used=%d limit=%d reliable=%v",
				tc.name, c, tc.wantUsed, tc.wantLimit, tc.wantReliable)
		}
	}
}

func TestWindowDurationMinutes(t *testing.T) {
	cases := []struct {
		window kimiWindow
		want   *int
	}{
		{kimiWindow{Duration: 5, TimeUnit: "TIME_UNIT_HOUR"}, intPtr(300)},
		{kimiWindow{Duration: 7, TimeUnit: "TIME_UNIT_DAY"}, intPtr(7 * 24 * 60)},
		{kimiWindow{Duration: 30, TimeUnit: "TIME_UNIT_MINUTE"}, intPtr(30)},
		{kimiWindow{Duration: 0, TimeUnit: "TIME_UNIT_HOUR"}, nil},
		{kimiWindow{Duration: -1, TimeUnit: "TIME_UNIT_DAY"}, nil},
		{kimiWindow{Duration: 5, TimeUnit: "TIME_UNIT_FORTNIGHT"}, nil},
	}
	for _, tc := range cases {
		got := tc.window.durationMinutes()
		if tc.want == nil {
			if got != nil {
				t.Errorf("%+v: durationMinutes = %v, want nil", tc.window, *got)
			}
			continue
		}
		if got == nil || *got != *tc.want {
			t.Errorf("%+v: durationMinutes = %v, want %v", tc.window, got, *tc.want)
		}
	}
}

func intPtr(v int) *int { return &v }

func TestDetailCoercion(t *testing.T) {
	var d kimiUsageDetail
	data := []byte(`{"limit": 150.0, "used": 12.5, "remaining": 0.4, "resetAt": 123456}`)
	if err := json.Unmarshal(data, &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Limit != "150" {
		t.Errorf("Limit = %q, want 150", d.Limit)
	}
	if d.Used != "12.5" {
		t.Errorf("Used = %q, want 12.5", d.Used)
	}
	if d.Remaining != "0.4" {
		t.Errorf("Remaining = %q, want 0.4", d.Remaining)
	}
	if d.ResetTime != "123456" {
		t.Errorf("ResetTime = %q, want 123456", d.ResetTime)
	}

	// resetAt/resetTime/reset_time/reset_at precedence.
	var d2 kimiUsageDetail
	if err := json.Unmarshal([]byte(`{"limit":"1","reset_time":"A","resetTime":"B"}`), &d2); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d2.ResetTime != "B" {
		t.Errorf("ResetTime = %q, want B (camelCase wins)", d2.ResetTime)
	}
	var d3 kimiUsageDetail
	if err := json.Unmarshal([]byte(`{"limit":"1","reset_at":"C"}`), &d3); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d3.ResetTime != "C" {
		t.Errorf("ResetTime = %q, want C", d3.ResetTime)
	}

	// Missing limit is a parse failure.
	var d4 kimiUsageDetail
	if err := json.Unmarshal([]byte(`{"used":"1"}`), &d4); err == nil {
		t.Fatal("missing limit: expected error")
	}
}

func TestSnapshotExtrasFiltering(t *testing.T) {
	ratio := 0.5
	disabled := false
	featureChat := "FEATURE_CHAT"
	featureOmni := "FEATURE_OMNI"
	typSubscription := "SUBSCRIPTION"
	typCredit := "CREDIT"
	weekly := kimiUsageDetail{Limit: "10", Used: "5"}

	// Non-OMNI feature and disabled ratio: extras dropped.
	stats := &kimiSubscriptionStatsResponse{
		SubscriptionBalance: &kimiSubscriptionBalance{
			Feature: &featureChat, Type: &typSubscription, AmountUsedRatio: &ratio,
		},
		RatelimitCode7d: &kimiSubscriptionRateLimit{Ratio: &ratio, Enabled: &disabled},
	}
	snap := buildSnapshot(weekly, nil, nil, stats, fixedNow, "default")
	if len(snap.Windows) != 1 {
		t.Fatalf("windows = %+v, want only primary", snap.Windows)
	}

	// OMNI feature, type nil, enabled nil: both extras present.
	stats2 := &kimiSubscriptionStatsResponse{
		SubscriptionBalance: &kimiSubscriptionBalance{
			Feature: &featureOmni, AmountUsedRatio: &ratio,
		},
		RatelimitCode7d: &kimiSubscriptionRateLimit{Ratio: &ratio},
	}
	snap2 := buildSnapshot(weekly, nil, nil, stats2, fixedNow, "default")
	if len(snap2.Windows) != 3 {
		t.Fatalf("windows = %+v, want primary + 2 extras", snap2.Windows)
	}

	// Non-SUBSCRIPTION type: monthly dropped.
	stats3 := &kimiSubscriptionStatsResponse{
		SubscriptionBalance: &kimiSubscriptionBalance{
			Feature: &featureOmni, Type: &typCredit, AmountUsedRatio: &ratio,
		},
	}
	snap3 := buildSnapshot(weekly, nil, nil, stats3, fixedNow, "default")
	if len(snap3.Windows) != 1 {
		t.Fatalf("windows = %+v, want only primary", snap3.Windows)
	}

	// NaN ratio: dropped.
	nan := math.NaN()
	stats4 := &kimiSubscriptionStatsResponse{
		SubscriptionBalance: &kimiSubscriptionBalance{AmountUsedRatio: &nan},
		RatelimitCode7d:     &kimiSubscriptionRateLimit{Ratio: &nan},
	}
	snap4 := buildSnapshot(weekly, nil, nil, stats4, fixedNow, "default")
	if len(snap4.Windows) != 1 {
		t.Fatalf("windows = %+v, want only primary", snap4.Windows)
	}
}

func TestMissingCounterDoesNotYieldZeroGauge(t *testing.T) {
	// A valid limit without used/remaining is unknown, not zero usage.
	snap := buildSnapshot(kimiUsageDetail{Limit: "100"}, nil, nil, nil, fixedNow, "default")
	if len(snap.Windows) != 0 {
		t.Fatalf("missing counters must omit the window, got %+v", snap.Windows)
	}
}

func TestFetchCodeAPIRejectsMissingWeeklyCounters(t *testing.T) {
	p, _ := newTestProvider(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != codeAPIUsagePath {
			return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
		}
		return jsonResponse(http.StatusOK, `{"usage":{"limit":"100"}}`), nil
	})
	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Account: "default",
		Creds:   provider.Credentials{APIKey: "test-key"},
	})
	var kimiErr *Error
	if !errors.As(err, &kimiErr) || kimiErr.Kind != KindParse {
		t.Fatalf("missing weekly counters must be a parse error, got %T %v", err, err)
	}
}

func TestSecondaryWindowMinutesFallbackAndOverride(t *testing.T) {
	detail := kimiUsageDetail{Limit: "100", Used: "10"}

	// No API window: session fallback (5h).
	snap := buildSnapshot(kimiUsageDetail{Limit: "1000"}, &detail, nil, nil, fixedNow, "default")
	wantMinutes(t, findWindow(t, snap, "secondary"), sessionWindowMinutes)

	// API window with unknown unit: nil minutes (mirrors Swift).
	win := kimiWindow{Duration: 5, TimeUnit: "TIME_UNIT_FORTNIGHT"}
	snap2 := buildSnapshot(kimiUsageDetail{Limit: "1000"}, &detail, &win, nil, fixedNow, "default")
	sec := findWindow(t, snap2, "secondary")
	if sec.WindowMinutes != nil {
		t.Fatalf("WindowMinutes = %v, want nil for unknown time unit", *sec.WindowMinutes)
	}
}

func TestParseDate(t *testing.T) {
	fractional := "2026-08-13T00:00:00.123Z"
	got := parseDate(fractional)
	if got == nil {
		t.Fatalf("parseDate(%q) = nil", fractional)
	}
	wantT, _ := time.Parse(time.RFC3339Nano, fractional)
	if !got.Equal(wantT) {
		t.Fatalf("parseDate(%q) = %v, want %v", fractional, got, wantT)
	}
	plain := parseDate("2026-08-13T00:00:00Z")
	if plain == nil {
		t.Fatal("parseDate(plain) = nil")
	}
	for _, bad := range []string{"", "not-a-date", "2026-08-13"} {
		if got := parseDate(bad); got != nil {
			t.Errorf("parseDate(%q) = %v, want nil", bad, got)
		}
	}
}

func TestCodeEndpoint(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"https://api.kimi.com", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/coding", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/coding/", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/coding/v1", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/coding/v1/", "https://api.kimi.com/coding/v1/usages"},
		{"https://api.kimi.com/v1/coding/v1", "https://api.kimi.com/v1/coding/v1/usages"},
		{"https://api.kimi.com/v1/coding", "https://api.kimi.com/v1/coding/v1/usages"},
		{"https://api.kimi.com/other", "https://api.kimi.com/other/coding/v1/usages"},
	}
	for _, tc := range cases {
		if got := appendCodeUsagePath(tc.base); got != tc.want {
			t.Errorf("appendCodeUsagePath(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}
