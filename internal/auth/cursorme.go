package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"leeway/internal/netutil"
)

// CursorMeURL is DashboardService/GetMe. The session JWT has no email
// claim; this RPC is how the dashboard (and cursor-agent status) learns
// the signed-in address. A var so tests can point at an httptest server.
var CursorMeURL = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetMe"

var cursorMeClient = &http.Client{Timeout: 15 * time.Second}

// LookupCursorEmail returns the account email for a session bearer.
// Empty string means the RPC succeeded but reported no email.
func LookupCursorEmail(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, CursorMeURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("User-Agent", "leeway")

	body, status, err := netutil.DoRequest(ctx, cursorMeClient, req, netutil.MaxBodyBytes)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("Cursor GetMe failed (HTTP %d)", status)
	}
	var me struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return "", err
	}
	return strings.TrimSpace(me.Email), nil
}
