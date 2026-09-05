package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"leeway/internal/auth"
	"leeway/internal/model"
	"leeway/internal/netutil"
	"leeway/internal/provider"
)

// Connect RPC URLs; vars so tests can point at an httptest server.
var (
	usageURL  = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	planURL   = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetPlanInfo"
	grantsURL = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetCreditGrantsBalance"
)

var defaultClient = &http.Client{Timeout: 15 * time.Second}

// --- token resolution -----------------------------------------------------

// resolveToken resolves a per-account token: env override wins, then the
// account's apiKey (config), then the official auth.json the cursor-agent
// CLI wrote into this account's private HOME (when `leeway auth login
// --provider cursor` was used), then the macOS keychain the official Cursor
// client maintains. An account that holds a private auth.json never falls
// back to the keychain: the keychain tracks the official CLI's own (possibly
// different) account, so falling through would silently mix accounts.
func resolveToken(ctx context.Context, req provider.FetchRequest) (string, error) {
	if v := strings.TrimSpace(os.Getenv("CURSOR_ACCESS_TOKEN")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(req.Creds.APIKey); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(req.Creds.AuthToken); v != "" {
		return v, nil
	}
	if ca, ok, err := auth.LoadCursorAuth(req.Account); err != nil {
		return "", err
	} else if ok {
		if ca.AccessToken == "" {
			return "", errors.New("Cursor auth file is empty — re-run: leeway auth login --provider cursor" + accountSuffix(req.Account))
		}
		if exp, ok := auth.JWTExpiry(ca.AccessToken); ok && !exp.After(time.Now()) {
			return "", fmt.Errorf("Cursor session for account %q expired at %s — re-run: leeway auth login --provider cursor --account %s",
				req.Account, exp.Local().Format("2006-01-02 15:04"), req.Account)
		}
		return ca.AccessToken, nil
	}
	if v, err := keychainToken(ctx); err == nil && v != "" {
		return v, nil
	} else if err != nil && !auth.IsKeychainItemMissing(err) {
		return "", err
	}
	return "", errors.New("no Cursor token — set CURSOR_ACCESS_TOKEN, run `leeway auth login --provider cursor`, or sign in with the Cursor CLI")
}

// accountSuffix renders the --account flag for an account in hints.
func accountSuffix(account string) string {
	if account != "" && account != "default" {
		return " --account " + account
	}
	return ""
}

// keychainToken reads the macOS keychain entry the Cursor client maintains.
// It shells out to `security` (the only stable public API for keychain
// generic-password reads on macOS); a missing entry is reported as a plain
// "not found" so callers fall through to the error message above.
func keychainToken(ctx context.Context) (string, error) {
	tok, err := auth.CursorKeychainToken(ctx)
	if err != nil {
		if auth.IsKeychainItemMissing(err) {
			return "", err
		}
		return "", fmt.Errorf("keychain lookup failed: %w", err)
	}
	return tok, nil
}

// --- usage fetch ----------------------------------------------------------

// usageData is the decoded GetCurrentPeriodUsage response.
type usageData struct {
	BillingCycleStart string `json:"billingCycleStart"` // epoch millis
	BillingCycleEnd   string `json:"billingCycleEnd"`   // epoch millis
	PlanUsage         struct {
		TotalSpend      float64 `json:"totalSpend"`
		IncludedSpend   float64 `json:"includedSpend"`
		Limit           float64 `json:"limit"`
		AutoPercentUsed float64 `json:"autoPercentUsed"`
		APIPercentUsed  float64 `json:"apiPercentUsed"`
		TotalPercent    float64 `json:"totalPercentUsed"`
	} `json:"planUsage"`
	Enabled bool `json:"enabled"`
}

func (p *cursorProvider) connectPost(ctx context.Context, url, token string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "leeway")
	return netutil.DoRequest(ctx, p.client, req, netutil.MaxBodyBytes)
}

func (p *cursorProvider) fetchUsage(ctx context.Context, token string) (*usageData, error) {
	body, status, err := p.connectPost(ctx, usageURL, token)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, fmt.Errorf("Cursor token rejected (HTTP %d) — re-run: leeway auth login --provider cursor", status)
	case status < 200 || status >= 300:
		return nil, fmt.Errorf("Cursor usage request failed (HTTP %d): %.120s", status, strings.TrimSpace(string(body)))
	}
	var data usageData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parsing Cursor usage response: %w", err)
	}
	return &data, nil
}

func (p *cursorProvider) fetchPlanName(ctx context.Context, token string) string {
	body, status, err := p.connectPost(ctx, planURL, token)
	if err != nil || status < 200 || status >= 300 {
		return ""
	}
	var resp struct {
		PlanInfo struct {
			PlanName string `json:"planName"`
		} `json:"planInfo"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	return strings.TrimSpace(resp.PlanInfo.PlanName)
}

type creditGrants struct {
	used, total float64
	ok          bool
}

func (p *cursorProvider) fetchGrants(ctx context.Context, token string) creditGrants {
	body, status, err := p.connectPost(ctx, grantsURL, token)
	if err != nil || status < 200 || status >= 300 {
		return creditGrants{}
	}
	var resp struct {
		HasCreditGrants bool    `json:"hasCreditGrants"`
		TotalCents      float64 `json:"totalCents"`
		UsedCents       float64 `json:"usedCents"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || !resp.HasCreditGrants || resp.TotalCents <= 0 {
		return creditGrants{}
	}
	used := resp.UsedCents / 100
	if used < 0 {
		used = 0
	}
	return creditGrants{used: used, total: resp.TotalCents / 100, ok: true}
}

func (p *cursorProvider) fetchEmail(ctx context.Context, token string) (string, error) {
	return auth.LookupCursorEmail(ctx, token)
}

// toModel projects the usage payload onto the leeway snapshot. The billing
// cycle end is the reset time. Cursor splits usage into Auto (composer)
// and API (named models). The table shows those percents plus included
// spend in dollars (RPC amounts are cents). Total is omitted — it is just
// the two buckets mixed together.
func (d *usageData) toModel(account, email string, now time.Time) *model.Snapshot {
	ident := account
	if email != "" {
		ident = email
	}
	monthly := monthlyWindowMinutes
	snap := &model.Snapshot{
		UpdatedAt: now,
		Identity:  &model.Identity{Account: ident},
		Windows:   []model.RateWindow{},
	}

	// Dashboard's paid lane is named-model / API usage.
	api := d.PlanUsage.APIPercentUsed
	snap.Windows = append(snap.Windows, model.RateWindow{
		ID:            "primary",
		Title:         "Monthly",
		UsedPercent:   &api,
		ResetsAt:      millisToTime(d.BillingCycleEnd),
		WindowMinutes: &monthly,
	})
	if d.PlanUsage.AutoPercentUsed != 0 {
		auto := d.PlanUsage.AutoPercentUsed
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID: "auto", Title: "Auto", UsedPercent: &auto,
		})
	}
	// Amounts are cents. Show spend / included limit in dollars, not a
	// fake remaining field the RPC does not return.
	if d.PlanUsage.Limit > 0 {
		spend := d.PlanUsage.IncludedSpend / 100
		limit := d.PlanUsage.Limit / 100
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID: "included", Title: "Included",
			Used: &spend, Limit: &limit, Currency: "USD",
		})
	}
	return snap
}

// millisToTime converts an epoch-millis string (Cursor's timestamps are
// strings) into a time; an unparseable value yields nil (no reset shown).
func millisToTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	t := time.UnixMilli(ms)
	return &t
}
