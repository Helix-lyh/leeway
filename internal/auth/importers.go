package auth

import "fmt"

// DomainsForProvider returns the cookie host domains a provider's session
// lives on. This is the single source of truth for the provider→domain
// mapping, shared by the browser importers and doctor/resolve helpers.
func DomainsForProvider(providerID string) ([]string, error) {
	switch providerID {
	case "grok":
		return []string{"grok.com"}, nil
	case "kimi":
		return []string{"www.kimi.com", "kimi.com"}, nil
	case "opencode-go":
		return []string{"opencode.ai"}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q — cookie import supports: grok, kimi, opencode-go", providerID)
	}
}

// importImpl dispatches to the concrete browser importers. profile names a
// specific Chrome profile ("Default", "Profile 1", …); empty auto-picks the
// profile with the most matching cookies.
func importImpl(providerID, browser, profile string) (string, error) {
	switch browser {
	case "manual":
		return "", errManualNotSupported(providerID)
	case "chrome", "edge":
		return importChrome(providerID, browser, profile)
	case "kimi-desktop":
		return importKimiDesktop(providerID)
	default:
		return "", errBrowserNotImplemented(browser)
	}
}

// errManualNotSupported points manual-cookie users at `cookie set` with the
// provider ID substituted (no literal "<id>" placeholder in user-facing text).
func errManualNotSupported(providerID string) error {
	return errorString("manual cookies are configured via: leeway cookie set --provider " + providerID + " --manual ...")
}

type errorString string

func (e errorString) Error() string { return string(e) }

func errBrowserNotImplemented(browser string) error {
	return errorString("browser importer for " + browser + " not implemented on this platform — use manual cookies instead (leeway cookie set --provider <id> --manual \"name=value; ...\")")
}
