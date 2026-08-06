package provider

import (
	"fmt"
	"sort"
	"sync"
)

var (
	mu       sync.RWMutex
	registry = map[string]Provider{}
)

// Register adds a provider; panics on duplicate ID (registration happens in
// init() functions, so a duplicate is a programming error).
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := registry[p.ID()]; ok {
		panic(fmt.Sprintf("provider %q already registered", p.ID()))
	}
	registry[p.ID()] = p
}

// All returns registered providers sorted by ID.
func All() []Provider {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Provider, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Get returns the provider with the given ID.
func Get(id string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[id]
	return p, ok
}
