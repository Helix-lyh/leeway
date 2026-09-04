// Package kimi implements the Kimi Code usage provider, mirroring CodexBar's
// Kimi usage fetcher (KimiUsageFetcher.swift / KimiUsageSnapshot.swift).
//
// Two auth paths are supported, mirroring CodexBar's strategy order (API key
// strategy first in auto mode, enriched by a web token when available):
//
//   - Kimi Code API key (KIMI_CODE_API_KEY, or `leeway config set-api-key`):
//     GET {base}/coding/v1/usages with Authorization: Bearer <apiKey>.
//   - Web session JWT (kimi-auth cookie via imported/manual cookies, or
//     KIMI_AUTH_TOKEN / kimi_auth_token env):
//     POST .../BillingService/GetUsages and
//     POST .../MembershipService/GetSubscriptionStats.
//
// The provider never touches the auth/cookie machinery itself; it only
// receives resolved Credentials from the fetch orchestrator.
package kimi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"leeway/internal/model"
	"leeway/internal/netutil"
	"leeway/internal/provider"
)

const (
	defaultUsageURL        = "https://www.kimi.com/apiv2/kimi.gateway.billing.v1.BillingService/GetUsages"
	defaultSubscriptionURL = "https://www.kimi.com/apiv2/kimi.gateway.membership.v2.MembershipService/GetSubscriptionStats"
	defaultCodeAPIBaseURL  = "https://api.kimi.com"
	codeBaseURLEnvKey      = "KIMI_CODE_BASE_URL"

	// subscriptionGraceSeconds mirrors KimiUsageFetcher.subscriptionGraceSeconds.
	subscriptionGraceSeconds = 2

	// Window constants mirror KimiProviderDescriptor.
	sessionWindowMinutes = 5 * 60       // secondary rate-limit lane
	weeklyWindowMinutes  = 7 * 24 * 60  // primary weekly lane
	monthlySentinel      = 30 * 24 * 60 // ProviderPaceCapability.monthlyWindowSentinelMinutes

	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
)

func init() { provider.Register(New()) }

// New returns the Kimi provider.
func New() provider.Provider { return newProvider(nil) }

// newProvider builds a provider; client and endpoint fields are injectable
// for tests. A nil client yields http.DefaultClient.
func newProvider(client *http.Client) kimiProvider {
	if client == nil {
		client = http.DefaultClient
	}
	return kimiProvider{
		client:            client,
		usageURL:          defaultUsageURL,
		subscriptionURL:   defaultSubscriptionURL,
		subscriptionGrace: subscriptionGraceSeconds * time.Second,
		now:               time.Now,
	}
}

type kimiProvider struct {
	client            *http.Client
	usageURL          string
	subscriptionURL   string
	codeBaseURL       string        // non-empty overrides env resolution (tests)
	subscriptionGrace time.Duration // grace for the best-effort subscription stats call
	now               func() time.Time
}

func (kimiProvider) ID() string          { return "kimi" }
func (kimiProvider) DisplayName() string { return "Kimi Code" }

func (kimiProvider) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{
		EnvAPIKey:       []string{"KIMI_CODE_API_KEY"},
		EnvAuthToken:    []string{"KIMI_AUTH_TOKEN", "kimi_auth_token"}, // KimiSettingsReader.authToken
		LocalAuthFiles:  []string{"~/.kimi-code/credentials/kimi-code.json"},
		LoginHint:       "run: kimi login",
		UsesCookies:     true,
		CookieDomains:   []string{"www.kimi.com", "kimi.com"},
		CookieFallbacks: []string{"kimi-desktop"},
	}
}

// DeclaredWindows mirrors KimiProviderDescriptor's pace lanes: primary is the
// weekly (7d) quota, secondary the 5-hour session rate limit.
func (kimiProvider) DeclaredWindows() []provider.WindowSpec {
	weekly := weeklyWindowMinutes
	session := sessionWindowMinutes
	return []provider.WindowSpec{
		{ID: "primary", Title: "Weekly", WindowMinutes: &weekly, Kind: provider.PercentWindow},
		{ID: "secondary", Title: "5-hour", WindowMinutes: &session, Kind: provider.PercentWindow},
	}
}

