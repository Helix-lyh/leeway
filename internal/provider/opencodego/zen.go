package opencodego

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Zen balance parsing mirrors OpenCodeGoZenBalanceParser: JSON keys first
// ("zenBalance", "currentBalance", ...), then "$" amount patterns near a
// balance label, then the billing-server response (raw integer balance with a
// customerID, scaled by 1/100_000_000).

const billingScale = 100_000_000.0

var (
	zenLocalizedPattern = regexp.MustCompile(
		`(?i)(?:current\s+balance|zen\s+balance|現在の残高)[^$]{0,80}\$\s*([0-9][0-9,]*(?:\.[0-9]+)?)`)
	zenNearbyPattern = regexp.MustCompile(
		`(?i)(?:balance|残高)[\s\S]{0,120}?\$\s*([0-9][0-9,]*(?:\.[0-9]+)?)`)
	zenCustomerIDPattern = regexp.MustCompile(
		`(?:"customerID"|customerID)\s*:\s*(?:\$R\[\d+\]\s*=\s*)?"[^"]+"`)
	zenBalanceNumberPattern = regexp.MustCompile(
		`(?:"balance"|balance)\s*:\s*(?:\$R\[\d+\]\s*=\s*)?(-?[0-9]+(?:\.[0-9]+)?)`)
)

var explicitBalanceKeys = map[string]bool{
	"zenbalance":        true,
	"zencurrentbalance": true,
	"currentbalance":    true,
	"currentbalanceusd": true,
	"balanceusd":        true,
	"usdbalance":        true,
}

// parseZenBalance mirrors parse(text): JSON first, then dollar patterns.
func parseZenBalance(text string) *float64 {
	if v := parseZenJSON(text); v != nil {
		return v
	}
	if v := extractZenDollar(zenLocalizedPattern, text); v != nil {
		return v
	}
	return extractZenDollar(zenNearbyPattern, text)
}

// parseZenBillingResponse mirrors parseBillingServerResponse(text): JSON with
// a customerID+balance pair, then the JS-payload regex fallback.
func parseZenBillingResponse(text string) *float64 {
	if object, ok := decodeJSON(text); ok {
		if raw := findRawBillingBalance(object); raw != nil {
			v := *raw / billingScale
			return &v
		}
	}
	if !zenCustomerIDPattern.MatchString(text) {
		return nil
	}
	if raw := extractZenNumber(zenBalanceNumberPattern, text); raw != nil {
		v := *raw / billingScale
		return &v
	}
	return nil
}

func parseZenJSON(text string) *float64 {
	object, ok := decodeJSON(text)
	if !ok {
		return nil
	}
	return findBalanceValue(object)
}

// findBalanceValue walks the JSON depth-first and returns the first value
// under an explicit balance key (mirrors findBalanceValue(in:path:)).
func findBalanceValue(v any) *float64 {
	switch t := v.(type) {
	case map[string]any:
		for key, sub := range t {
			if isExplicitBalanceAmountKey(key) {
				if n, ok := zenDoubleValue(sub); ok {
					return &n
				}
			}
			if found := findBalanceValue(sub); found != nil {
				return found
			}
		}
	case []any:
		for _, sub := range t {
			if found := findBalanceValue(sub); found != nil {
				return found
			}
		}
	}
	return nil
}

// findRawBillingBalance mirrors findRawBillingBalance(in:): a dict carrying
// both a non-empty customerID and a numeric balance.
func findRawBillingBalance(v any) *float64 {
	switch t := v.(type) {
	case map[string]any:
		if _, ok := t["balance"]; ok {
			customerID, ok := t["customerID"].(string)
			if !ok || strings.TrimSpace(customerID) == "" {
				return nil
			}
			if n, ok := zenDoubleValue(t["balance"]); ok {
				return &n
			}
			return nil
		}
		for _, sub := range t {
			if found := findRawBillingBalance(sub); found != nil {
				return found
			}
		}
	case []any:
		for _, sub := range t {
			if found := findRawBillingBalance(sub); found != nil {
				return found
			}
		}
	}
	return nil
}

func isExplicitBalanceAmountKey(key string) bool {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return explicitBalanceKeys[b.String()]
}

// zenDoubleValue mirrors doubleValue in the Zen parser: numbers and numeric
// strings with commas stripped; booleans rejected.
func zenDoubleValue(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case float64:
		return t, true
	case string:
		s := strings.ReplaceAll(strings.TrimSpace(t), ",", "")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func extractZenDollar(re *regexp.Regexp, text string) *float64 {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	s := strings.ReplaceAll(m[1], ",", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func extractZenNumber(re *regexp.Regexp, text string) *float64 {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return nil
	}
	return &f
}
