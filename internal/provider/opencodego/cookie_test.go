package opencodego

import (
	"strings"
	"testing"
)

func TestFilteredHeaderKeepsOnlyAuthCookies(t *testing.T) {
	raw := "session=abc; auth=token123; other=x; __Host-auth=jwt456"
	got, err := requestCookieHeader(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "auth=token123; __Host-auth=jwt456" {
		t.Fatalf("filtered header wrong: %q", got)
	}
}

func TestFilteredHeaderNoAuthCookies(t *testing.T) {
	_, err := requestCookieHeader("session=abc; other=x")
	if err == nil {
		t.Fatalf("expected invalidCredentials for header without auth cookies")
	}
	if !strings.Contains(err.Error(), "invalid or expired") {
		t.Fatalf("expected invalid/expired message, got %v", err)
	}
}

func TestFilteredHeaderNormalization(t *testing.T) {
	// Wrapped in quotes / prefixed with Cookie: / curl-style.
	cases := []struct {
		in, want string
	}{
		{`"auth=1"`, "auth=1"},
		{`Cookie: auth=1; session=2`, "auth=1"},
		{`cookie: auth=1`, "auth=1"},
		{`-H 'Cookie: auth=one; b=two'`, "auth=one"},
		{`-b auth=1`, "auth=1"},
		{`--cookie 'auth=1; other=2'`, "auth=1"},
	}
	for _, c := range cases {
		got, err := requestCookieHeader(c.in)
		if err != nil {
			t.Fatalf("requestCookieHeader(%q) error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("requestCookieHeader(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFilteredHeaderEmpty(t *testing.T) {
	if _, err := requestCookieHeader(""); err == nil {
		t.Fatalf("empty header must be an error")
	}
	if _, err := requestCookieHeader("   "); err == nil {
		t.Fatalf("blank header must be an error")
	}
}
