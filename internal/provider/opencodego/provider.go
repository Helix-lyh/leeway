// Package opencodego implements the OpenCode Go provider for leeway.
//
// It mirrors CodexBar's OpenCodeGoUsageFetcher / OpenCodeGoLocalUsageReader:
//
//   - per account: the opencode.ai web API (workspace usage page + the
//     opencode.ai/_server RPC for workspace IDs and the Zen billing balance)
//   - machine-level: the local opencode SQLite history in
//     ~/.local/share/opencode/opencode.db (read-only, never modified)
//
// The provider is multi-account: each account is one browser login session
// (a Cookie header) plus an optional workspace ID. When no workspace ID is
// configured the server is queried for the account's first workspace.
package opencodego

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

// New returns the OpenCode Go provider.
func New() provider.Provider { return newProvider() }

type opencodeGo struct {
	client    *http.Client
	local     *localCostReader
	base      string // "https://opencode.ai"
	server    string // "https://opencode.ai/_server"
	zenGoBase string // "https://opencode.ai/zen/go/v1"
}

func newProvider() *opencodeGo {
	return &opencodeGo{
		client:    newHTTPClient(),
		local:     &localCostReader{dbPath: defaultLocalDBPath(), now: time.Now},
		base:      baseURL,
		server:    serverURL,
		zenGoBase: zenGoBaseURL,
	}
}

type zenResult struct {
	balance *float64
	err     error
}

func (p *opencodeGo) ID() string          { return "opencode-go" }
func (p *opencodeGo) DisplayName() string { return "OpenCode Go" }

func (p *opencodeGo) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{
		EnvAPIKey:        []string{"OPENCODE_GO_API_KEY"},
		UsesCookies:      true,
		CookieDomains:    []string{"opencode.ai"},
		SupportsAccounts: true,
	}
}

func (p *opencodeGo) DeclaredWindows() []provider.WindowSpec {
	fiveHours := 5 * 60
	week := 7 * 24 * 60
	month := 30 * 24 * 60
	return []provider.WindowSpec{
		{ID: "primary", Title: "5-hour", WindowMinutes: &fiveHours, Kind: provider.PercentWindow},
		{ID: "secondary", Title: "Weekly", WindowMinutes: &week, Kind: provider.PercentWindow},
		{ID: "tertiary", Title: "Monthly", WindowMinutes: &month, Kind: provider.PercentWindow},
	}
}

// Fetch probes one account. An API-key subscription (req.Creds.APIKey) queries
// the gateway usage endpoint directly; a cookie account walks the workspace
// usage page + Zen balance path below.
func (p *opencodeGo) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	if req.Creds.APIKey != "" {
		return p.fetchByAPIKey(ctx, req.Creds.APIKey)
	}
	rawCookie := req.Creds.CookieHeader
	if strings.TrimSpace(rawCookie) == "" {
		return nil, fmt.Errorf("missing cookies — run: leeway cookie import (or: leeway cookie set --manual ...) for %q", p.ID())
	}
	cookieHeader, err := requestCookieHeader(rawCookie)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	// Resolve the workspace ID: configured value wins, otherwise the server
	// tells us (mirrors fetchUsage's workspaceIDOverride ?? fetchWorkspaceID).
	wsID := ""
	if override := normalizeWorkspaceID(req.Extra["workspaceID"]); override != "" {
		wsID = override
	} else {
		wsID, err = p.fetchWorkspaceID(ctx, cookieHeader)
		if err != nil {
			return nil, err
		}
	}

	// Zen balance is fetched concurrently with the usage page (mirrors the
	// Swift zenBalanceTask, including its 25ms start delay).
	zenStart := time.Now()
	zenCh := make(chan zenResult, 1)
	go func() {
		time.Sleep(optionalZenBalanceStartDelay)
		b, err := p.fetchZenBalance(ctx, wsID, cookieHeader)
		zenCh <- zenResult{balance: b, err: err}
	}()

	usageText, err := p.fetchUsagePage(ctx, wsID, cookieHeader)
	if err != nil {
		// Swift's requiredZenBalanceFallback: when the usage page carries no
		// parseable usage fields the Zen balance alone becomes the snapshot.
		if isMissingUsageFields(err) {
			return p.requiredZenFallback(zenCh, err, now)
		}
		return nil, err
	}
	sub, perr := parseSubscription(usageText, now)
	if perr != nil {
		// Same fallback for the parse path (missing usage fields).
		if !isMissingUsageFields(perr) {
			return nil, perr
		}
		return p.requiredZenFallback(zenCh, perr, now)
	}

	// Optional join: give the balance up to optionalZenBalanceTimeout measured
	// from when it started (Swift's optionalZenBalanceJoinTimeout with
	// waitForZenBalance = true; a CLI usage read wants completeness).
	var zen *float64
	budget := optionalZenBalanceTimeout - time.Since(zenStart)
	if budget > 0 {
		select {
		case zr := <-zenCh:
			if zr.err == nil {
				zen = zr.balance
			}
		case <-time.After(budget):
		}
	}
	return p.buildSnapshot(sub, now, zen), nil
}

