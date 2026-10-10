package opencodego

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"leeway/internal/provider"
)

// fakeOpenCode serves the opencode.ai surface used by the fetcher:
//
//	GET /_server?id=<workspacesServerID>         -> workspace ID JSON
//	GET /_server?id=<billingServerID>&args=[..]  -> billing JSON
//	GET /workspace/<id>/go                       -> usage page
//	GET /workspace/<id>                          -> zen dashboard page
//	GET /zen/go/v1/usage                         -> subscription usage (API key)
type fakeOpenCode struct {
	t              *testing.T
	server         *httptest.Server
	usagePage      string
	zenPage        string
	workspacesResp string
	billingResp    string
	usageAPI       string

	workspacesHits int32
	zenPageHits    int32
	billingHits    int32
	billingArgs    string
}

func newFakeOpenCode(t *testing.T) *fakeOpenCode {
	f := &fakeOpenCode{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/_server", f.handleServer)
	mux.HandleFunc("/workspace/", f.handleWorkspace)
	mux.HandleFunc("/zen/go/v1/usage", f.handleUsageAPI)
	// TLS: the fetcher's redirect guard only follows https redirects.
	f.server = httptest.NewTLSServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeOpenCode) handleUsageAPI(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
		f.t.Errorf("usage API authorization = %q, want Bearer", got)
	}
	if got := r.Header.Get("Accept"); !strings.HasPrefix(got, "application/json") {
		f.t.Errorf("usage API accept = %q", got)
	}
	if r.Method != http.MethodGet {
		f.t.Errorf("usage API method = %s", r.Method)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(f.usageAPI))
}

func (f *fakeOpenCode) handleServer(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("Cookie"); got != "auth=token123" {
		f.t.Errorf("server RPC cookie = %q, want filtered auth cookie", got)
	}
	if got := r.Header.Get("X-Server-Id"); got == "" {
		f.t.Error("missing X-Server-Id header")
	}
	if got := r.Header.Get("X-Server-Instance"); !strings.HasPrefix(got, "server-fn:") {
		f.t.Errorf("X-Server-Instance malformed: %q", got)
	}
	if got := r.Header.Get("Origin"); got != f.server.URL {
		f.t.Errorf("Origin = %q, want provider base %q", got, f.server.URL)
	}
	switch r.URL.Query().Get("id") {
	case workspacesServerID:
		atomic.AddInt32(&f.workspacesHits, 1)
		if r.Method != http.MethodGet {
			f.t.Errorf("workspaces RPC method = %s", r.Method)
		}
		if got := r.URL.Query().Get("args"); got != "" {
			f.t.Errorf("workspaces GET must not carry args, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(f.workspacesResp))
	case billingServerID:
		atomic.AddInt32(&f.billingHits, 1)
		got := r.URL.Query().Get("args")
		if got == "" {
			f.t.Error("billing GET missing args")
		}
		f.billingArgs = got
		if ref := r.Header.Get("Referer"); !strings.HasSuffix(ref, "/workspace/"+workspaceIDFromArgs(got)) {
			f.t.Errorf("billing referer = %q, args = %q", ref, got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(f.billingResp))
	default:
		f.t.Errorf("unknown server id %q", r.URL.Query().Get("id"))
		w.WriteHeader(http.StatusNotFound)
	}
}

func workspaceIDFromArgs(args string) string {
	return strings.TrimSuffix(strings.TrimPrefix(args, `["`), `"]`)
}

func (f *fakeOpenCode) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("Cookie"); got != "auth=token123" {
		f.t.Errorf("workspace page cookie = %q", got)
	}
	if got := r.Header.Get("Accept"); !strings.HasPrefix(got, "text/html") {
		f.t.Errorf("workspace page accept = %q", got)
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/go"):
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(f.usagePage))
	default:
		atomic.AddInt32(&f.zenPageHits, 1)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(f.zenPage))
	}
}

