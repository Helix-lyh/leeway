package auth

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Cursor keychain entry the official Cursor client maintains.
const (
	CursorKeychainService = "cursor-access-token"
	CursorKeychainAccount = "cursor-user"
)

// CursorKeychainToken reads the official Cursor access token from the macOS
// keychain. A missing item is reported so callers can fall through; timeout
// and ACL failures are returned as-is.
func CursorKeychainToken(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("security"); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-a", CursorKeychainAccount, "-s", CursorKeychainService, "-w")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// IsKeychainItemMissing reports a missing security binary or a keychain
// item-not-found (macOS exit 44). Other failures (timeout, ACL) are not
// missing.
func IsKeychainItemMissing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 44
	}
	return false
}
