package commandcode

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"leeway/internal/netutil"
)

func errMissingKey() error {
	return fmt.Errorf("missing Command Code API key — set COMMAND_CODE_API_KEY or: leeway config set-api-key --provider commandcode")
}

func errInvalidKey(status int) error {
	return fmt.Errorf("invalid Command Code API key (HTTP %d)", status)
}

func errParse(msg string) error {
	return fmt.Errorf("failed to parse Command Code usage: %s", msg)
}

func (p *commandCode) get(ctx context.Context, path, apiKey, orgID string) ([]byte, error) {
	u := p.baseURL + path
	if orgID != "" {
		u += "?orgId=" + url.QueryEscape(orgID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("Command Code network error: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	body, status, err := netutil.DoRequest(ctx, p.client, req, 0)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("Command Code network error: %w", err)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, errInvalidKey(status)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("Command Code API error: HTTP %d", status)
	}
	return body, nil
}
