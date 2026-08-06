package opencodego

import "testing"

func TestParseZenBalanceJSONKeys(t *testing.T) {
	cases := []struct {
		text string
		want float64
	}{
		{`{"zenBalance": 18.5}`, 18.5},
		{`{"zenCurrentBalance": 12.25}`, 12.25},
		{`{"currentBalance": 7}`, 7},
		{`{"currentBalanceUsd": 9.9}`, 9.9},
		{`{"balanceUsd": "3.50"}`, 3.5},
		{`{"usdBalance": 1.25}`, 1.25},
		{`{"nested": {"zenBalance": 4.75}}`, 4.75},
		{`{"arr": [{"currentBalance": 2.5}]}`, 2.5},
		// key normalization: punctuation stripped, case-insensitive
		{`{"current_balance": 11}`, 11}, // "_balance" -> "currentbalance" -> match
	}
	for _, c := range cases {
		if got := parseZenBalance(c.text); got == nil || *got != c.want {
			t.Fatalf("parseZenBalance(%s) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestParseZenBalanceJSONBoolIgnored(t *testing.T) {
	text := `{"currentBalance": true, "zenBalance": 5}`
	got := parseZenBalance(text)
	if got == nil || *got != 5 {
		t.Fatalf("bool balance must be skipped: %v", got)
	}
}

func TestParseZenBalanceDollarPatterns(t *testing.T) {
	// JSON path fails (not JSON), falls back to regex dollar patterns.
	cases := []struct {
		text string
		want float64
	}{
		{`Current balance: $18.50`, 18.5},
		{`Zen balance: $12,345.00`, 12345.0},
		{`現在の残高: $99.99`, 99.99},
		{`balance: $5.00`, 5.0},
		{`token balance shown as $2.25`, 2.25},
		{`garbage before balance, still matches: $0.75`, 0.75},
	}
	for _, c := range cases {
		if got := parseZenBalance(c.text); got == nil || *got != c.want {
			t.Fatalf("parseZenBalance(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestParseZenBalanceNoMatch(t *testing.T) {
	if got := parseZenBalance(`no balance here`); got != nil {
		t.Fatalf("expected nil, got %v", *got)
	}
}

func TestParseZenBillingResponseJSON(t *testing.T) {
	text := `{"data": {"customerID": "cus_abc", "balance": 1850000000}}`
	got := parseZenBillingResponse(text)
	if got == nil || *got != 18.5 {
		t.Fatalf("billing JSON wrong: %v", got)
	}
}

func TestParseZenBillingResponseMissingCustomerID(t *testing.T) {
	// balance without a customerID in the same dict is rejected.
	text := `{"data": {"balance": 1850000000}}`
	if got := parseZenBillingResponse(text); got != nil {
		t.Fatalf("expected nil without customerID, got %v", *got)
	}
}

func TestParseZenBillingResponseJSVariant(t *testing.T) {
	text := `$R[4]=Object.assign({balance:2000000000, customerID:"cus_xyz"});`
	got := parseZenBillingResponse(text)
	if got == nil || *got != 20.0 {
		t.Fatalf("JS billing wrong: %v", got)
	}
}

func TestParseZenBillingResponseNegative(t *testing.T) {
	text := `{"customerID": "cus_1", "balance": -50000000}`
	got := parseZenBillingResponse(text)
	if got == nil || *got != -0.5 {
		t.Fatalf("negative billing wrong: %v", got)
	}
}
