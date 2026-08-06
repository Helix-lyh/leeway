package codex

import (
	"errors"
	"fmt"
	"strings"
)

// errMissingSession is the clear error for absent Codex credentials; the
// fetch orchestrator maps missing credentials to "not_configured" before
// Fetch is called, so this is a defensive backstop (never panic).
var errMissingSession = errors.New("missing Codex session — run: codex login")

// errNoRateLimitsFound mirrors CodexBar's UsageError.noRateLimitsFound.
var errNoRateLimitsFound = errors.New("found sessions, but no rate limit events yet")

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
		return "session expired — run: codex login"
	case kindInvalidResponse:
		return "Invalid response from Codex usage API."
	case kindServerError:
		if body := strings.TrimSpace(e.body); body != "" {
			return fmt.Sprintf("Codex API error %d: %s", e.code, body)
		}
		return fmt.Sprintf("Codex API error %d.", e.code)
	case kindInvalidAccountsResponse:
		return "OpenAI account lookup returned an invalid response."
	case kindNetworkError:
		// Provider-prefixed like every other provider's network error, so a
		// multi-provider table shows which probe failed.
		return fmt.Sprintf("Codex network error: %v", e.cause)
	default:
		return "Codex API error."
	}
}

func (e *fetchError) Unwrap() error { return e.cause }
