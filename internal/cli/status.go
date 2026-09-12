package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"leeway/internal/auth"
	"leeway/internal/config"
	"leeway/internal/provider"
)

func runStatus(args []string) int {
	var jsonOut bool
	fs := newFlagSet("status", &jsonOut)
	fs.Help(statusHelp)
	if !fs.Parse(args) {
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}

	rows := collectAuthRows(cfg)
	hints := collectMissingHints(cfg, rows)
	path := configPath()

	if jsonOut {
		type jsonRow struct {
			Provider      string `json:"provider"`
			Account       string `json:"account"`
			Credential    string `json:"credential"`
			Status        string `json:"status"`
			Expires       string `json:"expires"`
			CookiePresent bool   `json:"cookiePresent"`
			APIKeyPresent bool   `json:"apiKeyPresent"`
		}
		type jsonHint struct {
			Provider string `json:"provider"`
			Hint     string `json:"hint"`
		}
		view := struct {
			Accounts   []jsonRow  `json:"accounts"`
			Hints      []jsonHint `json:"hints"`
			ConfigPath string     `json:"configPath"`
		}{
			Accounts:   make([]jsonRow, 0, len(rows)),
			Hints:      make([]jsonHint, 0, len(hints)),
			ConfigPath: path,
		}
		for _, r := range rows {
			cookie, key := rowPresence(cfg, r)
			view.Accounts = append(view.Accounts, jsonRow{
				Provider: r.provider, Account: r.account, Credential: r.cred,
				Status: r.status, Expires: r.expires,
				CookiePresent: cookie, APIKeyPresent: key,
			})
		}
		for _, h := range hints {
			view.Hints = append(view.Hints, jsonHint{Provider: h.provider, Hint: h.hint})
		}
		if code := newJSONEncoder().Encode(view); code != 0 {
			return code
		}
		return statusExit(rows, hints)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROVIDER\tACCOUNT\tCREDENTIAL\tSTATUS\tEXPIRES")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.provider, r.account, r.cred, r.status, r.expires)
	}
	for _, h := range hints {
		fmt.Fprintf(w, "%s\t—\t—\tmissing\t%s\n", h.provider, h.hint)
	}
	w.Flush()
	fmt.Fprintln(os.Stderr, "config:", path)
	return statusExit(rows, hints)
}

func statusExit(rows []authRow, hints []statusHint) int {
	if len(hints) > 0 || len(rows) == 0 {
		return 1
	}
	for _, r := range rows {
		if r.status != "ok" {
			return 1
		}
	}
	return 0
}

type statusHint struct{ provider, hint string }

func collectMissingHints(cfg *config.Config, rows []authRow) []statusHint {
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.provider] = true
	}
	var hints []statusHint
	for _, p := range provider.All() {
		id := p.ID()
		if !cfg.IsEnabled(id) || seen[id] {
			continue
		}
		if hint := missingHint(cfg, p); hint != "" {
			hints = append(hints, statusHint{id, hint})
		}
	}
	return hints
}

func missingHint(cfg *config.Config, p provider.Provider) string {
	if authStatusHasDefault(cfg, p.ID()) {
		return ""
	}
	if p.ID() == "cursor" {
		for _, a := range cursorAccounts(cfg) {
			if _, has, err := auth.LoadCursorAuth(a.Name); err == nil && has {
				return ""
			}
		}
	}
	return "leeway login " + p.ID()
}

func rowPresence(cfg *config.Config, r authRow) (cookie, apiKey bool) {
	if r.cred == "cookies" {
		cookie = true
	}
	if r.cred == "apiKey (config)" {
		apiKey = true
	}
	if cfg.APIKey(r.provider) != "" {
		apiKey = true
	}
	if a, ok := cfg.Account(r.provider, r.account); ok && a.APIKey != "" {
		apiKey = true
	}
	if h, err := auth.ResolveCookieHeader(r.provider, cfg.Provider(r.provider), r.account); err == nil && h != "" {
		cookie = true
	}
	return cookie, apiKey
}
