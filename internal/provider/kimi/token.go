package kimi

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"

	"leeway/internal/auth"
	"leeway/internal/provider"
)

// ResolveWebToken mirrors KimiWebFetchStrategy.resolveToken: the kimi-auth
// JWT is taken from the cookie header first (manual override or imported
// cache, as provided by the orchestrator), then from the KIMI_AUTH_TOKEN /
// kimi_auth_token environment token.
func ResolveWebToken(creds provider.Credentials) string {
	if tok := cookieToken(creds.CookieHeader); tok != "" {
		return tok
	}
	return cleaned(creds.AuthToken)
}

// enrichmentToken mirrors KimiCookieHeader.override's JWT shape check: only a
// bare JWT (eyJ... with three dot-separated parts) is accepted as an
// enrichment token for the API-key path. This filters out non-JWT values that
// the orchestrator may surface (e.g. a config API key falling back into
// AuthToken).
func enrichmentToken(token string) string {
	if isJWT(token) {
		return token
	}
	return ""
}

var (
	kimiAuthEqRe    = regexp.MustCompile(`(?i)kimi-auth=([A-Za-z0-9._\-+=/]+)`)
	kimiAuthColonRe = regexp.MustCompile(`(?i)kimi-auth:\s*([A-Za-z0-9._\-+=/]+)`)
)

// cookieToken mirrors KimiCookieHeader.override(from:): extract the kimi-auth
// token from a raw cookie header, a curl-style "-H 'Cookie: ...'" fragment, a
// "cookie: ..." dump, or accept a bare JWT. curl-fragment extraction is shared
// with opencode-go via auth.ExtractCookieHeader so quoting fixes reach both.
func cookieToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, re := range []*regexp.Regexp{kimiAuthEqRe, kimiAuthColonRe} {
		if tok := capture(re, raw); tok != "" {
			return tok
		}
	}
	if h := auth.ExtractCookieHeader(raw); h != "" {
		for _, kre := range []*regexp.Regexp{kimiAuthEqRe, kimiAuthColonRe} {
			if tok := capture(kre, h); tok != "" {
				return tok
			}
		}
	}
	if isJWT(raw) {
		return raw
	}
	return ""
}

func capture(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// isJWT mirrors the bare-JWT check in KimiCookieHeader.override.
func isJWT(s string) bool {
	return strings.HasPrefix(s, "eyJ") && strings.Count(s, ".") == 2
}

// cleaned mirrors KimiSettingsReader.cleaned: trim and strip a wrapping pair
// of matching quotes.
func cleaned(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
	}
	return strings.TrimSpace(v)
}

// sessionInfo mirrors KimiUsageFetcher.SessionInfo, decoded from the JWT
// payload (device_id / ssid / sub).
type sessionInfo struct {
	deviceID  string
	sessionID string
	trafficID string
}

// decodeSessionInfo mirrors KimiUsageFetcher.decodeSessionInfo.
func decodeSessionInfo(jwt string) *sessionInfo {
	parts := strings.SplitN(jwt, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	payload := strings.ReplaceAll(strings.ReplaceAll(parts[1], "-", "+"), "_", "/")
	for len(payload)%4 != 0 {
		payload += "="
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil
	}
	info := &sessionInfo{}
	info.deviceID, _ = claims["device_id"].(string)
	info.sessionID, _ = claims["ssid"].(string)
	info.trafficID, _ = claims["sub"].(string)
	return info
}
