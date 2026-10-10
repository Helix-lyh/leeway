package grok

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"leeway/internal/provider"
)

// --- test-side protobuf + grpc-web encoder ---------------------------------
// These craft the binary fixtures the tests assert on; no network is used.

func pbVarint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

func pbTag(field, wire int) []byte { return pbVarint(uint64(field)<<3 | uint64(wire)) }

func pbFixed32(f float32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], math.Float32bits(f))
	return b[:]
}

func pbVarintField(field int, v uint64) []byte { return append(pbTag(field, 0), pbVarint(v)...) }

func pbFixed32Field(field int, f float32) []byte { return append(pbTag(field, 5), pbFixed32(f)...) }

func pbMessageField(field int, p []byte) []byte { return append(pbTag(field, 2), pbBytes(p)...) }

func pbBytes(p []byte) []byte { return append(pbVarint(uint64(len(p))), p...) }

func grpcFrame(flag byte, payload []byte) []byte {
	out := make([]byte, 5, 5+len(payload))
	out[0] = flag
	out[1] = byte(len(payload) >> 24)
	out[2] = byte(len(payload) >> 16)
	out[3] = byte(len(payload) >> 8)
	out[4] = byte(len(payload))
	return append(out, payload...)
}

// testNow and futureReset place the parse clock in 2026-08-06 and the reset
// timestamps safely in the future (1_800_000_000 = 2027-01-15).
var (
	testNow         = time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	futureReset     = int64(1_800_000_000)
	futureLater     = int64(1_820_000_000)
	futureEarlier   = int64(1_810_000_000)
	restWeeklyReset = time.Date(2026, 8, 13, 12, 0, 0, 326992000, time.UTC)
)

// restWeekly14JSON matches the CLI chat-proxy billing envelope. 14.0 must
// survive as a float (the grpc-web fixed32 heuristic used to emit 13).
const restWeekly14JSON = `{
  "config": {
    "currentPeriod": {
      "type": "weekly",
      "start": "2026-08-06T12:00:00.000000+00:00",
      "end": "2026-08-13T12:00:00.326992+00:00"
    },
    "creditUsagePercent": 14.0,
    "productUsage": [{"product": "GrokBuild", "usagePercent": 14.0}],
    "prepaidBalance": {"val": 0},
    "billingPeriodEnd": "2026-08-13T12:00:00.326992+00:00"
  }
}`

// happyPayload: field 1 submessage with fixed32 62.5 at path [1,1], reset
// timestamp at [1,5,1], and a usage-period marker at [1,6].
func happyPayload() []byte {
	inner := pbFixed32Field(1, 62.5)
	inner = append(inner, pbMessageField(5, pbVarintField(1, uint64(futureReset)))...)
	inner = append(inner, pbVarintField(6, 1)...)
	return pbMessageField(1, inner)
}

func happyGRPCWebBody() []byte {
	return append(grpcFrame(0x00, happyPayload()), grpcFrame(0x80, []byte("grpc-status: 0\n"))...)
}

func errKind(err error) errorKind {
	var g *Error
	if !errors.As(err, &g) {
		return errorKind(-1)
	}
	return g.Kind
}

func assertErrorKind(t *testing.T, err error, want errorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error kind %d, got nil", want)
	}
	if got := errKind(err); got != want {
		t.Fatalf("expected error kind %d, got %d (%v)", want, got, err)
	}
}

// --- parseGRPCWebResponse ---------------------------------------------------

