package codex

import (
	"errors"
	"fmt"
	"strings"
)

// errMissingSession is the clear error for absent Codex credentials; the
// fetch orchestrator maps missing credentials to "not_configured" before
// Fetch is called, so this is a defensive backstop (never panic).
var errMissingSession = errors.New("没有 Codex 登录信息 — 请跑 leeway login codex")

// errNoRateLimitsFound mirrors CodexBar's UsageError.noRateLimitsFound.
var errNoRateLimitsFound = errors.New("找到了登录信息，但还没有额度记录")

// fetchError classifies Codex API failures the way CodexOAuthFetchError and
// CodexOpenAIWorkspaceResolverError do.
type fetchError struct {
	kind  fetchErrorKind
	code  int
	body  string
	cause error
}

type fetchErrorKind int

const (
	kindUnauthorized fetchErrorKind = iota
	kindInvalidResponse
	kindServerError
	kindInvalidAccountsResponse
	kindNetworkError
)

func (e *fetchError) Error() string {
	switch e.kind {
	case kindUnauthorized:
		// CodexBar: "Codex OAuth token expired or invalid. Run `codex` to
		// re-authenticate."; leeway contract keeps the same hint shorter.
		return "登录已过期 — 请跑 leeway login codex"
	case kindInvalidResponse:
		return "Codex 用量接口返回的内容不对。"
	case kindServerError:
		if body := strings.TrimSpace(e.body); body != "" {
			return fmt.Sprintf("Codex 接口出错 %d：%s", e.code, body)
		}
		return fmt.Sprintf("Codex 接口出错 %d。", e.code)
	case kindInvalidAccountsResponse:
		return "查询 OpenAI 账号时返回的内容不对。"
	case kindNetworkError:
		// Provider-prefixed like every other provider's network error, so a
		// multi-provider table shows which probe failed.
		return fmt.Sprintf("Codex 网络出错：%v", e.cause)
	default:
		return "Codex 接口出错。"
	}
}

func (e *fetchError) Unwrap() error { return e.cause }
