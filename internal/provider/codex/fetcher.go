package codex

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"leeway/internal/netutil"
)

// Endpoints mirror CodexOAuthUsageFetcher and CodexOpenAIWorkspaceResolver.
const (
	defaultChatGPTBaseURL = "https://chatgpt.com/backend-api/"
	chatGPTUsagePath      = "/wham/usage"
	codexUsagePath        = "/api/codex/usage"
	defaultAccountsURL    = "https://chatgpt.com/backend-api/accounts"

	usageTimeout    = 30 * time.Second
	accountsTimeout = 20 * time.Second

	maxBodyBytes = 8 << 20
)

// apiClient issues the authenticated GETs of the Codex provider. Tests
// override usageURL/accountsURL to point at a local httptest server.
type apiClient struct {
	httpClient  *http.Client
	usageURL    string
	accountsURL string
}

func newAPIClient() *apiClient {
	return &apiClient{
		httpClient:  newHTTPClient(usageTimeout),
		usageURL:    resolveUsageURL(codexHome()),
		accountsURL: defaultAccountsURL,
	}
}

// newHTTPClient mirrors CodexBar's redirect guard: follow only HTTPS
// redirects that stay on the same origin, so Authorization cannot leak to a
// foreign host.
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) == 0 {
				return nil
			}
			prev := via[len(via)-1]
			if req.URL.Scheme != "https" || prev.URL.Scheme != "https" {
				return http.ErrUseLastResponse
			}
			if req.URL.Host != prev.URL.Host {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// fetchUsage mirrors CodexOAuthUsageFetcher.fetchUsage: GET the usage
// endpoint and classify the response.
func (c *apiClient) fetchUsage(ctx context.Context, accessToken, accountID string) (*usageResponse, error) {
	body, status, err := c.doGet(ctx, c.usageURL, accessToken, accountID, "leeway")
	if err != nil {
		return nil, err
	}
	switch {
	case status >= 200 && status < 300:
		resp, perr := parseUsageResponse(body)
		if perr != nil {
			return nil, &fetchError{kind: kindInvalidResponse}
		}
		return resp, nil
	case status == 401 || status == 403:
		return nil, &fetchError{kind: kindUnauthorized}
	default:
		// Cap the body embedded in the error: a pathological 5xx/3xx page
		// would otherwise balloon the provider.error field of the JSON
		// contract (and the snapshot cache) to megabytes.
		return nil, &fetchError{kind: kindServerError, code: status, body: truncateBody(string(body))}
	}
}

// fetchAccounts mirrors CodexOpenAIWorkspaceResolver.listWorkspaces.
func (c *apiClient) fetchAccounts(ctx context.Context, accessToken, accountID string) ([]accountItem, error) {
	body, status, err := c.doGet(ctx, c.accountsURL, accessToken, accountID, "codex-cli")
	if err != nil {
		return nil, err
	}
	switch {
	case status >= 200 && status < 300:
		var resp struct {
			Items []accountItem `json:"items"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, &fetchError{kind: kindInvalidAccountsResponse}
		}
		return resp.Items, nil
	case status == 401 || status == 403:
		return nil, &fetchError{kind: kindUnauthorized}
	default:
		return nil, &fetchError{kind: kindInvalidAccountsResponse}
	}
}

func (c *apiClient) doGet(ctx context.Context, rawURL, accessToken, accountID, userAgent string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, &fetchError{kind: kindNetworkError, cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
	body, status, err := netutil.DoRequest(ctx, c.httpClient, req, maxBodyBytes)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, 0, err
		}
		return nil, 0, &fetchError{kind: kindNetworkError, cause: err}
	}
	return body, status, nil
}

// truncateBody caps an embedded response body at 400 bytes so a pathological
// error page cannot balloon the error message.
func truncateBody(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// --- base URL resolution (~/.codex/config.toml chatgpt_base_url) ---

// codexHome returns the Codex home directory, honoring CODEX_HOME the way
// CodexBar's CodexHomeScope does.
func codexHome() string {
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// AuthFilePath returns the Codex CLI session file path, resolved through the
// same CODEX_HOME logic as the usage base URL so credentials and endpoints
// never diverge. Shared with the fetch orchestrator's credential loader and
// the doctor check.
func AuthFilePath() (string, error) {
	home := codexHome()
	if home == "" {
		return "", errors.New("找不到 Codex 主目录")
	}
	return filepath.Join(home, "auth.json"), nil
}

// resolveUsageURL mirrors CodexOAuthUsageFetcher.resolveUsageURL: default
// https://chatgpt.com/backend-api/, overridable via chatgpt_base_url in
// $CODEX_HOME/config.toml (or ~/.codex/config.toml).
func resolveUsageURL(codexHomeDir string) string {
	base := defaultChatGPTBaseURL
	if codexHomeDir != "" {
		if contents, err := os.ReadFile(filepath.Join(codexHomeDir, "config.toml")); err == nil {
			if parsed, ok := parseChatGPTBaseURL(string(contents)); ok {
				base = parsed
			}
		}
	}
	normalized := normalizeChatGPTBaseURL(base)
	if strings.Contains(normalized, "/backend-api") {
		return normalized + chatGPTUsagePath
	}
	return normalized + codexUsagePath
}

func normalizeChatGPTBaseURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = defaultChatGPTBaseURL
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if (strings.HasPrefix(trimmed, "https://chatgpt.com") ||
		strings.HasPrefix(trimmed, "https://chat.openai.com")) &&
		!strings.Contains(trimmed, "/backend-api") {
		trimmed += "/backend-api"
	}
	return trimmed
}

// parseChatGPTBaseURL extracts `chatgpt_base_url` from a codex config.toml
// (simple key = value lines, # comments, optional surrounding quotes).
func parseChatGPTBaseURL(contents string) (string, bool) {
	for _, rawLine := range strings.Split(contents, "\n") {
		line := strings.SplitN(rawLine, "#", 2)[0]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != "chatgpt_base_url" {
			continue
		}
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}
		return strings.TrimSpace(value), true
	}
	return "", false
}
