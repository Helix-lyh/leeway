package commandcode

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"leeway/internal/netutil"
)

func errMissingKey() error {
	return fmt.Errorf("没有 Command Code API 密钥 — 请设置 COMMAND_CODE_API_KEY，或跑 leeway login commandcode")
}

func errInvalidKey(status int) error {
	return fmt.Errorf("Command Code API 密钥不合法（HTTP %d）", status)
}

func errParse(msg string) error {
	return fmt.Errorf("解析 Command Code 用量失败：%s", msg)
}

func (p *commandCode) get(ctx context.Context, path, apiKey, orgID string) ([]byte, error) {
	u := p.baseURL + path
	if orgID != "" {
		u += "?orgId=" + url.QueryEscape(orgID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("Command Code 网络出错：%w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	body, status, err := netutil.DoRequest(ctx, p.client, req, 0)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("Command Code 网络出错：%w", err)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil, errInvalidKey(status)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("Command Code 接口出错：HTTP %d", status)
	}
	return body, nil
}
