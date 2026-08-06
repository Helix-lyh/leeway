package codex

import (
	"encoding/json"
	"strconv"
	"strings"
)

// This file decodes the ChatGPT backend-API usage payload
// (CodexUsageResponse in CodexOAuthUsageFetcher.swift). Decoding is lossy on
// purpose, mirroring the Swift `try?` calls: a malformed nested value never
// discards its valid siblings, and only the top-level JSON is strict.

type usageResponse struct {
	planType             string
	rateLimit            *rateLimitDetails
	credits              *creditDetails
	individualLimit      *spendControlLimit
	additionalRateLimits []additionalRateLimit
}

type rateLimitDetails struct {
	primaryWindow   *windowSnapshot
	secondaryWindow *windowSnapshot
	individualLimit *spendControlLimit
}

// windowSnapshot mirrors CodexUsageResponse.WindowSnapshot. Swift decodes it
// strictly (used_percent/reset_at/limit_window_seconds are non-optional Int),
// so all three must decode to numbers here too.
type windowSnapshot struct {
	usedPercent        *int
	resetAt            *int
	limitWindowSeconds *int
}

type spendControlLimit struct {
	limit            *float64
	used             *float64
	remainingPercent *float64
	resetsAt         *int64
}

type creditDetails struct {
	hasCredits bool
	unlimited  bool
	balance    *float64
}

type additionalRateLimit struct {
	limitName      string
	meteredFeature string
	rateLimit      *rateLimitDetails
}

type accountItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// parseUsageResponse decodes the wham/usage payload. Errors are top-level
// JSON failures (the Swift strict decode that becomes invalidResponse).
func parseUsageResponse(data []byte) (*usageResponse, error) {
	var raw struct {
		PlanType             string          `json:"plan_type"`
		RateLimit            json.RawMessage `json:"rate_limit"`
		Credits              json.RawMessage `json:"credits"`
		IndividualLimit      json.RawMessage `json:"individual_limit"`
		IndividualLimitCamel json.RawMessage `json:"individualLimit"`
		AdditionalRateLimits json.RawMessage `json:"additional_rate_limits"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	resp := &usageResponse{planType: raw.PlanType}
	resp.rateLimit = decodeRateLimit(raw.RateLimit)
	resp.credits = decodeCredits(raw.Credits)
	resp.individualLimit = decodeSpendControl(firstRaw(raw.IndividualLimit, raw.IndividualLimitCamel))
	resp.additionalRateLimits = decodeAdditionalRateLimits(raw.AdditionalRateLimits)
	return resp, nil
}

func decodeRateLimit(raw json.RawMessage) *rateLimitDetails {
	if isNull(raw) {
		return nil
	}
	var r struct {
		Primary       json.RawMessage `json:"primary_window"`
		Secondary     json.RawMessage `json:"secondary_window"`
		IndLimit      json.RawMessage `json:"individual_limit"`
		IndLimitCamel json.RawMessage `json:"individualLimit"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil
	}
	return &rateLimitDetails{
		primaryWindow:   decodeWindow(r.Primary),
		secondaryWindow: decodeWindow(r.Secondary),
		individualLimit: decodeSpendControl(firstRaw(r.IndLimit, r.IndLimitCamel)),
	}
}

func decodeWindow(raw json.RawMessage) *windowSnapshot {
	if isNull(raw) {
		return nil
	}
	var w struct {
		UsedPercent        *int `json:"used_percent"`
		ResetAt            *int `json:"reset_at"`
		LimitWindowSeconds *int `json:"limit_window_seconds"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil
	}
	if w.UsedPercent == nil || w.ResetAt == nil || w.LimitWindowSeconds == nil {
		return nil
	}
	return &windowSnapshot{
		usedPercent:        w.UsedPercent,
		resetAt:            w.ResetAt,
		limitWindowSeconds: w.LimitWindowSeconds,
	}
}

func decodeSpendControl(raw json.RawMessage) *spendControlLimit {
	if isNull(raw) {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return &spendControlLimit{
		limit:            flexFloat(m["limit"]),
		used:             flexFloat(m["used"]),
		remainingPercent: flexFloat(firstRaw(m["remaining_percent"], m["remainingPercent"])),
		resetsAt:         flexInt(firstRaw(m["resets_at"], m["resetsAt"])),
	}
}

func decodeCredits(raw json.RawMessage) *creditDetails {
	if isNull(raw) {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	c := &creditDetails{}
	if v := m["has_credits"]; !isNull(v) {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			c.hasCredits = b
		}
	}
	if v := m["unlimited"]; !isNull(v) {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			c.unlimited = b
		}
	}
	c.balance = flexFloat(m["balance"])
	return c
}

func decodeAdditionalRateLimits(raw json.RawMessage) []additionalRateLimit {
	if isNull(raw) {
		return nil
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil
	}
	out := make([]additionalRateLimit, 0, len(elems))
	for _, e := range elems {
		if isNull(e) {
			continue
		}
		var m struct {
			LimitName      string          `json:"limit_name"`
			MeteredFeature string          `json:"metered_feature"`
			RateLimit      json.RawMessage `json:"rate_limit"`
		}
		if err := json.Unmarshal(e, &m); err != nil {
			continue // one malformed entry cannot discard its valid siblings
		}
		out = append(out, additionalRateLimit{
			limitName:      m.LimitName,
			meteredFeature: m.MeteredFeature,
			rateLimit:      decodeRateLimit(m.RateLimit),
		})
	}
	return out
}

// --- flexible scalar decoding (CodexUsageResponse.SpendControlLimitSnapshot) ---

// flexFloat decodes a Double that the API may also send as an Int or String.
func flexFloat(raw json.RawMessage) *float64 {
	if isNull(raw) {
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if f, err := n.Float64(); err == nil {
			return &f
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return &f
		}
	}
	return nil
}

// flexInt decodes an Int that the API may also send as a Double or String.
func flexInt(raw json.RawMessage) *int64 {
	if isNull(raw) {
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if i, err := n.Int64(); err == nil {
			return &i
		}
		if f, err := n.Float64(); err == nil {
			i := int64(f)
			return &i
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		trimmed := strings.TrimSpace(s)
		if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return &i
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			i := int64(f)
			return &i
		}
	}
	return nil
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

func firstRaw(a, b json.RawMessage) json.RawMessage {
	if isNull(a) {
		return b
	}
	return a
}