func TestParseGRPCWebResponse_HappyPath(t *testing.T) {
	snap, err := parseGRPCWebResponse(happyGRPCWebBody(), testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.usedPercent == nil || *snap.usedPercent != 62.5 {
		t.Errorf("usedPercent = %v, want 62.5", snap.usedPercent)
	}
	if snap.resetsAt == nil || snap.resetsAt.Unix() != futureReset {
		t.Errorf("resetsAt = %v, want unix %d", snap.resetsAt, futureReset)
	}
}

func TestParseGRPCWebResponse_NoUsageYet(t *testing.T) {
	// No fixed32 fields, but a reset at [1,5,1] plus usage-period marker [1,6].
	inner := pbMessageField(5, pbVarintField(1, uint64(futureReset)))
	inner = append(inner, pbVarintField(6, 1)...)
	payload := pbMessageField(1, inner)
	snap, err := parseGRPCWebResponse(grpcFrame(0x00, payload), testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.usedPercent == nil || *snap.usedPercent != 0 {
		t.Errorf("usedPercent = %v, want 0 (no usage yet)", snap.usedPercent)
	}
	if snap.resetsAt == nil || snap.resetsAt.Unix() != futureReset {
		t.Errorf("resetsAt = %v, want unix %d", snap.resetsAt, futureReset)
	}
}

func TestParseGRPCWebResponse_NoUsageYetRequiresReset(t *testing.T) {
	// Usage-period marker but no reset candidate -> parse failed.
	inner := pbVarintField(6, 1)
	payload := pbMessageField(1, inner)
	_, err := parseGRPCWebResponse(grpcFrame(0x00, payload), testNow)
	assertErrorKind(t, err, kindParseFailed)
}

func TestParseGRPCWebResponse_SelectsShallowestPercent(t *testing.T) {
	// NaN and out-of-range fixed32s are filtered; [1,1]=50 (depth 2) beats
	// [1,2,1]=99 (depth 3).
	inner := pbFixed32Field(1, float32(math.NaN()))
	inner = append(inner, pbFixed32Field(1, 50)...)
	inner = append(inner, pbMessageField(2, pbFixed32Field(1, 99))...)
	inner = append(inner, pbFixed32Field(1, 150)...)
	inner = append(inner, pbFixed32Field(3, 42)...) // path ends in field 3, ignored
	payload := pbMessageField(1, inner)
	snap, err := parseGRPCWebResponse(grpcFrame(0x00, payload), testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *snap.usedPercent != 50 {
		t.Errorf("usedPercent = %v, want 50 (shallowest valid path wins)", *snap.usedPercent)
	}
}

func TestParseGRPCWebResponse_PrefersPath151Reset(t *testing.T) {
	// A later [1,5,1] reset beats an earlier reset at another path.
	inner := pbMessageField(5, pbVarintField(1, uint64(futureLater)))
	inner = append(inner, pbVarintField(7, uint64(futureEarlier))...)
	inner = append(inner, pbFixed32Field(1, 20)...)
	payload := pbMessageField(1, inner)
	snap, err := parseGRPCWebResponse(grpcFrame(0x00, payload), testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.resetsAt == nil || snap.resetsAt.Unix() != futureLater {
		t.Errorf("resetsAt = %v, want preferred path [1,5,1] unix %d", snap.resetsAt, futureLater)
	}
}

func TestParseGRPCWebResponse_ResetFallsBackToEarliest(t *testing.T) {
	inner := pbVarintField(7, uint64(futureEarlier))
	inner = append(inner, pbVarintField(9, uint64(futureLater))...)
	inner = append(inner, pbFixed32Field(1, 20)...)
	payload := pbMessageField(1, inner)
	snap, err := parseGRPCWebResponse(grpcFrame(0x00, payload), testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.resetsAt == nil || snap.resetsAt.Unix() != futureEarlier {
		t.Errorf("resetsAt = %v, want earliest future reset %d", snap.resetsAt, futureEarlier)
	}
}

func TestParseGRPCWebResponse_IgnoresPastResets(t *testing.T) {
	// 1_750_000_000 (June 2025) is before the parse clock and must be ignored.
	inner := pbVarintField(7, 1_750_000_000)
	inner = append(inner, pbFixed32Field(1, 20)...)
	payload := pbMessageField(1, inner)
	snap, err := parseGRPCWebResponse(grpcFrame(0x00, payload), testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.resetsAt != nil {
		t.Errorf("resetsAt = %v, want nil (only past timestamps present)", snap.resetsAt)
	}
}

func TestParseGRPCWebResponse_Empty(t *testing.T) {
	_, err := parseGRPCWebResponse(nil, testNow)
	assertErrorKind(t, err, kindEmptyResponse)
}

func TestParseGRPCWebResponse_MalformedFrames(t *testing.T) {
	// Frame header claims more bytes than exist -> no data frames.
	_, err := parseGRPCWebResponse([]byte{0x00, 0x00, 0x00, 0xFF, 0xFF, 0x01}, testNow)
	assertErrorKind(t, err, kindEmptyResponse)
}

func TestParseGRPCWebResponse_ParseFailed(t *testing.T) {
	// Well-framed but unrecognizable protobuf: no percent, no reset, no
	// usage-period marker.
	data := grpcFrame(0x00, []byte{0x08, 0x01, 0x10, 0x02})
	_, err := parseGRPCWebResponse(data, testNow)
	assertErrorKind(t, err, kindParseFailed)
}

// --- grpc-web trailers and HTTP headers ------------------------------------

func TestValidateGRPCWebTrailers(t *testing.T) {
	data := grpcFrame(0x80, []byte("grpc-status: 7\ngrpc-message: bad-credentials"))
	err := validateGRPCWebTrailers(data)
	assertErrorKind(t, err, kindRPCFailed)
	if g, ok := err.(*Error); !ok || g.Status != 7 {
		t.Errorf("expected grpc status 7, got %v", err)
	}
	if err.Error() != reauthMessage {
		t.Errorf("auth failure should map to reauth message, got: %s", err.Error())
	}
}

func TestValidateGRPCWebTrailers_NonAuthRPCError(t *testing.T) {
	data := grpcFrame(0x80, []byte("grpc-status: 1\ngrpc-message: deadline exceeded"))
	err := validateGRPCWebTrailers(data)
	assertErrorKind(t, err, kindRPCFailed)
	if err.Error() != "Grok 网页版账单 RPC 失败（状态 1）：deadline exceeded" {
		t.Errorf("unexpected message: %s", err.Error())
	}
}

func TestValidateGRPCWebTrailers_ZeroStatusOK(t *testing.T) {
	data := grpcFrame(0x80, []byte("grpc-status: 0"))
	if err := validateGRPCWebTrailers(data); err != nil {
		t.Errorf("grpc-status 0 must not error, got: %v", err)
	}
}

func TestValidateGRPCWebTrailers_CRLFLines(t *testing.T) {
	data := grpcFrame(0x80, []byte("grpc-status: 16\r\ngrpc-message: token expired\r\n"))
	err := validateGRPCWebTrailers(data)
	assertErrorKind(t, err, kindRPCFailed)
	if err.Error() != reauthMessage {
		t.Errorf("status 16 is always an auth failure, got: %s", err.Error())
	}
}

func TestGRPCWebTrailerFields_PercentDecoding(t *testing.T) {
	data := grpcFrame(0x80, []byte("grpc-status: 7\ngrpc-message: bad%20credentials\n"))
	fields := grpcWebTrailerFields(data)
	if fields["grpc-message"] != "bad credentials" {
		t.Errorf("grpc-message = %q, want percent-decoded %q", fields["grpc-message"], "bad credentials")
	}
}

func TestGRPCHeaderFields(t *testing.T) {
	h := http.Header{
		"Grpc-Status":  {"7"},
		"Grpc-Message": {"unauthenticated"},
		"X-Foo":        {"bar"},
	}
	fields := grpcHeaderFields(h)
	if fields["grpc-status"] != "7" {
		t.Errorf("grpc-status = %q, want 7", fields["grpc-status"])
	}
	if _, ok := fields["x-foo"]; ok {
		t.Error("non-grpc headers must be excluded")
	}
	err := validateGRPCStatusFields(fields)
	assertErrorKind(t, err, kindRPCFailed)
	if err.Error() != reauthMessage {
		t.Errorf("unauthenticated should map to reauth message, got: %s", err.Error())
	}

	if err := validateGRPCStatusFields(grpcHeaderFields(http.Header{"Grpc-Status": {"0"}})); err != nil {
		t.Errorf("grpc-status 0 must not error, got: %v", err)
	}
}

func TestIsAuthenticationFailure(t *testing.T) {
	cases := []struct {
		status  int
		message string
		want    bool
	}{
		{16, "", true},
		{7, "bad-credentials", true},
		{7, "Unauthenticated", true},
		{7, "oauth2 token could not be validated", true},
		{7, "access token is invalid", true},
		{7, "access token expired", true},
		{7, "access token could not be validated", true},
		{7, "something else", false},
		{3, "bad-credentials", false},
	}
	for _, c := range cases {
		if got := isAuthenticationFailure(c.status, c.message); got != c.want {
			t.Errorf("isAuthenticationFailure(%d, %q) = %v, want %v", c.status, c.message, got, c.want)
		}
	}
}

// --- retry classification ---------------------------------------------------

func TestShouldRetry(t *testing.T) {
	transient := []error{&url.Error{Op: "Post", URL: endpoint, Err: context.DeadlineExceeded}}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"http 503", errRequestFailed(503, "unavailable"), true},
		{"http 502", errRequestFailed(502, "gateway"), true},
		{"http 504", errRequestFailed(504, "gateway timeout"), true},
		{"http 408", errRequestFailed(408, "request timeout"), true},
		{"http 500", errRequestFailed(500, "boom"), false},
		{"http 401", errRequestFailed(401, "denied"), false},
		{"body timeout", errRequestFailed(429, "upstream timeout"), true},
		{"body deadline", errRequestFailed(200, "deadline exceeded"), true},
		{"rpc 4", errRPCFailed(4, ""), true},
		{"rpc 1 expired", errRPCFailed(1, "request expired"), true},
		{"rpc 1 other", errRPCFailed(1, "nope"), false},
		{"rpc 7 auth", errRPCFailed(7, "bad-credentials"), false},
		{"context deadline", context.DeadlineExceeded, true},
		{"url error", transient[0], true},
		{"plain", errors.New("boom"), false},
	}
	for _, c := range cases {
		if got := shouldRetry(c.err); got != c.want {
			t.Errorf("%s: shouldRetry = %v, want %v", c.name, got, c.want)
		}
	}
}

// --- HTTP fetch (fake transport, no network) --------------------------------

type fakeRoundTripper struct {
	calls     int
	status    int
	headers   http.Header
	body      []byte
	transient []error
	reqs      []*http.Request
	bodies    [][]byte
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.calls++
	f.reqs = append(f.reqs, req)
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	f.bodies = append(f.bodies, body)
	if len(f.transient) > 0 {
		err := f.transient[0]
		f.transient = f.transient[1:]
		return nil, err
	}
	return &http.Response{
		StatusCode: f.status,
		Header:     f.headers,
		Body:       io.NopCloser(bytes.NewReader(f.body)),
		Request:    req,
	}, nil
}

func newFakeClient(rt *fakeRoundTripper) *http.Client { return &http.Client{Transport: rt} }

func TestFetch_SendsExpectedRequest(t *testing.T) {
	rt := &fakeRoundTripper{status: 200, body: happyGRPCWebBody()}
	snap, err := fetchWebBilling(context.Background(), "sess=abc; auth=xyz", "", newFakeClient(rt), testNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.usedPercent == nil || *snap.usedPercent != 62.5 {
		t.Errorf("usedPercent = %v, want 62.5", snap.usedPercent)
	}
	if rt.calls != 1 {
		t.Fatalf("calls = %d, want 1", rt.calls)
	}
	req := rt.reqs[0]
	if req.Method != http.MethodPost || req.URL.String() != endpoint {
		t.Errorf("method/URL = %s %s, want POST %s", req.Method, req.URL, endpoint)
	}
	if body := rt.bodies[0]; !bytes.Equal(body, []byte{0, 0, 0, 0, 0}) {
		t.Errorf("body = %v, want five zero bytes", body)
	}
	h := req.Header
	want := map[string]string{
		"Cookie":       "sess=abc; auth=xyz",
		"Origin":       "https://grok.com",
		"Referer":      "https://grok.com/?_s=usage",
		"Accept":       "*/*",
		"Content-Type": "application/grpc-web+proto",
		"x-grpc-web":   "1",
		"x-user-agent": "connect-es/2.1.1",
		"User-Agent":   "leeway",
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
}

func TestFetch_RetriesOnceOnTransientFailure(t *testing.T) {
	rt := &fakeRoundTripper{
		status:    200,
		body:      happyGRPCWebBody(),
		transient: []error{&url.Error{Op: "Post", URL: endpoint, Err: context.DeadlineExceeded}},
	}
	snap, err := fetchWebBilling(context.Background(), "sess=abc", "", newFakeClient(rt), testNow)
	if err != nil {
		t.Fatalf("retry should have succeeded: %v", err)
	}
	if rt.calls != 2 {
		t.Errorf("calls = %d, want 2 (retry once)", rt.calls)
	}
	if *snap.usedPercent != 62.5 {
		t.Errorf("usedPercent = %v, want 62.5", *snap.usedPercent)
	}
}

func TestFetch_RetriesAtMostOnce(t *testing.T) {
	rt := &fakeRoundTripper{
		status: 503,
		body:   []byte("unavailable"),
	}
	_, err := fetchWebBilling(context.Background(), "sess=abc", "", newFakeClient(rt), testNow)
	assertErrorKind(t, err, kindRequestFailed)
	if rt.calls != 2 {
		t.Errorf("calls = %d, want 2 (single retry only)", rt.calls)
	}
}

func TestFetch_HTTP401MapsToReauth(t *testing.T) {
	rt := &fakeRoundTripper{status: 401, body: []byte("denied")}
	_, err := fetchWebBilling(context.Background(), "sess=abc", "", newFakeClient(rt), testNow)
	assertErrorKind(t, err, kindRequestFailed)
	if err.Error() != reauthMessage {
		t.Errorf("401 must map to reauth message, got: %s", err.Error())
	}
}

func TestFetch_HTTP500(t *testing.T) {
	rt := &fakeRoundTripper{status: 500, body: []byte("boom")}
	_, err := fetchWebBilling(context.Background(), "sess=abc", "", newFakeClient(rt), testNow)
	assertErrorKind(t, err, kindRequestFailed)
	if err.Error() != "Grok 网页版账单请求失败（HTTP 500）：boom" {
		t.Errorf("unexpected message: %s", err.Error())
	}
}

func TestFetch_GRPCStatusInHTTPHeaders(t *testing.T) {
	rt := &fakeRoundTripper{
		status:  200,
		body:    happyGRPCWebBody(),
		headers: http.Header{"Grpc-Status": {"7"}, "Grpc-Message": {"bad-credentials"}},
	}
	_, err := fetchWebBilling(context.Background(), "sess=abc", "", newFakeClient(rt), testNow)
	assertErrorKind(t, err, kindRPCFailed)
	if err.Error() != reauthMessage {
		t.Errorf("expected reauth message, got: %s", err.Error())
	}
}

// --- provider plumbing ------------------------------------------------------

func TestProviderFetch_MissingCredentials(t *testing.T) {
	// Isolate the local-session fallback so the test never touches a real
	// ~/.grok/auth.json (or refreshes real tokens over the network).
	t.Setenv("GROK_HOME", t.TempDir())
	_, err := New().Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{},
		Account: "default",
	})
	assertErrorKind(t, err, kindMissingCredentials)
	var gerr *Error
	if !errors.As(err, &gerr) || gerr.Kind != kindMissingCredentials {
		t.Errorf("error must be kindMissingCredentials, got %T %v", err, err)
	}
}

func TestProviderFetch_SnapshotShape(t *testing.T) {
	p := &grokProvider{client: newFakeClient(&fakeRoundTripper{status: 200, body: []byte(restWeekly14JSON)}), now: func() time.Time { return testNow }}
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{CookieHeader: "sess=abc"},
		Account: "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Identity == nil || snap.Identity.Account != "default" {
		t.Errorf("identity = %+v, want account default", snap.Identity)
	}
	if len(snap.Windows) != 1 {
		t.Fatalf("windows = %d, want exactly 1", len(snap.Windows))
	}
	w := snap.Windows[0]
	if w.ID != "primary" || w.Title != "Weekly" {
		t.Errorf("window id/title = %q/%q, want primary/Weekly", w.ID, w.Title)
	}
	if w.UsedPercent == nil || *w.UsedPercent != 14.0 {
		t.Errorf("usedPercent = %v, want 14.0", w.UsedPercent)
	}
	if w.Used != nil || w.Limit != nil {
		t.Errorf("raw window fields must stay nil, got %+v", w)
	}
	if w.WindowMinutes == nil || *w.WindowMinutes != weeklyWindowMinutes {
		t.Errorf("windowMinutes = %v, want %d", w.WindowMinutes, weeklyWindowMinutes)
	}
	if w.ResetsAt == nil || !w.ResetsAt.Equal(restWeeklyReset) {
		t.Errorf("resetsAt = %v, want %v", w.ResetsAt, restWeeklyReset)
	}
}

func TestProviderFetch_ReauthErrorIsCookieAuthFailure(t *testing.T) {
	p := &grokProvider{client: newFakeClient(&fakeRoundTripper{status: 403, body: []byte("nope")}), now: func() time.Time { return testNow }}
	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{CookieHeader: "sess=abc"},
		Account: "default",
	})
	if !isCookieAuthenticationFailure(err) {
		t.Errorf("403 must classify as cookie authentication failure, got %v", err)
	}
}

// A valid `grok login` session must reach the billing endpoint with
// `Authorization: Bearer <key>` and no Cookie header.
func TestProviderFetch_BearerSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GROK_HOME", dir)
	writeGrokAuth(t, dir, `{
	  "https://auth.x.ai::scope1": {"key": "bearer-key", "refresh_token": "rt", "expires_at": "2099-01-01T00:00:00Z", "oidc_issuer": "https://auth.x.ai", "oidc_client_id": "cid"}
	}`)
	rt := &fakeRoundTripper{status: 200, body: []byte(restWeekly14JSON)}
	p := &grokProvider{client: newFakeClient(rt), now: func() time.Time { return testNow }}
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{},
		Account: "default",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if snap == nil || snap.Windows[0].UsedPercent == nil || *snap.Windows[0].UsedPercent != 14.0 {
		t.Fatalf("snapshot wrong: %+v", snap)
	}
	if len(rt.reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(rt.reqs))
	}
	req := rt.reqs[0]
	if req.Method != http.MethodGet || req.URL.String() != restBillingURL {
		t.Errorf("method/URL = %s %s, want GET %s", req.Method, req.URL, restBillingURL)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer bearer-key" {
		t.Errorf("Authorization = %q, want Bearer bearer-key", got)
	}
	if got := req.Header.Get("x-xai-token-auth"); got != restTokenAuth {
		t.Errorf("x-xai-token-auth = %q, want %s", got, restTokenAuth)
	}
	if got := req.Header.Get("Cookie"); got != "" {
		t.Errorf("Cookie = %q, want empty on the bearer path", got)
	}
}