// Fetch resolves a usage snapshot. Credentials are resolved upstream; a
// missing web token and missing API key yield a clear error (the orchestrator
// maps it to not_configured before calling us, but we guard anyway).
func (p kimiProvider) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	webToken := ResolveWebToken(req.Creds)
	apiKey := strings.TrimSpace(req.Creds.APIKey)

	// Local CLI session: the Kimi Code CLI's OAuth session (kimi-code.json)
	// authenticates the Code API (api.kimi.com/coding) — the same endpoint as
	// the API key path — NOT the web console API. It auto-refreshes, so it is
	// preferred over a possibly-stale web cookie. A present-but-expired
	// session surfaces its own hint; a missing file falls through to the
	// generic missing-token error.
	var localErr error
	var cliToken string
	{
		tok, err := localCLIToken(ctx, p.now())
		switch {
		case err == nil:
			cliToken = tok
		case isExpiredSession(err):
			localErr = err
		case err != nil && !errors.Is(err, errSessionMissing):
			localErr = err
		}
	}

	switch {
	case apiKey != "":
		// API key path (KimiAPIFetchStrategy); enrichment mirrors
		// KimiWebEnrichmentTokenResolver, which only accepts a JWT-shaped token.
		return p.fetchCodeAPI(ctx, apiKey, enrichmentToken(webToken), req.Account)
	case cliToken != "":
		// CLI OAuth session path (kimiCodeAccessToken in Swift), self-refreshing.
		return p.fetchCodeAPI(ctx, cliToken, "", req.Account)
	case webToken != "":
		return p.fetchWeb(ctx, webToken, req.Account)
	case localErr != nil:
		return nil, localErr
	default:
		return nil, MissingToken()
	}
}

// fetchWeb mirrors KimiUsageFetcher.fetchUsage: the GetUsages call is
// required; GetSubscriptionStats runs concurrently and is best-effort with a
// grace period after the main fetch completes.
func (p kimiProvider) fetchWeb(ctx context.Context, token, account string) (*model.Snapshot, error) {
	sess := decodeSessionInfo(token)

	type subResult struct {
		stats *kimiSubscriptionStatsResponse
	}
	subCh := make(chan subResult, 1)
	go func() {
		stats, err := p.fetchSubscriptionStats(ctx, token, sess)
		if err != nil {
			// Only cancellation surfaces as an error here.
			return
		}
		subCh <- subResult{stats: stats}
	}()

	codingUsage, err := p.fetchRequiredUsage(ctx, token, sess)
	if err != nil {
		return nil, err
	}

	var stats *kimiSubscriptionStatsResponse
	select {
	case r := <-subCh:
		stats = r.stats
	case <-time.After(p.subscriptionGrace):
		log.Printf("kimi: subscription stats timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return p.buildFromUsage(codingUsage, stats, account)
}

// fetchRequiredUsage mirrors KimiUsageFetcher.fetchRequiredUsage.
func (p kimiProvider) fetchRequiredUsage(ctx context.Context, token string, sess *sessionInfo) (*kimiUsage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.usageURL,
		bytes.NewBufferString(`{"scope":["FEATURE_CODING"]}`))
	if err != nil {
		return nil, InvalidRequest(err.Error())
	}
	setWebHeaders(req, token, sess)

	data, status, err := netutil.DoRequest(ctx, p.client, req, 0)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, NetworkError(err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, InvalidToken()
		case http.StatusBadRequest:
			return nil, InvalidRequest("Bad request")
		default:
			return nil, APIError(fmt.Sprintf("HTTP %d", status))
		}
	}

	var usageResp kimiUsageResponse
	if err := json.Unmarshal(data, &usageResp); err != nil {
		return nil, ParseFailed(err.Error())
	}
	for i := range usageResp.Usages {
		if usageResp.Usages[i].Scope == "FEATURE_CODING" {
			return &usageResp.Usages[i], nil
		}
	}
	return nil, ParseFailed("FEATURE_CODING scope not found in response")
}

// fetchSubscriptionStats mirrors KimiUsageFetcher.fetchSubscriptionStats:
// best-effort — any non-200, transport or parse failure yields (nil, nil);
// only cancellation propagates.
func (p kimiProvider) fetchSubscriptionStats(ctx context.Context, token string, sess *sessionInfo) (*kimiSubscriptionStatsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.subscriptionURL,
		bytes.NewBufferString("{}"))
	if err != nil {
		return nil, err
	}
	setWebHeaders(req, token, sess)

	data, status, err := netutil.DoRequest(ctx, p.client, req, 0)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		log.Printf("kimi: subscription stats unavailable: %v", err)
		return nil, nil
	}
	if status != http.StatusOK {
		log.Printf("kimi: subscription stats returned %d", status)
		return nil, nil
	}
	var stats kimiSubscriptionStatsResponse
	if err := json.Unmarshal(data, &stats); err != nil {
		log.Printf("kimi: subscription stats unavailable: %v", err)
		return nil, nil
	}
	return &stats, nil
}

