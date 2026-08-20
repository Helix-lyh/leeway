package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"leeway/internal/model"
	"leeway/internal/netutil"
	"leeway/internal/provider"
)

// usageURL is the internal Connect RPC endpoint the web dashboard calls with
// the OAuth bearer token; the response carries the current billing cycle's
// plan usage. It is a var so tests can point at an httptest server.
var usageURL = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetCurrentPeriodUsage"

// keychainTokenService/Account identify the token the cursor IDE/CLI keeps in
// the macOS keychain (same service names the official client uses).
const (
	keychainService = "cursor-access-token"
	keychainAccount = "cursor-user"
)

var defaultClient = &http.Client{Timeout: 15 * time.Second}

// --- token resolution -----------------------------------------------------

// resolveToken resolves a per-account token: env override wins, then the
// account's apiKey (config), then the macOS keychain interactive session.
func resolveToken(req provider.FetchRequest) (string, error) {
	if v := strings.TrimSpace(os.Getenv("CURSOR_ACCESS_TOKEN")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(req.Creds.APIKey); v != "" {
		return v, nil
	}
	if v, err := keychainToken(); err == nil && v != "" {
		return v, nil
	}
	return "", errors.New("no Cursor token — set CURSOR_ACCESS_TOKEN, configure account apiKey (leeway config set-api-key --provider cursor --account NAME), or sign in with the Cursor CLI")
}

// keychainToken reads the macOS keychain entry the Cursor client maintains.
// It shells out to `security` (the only stable public API for keychain
// generic-password reads on macOS); a missing entry is reported as a plain
// "not found" so callers fall through to the error message above.
func keychainToken() (string, error) {
	if _, err := exec.LookPath("security"); err != nil {
		return "", fmt.Errorf("security CLI unavailable: %w", err)
	}
	cmd := exec.Command("security", "find-generic-password",
		"-a", keychainAccount, "-s", keychainService, "-w")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("keychain lookup failed: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

// --- usage fetch ----------------------------------------------------------

// usageData is the decoded GetCurrentPeriodUsage response.
type usageData struct {
	BillingCycleStart string `json:"billingCycleStart"` // epoch millis
	BillingCycleEnd   string `json:"billingCycleEnd"`   // epoch millis
	PlanUsage         struct {
		TotalSpend      float64 `json:"totalSpend"`
		IncludedSpend   float64 `json:"includedSpend"`
		Remaining       float64 `json:"remaining"`
		Limit           float64 `json:"limit"`
		AutoPercentUsed float64 `json:"autoPercentUsed"`
		APIPercentUsed  float64 `json:"apiPercentUsed"`
		TotalPercent    float64 `json:"totalPercentUsed"`
	} `json:"planUsage"`
	Enabled bool `json:"enabled"`
}

func (p *cursorProvider) fetchUsage(ctx context.Context, token string) (*usageData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, usageURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cursor-agent/2026.08.11")

	body, status, err := netutil.DoRequest(ctx, p.client, req, netutil.MaxBodyBytes)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, fmt.Errorf("Cursor token rejected (HTTP %d) — re-run the Cursor CLI login or set CURSOR_ACCESS_TOKEN", status)
	case status < 200 || status >= 300:
		return nil, fmt.Errorf("Cursor usage request failed (HTTP %d): %.120s", status, strings.TrimSpace(string(body)))
	}
	var data usageData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parsing Cursor usage response: %w", err)
	}
	return &data, nil
}

// toModel projects the usage payload onto the leeway snapshot. The billing
// cycle end is the reset time; the Monthly window carries the server-computed
// total percent. Auto/API buckets are kept as extra percent windows (JSON
// only — the table projects one canonical window per period). The remaining/
// limit balance renders as the cash lane.
func (d *usageData) toModel(account string, now time.Time) *model.Snapshot {
	monthly := monthlyWindowMinutes
	snap := &model.Snapshot{
		UpdatedAt: now,
		Identity:  &model.Identity{Account: account},
		Windows:   []model.RateWindow{},
	}

	total := d.PlanUsage.TotalPercent
	snap.Windows = append(snap.Windows, model.RateWindow{
		ID:            "primary",
		Title:         "Monthly",
		UsedPercent:   &total,
		ResetsAt:      millisToTime(d.BillingCycleEnd),
		WindowMinutes: &monthly,
	})

	auto := d.PlanUsage.AutoPercentUsed
	api := d.PlanUsage.APIPercentUsed
	snap.Windows = append(snap.Windows, model.RateWindow{
		ID: "auto", Title: "Auto", UsedPercent: &auto,
	})
	snap.Windows = append(snap.Windows, model.RateWindow{
		ID: "api", Title: "API", UsedPercent: &api,
	})

	if d.PlanUsage.Limit > 0 {
		remaining := d.PlanUsage.Remaining
		limit := d.PlanUsage.Limit
		snap.Windows = append(snap.Windows, model.RateWindow{
			ID: "credits", Title: "Credits",
			Used: &remaining, Limit: &limit,
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