func (f *fakeOpenCode) provider() *opencodeGo {
	client := newHTTPClient()
	// Trust the test server's self-signed cert while keeping the fetcher's
	// redirect guard.
	client.Transport = f.server.Client().Transport
	return &opencodeGo{
		client:    client,
		base:      f.server.URL,
		server:    f.server.URL + "/_server",
		zenGoBase: f.server.URL + "/zen/go/v1",
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func defaultCookieReq() provider.FetchRequest {
	return provider.FetchRequest{
		Creds:       provider.Credentials{CookieHeader: "session=abc; auth=token123; other=x"},
		Account:     "default",
		WorkspaceID: "wrk_test123",
	}
}

func TestFetchHappyPath(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = fixture(t, "zen_balance.json")
	f.billingResp = fixture(t, "billing.json")

	snap, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(snap.Windows) != 4 { // primary, secondary, tertiary, renewal
		t.Fatalf("windows = %d, want 4", len(snap.Windows))
	}
	if snap.Windows[0].Title != "5-hour" || *snap.Windows[0].UsedPercent != 41 {
		t.Fatalf("primary wrong: %+v", snap.Windows[0])
	}
	if snap.Windows[1].Title != "Weekly" || *snap.Windows[1].UsedPercent != 60 {
		t.Fatalf("secondary wrong: %+v", snap.Windows[1])
	}
	if snap.Windows[2].Title != "Monthly" || *snap.Windows[2].UsedPercent != 10 {
		t.Fatalf("tertiary wrong: %+v", snap.Windows[2])
	}
	if snap.Windows[3].Title != "Renews" {
		t.Fatalf("renewal wrong: %+v", snap.Windows[3])
	}
	if snap.Cost == nil || *snap.Cost.Amount != 18.5 || snap.Cost.Period != "Zen balance" || snap.Cost.Currency != "USD" {
		t.Fatalf("cost wrong: %+v", snap.Cost)
	}
	// Configured workspace ID means no workspace-ID RPC.
	if n := atomic.LoadInt32(&f.workspacesHits); n != 0 {
		t.Fatalf("workspaces RPC hit %d times, want 0", n)
	}
	if n := atomic.LoadInt32(&f.zenPageHits); n != 1 {
		t.Fatalf("zen page hit %d times, want 1", n)
	}
	if n := atomic.LoadInt32(&f.billingHits); n != 0 {
		t.Fatalf("billing RPC hit %d times, want 0 (balance came from zen page)", n)
	}
}

func TestFetchResolvesWorkspaceID(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = "no balance here"
	f.workspacesResp = fixture(t, "workspaces.json")
	f.billingResp = `{"customerID": "cus_1", "balance": 200000000}`

	req := defaultCookieReq()
	req.WorkspaceID = "" // no configured workspace ID -> resolve via RPC
	snap, err := f.provider().Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if n := atomic.LoadInt32(&f.workspacesHits); n != 1 {
		t.Fatalf("workspaces RPC hit %d times, want 1", n)
	}
	if len(snap.Windows) != 4 { // usage_nested.json carries renewAt
		t.Fatalf("windows = %d, want 4", len(snap.Windows))
	}
	if snap.Windows[0].Title != "5-hour" {
		t.Fatalf("primary wrong: %+v", snap.Windows[0])
	}
	if f.billingArgs != `["wrk_fixture123"]` {
		t.Fatalf("billing args = %q, want workspace from fixtures", f.billingArgs)
	}
	if snap.Cost == nil || *snap.Cost.Amount != 2.0 {
		t.Fatalf("billing-derived balance wrong: %+v", snap.Cost)
	}
}

func TestFetchWorkspaceIDPostFallback(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = "no balance here"
	f.workspacesResp = `{"workspaces": []}` // GET finds nothing
	f.billingResp = `{"customerID": "cus_1", "balance": 100000000}`

	var postPath string
	var postCT string
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postPath = r.URL.Path
			postCT = r.Header.Get("Content-Type")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"workspaces": [{"id": "wrk_post123"}]}`))
			return
		}
		if r.URL.Path == "/_server" {
			f.handleServer(w, r)
			return
		}
		f.handleWorkspace(w, r)
	})

	req := defaultCookieReq()
	req.WorkspaceID = ""
	_, err := f.provider().Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if postPath != "/_server" {
		t.Fatalf("POST went to %q, want /_server", postPath)
	}
	if postCT != "application/json" {
		t.Fatalf("POST content type = %q", postCT)
	}
	if f.billingArgs != `["wrk_post123"]` {
		t.Fatalf("billing args = %q, want workspace from POST fallback", f.billingArgs)
	}
}

func TestFetchBalanceOnlyFallback(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = `<html>no usage fields at all</html>` // triggers Missing usage fields
	f.zenPage = fixture(t, "zen_balance.json")

	snap, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(snap.Windows) != 0 {
		t.Fatalf("balance-only snapshot must have no windows: %+v", snap.Windows)
	}
	if snap.Cost == nil || *snap.Cost.Amount != 18.5 {
		t.Fatalf("balance-only cost wrong: %+v", snap.Cost)
	}
	if n := atomic.LoadInt32(&f.zenPageHits); n != 1 {
		t.Fatalf("zen page hit %d times, want 1 (single shared task)", n)
	}
}

func TestFetchZenFailureKeepsWindows(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = `{"error": "boom"}` // no balance keys anywhere

	snap, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(snap.Windows) != 4 {
		t.Fatalf("windows = %d, want 4", len(snap.Windows))
	}
	if snap.Cost != nil {
		t.Fatalf("optional zen failure must not fail the fetch: %+v", snap.Cost)
	}
}

func TestFetchZenFromDollarPattern(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = "Zen balance: $14.25" // regex path on the dashboard page
	f.billingResp = fixture(t, "billing.json")

	snap, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if snap.Cost == nil || *snap.Cost.Amount != 14.25 {
		t.Fatalf("regex dollar balance wrong: %+v", snap.Cost)
	}
	if n := atomic.LoadInt32(&f.billingHits); n != 0 {
		t.Fatalf("billing RPC must not run when the dashboard carries the balance")
	}
}

func TestFetchBillingServerResponse(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = "no balance here" // forces the billing RPC
	f.billingResp = fixture(t, "billing.json")

	snap, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if snap.Cost == nil || *snap.Cost.Amount != 18.5 {
		t.Fatalf("billing balance wrong: %+v", snap.Cost)
	}
	if n := atomic.LoadInt32(&f.billingHits); n != 1 {
		t.Fatalf("billing RPC hit %d times, want 1", n)
	}
	if f.billingArgs != `["wrk_test123"]` {
		t.Fatalf("billing args = %q", f.billingArgs)
	}
}

func TestFetchMissingCookie(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = fixture(t, "zen_balance.json")

	_, err := f.provider().Fetch(context.Background(), provider.FetchRequest{Account: "default"})
	if err == nil {
		t.Fatalf("missing cookie must error")
	}
	if !strings.Contains(err.Error(), "没有登录状态") {
		t.Fatalf("expected missing-cookies hint, got %v", err)
	}
}

func TestFetchCookieWithoutAuthCookie(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = fixture(t, "zen_balance.json")

	req := defaultCookieReq()
	req.Creds.CookieHeader = "session=abc; other=x"
	_, err := f.provider().Fetch(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "不合法或已过期") {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}

func TestFetchUnauthorized(t *testing.T) {
	f := newFakeOpenCode(t)
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err == nil || !strings.Contains(err.Error(), "不合法或已过期") {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}

func TestFetchSignedOutPage(t *testing.T) {
	f := newFakeOpenCode(t)
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><a href="/auth/login">Log in</a></html>`))
	})
	_, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err == nil || !strings.Contains(err.Error(), "不合法或已过期") {
		t.Fatalf("expected invalid credentials for signed-out page, got %v", err)
	}
}

