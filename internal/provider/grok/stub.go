// Package grok implements the Grok provider. It mirrors CodexBar's
// GrokWebBillingFetcher: a grpc-web POST to grok.com's GetGrokCreditsConfig
// endpoint authenticated with a browser Cookie header, whose binary protobuf
// payload is decoded with a heuristic wire-format scanner (the billing
// response has no published schema).
package grok

import (
	"context"
	"net/http"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

// Grok's paid-plan allowance is one shared weekly pool across Build, Chat,
// Imagine, Voice, and API. The billing endpoint returns the used percentage
// and the current weekly reset timestamp, not a duration discriminator.
const weeklyWindowMinutes = 7 * 24 * 60

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
		UsesCookies:    true,
		CookieDomains:  []string{"grok.com"},
		LocalAuthFiles: []string{"~/.grok/auth.json"},
		LoginHint:      "run: grok login",
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

// Fetch probes grok.com web billing with the resolved cookie header, falling
// back to the `grok login` local session (OIDC bearer, refreshed when
// expired), and returns the shared weekly subscription window.
func (g *grokProvider) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	var snap billingSnapshot
	var err error
	switch {
	case req.Creds.CookieHeader != "":
		snap, err = fetchWebBilling(ctx, req.Creds.CookieHeader, "", g.client, g.now())
	default:
		bearer, lerr := bearerTokenFromLocalSession(ctx, g.now())
		if lerr != nil {
			// Classify local-session failures (missing/expired/unrefreshable)
			// as missing credentials so callers can distinguish them.
			return nil, &Error{Kind: kindMissingCredentials, Message: lerr.Error()}
		}
		snap, err = fetchWebBilling(ctx, "", "Bearer "+bearer, g.client, g.now())
	}
	if err != nil {
		return nil, err
	}
	return snap.toModel(req.Account, g.now()), nil
}

// toModel projects the parsed billing snapshot into the leeway model. The
// endpoint's reset timestamp is the end of the current weekly cycle; the time
// remaining until that timestamp must not be mistaken for the cycle length.
func (s billingSnapshot) toModel(account string, now time.Time) *model.Snapshot {
	percent := *s.usedPercent
	weekly := weeklyWindowMinutes
	var resetsAt *time.Time
	if s.resetsAt != nil {
		r := *s.resetsAt
		resetsAt = &r
	}
	return &model.Snapshot{
		UpdatedAt: now,
		Identity:  &model.Identity{Account: account},
		Windows: []model.RateWindow{{
			ID:            "primary",
			Title:         "Weekly",
			UsedPercent:   &percent,
			ResetsAt:      resetsAt,
			WindowMinutes: &weekly,
		}},
	}
}
