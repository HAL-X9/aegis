package router

import (
	"fmt"

	"github.com/aegis/internal/config/controlplane"
)

// Engine encapsulates compiled routing structures required at request time.
type Engine struct {
	trie *RadixTrie
}

// BuildEngine compiles routing configuration and prepares runtime lookup
// structures used by the dataplane.
func BuildEngine(config *controlplane.AegisManifest) (*Engine, error) {
	compiled, err := Compile(config)
	if err != nil {
		return nil, fmt.Errorf("failed to compile routing configuration: %w", err)
	}
	if compiled == nil {
		return nil, fmt.Errorf("invalid compile result: nil manifest with no error")
	}

	entries := make([]*RouteIndexEntry, 0, len(compiled.Routes))

	for i := range compiled.Routes {
		entries = append(entries, &RouteIndexEntry{
			Route: &compiled.Routes[i],
		})
	}

	trie := BuildRadixTrie(entries)

	engine := &Engine{trie: trie}

	return engine, nil
}
