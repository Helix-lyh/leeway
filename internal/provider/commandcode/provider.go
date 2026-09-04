// Package commandcode implements the Command Code provider for leeway.
//
// Auth is a subscription API key (user_… from commandcode.ai/studio), the
// same key the CLI and Provider API share. Multi-account is accounts[].apiKey,
// matching OpenCode Go's subscription-key model.
//
// Usage windows come from GET /alpha/billing/credits (windowLimits.fiveHour /
// weekly as $used/$cap, plus remaining monthlyCredits). whoami is best-effort
// org scoping; subscriptions is best-effort plan name and monthly ceiling.
// The credits JSON shape is the community-documented one used by CodexBar
// and pi-commandcode-provider; there is no official public schema.
package commandcode

import (
	"context"
	"net/http"
	"strings"
	"time"

	"leeway/internal/model"
	"leeway/internal/provider"
)

func init() { provider.Register(New()) }

const (
	defaultBaseURL = "https://api.commandcode.ai"
	requestTimeout = 15 * time.Second
)

// New returns the Command Code provider.
func New() provider.Provider { return newProvider(nil, "", nil) }

func newProvider(client *http.Client, baseURL string, now func() time.Time) *commandCode {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if now == nil {
		now = time.Now
	}
	return &commandCode{client: client, baseURL: baseURL, now: now}
}

type commandCode struct {
	client  *http.Client
	baseURL string
	now     func() time.Time
}

func (*commandCode) ID() string          { return "commandcode" }
func (*commandCode) DisplayName() string { return "Command Code" }

func (*commandCode) AuthSpec() provider.AuthSpec {
	return provider.AuthSpec{
		EnvAPIKey:        []string{"COMMAND_CODE_API_KEY", "COMMANDCODE_API_KEY"},
		SupportsAccounts: true,
		LoginHint:        "run: leeway auth login --provider commandcode",
	}
}

func (*commandCode) DeclaredWindows() []provider.WindowSpec {
	fiveHours := 5 * 60
	week := 7 * 24 * 60
	month := 30 * 24 * 60
	return []provider.WindowSpec{
		{ID: "primary", Title: "5-hour", WindowMinutes: &fiveHours, Kind: provider.BalanceWindow},
		{ID: "secondary", Title: "Weekly", WindowMinutes: &week, Kind: provider.BalanceWindow},
		{ID: "tertiary", Title: "Monthly", WindowMinutes: &month, Kind: provider.BalanceWindow},
	}
}

func (p *commandCode) Fetch(ctx context.Context, req provider.FetchRequest) (*model.Snapshot, error) {
	key := strings.TrimSpace(req.Creds.APIKey)
	if key == "" {
		return nil, errMissingKey()
	}
	now := p.now()

	orgID := ""
	if body, err := p.get(ctx, "/alpha/whoami", key, ""); err == nil {
		orgID = parseOrgID(body)
	}
	credits, err := p.get(ctx, "/alpha/billing/credits", key, orgID)
	if err != nil {
		return nil, err
	}
	u := parseUsage(credits)
	planID := ""
	if body, err := p.get(ctx, "/alpha/billing/subscriptions", key, orgID); err == nil {
		planID = parsePlanID(body)
	}
	if !u.fiveHour.ok && !u.weekly.ok && !u.monthly.ok && u.remaining == nil {
		return nil, errParse("missing usage windows")
	}
	return buildSnapshot(u, now, req.Account, planID), nil
}