// An expired session whose refresh fails (issuer returns 400) surfaces
// kindMissingCredentials with the "leeway login grok" hint — the classification
// doctor/agents rely on.
func TestProviderFetch_ExpiredSessionRefreshFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Setenv("GROK_HOME", dir)
	writeGrokAuth(t, dir, `{
	  "https://auth.x.ai::scope1": {"key": "old-key", "refresh_token": "rt", "expires_at": "2026-08-06T10:00:00Z", "oidc_issuer": "`+srv.URL+`", "oidc_client_id": "cid"}
	}`)
	p := &grokProvider{client: newFakeClient(&fakeRoundTripper{status: 200, body: happyGRPCWebBody()}), now: func() time.Time { return testNow }}
	_, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{},
		Account: "default",
	})
	assertErrorKind(t, err, kindMissingCredentials)
	if err == nil || !strings.Contains(err.Error(), "leeway login grok") {
		t.Errorf("err = %v, want re-login hint", err)
	}
}

func TestToModel_IdentityAndWindow(t *testing.T) {
	percent := 40.0
	reset := testNow.Add(7 * 24 * time.Hour)
	snap := billingSnapshot{usedPercent: &percent, resetsAt: &reset}.toModel("work", testNow)
	if snap.Identity == nil || snap.Identity.Account != "work" {
		t.Errorf("identity = %+v, want account work", snap.Identity)
	}
	if len(snap.Windows) != 1 || snap.Windows[0].Title != "Weekly" ||
		snap.Windows[0].WindowMinutes == nil || *snap.Windows[0].WindowMinutes != weeklyWindowMinutes {
		t.Errorf("unexpected windows: %+v", snap.Windows)
	}
	if snap.Cost != nil {
		t.Errorf("cost must be nil for grok, got %+v", snap.Cost)
	}
}

