package grok

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// restBillingURL is the CLI chat-proxy credits endpoint. It is a var so tests
// can point at an httptest server.
var restBillingURL = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"

const restTokenAuth = "xai-grok-cli"

type restBillingResponse struct {
	Config restBillingConfig `json:"config"`
}

type restBillingConfig struct {
	CreditUsagePercent *float64         `json:"creditUsagePercent"`
	CurrentPeriod      restPeriod       `json:"currentPeriod"`
	BillingPeriodEnd   string           `json:"billingPeriodEnd"`
	OnDemandCap        *restOnDemandCap `json:"onDemandCap"`
}

type restOnDemandCap struct {
	Val json.RawMessage `json:"val"`
}

type restPeriod struct {
	Type string `json:"type"`
	End  string `json:"end"`
}

func fetchRESTBilling(ctx context.Context, cookieHeader, authorization string, client *http.Client) (billingSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, restBillingURL, nil)
	if err != nil {
		return billingSnapshot{}, err
	}
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	req.Header.Set("x-xai-token-auth", restTokenAuth)
	req.Header.Set("Accept", "application/json")
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
	return parseRESTBilling(data)
}

func parseRESTBilling(data []byte) (billingSnapshot, error) {
	var resp restBillingResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return billingSnapshot{}, &Error{Kind: kindParseFailed}
	}
	// proto-JSON omits zeros: a missing percent is 0, not a schema error.
	percent := 0.0
	if resp.Config.CreditUsagePercent != nil {
		percent = *resp.Config.CreditUsagePercent
	}
	snap := billingSnapshot{
		usedPercent: &percent,
		periodType:  resp.Config.CurrentPeriod.Type,
		fromREST:    true,
		onDemandCap: parseOnDemandCap(resp.Config.OnDemandCap),
	}
	if t, ok := parseRESTTime(resp.Config.BillingPeriodEnd); ok {
		snap.resetsAt = &t
	} else if t, ok := parseRESTTime(resp.Config.CurrentPeriod.End); ok {
		snap.resetsAt = &t
	}
	return snap, nil
}

func parseRESTTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseOnDemandCap(cap *restOnDemandCap) float64 {
	if cap == nil || len(cap.Val) == 0 {
		return 0
	}
	var n float64
	if err := json.Unmarshal(cap.Val, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(cap.Val, &s); err == nil {
		if n, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return n
		}
	}
	return 0
}

func windowFromPeriodType(periodType string) (title string, minutes *int, ok bool) {
	s := strings.ToLower(strings.TrimSpace(periodType))
	s = strings.TrimPrefix(s, "usage_period_type_")
	switch s {
	case "weekly":
		m := weeklyWindowMinutes
		return "Weekly", &m, true
	case "monthly":
		m := monthlyWindowMinutes
		return "Monthly", &m, true
	default:
		return "", nil, false
	}
}

// restUnusable reports REST 4xx/5xx or a transport failure. Parse errors stay
// on the REST path (no grpc-web fallback).
func restUnusable(err error) bool {
	var gerr *Error
	if errors.As(err, &gerr) {
		return gerr.Kind == kindRequestFailed
	}
	return err != nil
}
