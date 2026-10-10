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
		Msg: "没有 Kimi 登录令牌。请在 Kimi 控制台取 JWT 令牌填上（或：leeway login kimi --cookie）。"}
}

// InvalidToken mirrors KimiAPIError.invalidToken. The hint covers both the
// env-token path and cookie-imported sessions (there is no leeway command to
// refresh a web token, so re-importing the browser session is the fix).
func InvalidToken() error {
	return &Error{Kind: KindInvalidToken,
		Msg: "Kimi 登录令牌不合法或已过期。请刷新（KIMI_AUTH_TOKEN，或：leeway login kimi --cookie --force）。"}
}

// MissingAPIKey mirrors KimiAPIError.missingAPIKey.
func MissingAPIKey() error {
	return &Error{Kind: KindMissingAPIKey,
		Msg: "没有 Kimi Code API 密钥。请在「设置 > 服务商 > Kimi」里填上，或设置 KIMI_CODE_API_KEY。"}
}

// InvalidAPIKey mirrors KimiAPIError.invalidAPIKey.
func InvalidAPIKey() error {
	return &Error{Kind: KindInvalidAPIKey,
		Msg: "Kimi Code API 密钥不合法或已过期。请刷新 API 密钥。"}
}

// InvalidRequest mirrors KimiAPIError.invalidRequest.
func InvalidRequest(msg string) error {
	return &Error{Kind: KindInvalidRequest, Msg: "请求不合法：" + msg}
}

// NetworkError mirrors KimiAPIError.networkError.
func NetworkError(err error) error {
	return &Error{Kind: KindNetwork, Msg: fmt.Sprintf("Kimi 网络出错：%v", err)}
}

// APIError mirrors KimiAPIError.apiError.
func APIError(msg string) error {
	return &Error{Kind: KindAPI, Msg: "Kimi 接口出错：" + msg}
}

// ParseFailed mirrors KimiAPIError.parseFailed.
func ParseFailed(msg string) error {
	return &Error{Kind: KindParse, Msg: "解析 Kimi 用量数据失败：" + msg}
}
