// Package deepseek implements the DeepSeek usage provider for leeway,
// mirroring CodexBar's DeepSeekUsageFetcher, DeepSeekUsageCostParser and
// DeepSeekProviderDescriptor.
//
// Two credential layers, matching CodexBar's API strategy:
//
//   - Required: DeepSeek API key (DEEPSEEK_API_KEY or DEEPSEEK_KEY, or
//     `leeway config set-api-key`). The wallet balance comes from
//     GET https://api.deepseek.com/user/balance with
//     Authorization: Bearer <apiKey> (platform.deepseek.com is WAF-protected
//     and rejects API-key calls).
//   - Optional enrichment: platform session token (DEEPSEEK_PLATFORM_TOKEN
//     or DEEPSEEK_USER_TOKEN). When present, per-day usage windows are
//     attached from GET /api/v0/usage/amount and GET /api/v0/usage/cost
//     (query params month/year, UTC). The API-key path works without it.
//
// The provider never touches the auth/cookie machinery; it only receives
// resolved Credentials from the fetch orchestrator and reads the optional
// platform token straight from the environment (AuthSpec has no slot for an
// optional secondary token).
package deepseek

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

// New returns the DeepSeek provider.
func New() provider.Provider { return newProvider(nil, nil) }

// newProvider builds a provider; client and now are injectable for tests.
// A nil client yields an HTTP client with the same 15s timeout as
// DeepSeekUsageFetcher.timeoutSeconds.
func newProvider(client *http.Client, now func() time.Time) *deepseekProvider {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	if now == nil {
		now = time.Now
	}
	return &deepseekProvider{
		client:         client,
		balanceURL:     defaultBalanceURL,
		usageAmountURL: defaultUsageAmountURL,
		usageCostURL:   defaultUsageCostURL,
		now:            now,
	}
}

type deepseekProvider struct {
	client         *http.Client
	balanceURL     string
	usageAmountURL string
	usageCostURL   string
	now            func() time.Time
}

func (deepseekProvider) ID() string          { return "deepseek" }
func (deepseekProvider) DisplayName() string { return "DeepSeek" }

// AuthSpec mirrors DeepSeekProviderDescriptor's credential adapter: an API
// key from the environment is the only required credential; no cookies, no
// accounts.
func (deepseekProvider) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{
		EnvAPIKey: []string{"DEEPSEEK_API_KEY", "DEEPSEEK_KEY"},
	}
}

// DeclaredWindows declares the balance lane (sessionLabel "Balance" in
// DeepSeekProviderDescriptor). Per-day usage windows are dynamic extras
// produced by Fetch when a platform token is present.
func (deepseekProvider) DeclaredWindows() []provider.WindowSpec {
	return []provider.WindowSpec{
		{ID: "primary", Title: "Balance", Kind: provider.BalanceWindow},
	}
}

// Fetch mirrors DeepSeekUsageFetcher.fetchUsage: the balance call is
// required; the detailed per-day usage (amount + cost endpoints) is optional
// enrichment that degrades to a balance-only snapshot on failure, like the
// Swift fetcher's joined optional summary.
func (p *deepseekProvider) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	apiKey := strings.TrimSpace(req.Creds.APIKey)
	if apiKey == "" {
		return nil, MissingCredentials()
	}

	data, err := p.get(ctx, p.balanceURL, apiKey, classifyAPIKeyStatus)
	if err != nil {
		return nil, err
	}
	bal, err := parseBalance(data)
	if err != nil {
		return nil, err
	}

	now := p.now().UTC()
	total := bal.totalBalance
	snap := &model.Snapshot{
		UpdatedAt: now,
		Identity:  &model.Identity{Account: req.Account},
		Windows: []model.RateWindow{{
			ID:       "primary",
			Title:    "Balance",
			Used:     &total,
			Currency: bal.currency,
		}},
		Cost: &model.Cost{Period: "Balance", Currency: bal.currency, Amount: &total},
	}

	if token := platformTokenFromEnv(); token != "" {
		month, year := usagePeriod(now)
		usage, err := p.fetchUsageSummary(ctx, token, month, year, now)
		if err != nil {
			// Best-effort, mirroring the Swift fetcher's optional-summary
			// join: a failure only degrades the snapshot, never fails it.
			log.Printf("deepseek: detailed usage unavailable: %v", err)
		} else {
			for _, d := range usage.daily {
				tokens := float64(d.totalTokens)
				snap.Windows = append(snap.Windows, model.RateWindow{
					ID:       "day-" + d.date,
					Title:    d.date,
					Used:     &tokens,
					Currency: usage.currency,
				})
			}
		}
	}
	return snap, nil
}
