package provider

import (
	"context"
	"testing"

	"leeway/internal/model"
)

type testProvider struct{ id string }

func (p testProvider) ID() string          { return p.id }
func (p testProvider) DisplayName() string { return p.id }
func (p testProvider) AuthSpec() AuthSpec  { return AuthSpec{} }
func (p testProvider) DeclaredWindows() []WindowSpec {
	return []WindowSpec{{ID: "primary", Title: "Weekly", Kind: PercentWindow}}
}
func (p testProvider) Fetch(context.Context, FetchRequest) (*model.Snapshot, error) {
	return nil, nil
}

func TestRegisterAllSorted(t *testing.T) {
	// Use a fresh registry to avoid clobbering the real one.
	old := registry
	registry = map[string]Provider{}
	defer func() { registry = old }()

	Register(testProvider{"zeta"})
	Register(testProvider{"alpha"})
	Register(testProvider{"mid"})

	all := All()
	if len(all) != 3 {
		t.Fatalf("want 3 providers, got %d", len(all))
	}
	if all[0].ID() != "alpha" || all[1].ID() != "mid" || all[2].ID() != "zeta" {
		t.Fatalf("not sorted: %s %s %s", all[0].ID(), all[1].ID(), all[2].ID())
	}
	if _, ok := Get("mid"); !ok {
		t.Fatal("Get(mid) failed")
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("Get(nope) should fail")
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	old := registry
	registry = map[string]Provider{}
	defer func() { registry = old }()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	Register(testProvider{"dup"})
	Register(testProvider{"dup"})
}
