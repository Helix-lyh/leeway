// Package grok implements the Grok provider. Fetch prefers the CLI chat-proxy
// REST billing API and falls back to grok.com's GetGrokCreditsConfig grpc-web
// endpoint when REST returns 4xx/5xx or is unreachable. The grpc-web payload
// is decoded with a heuristic protobuf scanner (no published schema).
package grok

import (
	"context"
	"net/http"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

const (
	weeklyWindowMinutes  = 7 * 24 * 60
	monthlyWindowMinutes = 30 * 24 * 60
)

// New returns the Grok provider.
func New() provider.Provider { return &grokProvider{client: defaultClient, now: time.Now} }

type grokProvider struct {
	// client is injectable for tests; production uses defaultClient.
	client *http.Client
	// now is the clock used for expiry checks and snapshot timestamps;
	// injectable so tests do not depend on wall time.
	now func() time.Time
}

func (*grokProvider) ID() string          { return "grok" }
func (*grokProvider) DisplayName() string { return "Grok" }
func (*grokProvider) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{
		OfficialCLI:      true, // `leeway login grok` runs the official grok CLI
		UsesCookies:      true,
		CookieDomains:    []string{"grok.com"},
		LocalAuthFiles:   []string{"~/.grok/auth.json"},
		LoginHint:        "leeway login grok",
		SupportsAccounts: true, // per-account auth.json via account.authFile
	}
}

// DeclaredWindows exposes the shared weekly subscription pool. Extra Usage
// Credits are a separate purchased balance and are not decoded by this
// provider's current protobuf scanner.
func (*grokProvider) DeclaredWindows() []provider.WindowSpec {
	weekly := weeklyWindowMinutes
	return []provider.WindowSpec{
		{ID: "primary", Title: "Weekly", WindowMinutes: &weekly, Kind: provider.PercentWindow},
	}
}

// Fetch probes CLI chat-proxy REST billing first, then grok.com grpc-web.
// Auth is a browser Cookie header, or the `grok login` local session (OIDC
// bearer, refreshed when expired). When the account declares an authFile
// (req.AuthFile), that session file is used instead of the default
// ~/.grok/auth.json — multi-account support.
func (g *grokProvider) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	var cookie, authorization string
	if req.Creds.CookieHeader != "" {
		cookie = req.Creds.CookieHeader
	} else {
		bearer, lerr := bearerTokenFromLocalSession(ctx, g.now(), req.AuthFile)
		if lerr != nil {
			// Classify local-session failures (missing/expired/unrefreshable)
			// as missing credentials so callers can distinguish them.
			return nil, &Error{Kind: kindMissingCredentials, Message: lerr.Error()}
		}
		authorization = "Bearer " + bearer
	}
	snap, err := fetchRESTBilling(ctx, cookie, authorization, g.client)
	if err != nil && ctx.Err() == nil && restUnusable(err) {
		snap, err = fetchWebBilling(ctx, cookie, authorization, g.client, g.now())
	}
	if err != nil {
		return nil, err
	}
	ident := req.Account
	if req.Email != "" {
		ident = req.Email
	}
	return snap.toModel(ident, g.now()), nil
}

// toModel projects the parsed billing snapshot into the leeway model. REST
// window title/length come from currentPeriod.type; grpc-web falls back to
// guessing from the reset distance. The reset timestamp is the end of the
// current cycle, not the cycle length.
func (s billingSnapshot) toModel(account string, now time.Time) *model.Snapshot {
	percent := *s.usedPercent
	title, minutes := s.primaryLabel(now)
	var resetsAt *time.Time
	if s.resetsAt != nil {
		r := *s.resetsAt
		resetsAt = &r
	}
	windows := []model.RateWindow{{
		ID:            "primary",
		Title:         title,
		UsedPercent:   &percent,
		ResetsAt:      resetsAt,
		WindowMinutes: minutes,
	}}
	if s.onDemandCap > 0 {
		cap := s.onDemandCap
		windows = append(windows, model.RateWindow{ID: "ondemand", Title: "On-demand", Used: &cap})
	}
	return &model.Snapshot{
		UpdatedAt: now,
		Identity:  &model.Identity{Account: account},
		Windows:   windows,
	}
}

func (s billingSnapshot) primaryLabel(now time.Time) (string, *int) {
	if s.fromREST {
		if title, minutes, ok := windowFromPeriodType(s.periodType); ok {
			return title, minutes
		}
		return "Credits", nil
	}
	return primaryLabelFromReset(s.resetsAt, now)
}

// primaryLabelFromReset is the grpc-web fallback: 4–12d → Weekly, 20–45d →
// Monthly, otherwise Credits. REST uses currentPeriod.type instead.
func primaryLabelFromReset(resetsAt *time.Time, now time.Time) (string, *int) {
	if resetsAt == nil {
		return "Credits", nil
	}
	days := resetsAt.Sub(now).Hours() / 24
	switch {
	case days >= 4 && days <= 12:
		m := weeklyWindowMinutes
		return "Weekly", &m
	case days >= 20 && days <= 45:
		m := monthlyWindowMinutes
		return "Monthly", &m
	default:
		return "Credits", nil
	}
}
