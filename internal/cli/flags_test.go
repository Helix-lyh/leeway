package cli

import (
	"reflect"
	"testing"
)

func TestArrangeArgs(t *testing.T) {
	valued := map[string]bool{
		"provider": true,
		"account":  true,
		"watch":    true,
		"json":     false,
	}
	cases := []struct {
		in, want []string
	}{
		{nil, nil},
		{[]string{"--json"}, []string{"--json"}},
		{[]string{"cursor", "--json"}, []string{"--json", "cursor"}},
		{[]string{"cursor", "work", "--json"}, []string{"--json", "cursor", "work"}},
		{[]string{"--watch", "30", "cursor"}, []string{"--watch", "30", "cursor"}},
		{[]string{"cursor", "--watch", "30"}, []string{"--watch", "30", "cursor"}},
		{[]string{"--provider", "cursor", "work"}, []string{"--provider", "cursor", "work"}},
		{[]string{"cursor", "--account=work"}, []string{"--account=work", "cursor"}},
		{[]string{"--", "--json", "cursor"}, []string{"--", "--json", "cursor"}},
		{[]string{"-h", "cursor"}, []string{"-h", "cursor"}},
	}
	for _, c := range cases {
		got := arrangeArgs(c.in, valued)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("arrangeArgs(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseProviderAccountPositionals(t *testing.T) {
	var provider, account string
	var jsonOut bool
	fs := newFlagSet("use", &jsonOut)
	fs.providerAccount(&provider, &account)
	if !fs.Parse([]string{"cursor", "work", "--json"}) {
		t.Fatal("parse")
	}
	if provider != "cursor" || account != "work" || !jsonOut {
		t.Fatalf("provider=%q account=%q json=%v", provider, account, jsonOut)
	}
}

func TestParseFlagWinsOverPositional(t *testing.T) {
	var provider, account string
	fs := newFlagSet("use", nil)
	fs.providerAccount(&provider, &account)
	if fs.Parse([]string{"--provider", "grok", "--account", "home", "cursor"}) {
		t.Fatal("leftover positional should be unexpected when flags already set")
	}
	fs = newFlagSet("use", nil)
	provider, account = "", ""
	fs.providerAccount(&provider, &account)
	if !fs.Parse([]string{"--provider", "grok", "--account", "home"}) {
		t.Fatal("parse")
	}
	if provider != "grok" || account != "home" {
		t.Fatalf("provider=%q account=%q", provider, account)
	}
}

func TestParseUnexpectedPositional(t *testing.T) {
	var provider, account string
	fs := newFlagSet("use", nil)
	fs.providerAccount(&provider, &account)
	if fs.Parse([]string{"cursor", "work", "extra"}) {
		t.Fatal("expected unexpected argument")
	}
	if fs.extraUnexpected != "extra" {
		t.Fatalf("extraUnexpected = %q", fs.extraUnexpected)
	}
}