// fetchCodeAPI mirrors KimiUsageFetcher.fetchCodeAPIUsage for the plain API key
// strategy (identityHeaders empty — the X-Msh-* identity headers only appear
// on the CLI credential path, which leeway does not implement). Enrichment
// with a web token is best-effort.
func (p kimiProvider) fetchCodeAPI(ctx context.Context, apiKey, webToken, account string) (*model.Snapshot, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, MissingAPIKey()
	}
	endpoint, err := p.codeEndpoint()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, InvalidRequest(err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	data, status, err := netutil.DoRequest(ctx, p.client, req, 0)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, NetworkError(err)
	}
	if status != http.StatusOK {
		// Mirrors KimiUsageFetcher.codeAPIError.
		switch status {
		case http.StatusBadRequest:
			return nil, InvalidRequest("Bad request")
		case http.StatusUnauthorized:
			return nil, InvalidAPIKey()
		case http.StatusForbidden:
			return nil, APIError("HTTP 403 (permission or quota denied)")
		default:
			return nil, APIError(fmt.Sprintf("HTTP %d", status))
		}
	}

	var usageResp kimiCodeAPIUsageResponse
	if err := json.Unmarshal(data, &usageResp); err != nil {
		return nil, ParseFailed(err.Error())
	}
	snap, err := p.buildFromUsage(&kimiUsage{Scope: "FEATURE_CODING", Detail: usageResp.Usage, Limits: usageResp.Limits}, nil, account)
	if err != nil {
		return nil, err
	}

	if webToken == "" {
		return snap, nil
	}
	// Enrichment mirrors enrichCodeAPIUsage: best-effort, snapshot returned
	// unchanged when subscription stats are unavailable.
	sess := decodeSessionInfo(webToken)
	stats, err := p.fetchSubscriptionStats(ctx, webToken, sess)
	if err != nil {
		return nil, err // cancellation
	}
	if stats == nil {
		return snap, nil
	}
	return p.buildFromUsage(&kimiUsage{Scope: "FEATURE_CODING", Detail: usageResp.Usage, Limits: usageResp.Limits}, stats, account)
}

// buildFromUsage converts a Kimi usage payload into a model snapshot,
// rejecting a missing/malformed required weekly counter instead of silently
// presenting it as zero usage.
func (p kimiProvider) buildFromUsage(u *kimiUsage, stats *kimiSubscriptionStatsResponse, account string) (*model.Snapshot, error) {
	if u == nil {
		return nil, ParseFailed("FEATURE_CODING usage is missing")
	}
	if _, ok := parseCounts(u.Detail); !ok {
		return nil, ParseFailed("weekly usage counters are missing or invalid")
	}
	var rateLimitDetail *kimiUsageDetail
	var rateLimitWindow *kimiWindow
	if len(u.Limits) > 0 {
		rl := u.Limits[0]
		rateLimitDetail = &rl.Detail
		rateLimitWindow = &rl.Window
	}
	return buildSnapshot(u.Detail, rateLimitDetail, rateLimitWindow, stats, p.now(), account), nil
}

// codeEndpoint resolves the Kimi Code usage endpoint from the base URL,
// mirroring KimiUsageFetcher.codeAPIUsageEndpoint.
func (p kimiProvider) codeEndpoint() (string, error) {
	base := p.codeBaseURL
	if base == "" {
		var err error
		base, err = codeBaseURLFromEnv()
		if err != nil {
			return "", err
		}
	}
	return appendCodeUsagePath(base), nil
}

func codeBaseURLFromEnv() (string, error) {
	raw := os.Getenv(codeBaseURLEnvKey)
	if raw == "" {
		return defaultCodeAPIBaseURL, nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return "", InvalidRequest("Kimi Code API base URL must use HTTPS without user info")
	}
	return u.String(), nil
}

func appendCodeUsagePath(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + "/coding/v1/usages"
	}
	path := strings.Trim(u.Path, "/")
	switch {
	case path == "coding/v1" || strings.HasSuffix(path, "/coding/v1"):
		u.Path = strings.TrimRight(u.Path, "/") + "/usages"
	case path == "coding" || strings.HasSuffix(path, "/coding"):
		u.Path = strings.TrimRight(u.Path, "/") + "/v1/usages"
	default:
		u.Path = strings.TrimRight(u.Path, "/") + "/coding/v1/usages"
	}
	return u.String()
}

// setWebHeaders mirrors KimiUsageFetcher.webRequest.
func setWebHeaders(req *http.Request, token string, sess *sessionInfo) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Cookie", "kimi-auth="+token)
	req.Header.Set("Origin", "https://www.kimi.com")
	req.Header.Set("Referer", "https://www.kimi.com/code/console")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("connect-protocol-version", "1")
	req.Header.Set("x-language", "en-US")
	req.Header.Set("x-msh-platform", "web")
	req.Header.Set("r-timezone", timezoneID())
	if sess == nil {
		return
	}
	if sess.deviceID != "" {
		req.Header.Set("x-msh-device-id", sess.deviceID)
	}
	if sess.sessionID != "" {
		req.Header.Set("x-msh-session-id", sess.sessionID)
	}
	if sess.trafficID != "" {
		req.Header.Set("x-traffic-id", sess.trafficID)
	}
}

// timezoneID approximates Swift's TimeZone.current.identifier for the
// r-timezone header: TZ env, then the /etc/localtime symlink, else UTC.
func timezoneID() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			return target[i+len("zoneinfo/"):]
		}
		parts := strings.Split(target, "/")
		if len(parts) >= 2 {
			return strings.Join(parts[len(parts)-2:], "/")
		}
		if len(parts) == 1 {
			return parts[0]
		}
	}
	return "UTC"
}
