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
	return &Error{Kind: KindMissingCredentials, Msg: "没有 DeepSeek API 密钥。"}
}

// InvalidAPIKey reports a rejected API key on the balance endpoint (HTTP 401).
// The Swift fetcher only says "HTTP 401"; the message keeps the status code
// and adds the classification for doctor-style output.
func InvalidAPIKey() error {
	return &Error{Kind: KindInvalidAPIKey, Msg: "DeepSeek 接口出错：HTTP 401（API 密钥不合法）"}
}

// InvalidPlatformToken mirrors DeepSeekUsageError.invalidPlatformToken.
func InvalidPlatformToken() error {
	return &Error{Kind: KindInvalidPlatformToken, Msg: "DeepSeek 平台的登录信息没有或已过期。"}
}

// NetworkError mirrors DeepSeekUsageError.networkError.
func NetworkError(err error) error {
	return &Error{Kind: KindNetwork, Msg: fmt.Sprintf("DeepSeek 网络出错：%v", err)}
}

// APIError mirrors DeepSeekUsageError.apiError.
func APIError(msg string) error {
	return &Error{Kind: KindAPI, Msg: "DeepSeek 接口出错：" + msg}
}

// ParseFailed mirrors DeepSeekUsageError.parseFailed.
func ParseFailed(msg string) error {
	return &Error{Kind: KindParse, Msg: "解析 DeepSeek 返回失败：" + msg}
}
