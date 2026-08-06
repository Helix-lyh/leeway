package opencodego

import (
	"errors"
	"fmt"
	"strings"
)

// Error classification mirrors OpenCodeGoUsageError in the Swift client:
//
//	invalidCredentials — cookie missing/invalid/expired or a signed-out page
//	networkError        — transport failures
//	apiError            — non-200 responses
//	parseFailed         — bodies that do not carry the expected data
type usageErrorKind int

const (
	kindInvalidCredentials usageErrorKind = iota
	kindNetwork
	kindAPI
	kindParse
)

type usageError struct {
	kind    usageErrorKind
	message string
}

func (e *usageError) Error() string {
	switch e.kind {
	case kindInvalidCredentials:
		return "OpenCode Go session cookie is invalid or expired."
	case kindNetwork:
		return "OpenCode Go network error: " + e.message
	case kindAPI:
		return "OpenCode Go API error: " + e.message
	default:
		return "OpenCode Go parse error: " + e.message
	}
}

func invalidCredentials() error { return &usageError{kind: kindInvalidCredentials} }

func networkErrorf(format string, args ...any) error {
	return &usageError{kind: kindNetwork, message: sprintf(format, args...)}
}

func apiErrorf(format string, args ...any) error {
	return &usageError{kind: kindAPI, message: sprintf(format, args...)}
}

func parseErrorf(format string, args ...any) error {
	return &usageError{kind: kindParse, message: sprintf(format, args...)}
}

// isMissingUsageFields reports whether err is the parse failure that triggers
// the Zen-balance-only fallback (Swift's "Missing usage fields." message).
func isMissingUsageFields(err error) bool {
	var ue *usageError
	if !errors.As(err, &ue) {
		return false
	}
	return ue.kind == kindParse && strings.Contains(ue.message, "Missing usage fields")
}

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
