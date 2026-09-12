package grok

import (
	"encoding/json"
	"os"
	"strings"
)

// HasOfficialShape is true when an xAI entry already has the profile fields
// official grok writes after a browser login. Device-auth tokens alone — even
// a JWT with sub — are not enough; inventing those fields still forces a
// web re-login.
func HasOfficialShape(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	m, err := grokRegistry(raw)
	if err != nil {
		return false
	}
	for k, v := range m {
		if !strings.HasPrefix(k, "https://auth.x.ai::") {
			continue
		}
		entry := map[string]any{}
		if json.Unmarshal(v, &entry) != nil {
			continue
		}
		if strings.TrimSpace(strField(entry, "key")) == "" {
			continue
		}
		if strField(entry, "auth_mode") != "oidc" {
			continue
		}
		if strField(entry, "user_id") != "" || strField(entry, "principal_id") != "" {
			return true
		}
	}
	return false
}

func grokRegistry(raw []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	return m, nil
}

func strField(entry map[string]any, field string) string {
	s, _ := entry[field].(string)
	return strings.TrimSpace(s)
}
