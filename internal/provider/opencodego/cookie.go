package opencodego

import (
	"strings"

	"leeway/internal/auth"
)

// Cookie handling mirrors OpenCodeWebCookieSupport.requestCookieHeader:
// the raw pasted/imported header is normalized, split into name=value pairs,
// and only the session cookies OpenCode Go actually uses ("auth" and
// "__Host-auth") are kept on the wire.

var allowedCookieNames = map[string]bool{"auth": true, "__Host-auth": true}

// requestCookieHeader filters raw to the OpenCode Go session cookies. It
// returns an invalidCredentials error when the header carries none of them.
func requestCookieHeader(raw string) (string, error) {
	header := filteredHeader(raw)
	if header == "" {
		return "", invalidCredentials()
	}
	return header, nil
}

func filteredHeader(raw string) string {
	value := normalizeCookieHeader(raw)
	if value == "" {
		return ""
	}
	var kept []string
	for _, part := range strings.Split(value, ";") {
		trimmed := strings.TrimSpace(part)
		eq := strings.IndexByte(trimmed, '=')
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(trimmed[:eq])
		cval := strings.TrimSpace(trimmed[eq+1:])
		if !allowedCookieNames[name] {
			continue
		}
		kept = append(kept, name+"="+cval)
	}
	return strings.Join(kept, "; ")
}

func normalizeCookieHeader(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if extracted := extractHeader(value); extracted != "" {
		value = extracted
	}
	value = stripCookiePrefix(value)
	value = stripWrappingQuotes(value)
	return strings.TrimSpace(value)
}

// extractHeader pulls the cookie header out of a pasted curl fragment,
// sharing the pattern list with the kimi provider via auth.ExtractCookieHeader.
func extractHeader(raw string) string {
	return auth.ExtractCookieHeader(raw)
}

func stripCookiePrefix(raw string) string {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "cookie:") {
		return trimmed
	}
	return strings.TrimSpace(trimmed[len("cookie:"):])
}

func stripWrappingQuotes(raw string) string {
	if len(raw) >= 2 {
		if (strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`)) ||
			(strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'")) {
			return raw[1 : len(raw)-1]
		}
	}
	return raw
}
