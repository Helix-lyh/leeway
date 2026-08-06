package deepseek

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"leeway/internal/netutil"
)

const (
	// Endpoints mirror DeepSeekUsageFetcher (balance URL per leeway's spec:
	// the user summary endpoint carries the API-key balance; the Swift
	// fetcher used api.deepseek.com/user/balance for the same payload).
	// Balance comes from the public API endpoint (Swift's API-key path);
	// platform.deepseek.com is WAF-protected and rejects API-key calls.
	defaultBalanceURL     = "https://api.deepseek.com/user/balance"
	defaultUsageAmountURL = "https://platform.deepseek.com/api/v0/usage/amount"
	defaultUsageCostURL   = "https://platform.deepseek.com/api/v0/usage/cost"

	// requestTimeout mirrors DeepSeekUsageFetcher.timeoutSeconds (15).
	requestTimeout = 15 * time.Second

	platformTokenEnvKeys = "DEEPSEEK_PLATFORM_TOKEN,DEEPSEEK_USER_TOKEN"
)

// usagePeriod mirrors DeepSeekUsageFetcher.usagePeriod: the current UTC
// month and year used as query params for the usage endpoints (the Swift
// fetcher uses a gregorian calendar in the GMT time zone).
func usagePeriod(now time.Time) (month, year int) {
	return int(now.Month()), now.Year()
}

// platformTokenFromEnv mirrors DeepSeekSettingsReader.platformToken: the
// optional platform session token used to enrich the snapshot with per-day
// usage. The API-key path must work without it.
func platformTokenFromEnv() string {
	for _, key := range strings.Split(platformTokenEnvKeys, ",") {
		if v := cleanSecret(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// cleanSecret mirrors DeepSeekSettingsReader.cleaned: strips a matching pair
// of surrounding quotes and trims whitespace.
func cleanSecret(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
	}
	return strings.TrimSpace(v)
}

// statusClassifier maps a non-200 status to a typed error.
type statusClassifier func(status int) error

// classifyAPIKeyStatus classifies balance-endpoint failures. The Swift
// fetcher only reports the status; leeway additionally classifies 401 as an
// invalid API key.
func classifyAPIKeyStatus(status int) error {
	if status == http.StatusUnauthorized {
		return InvalidAPIKey()
	}
	return APIError(fmt.Sprintf("HTTP %d", status))
}

// classifyPlatformTokenStatus mirrors DeepSeekUsageFetcher.fetchAmount /
// fetchCost: 401 and 403 mean the platform session is missing or expired.
func classifyPlatformTokenStatus(status int) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return InvalidPlatformToken()
	}
	return APIError(fmt.Sprintf("HTTP %d", status))
}

// get performs one GET with Authorization: Bearer <token> and
// Accept: application/json, mirroring the Swift fetcher's request setup.
func (p *deepseekProvider) get(ctx context.Context, endpoint, token string, classify statusClassifier) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, NetworkError(fmt.Errorf("invalid URL %q", endpoint))
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	data, status, err := netutil.DoRequest(ctx, p.client, req, 0)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, NetworkError(err)
	}
	if status != http.StatusOK {
		return nil, classify(status)
	}
	return data, nil
}

// fetchUsageSummary mirrors DeepSeekUsageFetcher.fetchUsageSummary: the
// amount and cost payloads are fetched concurrently; both are required. The
// month/year query params are UTC-based.
func (p *deepseekProvider) fetchUsageSummary(ctx context.Context, platformToken string, month, year int, now time.Time) (*usageSummary, error) {
	amountURL := p.usageAmountURL + "?" + usageQuery(month, year).Encode()
	costURL := p.usageCostURL + "?" + usageQuery(month, year).Encode()

	// Each result is tagged with its role so that completion order cannot
	// swap the two payloads (the Swift fetcher tags them with
	// UsageSummaryPayload.amount/.cost).
	type payloadResult struct {
		role string
		data []byte
		err  error
	}
	results := make(chan payloadResult, 2)
	fetch := func(role, endpoint string) {
		data, err := p.get(ctx, endpoint, platformToken, classifyPlatformTokenStatus)
		results <- payloadResult{role: role, data: data, err: err}
	}
	go fetch("amount", amountURL)
	go fetch("cost", costURL)

	var amountData, costData []byte
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			return nil, r.err
		}
		switch r.role {
		case "amount":
			amountData = r.data
		default:
			costData = r.data
		}
	}
	return parseUsageSummary(amountData, costData, now)
}

func usageQuery(month, year int) url.Values {
	q := url.Values{}
	q.Set("month", strconv.Itoa(month))
	q.Set("year", strconv.Itoa(year))
	return q
}
