// Package cursor implements the Cursor provider for leeway.
//
// Cursor exposes no public usage API; the web dashboard itself calls the
// internal Connect RPC endpoint aiserver.v1.DashboardService/
// GetCurrentPeriodUsage on api2.cursor.sh, which returns the current billing
// cycle's plan usage (included spend, remaining, limit) and the server-
// computed percent used for the auto and API buckets. leeway reuses that
// endpoint with the same token the cursor CLI/IDE keeps in the macOS
// keychain (service "cursor-access-token", account "cursor-user"), so no
// extra login is required on machines where cursor is already signed in.
//
// Token resolution order (per account):
//
//  1. CURSOR_ACCESS_TOKEN env var (explicit override / CI)
//  2. account.apiKey in ~/.config/leeway/config.json (named accounts)
//  3. macOS keychain "cursor-access-token" (the interactive sign-in)
//
// The provider is multi-account: each account is one token, defaulting to
// the keychain session. Configure extra accounts with
// `leeway config set-api-key --provider cursor --account NAME`.
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

// AuthSpec declares the token sources: an env var is checked first, then the
// account-level APIKey (config), then the macOS keychain. The provider itself
// performs keychain resolution inside Fetch because it depends on the account
// name (keychain only backs the default interactive session).
func (*cursorProvider) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{
		EnvAuthToken:     []string{"CURSOR_ACCESS_TOKEN"},
		UsesKeychain:     true,  // interactive sign-in token lives in the macOS keychain
		SupportsAccounts: true,  // per-account token via account.apiKey
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
// snapshot carries the Monthly percent window (server-computed total usage)
// with the billing-cycle reset time, the Auto/API bucket percentages, and a
// raw remaining/limit lane rendered as cash.
func (p *cursorProvider) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	token, err := resolveToken(req)
	if err != nil {
		return nil, err
	}
	data, err := p.fetchUsage(ctx, token)
	if err != nil {
		return nil, err
	}
	return data.toModel(req.Account, p.now()), nil
}
