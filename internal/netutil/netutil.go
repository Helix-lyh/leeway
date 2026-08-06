// Package netutil shares the ctx-aware HTTP request pattern used by every
// provider: perform a request, classify cancellation vs. network failure, and
// read the response body. Providers map the raw error and status onto their
// own typed errors afterwards.
package netutil

import (
	"context"
	"io"
	"net/http"
)

// MaxBodyBytes caps how much of a response body a caller reads when a limit is
// requested (mirrors the Codex fetcher's guard).
const MaxBodyBytes = 8 << 20

// DoRequest performs req with the given client and returns the full response
// body and status. A transport error that happens while ctx is done is
// reported as ctx.Err() so callers can distinguish cancellation from network
// failure. limit > 0 caps the body read (silent truncation, like io.LimitReader).
func DoRequest(ctx context.Context, client *http.Client, req *http.Request, limit int64) (body []byte, status int, err error) {
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, err
	}
	defer resp.Body.Close()
	var r io.Reader = resp.Body
	if limit > 0 {
		r = io.LimitReader(resp.Body, limit)
	}
	body, err = io.ReadAll(r)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}
