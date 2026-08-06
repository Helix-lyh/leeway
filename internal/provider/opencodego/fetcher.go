package opencodego

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"leeway/internal/netutil"
)

// Server endpoints and constants, mirroring OpenCodeGoUsageFetcher.
const (
	baseURL            = "https://opencode.ai"
	serverURL          = "https://opencode.ai/_server"
	workspacesServerID = "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f"
	billingServerID    = "c83b78a614689c38ebee981f9b39a8b377716db85c1fd7dbab604adc02d3313d"

	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"

	optionalZenBalanceStartDelay = 25 * time.Millisecond
	optionalZenBalanceTimeout    = 5 * time.Second
)

type serverRequest struct {
	serverID string
	args     *string
	method   string
	referer  string
}

// newHTTPClient mirrors the Swift redirect-guard session: only same-host
// https redirects are followed (cookies are preserved on same-host hops).
func newHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			prev := via[len(via)-1].URL
			if req.URL.Host != prev.Host || req.URL.Scheme != "https" {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func (p *opencodeGo) fetchWorkspaceID(ctx context.Context, cookieHeader string) (string, error) {
	text, err := p.fetchServerText(ctx, serverRequest{
		serverID: workspacesServerID,
		args:     nil,
		method:   "GET",
		referer:  p.base,
	}, cookieHeader)
	if err != nil {
		return "", err
	}
	if looksSignedOut(text) {
		return "", invalidCredentials()
	}
	ids := parseWorkspaceIDs(text)
	if len(ids) == 0 {
		ids = parseWorkspaceIDsFromJSON(text)
	}
	if len(ids) > 0 {
		return ids[0], nil
	}

	// GET found no workspace IDs; retry with POST (mirrors the Swift fallback).
	args := "[]"
	fallback, err := p.fetchServerText(ctx, serverRequest{
		serverID: workspacesServerID,
		args:     &args,
		method:   "POST",
		referer:  p.base,
	}, cookieHeader)
	if err != nil {
		return "", err
	}
	if looksSignedOut(fallback) {
		return "", invalidCredentials()
	}
	ids = parseWorkspaceIDs(fallback)
	if len(ids) == 0 {
		ids = parseWorkspaceIDsFromJSON(fallback)
	}
	if len(ids) == 0 {
		return "", parseErrorf("Missing workspace id.")
	}
	return ids[0], nil
}

func (p *opencodeGo) fetchUsagePage(ctx context.Context, wsID, cookieHeader string) (string, error) {
	pageURL := p.base + "/workspace/" + url.PathEscape(wsID) + "/go"
	text, err := p.fetchPageText(ctx, pageURL, cookieHeader)
	if err != nil {
		return "", err
	}
	if looksSignedOut(text) {
		return "", invalidCredentials()
	}
	if !usageFieldsPresent(text) {
		return "", parseErrorf("Missing usage fields.")
	}
	return text, nil
}

func (p *opencodeGo) fetchZenBalance(ctx context.Context, wsID, cookieHeader string) (*float64, error) {
	pageText, err := p.fetchPageText(ctx, p.zenDashboardURL(wsID), cookieHeader)
	if err != nil {
		return nil, err
	}
	if looksSignedOut(pageText) {
		return nil, invalidCredentials()
	}
	if balance := parseZenBalance(pageText); balance != nil {
		return balance, nil
	}

	// Fall back to the billing server RPC.
	billingText, err := p.fetchZenBillingText(ctx, wsID, cookieHeader)
	if err != nil {
		return nil, err
	}
	if looksSignedOut(billingText) {
		return nil, invalidCredentials()
	}
	return parseZenBillingResponse(billingText), nil
}

func (p *opencodeGo) fetchZenBillingText(ctx context.Context, wsID, cookieHeader string) (string, error) {
	// Marshal the args so a workspace ID with quotes/backslashes cannot
	// produce malformed JSON in the _server RPC args.
	raw, err := json.Marshal([]string{wsID})
	if err != nil {
		return "", apiErrorf("workspace ID not serializable: %v", err)
	}
	argsData := string(raw)
	return p.fetchServerText(ctx, serverRequest{
		serverID: billingServerID,
		args:     &argsData,
		method:   "GET",
		referer:  p.zenDashboardURL(wsID),
	}, cookieHeader)
}

func (p *opencodeGo) zenDashboardURL(wsID string) string {
	return p.base + "/workspace/" + url.PathEscape(wsID)
}

// fetchServerText mirrors fetchServerText: the opencode.ai/_server RPC with
// the X-Server-Id / X-Server-Instance headers.
func (p *opencodeGo) fetchServerText(ctx context.Context, sr serverRequest, cookieHeader string) (string, error) {
	var reqURL string
	if sr.method == "GET" {
		reqURL = p.server + "?id=" + url.QueryEscape(sr.serverID)
		if sr.args != nil && *sr.args != "" {
			reqURL += "&args=" + url.QueryEscape(*sr.args)
		}
	} else {
		reqURL = p.server
	}
	req, err := http.NewRequestWithContext(ctx, sr.method, reqURL, nil)
	if err != nil {
		return "", networkErrorf("%v", err)
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("X-Server-Id", sr.serverID)
	req.Header.Set("X-Server-Instance", "server-fn:"+randomUUID())
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", p.base)
	req.Header.Set("Referer", sr.referer)
	req.Header.Set("Accept", "text/javascript, application/json;q=0.9, */*;q=0.8")
	if sr.method != "GET" && sr.args != nil {
		req.Body = io.NopCloser(strings.NewReader(*sr.args))
		req.ContentLength = int64(len(*sr.args))
		req.Header.Set("Content-Type", "application/json")
	}
	return p.doRequest(ctx, req)
}

// fetchPageText mirrors fetchPageText: plain page GETs (workspace dashboard,
// usage page) with the HTML accept header.
func (p *opencodeGo) fetchPageText(ctx context.Context, pageURL, cookieHeader string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return "", networkErrorf("%v", err)
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	return p.doRequest(ctx, req)
}

func (p *opencodeGo) doRequest(ctx context.Context, req *http.Request) (string, error) {
	body, status, err := netutil.DoRequest(ctx, p.client, req, 0)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", networkErrorf("%v", err)
	}
	if status != http.StatusOK {
		bodyText := string(body)
		if looksSignedOut(bodyText) {
			return "", invalidCredentials()
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return "", invalidCredentials()
		}
		if message := extractServerErrorMessage(bodyText); message != "" {
			return "", apiErrorf("HTTP %d: %s", status, message)
		}
		return "", apiErrorf("HTTP %d", status)
	}
	if !utf8.Valid(body) {
		return "", parseErrorf("Response was not UTF-8.")
	}
	return string(body), nil
}

func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand never fails on supported platforms; keep a fallback so
		// the header is still well-formed.
		for i := range b {
			b[i] = byte(i * 37)
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