func TestFetchAPIErrorWithMessage(t *testing.T) {
	f := newFakeOpenCode(t)
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "rate limited"}`))
	})
	_, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err == nil {
		t.Fatalf("expected API error")
	}
	if !strings.Contains(err.Error(), "OpenCode Go 接口出错：HTTP 500: rate limited") {
		t.Fatalf("error classification wrong: %v", err)
	}
}

func TestFetchNonUTF8Body(t *testing.T) {
	f := newFakeOpenCode(t)
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte{0xff, 0xfe, 0x00, 0x41})
	})
	_, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err == nil || !strings.Contains(err.Error(), "返回的内容不是 UTF-8") {
		t.Fatalf("expected UTF-8 parse error, got %v", err)
	}
}

func TestFetchSameHostRedirectFollowed(t *testing.T) {
	f := newFakeOpenCode(t)
	f.zenPage = fixture(t, "zen_balance.json")
	redirected := false
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/go") && !redirected:
			redirected = true
			http.Redirect(w, r, f.server.URL+"/workspace/wrk_test123/redirected", http.StatusFound)
		case strings.HasSuffix(r.URL.Path, "/redirected"):
			w.Write([]byte(fixture(t, "usage_nested.json")))
		default:
			f.handleWorkspace(w, r)
		}
	})
	snap, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err != nil {
		t.Fatalf("same-host redirect must be followed: %v", err)
	}
	if len(snap.Windows) == 0 {
		t.Fatalf("expected windows after redirect")
	}
}

func TestFetchCrossHostRedirectBlocked(t *testing.T) {
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("cross-host redirect must not be followed")
	}))
	defer other.Close()

	f := newFakeOpenCode(t)
	f.zenPage = fixture(t, "zen_balance.json")
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/go") {
			http.Redirect(w, r, other.URL+"/evil", http.StatusFound)
			return
		}
		f.handleWorkspace(w, r)
	})
	_, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err == nil {
		t.Fatalf("cross-host redirect must produce an error")
	}
}

func TestFetchCancellation(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_nested.json")
	f.zenPage = fixture(t, "zen_balance.json")
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := f.provider().Fetch(ctx, defaultCookieReq())
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
}

func TestFetchNoWorkspaceIDError(t *testing.T) {
	f := newFakeOpenCode(t)
	f.workspacesResp = `{"workspaces": []}`
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workspaces": []}`))
	})
	req := defaultCookieReq()
	req.WorkspaceID = ""
	_, err := f.provider().Fetch(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "缺少工作区 ID") {
		t.Fatalf("expected missing workspace id error, got %v", err)
	}
}

