package kimi

import "fmt"

// ErrorKind classifies Kimi fetch failures, mirroring KimiAPIError.
type ErrorKind int

const (
	KindMissingToken ErrorKind = iota
	KindInvalidToken
	KindMissingAPIKey
	KindInvalidAPIKey
	KindInvalidRequest
	KindNetwork
	KindAPI
	KindParse
)

// Error is a Kimi-specific error with a stable kind for classification.
type Error struct {
	Kind ErrorKind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// MissingToken mirrors KimiAPIError.missingToken.
func MissingToken() error {
	return &Error{Kind: KindMissingToken,
		Msg: "Kimi auth token is missing. Add your JWT token from the Kimi console (or: leeway cookie import --provider kimi)."}
}

// InvalidToken mirrors KimiAPIError.invalidToken. The hint covers both the
// env-token path and cookie-imported sessions (there is no leeway command to
// refresh a web token, so re-importing the browser session is the fix).
func InvalidToken() error {
	return &Error{Kind: KindInvalidToken,
		Msg: "Kimi auth token is invalid or expired. Refresh it (KIMI_AUTH_TOKEN, or: leeway cookie import --force --provider kimi; or: kimi login)."}
}

// MissingAPIKey mirrors KimiAPIError.missingAPIKey.
func MissingAPIKey() error {
	return &Error{Kind: KindMissingAPIKey,
		Msg: "Kimi Code API key is missing. Add it in Settings > Providers > Kimi or set KIMI_CODE_API_KEY."}
}

// InvalidAPIKey mirrors KimiAPIError.invalidAPIKey.
func InvalidAPIKey() error {
	return &Error{Kind: KindInvalidAPIKey,
		Msg: "Kimi Code API key is invalid or expired. Please refresh your API key."}
}

// InvalidRequest mirrors KimiAPIError.invalidRequest.
func InvalidRequest(msg string) error {
	return &Error{Kind: KindInvalidRequest, Msg: "Invalid request: " + msg}
}

// NetworkError mirrors KimiAPIError.networkError.
func NetworkError(err error) error {
	return &Error{Kind: KindNetwork, Msg: fmt.Sprintf("Kimi network error: %v", err)}
}

// APIError mirrors KimiAPIError.apiError.
func APIError(msg string) error {
	return &Error{Kind: KindAPI, Msg: "Kimi API error: " + msg}
}

// ParseFailed mirrors KimiAPIError.parseFailed.
func ParseFailed(msg string) error {
	return &Error{Kind: KindParse, Msg: "Failed to parse Kimi usage data: " + msg}
}
