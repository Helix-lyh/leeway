package grok

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

const (
	// requestTimeout mirrors GrokWebBillingFetcher.requestTimeoutSeconds.
	requestTimeout = 15 * time.Second
	// userAgent identifies leeway (CodexBar sends "CodexBar").
	userAgent = "leeway"
)

// endpoint is the grpc-web billing URL. It is a var so tests can point at an
// httptest server when exercising the REST→grpc-web fallback.
var endpoint = "https://grok.com/grok_api_v2.GrokBuildBilling/GetGrokCreditsConfig"

// defaultClient is the production HTTP client.
var defaultClient = &http.Client{Timeout: requestTimeout}

// fetchWebBilling performs the POST with a single retry on transient
// failures, mirroring GrokWebBillingFetcher.fetch. Exactly one of
// cookieHeader/authorization is set (cookie session vs OIDC bearer); the
// retry reuses the same arguments so its transient-only intent is visible.
func fetchWebBilling(ctx context.Context, cookieHeader, authorization string, client *http.Client, now time.Time) (billingSnapshot, error) {
	snap, err := fetchOnce(ctx, cookieHeader, authorization, client, now)
	if err == nil || !shouldRetry(err) {
		return snap, err
	}
	return fetchOnce(ctx, cookieHeader, authorization, client, now)
}

// fetchOnce mirrors GrokWebBillingFetcher.fetchOnce: POST a 5-zero-byte body
// with grpc-web headers, validate HTTP status and grpc status fields/trailers,
// then parse the protobuf payload. Exactly one of cookieHeader/authorization
// is set. now is the parse clock (injectable in tests).
func fetchOnce(ctx context.Context, cookieHeader, authorization string, client *http.Client, now time.Time) (billingSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte{0, 0, 0, 0, 0}))
	if err != nil {
		return billingSnapshot{}, &Error{Kind: kindInvalidResponse}
	}
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	req.Header.Set("Origin", "https://grok.com")
	req.Header.Set("Referer", "https://grok.com/?_s=usage")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/grpc-web+proto")
	req.Header.Set("x-grpc-web", "1")
	req.Header.Set("x-user-agent", "connect-es/2.1.1")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return billingSnapshot{}, ctx.Err()
		}
		return billingSnapshot{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return billingSnapshot{}, ctx.Err()
		}
		return billingSnapshot{}, err
	}
	if resp.StatusCode != http.StatusOK {
		prefix := data
		if len(prefix) > 400 {
			prefix = prefix[:400]
		}
		return billingSnapshot{}, errRequestFailed(resp.StatusCode, string(prefix))
	}
	if err := validateGRPCStatusFields(grpcHeaderFields(resp.Header)); err != nil {
		return billingSnapshot{}, err
	}
	if err := validateGRPCWebTrailers(data); err != nil {
		return billingSnapshot{}, err
	}
	return parseGRPCWebResponse(data, now)
}
