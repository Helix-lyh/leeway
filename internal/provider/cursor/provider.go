// Package cursor implements the Cursor provider for leeway.
//
// Cursor exposes no public usage API; the web dashboard itself calls the
// internal Connect RPC endpoint aiserver.v1.DashboardService/
// GetCurrentPeriodUsage on api2.cursor.sh, which returns the current billing
// cycle's plan usage (included spend, limit, auto/API/total percent) and
// billing-cycle bounds. leeway reuses that endpoint.
//
// Sign-in is delegated to the official cursor-agent CLI
// (`leeway login cursor`): leeway runs it with
// AGENT_CLI_CREDENTIAL_STORE=file and a private HOME, then reads the
// auth.json the CLI wrote. The browser PKCE/poll protocol is not
// reimplemented. Each account has its own HOME, isolated from the official
// CLI/keychain and from every other leeway account.
//
// Token resolution order (per account):
//
//  1. CURSOR_ACCESS_TOKEN env var (explicit override / CI)
//  2. account.apiKey in ~/.config/leeway/config.json (named accounts)
//  3. official auth.json in this account's private HOME
//  4. macOS keychain "cursor-access-token" (fallback only when no private
//     auth.json exists, so a logged-in leeway account never mixes with
//     the official CLI's session)
package cursor

import (
	"context"
	"net/http"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

// Cursor's plan usage resets on a billing cycle (typically monthly); the
// table maps the primary window onto the Monthly column.
const monthlyWindowMinutes = 30 * 24 * 60

// New returns the Cursor provider.
func New() provider.Provider { return newProvider(defaultClient, time.Now) }

// newProvider builds a provider; client and now are injectable for tests.
// A nil client yields the shared 15s-timeout client.
func newProvider(client *http.Client, now func() time.Time) *cursorProvider {
	if client == nil {
		client = defaultClient
	}
	if now == nil {
		now = time.Now
	}
	return &cursorProvider{client: client, now: now}
}

type cursorProvider struct {
	client *http.Client
	now    func() time.Time
}

func (*cursorProvider) ID() string          { return "cursor" }
func (*cursorProvider) DisplayName() string { return "Cursor" }

// AuthSpec declares the token sources: env, account apiKey, then a private
// auth.json / keychain resolved inside Fetch. UsesKeychain lets the
// orchestrator attempt Fetch even before a private login exists, so a
// missing token surfaces as the login hint rather than not_configured.
func (*cursorProvider) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{
		EnvAuthToken:     []string{"CURSOR_ACCESS_TOKEN"},
		UsesKeychain:     true, // interactive sign-in token lives in the macOS keychain
		SupportsAccounts: true, // per-account token via account.apiKey
	}
}

// DeclaredWindows exposes the Monthly plan-usage lane (API bucket — the
// consumption that depletes a named-model plan) plus the Total and Auto
// buckets that remain visible in JSON output.
func (*cursorProvider) DeclaredWindows() []provider.WindowSpec {
	monthly := monthlyWindowMinutes
	return []provider.WindowSpec{
		{ID: "primary", Title: "Monthly", WindowMinutes: &monthly, Kind: provider.PercentWindow},
		{ID: "total", Title: "Total", Kind: provider.PercentWindow},
		{ID: "auto", Title: "Auto", Kind: provider.PercentWindow},
	}
}

// Fetch queries the current-period usage endpoint for one account. The
// snapshot carries the Monthly percent window (API-bucket usage) with the
// billing-cycle reset time, the Auto/Total bucket percentages, and a cash
// lane of remaining included quota (limit − includedSpend).
func (p *cursorProvider) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	token, err := resolveToken(ctx, req)
	if err != nil {
		return nil, err
	}
	data, err := p.fetchUsage(ctx, token)
	if err != nil {
		return nil, err
	}
	email := req.Email
	if email == "" {
		if got, err := p.fetchEmail(ctx, token); err == nil {
			email = got
		}
	}
	snap := data.toModel(req.Account, email, p.now())
	if plan := p.fetchPlanName(ctx, token); plan != "" {
		if snap.Identity == nil {
			snap.Identity = &model.Identity{}
		}
		snap.Identity.Plan = plan
	}
	if g := p.fetchGrants(ctx, token); g.ok {
		used, total := g.used, g.total
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID: "credits", Title: "Credits",
			Used: &used, Limit: &total, Currency: "USD",
		})
	}
	return snap, nil
}
