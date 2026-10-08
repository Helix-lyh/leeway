package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	return CursorKeychainTokenFor(ctx, CursorKeychainService)
}

// CursorKeychainTokenFor reads one keychain service by the official account
// name. service is a parameter so callers can point at the item they resolved.
func CursorKeychainTokenFor(ctx context.Context, service string) (string, error) {
	if _, err := exec.LookPath("security"); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-a", CursorKeychainAccount, "-s", service, "-w")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// CursorKeychainItemExists reports whether the official Cursor keychain item
// is there. It reads attributes, not the secret, so no ACL prompt appears.
func CursorKeychainItemExists(ctx context.Context, service string) bool {
	if _, err := exec.LookPath("security"); err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, "security", "find-generic-password",
		"-a", CursorKeychainAccount, "-s", service)
	return cmd.Run() == nil
}

// SetCursorKeychainToken replaces the token the official Cursor client reads.
// The command goes in through `security -i` on stdin: the token never reaches
// the process table, and unlike the interactive `-w` prompt — which cuts the
// value at 128 bytes without saying so — the whole token lands. The value is
// read back, because a short write here corrupts the very login this is for.
func SetCursorKeychainToken(ctx context.Context, service, token string) error {
	if _, err := exec.LookPath("security"); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a %s -s %s -w %s\n",
		CursorKeychainAccount, service, token))
	// Neither stream is shown: `security -i` echoes what it reads, and what it
	// reads is the token.
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("写 Cursor keychain 失败：%v", err)
	}
	got, err := CursorKeychainTokenFor(ctx, service)
	if err != nil {
		return fmt.Errorf("写 Cursor keychain 后回读失败：%v", err)
	}
	if got != token {
		return fmt.Errorf("写 Cursor keychain 后回读不一致：写入 %d 字节，读回 %d 字节", len(token), len(got))
	}
	return nil
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
