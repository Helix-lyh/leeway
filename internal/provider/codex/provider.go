// Package codex implements the Codex provider of leeway.
//
// It reuses the Codex CLI's login session (~/.codex/auth.json, resolved by the
// fetch orchestrator into Credentials.CodexAuth) and probes the ChatGPT
// backend API the same way CodexBar's OAuth strategy does: identity from
// GET https://chatgpt.com/backend-api/accounts, usage from
// GET https://chatgpt.com/backend-api/wham/usage (the base URL honors
// `chatgpt_base_url` in ~/.codex/config.toml).
package codex

import (
	"context"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

// New returns the Codex provider.
func New() provider.Provider { return codexProvider{} }

type codexProvider struct{}

func (codexProvider) ID() string          { return "codex" }
func (codexProvider) DisplayName() string { return "Codex" }

// AuthSpec declares that credentials come from the Codex CLI session
// (~/.codex/auth.json).
func (codexProvider) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{UsesCodexCLI: true}
}

// DeclaredWindows mirrors CodexProviderDescriptor's metadata lanes:
// sessionLabel "Session" (5h window), weeklyLabel "Weekly" (7d window).
// CodexBar never produces a tertiary lane, so none is declared.
func (codexProvider) DeclaredWindows() []provider.WindowSpec {
	session := sessionWindowMinutes
	weekly := weeklyWindowMinutes
	return []provider.WindowSpec{
		{ID: "primary", Title: "Session", WindowMinutes: &session, Kind: provider.PercentWindow},
		{ID: "secondary", Title: "Weekly", WindowMinutes: &weekly, Kind: provider.PercentWindow},
	}
}

// Fetch mirrors CodexOAuthFetchStrategy.fetch: identity from the accounts
// endpoint, usage windows from the wham/usage response, and error
// classification from CodexOAuthFetchError.
func (codexProvider) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	cx := req.Creds.CodexAuth
	if cx == nil || cx.AccessToken == "" {
		// Defensive backstop: the orchestrator normally reports
		// "not_configured" before Fetch is called.
		return nil, errMissingSession
	}
	return fetchCodex(ctx, newAPIClient(), cx.AccessToken, cx.AccountID)
}

// fetchCodex runs one fetch with an injectable client (tests override the
// endpoints). Mirrors CodexReconciledState.fromOAuth + CodexOAuthFetchStrategy.makeResult.
func fetchCodex(ctx context.Context, c *apiClient, accessToken, accountID string) (*model.Snapshot, error) {
	usage, err := c.fetchUsage(ctx, accessToken, accountID)
	if err != nil {
		return nil, err
	}

	primary, secondary := windowsFromUsage(usage)
	extras := extraRateWindows(usage.additionalRateLimits)

	// Mirror makeResult: extra windows never resurrect a snapshot on their
	// own; without primary/secondary windows the snapshot survives only when
	// credits are present.
	hasCredits := usage.credits != nil && usage.credits.balance != nil
	if primary == nil && secondary == nil && !hasCredits {
		return nil, errNoRateLimitsFound
	}

	snap := &model.Snapshot{
		UpdatedAt: time.Now(),
		Windows:   []model.RateWindow{},
	}
	if primary != nil {
		snap.Windows = append(snap.Windows, *primary)
	}
	if secondary != nil {
		snap.Windows = append(snap.Windows, *secondary)
	}
	snap.Windows = append(snap.Windows, extras...)

	if hasCredits {
		snap.Cost = &model.Cost{Period: "Credits", Currency: "USD", Amount: usage.credits.balance}
	}
	snap.Identity = c.resolveIdentity(ctx, accessToken, accountID)
	return snap, nil
}

// windowsFromUsage maps the raw rate_limit windows through the
// CodexRateWindowNormalizer roles and assigns the primary/secondary lanes.
func windowsFromUsage(usage *usageResponse) (primary, secondary *model.RateWindow) {
	if usage == nil || usage.rateLimit == nil {
		return nil, nil
	}
	return normalizeWindows(
		windowFromSnapshot(usage.rateLimit.primaryWindow),
		windowFromSnapshot(usage.rateLimit.secondaryWindow))
}
