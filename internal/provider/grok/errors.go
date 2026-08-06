package grok

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
)

// errorKind mirrors the cases of CodexBar's GrokWebBillingError.
type errorKind int

const (
	kindMissingCredentials errorKind = iota
	kindEmptyResponse
	kindInvalidResponse
	kindRequestFailed // HTTP status != 200
	kindRPCFailed     // non-zero grpc-status (HTTP headers or trailers)
	kindParseFailed
)

// Error mirrors GrokWebBillingError with its exact user-facing messages.
type Error struct {
	Kind    errorKind
	Status  int    // HTTP status for kindRequestFailed, grpc status for kindRPCFailed
	Body    string // response body prefix (<=400 bytes) for kindRequestFailed
	Message string // grpc-message for kindRPCFailed
}

// ErrMissingCredentials is returned when no cookie header is configured
// (the fetch orchestrator maps the underlying condition to "not_configured").
var ErrMissingCredentials = &Error{Kind: kindMissingCredentials}

// reauthMessage mirrors GrokWebBillingError.reauthMessage.
const reauthMessage = "Grok web billing rejected credentials. Sign in to grok.com in Chrome or run `grok login` to refresh xAI auth."

func (e *Error) Error() string {
	switch e.Kind {
	case kindMissingCredentials:
		if e.Message != "" {
			return e.Message
		}
		return "Grok web billing requires a signed-in grok.com browser session or `grok login`."
	case kindEmptyResponse:
		return "Grok web billing returned no protobuf payload."
	case kindInvalidResponse:
		return "Grok web billing returned an invalid response."
	case kindRequestFailed:
		if e.Status == 401 || e.Status == 403 {
			return reauthMessage
		}
		return fmt.Sprintf("Grok web billing request failed with HTTP %d: %s", e.Status, e.Body)
	case kindRPCFailed:
		if isAuthenticationFailure(e.Status, e.Message) {
			return reauthMessage
		}
		return fmt.Sprintf("Grok web billing RPC failed with status %d: %s", e.Status, e.Message)
	case kindParseFailed:
		return "Could not parse Grok web billing usage."
	default:
		return "Grok web billing error"
	}
}

func errRequestFailed(status int, body string) error {
	return &Error{Kind: kindRequestFailed, Status: status, Body: body}
}

func errRPCFailed(status int, message string) error {
	return &Error{Kind: kindRPCFailed, Status: status, Message: message}
}

// isAuthenticationFailure mirrors GrokWebBillingError.isAuthenticationFailure.
func isAuthenticationFailure(status int, message string) bool {
	if status == 16 {
		return true
	}
	if status != 7 {
		return false
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "bad-credentials") ||
		strings.Contains(lower, "unauthenticated") ||
		(strings.Contains(lower, "oauth2") && strings.Contains(lower, "could not be validated")) ||
		(strings.Contains(lower, "access token") &&
			(strings.Contains(lower, "invalid") ||
				strings.Contains(lower, "expired") ||
				strings.Contains(lower, "could not be validated")))
}

// isCookieAuthenticationFailure mirrors
// GrokWebFetchStrategy.isCookieAuthenticationFailure (HTTP 401/403 or an
// RPC-level authentication failure).
func isCookieAuthenticationFailure(err error) bool {
	var g *Error
	if !errors.As(err, &g) {
		return false
	}
	switch g.Kind {
	case kindRequestFailed:
		return g.Status == 401 || g.Status == 403
	case kindRPCFailed:
		return isAuthenticationFailure(g.Status, g.Message)
	default:
		return false
	}
}

// shouldRetry mirrors GrokWebBillingFetcher.shouldRetry: network timeouts /
// connection loss, HTTP 408/502/503/504 or "timeout"/"deadline" bodies, and
// grpc status 4 (deadline exceeded) or status 1 (canceled) with a
// timeout/deadline/expired message.
func shouldRetry(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return true
	}
	var g *Error
	if !errors.As(err, &g) {
		return false
	}
	switch g.Kind {
	case kindRequestFailed:
		switch g.Status {
		case 408, 502, 503, 504:
			return true
		}
		lower := strings.ToLower(g.Body)
		return strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline")
	case kindRPCFailed:
		if g.Status == 4 {
			return true
		}
		if g.Status != 1 {
			return false
		}
		lower := strings.ToLower(g.Message)
		return strings.Contains(lower, "timeout") ||
			strings.Contains(lower, "deadline") ||
			strings.Contains(lower, "expired")
	default:
		return false
	}
}