func TestFetchAPIKeyUsage(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usageAPI = fixture(t, "api_usage.json")

	req := provider.FetchRequest{Creds: provider.Credentials{APIKey: "sk-sub123"}, Account: "work"}
	snap, err := f.provider().Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(snap.Windows) != 3 {
		t.Fatalf("windows = %d, want 3", len(snap.Windows))
	}
	if snap.Windows[0].ID != "primary" || snap.Windows[0].Title != "5-hour" || *snap.Windows[0].UsedPercent != 15 {
		t.Fatalf("rolling window wrong: %+v", snap.Windows[0])
	}
	if snap.Windows[1].ID != "secondary" || snap.Windows[1].Title != "Weekly" || *snap.Windows[1].UsedPercent != 90 {
		t.Fatalf("weekly window wrong: %+v", snap.Windows[1])
	}
	if snap.Windows[2].ID != "tertiary" || snap.Windows[2].Title != "Monthly" || *snap.Windows[2].UsedPercent != 55 {
		t.Fatalf("monthly window wrong: %+v", snap.Windows[2])
	}
	// The API-key surface has no Zen balance.
	if snap.Cost != nil {
		t.Fatalf("API-key fetch must not carry a balance: %+v", snap.Cost)
	}
}

func TestFetchAPIKeyUnauthorized(t *testing.T) {
	f := newFakeOpenCode(t)
	f.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := f.provider().Fetch(context.Background(),
		provider.FetchRequest{Creds: provider.Credentials{APIKey: "sk-bad"}})
	if err == nil || !strings.Contains(err.Error(), "不合法或已过期") {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}

func TestFetchAPIKeyMissingUsage(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usageAPI = `{"error": "no usage"}`
	_, err := f.provider().Fetch(context.Background(),
		provider.FetchRequest{Creds: provider.Credentials{APIKey: "sk-sub"}})
	if err == nil || !strings.Contains(err.Error(), "用量字段缺失") {
		t.Fatalf("expected missing usage fields, got %v", err)
	}
}

// usage_snake.json exercises the snake_case server page variant
// (usage_5h/usage_week/usage_month with used_percent/reset_sec).
func TestFetchSnakeCaseUsagePage(t *testing.T) {
	f := newFakeOpenCode(t)
	f.usagePage = fixture(t, "usage_snake.json")
	f.zenPage = fixture(t, "zen_balance.json")
	f.workspacesResp = fixture(t, "workspaces.json")
	f.billingResp = fixture(t, "billing.json")

	snap, err := f.provider().Fetch(context.Background(), defaultCookieReq())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(snap.Windows) != 3 {
		t.Fatalf("windows = %d, want 3 (5h/week/month from snake-case page)", len(snap.Windows))
	}
	if snap.Windows[0].ID != "primary" || *snap.Windows[0].UsedPercent != 33 {
		t.Fatalf("primary wrong: %+v", snap.Windows[0])
	}
	if snap.Windows[1].ID != "secondary" || *snap.Windows[1].UsedPercent != 55 {
		t.Fatalf("secondary wrong: %+v", snap.Windows[1])
	}
	if snap.Windows[2].ID != "tertiary" || *snap.Windows[2].UsedPercent != 77 {
		t.Fatalf("tertiary wrong: %+v", snap.Windows[2])
	}
}
