package fetch

import (
	"encoding/json"
	"errors"
	"os"

	"leeway/internal/provider"
	"leeway/internal/provider/codex"
)

// codexAuthFile is ~/.codex/auth.json (Codex CLI session). Path resolution
// goes through codex.AuthFilePath so CODEX_HOME is honored exactly like the
// usage base URL resolution.
type codexAuthFile struct {
	Tokens struct {
		AccessToken string `json:"access_token"`
	} `json:"tokens"`
	AccountID string `json:"account_id"`
	OpenAIKey string `json:"OPENAI_API_KEY"` // legacy single-key layout
}

// loadCodexAuth reads the Codex CLI session file if present.
func loadCodexAuth() (*provider.CodexAuth, error) {
	p, err := codex.AuthFilePath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	var f codexAuthFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	cx := &provider.CodexAuth{
		AccessToken: f.Tokens.AccessToken,
		AccountID:   f.AccountID,
	}
	if cx.AccessToken == "" {
		cx.AccessToken = f.OpenAIKey
	}
	return cx, nil
}

// HasCodexAuth reports whether a usable Codex CLI session exists, using the
// same resolution as credential loading (doctor uses it so its diagnostic
// matches what fetch actually needs).
func HasCodexAuth() bool {
	cx, err := loadCodexAuth()
	return err == nil && cx != nil && cx.AccessToken != ""
}