// fetchByAPIKey queries one subscription's usage windows (rolling 5-hour /
// Weekly / Monthly) via the OpenAI-compatible gateway with the subscription
// API key. No Zen balance is available on this surface.
func (p *opencodeGo) fetchByAPIKey(ctx context.Context, apiKey string) (*model.Snapshot, error) {
	now := time.Now()
	text, err := p.fetchUsageByKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	sub, err := parseAPIUsage(text, now)
	if err != nil {
		return nil, err
	}
	return p.buildSnapshot(sub, now, nil), nil
}

// requiredZenFallback mirrors Swift's requiredZenBalanceFallback: the Zen
// balance is awaited without the optional join budget; a failed or absent
// balance rethrows the original usage error.
func (p *opencodeGo) requiredZenFallback(zenCh chan zenResult, fallbackErr error, now time.Time) (*model.Snapshot, error) {
	zr := <-zenCh
	if zr.err != nil {
		return nil, zr.err
	}
	if zr.balance == nil {
		return nil, fallbackErr
	}
	return p.buildSnapshot(balanceOnly(), now, zr.balance), nil
}

// FetchLocalCost implements provider-local machine-level cost history
// (LocalCostProvider): sum of the latest calendar-day bucket from the local
// opencode.db. A missing database or empty history yields (nil, nil) so the
// orchestrator simply leaves LocalCost unset.
func (p *opencodeGo) FetchLocalCost(ctx context.Context) (*model.Cost, error) {
	return p.local.fetch(ctx)
}

func (p *opencodeGo) buildSnapshot(sub *subscription, now time.Time, zen *float64) *model.Snapshot {
	snap := &model.Snapshot{UpdatedAt: now}
	if zen != nil {
		amount := *zen
		snap.Cost = &model.Cost{Period: "Zen balance", Currency: "USD", Amount: &amount}
	}
	if sub.isBalanceOnly {
		return snap
	}

	rollingReset := now.Add(time.Duration(sub.rollingResetSec) * time.Second)
	fiveHours := 5 * 60
	snap.Windows = append(snap.Windows, model.RateWindow{
		ID: "primary", Title: "5-hour",
		UsedPercent:   &sub.rollingPercent,
		ResetsAt:      &rollingReset,
		WindowMinutes: &fiveHours,
	})
	if sub.hasWeeklyUsage {
		weeklyReset := now.Add(time.Duration(sub.weeklyResetSec) * time.Second)
		week := 7 * 24 * 60
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID: "secondary", Title: "Weekly",
			UsedPercent:   &sub.weeklyPercent,
			ResetsAt:      &weeklyReset,
			WindowMinutes: &week,
		})
	}
	if sub.hasMonthlyUsage {
		monthlyReset := now.Add(time.Duration(sub.monthlyResetSec) * time.Second)
		month := 30 * 24 * 60
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID: "tertiary", Title: "Monthly",
			UsedPercent:   &sub.monthlyPercent,
			ResetsAt:      &monthlyReset,
			WindowMinutes: &month,
		})
	}
	if sub.renewsAt != nil {
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID:       "renewal",
			Title:    "Renews",
			ResetsAt: sub.renewsAt,
		})
	}
	return snap
}
