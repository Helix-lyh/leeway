package deepseek

import "fmt"

// ErrorKind classifies DeepSeek fetch failures, mirroring DeepSeekUsageError.
type ErrorKind int

const (
	KindMissingCredentials ErrorKind = iota
	KindInvalidAPIKey
	KindInvalidPlatformToken
	KindNetwork
	KindAPI
	KindParse
)

// Error is a DeepSeek-specific error with a stable kind for classification.
type Error struct {
	Kind ErrorKind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// MissingCredentials mirrors DeepSeekUsageError.missingCredentials.
func MissingCredentials() error {
	return &Error{Kind: KindMissingCredentials, Msg: "Missing DeepSeek API key."}
}

// InvalidAPIKey reports a rejected API key on the balance endpoint (HTTP 401).
// The Swift fetcher only says "HTTP 401"; the message keeps the status code
// and adds the classification for doctor-style output.
func InvalidAPIKey() error {
	return &Error{Kind: KindInvalidAPIKey, Msg: "DeepSeek API error: HTTP 401 (invalid API key)"}
}

// InvalidPlatformToken mirrors DeepSeekUsageError.invalidPlatformToken.
func InvalidPlatformToken() error {
	return &Error{Kind: KindInvalidPlatformToken, Msg: "DeepSeek Platform session is missing or expired."}
}

// NetworkError mirrors DeepSeekUsageError.networkError.
func NetworkError(err error) error {
	return &Error{Kind: KindNetwork, Msg: fmt.Sprintf("DeepSeek network error: %v", err)}
}

// APIError mirrors DeepSeekUsageError.apiError.
func APIError(msg string) error {
	return &Error{Kind: KindAPI, Msg: "DeepSeek API error: " + msg}
}

// ParseFailed mirrors DeepSeekUsageError.parseFailed.
func ParseFailed(msg string) error {
	return &Error{Kind: KindParse, Msg: "Failed to parse DeepSeek response: " + msg}
}