func TestDeclaredWindows(t *testing.T) {
	specs := New().DeclaredWindows()
	if len(specs) != 1 || specs[0].Title != "Weekly" ||
		specs[0].WindowMinutes == nil || *specs[0].WindowMinutes != weeklyWindowMinutes {
		t.Errorf("declared windows = %+v, want one 7-day Weekly window", specs)
	}
}

func TestParseRESTBilling_Weekly14(t *testing.T) {
	snap, err := parseRESTBilling([]byte(restWeekly14JSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.usedPercent == nil || *snap.usedPercent != 14.0 {
		t.Errorf("usedPercent = %v, want 14.0 (not a truncated 13)", snap.usedPercent)
	}
	if !snap.fromREST || snap.periodType != "weekly" {
		t.Errorf("fromREST/periodType = %v/%q, want true/weekly", snap.fromREST, snap.periodType)
	}
	if snap.resetsAt == nil || !snap.resetsAt.Equal(restWeeklyReset) {
		t.Errorf("resetsAt = %v, want %v", snap.resetsAt, restWeeklyReset)
	}
	m := snap.toModel("default", testNow)
	if len(m.Windows) != 1 {
		t.Fatalf("windows = %d, want 1", len(m.Windows))
	}
	w := m.Windows[0]
	if w.Title != "Weekly" {
		t.Errorf("title = %q, want Weekly", w.Title)
	}
	if w.UsedPercent == nil || *w.UsedPercent != 14.0 {
		t.Errorf("usedPercent = %v, want 14.0", w.UsedPercent)
	}
	if w.WindowMinutes == nil || *w.WindowMinutes != weeklyWindowMinutes {
		t.Errorf("windowMinutes = %v, want %d", w.WindowMinutes, weeklyWindowMinutes)
	}
}

func TestParseRESTBilling_UsagePeriodTypeWeekly(t *testing.T) {
	body := []byte(`{"config":{"creditUsagePercent":14.0,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","end":"2026-08-13T12:00:00Z"}}}`)
	snap, err := parseRESTBilling(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	title, minutes := snap.primaryLabel(testNow)
	if title != "Weekly" || minutes == nil || *minutes != weeklyWindowMinutes {
		t.Errorf("label = %q %v, want Weekly %d", title, minutes, weeklyWindowMinutes)
	}
}

func TestParseRESTBilling_MissingPercentIsZero(t *testing.T) {
	body := []byte(`{"config":{"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","end":"2026-08-13T12:00:00Z"},"onDemandCap":{"val":2500}}}`)
	snap, err := parseRESTBilling(body)
	if err != nil {
		t.Fatalf("absent percent must parse as 0: %v", err)
	}
	if snap.usedPercent == nil || *snap.usedPercent != 0 {
		t.Fatalf("usedPercent = %v, want 0", snap.usedPercent)
	}
	if snap.onDemandCap != 2500 {
		t.Fatalf("onDemandCap = %v, want 2500", snap.onDemandCap)
	}
	m := snap.toModel("default", testNow)
	if m.Windows[0].Title != "Weekly" {
		t.Fatalf("title = %q, want Weekly", m.Windows[0].Title)
	}
	if len(m.Windows) != 2 || m.Windows[1].Title != "On-demand" || m.Windows[1].Used == nil || *m.Windows[1].Used != 2500 {
		t.Fatalf("on-demand window = %+v", m.Windows)
	}
}

func TestParseRESTBilling_MonthlyIsNotWeekly(t *testing.T) {
	body := []byte(`{"config":{"creditUsagePercent":40.0,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_MONTHLY","end":"2026-09-06T12:00:00Z"}}}`)
	snap, err := parseRESTBilling(body)
	if err != nil {
		t.Fatal(err)
	}
	m := snap.toModel("default", testNow)
	if m.Windows[0].Title == "Weekly" {
		t.Fatalf("non-weekly period must not render as Weekly: %+v", m.Windows[0])
	}
	if m.Windows[0].Title != "Monthly" {
		t.Fatalf("title = %q, want Monthly", m.Windows[0].Title)
	}
}

func TestProviderFetch_UsesREST(t *testing.T) {
	var restHits, grpcHits int
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restHits++
		if r.Method != http.MethodGet {
			t.Errorf("REST method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/billing" || r.URL.Query().Get("format") != "credits" {
			t.Errorf("REST URL = %s, want /v1/billing?format=credits", r.URL)
		}
		if got := r.Header.Get("x-xai-token-auth"); got != restTokenAuth {
			t.Errorf("x-xai-token-auth = %q, want %s", got, restTokenAuth)
		}
		if got := r.Header.Get("Cookie"); got != "sess=abc" {
			t.Errorf("Cookie = %q, want sess=abc", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(restWeekly14JSON))
	}))
	defer restSrv.Close()
	grpcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grpcHits++
		w.Write(happyGRPCWebBody())
	}))
	defer grpcSrv.Close()
	withBillingURLs(t, restSrv.URL+"/v1/billing?format=credits", grpcSrv.URL)

	p := &grokProvider{client: restSrv.Client(), now: func() time.Time { return testNow }}
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{CookieHeader: "sess=abc"},
		Account: "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restHits != 1 || grpcHits != 0 {
		t.Errorf("hits rest=%d grpc=%d, want rest=1 grpc=0", restHits, grpcHits)
	}
	if snap.Windows[0].UsedPercent == nil || *snap.Windows[0].UsedPercent != 14.0 {
		t.Errorf("usedPercent = %v, want 14.0", snap.Windows[0].UsedPercent)
	}
	if snap.Windows[0].Title != "Weekly" {
		t.Errorf("title = %q, want Weekly", snap.Windows[0].Title)
	}
}

