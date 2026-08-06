// Package grok implements the Grok provider. It mirrors CodexBar's
// GrokWebBillingFetcher: a grpc-web POST to grok.com's GetGrokCreditsConfig
// endpoint authenticated with a browser Cookie header, whose binary protobuf
// payload is decoded with a heuristic wire-format scanner (the billing
// response has no published schema).
package grok

import (
	"context"
	"math"
	"net/http"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

// New returns the Grok provider.
func New() provider.Provider { return &grokProvider{client: defaultClient, now: time.Now} }

type grokProvider struct {
	// client is injectable for tests; production uses defaultClient.
	client *http.Client
	// now is the clock used for expiry checks and snapshot labeling;
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

// DeclaredWindows mirrors GrokProviderDescriptor: the primary lane is labeled
// "Credits" (sessionLabel) and the secondary lane "On-demand" (weeklyLabel).
// The web billing path only ever emits the primary window (CodexBar's
// toUsageSnapshot sets secondary to nil), so Fetch never produces a snapshot
// with an On-demand lane.
func (*grokProvider) DeclaredWindows() []provider.WindowSpec {
	return []provider.WindowSpec{
		{ID: "primary", Title: "Credits", Kind: provider.PercentWindow},
		{ID: "secondary", Title: "On-demand", Kind: provider.PercentWindow},
	}
}

// Fetch probes grok.com web billing with the resolved cookie header, falling
// back to the `grok login` local session (OIDC bearer, refreshed when
// expired), and returns a single primary credits window.
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

// toModel projects the parsed billing snapshot into the leeway model,
// mirroring GrokUsageSnapshot.toUsageSnapshot for the web path: a single
// primary window with usedPercent + resetsAt, no windowMinutes, and the title
// derived from the reset distance ("Weekly"/"Monthly"/"Credits").
func (s billingSnapshot) toModel(account string, now time.Time) *model.Snapshot {
	percent := *s.usedPercent
	title := "Credits"
	if s.resetsAt != nil {
		if label := primaryLabel(*s.resetsAt, now); label != "" {
			title = label
		}
	}
	var resetsAt *time.Time
	if s.resetsAt != nil {
		r := *s.resetsAt
		resetsAt = &r
	}
	return &model.Snapshot{
		UpdatedAt: now,
		Identity:  &model.Identity{Account: account},
		Windows: []model.RateWindow{{
			ID:          "primary",
			Title:       title,
			UsedPercent: &percent,
			ResetsAt:    resetsAt,
		}},
	}
}

// primaryLabel mirrors GrokProviderDescriptor.primaryLabel(resetsAt:now:):
// "Weekly" for 4-12 days until reset, "Monthly" for 20-45 days, otherwise ""
// (the caller falls back to the session label "Credits"). Note the Swift
// exact ranges: 13-19 days and 46+ days get no label at all.
func primaryLabel(resetsAt, now time.Time) string {
	seconds := resetsAt.Sub(now).Seconds()
	if seconds <= 3600 {
		return ""
	}
	days := int(math.Round(seconds / 86400))
	if days >= 4 && days <= 12 {
		return "Weekly"
	}
	if days >= 20 && days <= 45 {
		return "Monthly"
	}
	return ""
}