func TestProviderFetch_RESTFallbackGRPCWeb(t *testing.T) {
	var restHits, grpcHits int
	restSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restHits++
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer restSrv.Close()
	grpcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grpcHits++
		if r.Method != http.MethodPost {
			t.Errorf("grpc method = %s, want POST", r.Method)
		}
		w.Write(happyGRPCWebBody())
	}))
	defer grpcSrv.Close()
	withBillingURLs(t, restSrv.URL+"/v1/billing?format=credits", grpcSrv.URL)

	p := &grokProvider{client: restSrv.Client(), now: func() time.Time { return testNow }}
	snap, err := p.Fetch(context.Background(), provider.FetchRequest{
		Creds:   provider.Credentials{CookieHeader: "sess=abc"},
		Account: "default",
	})
	if err != nil {
		t.Fatalf("fallback should have succeeded: %v", err)
	}
	if restHits != 1 || grpcHits != 1 {
		t.Errorf("hits rest=%d grpc=%d, want rest=1 grpc=1", restHits, grpcHits)
	}
	if snap.Windows[0].UsedPercent == nil || *snap.Windows[0].UsedPercent != 62.5 {
		t.Errorf("usedPercent = %v, want 62.5 from grpc-web", snap.Windows[0].UsedPercent)
	}
}

func withBillingURLs(t *testing.T, restURL, grpcURL string) {
	t.Helper()
	origREST, origGRPC := restBillingURL, endpoint
	restBillingURL = restURL
	endpoint = grpcURL
	t.Cleanup(func() {
		restBillingURL = origREST
		endpoint = origGRPC
	})
}
