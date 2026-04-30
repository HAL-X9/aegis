package router

import (
	"fmt"

	"github.com/aegis/internal/controlplane/compiler"
	"github.com/aegis/internal/controlplane/model"
)

// Engine encapsulates compiled routing structures required at request time.
type Engine struct {
	trie *RadixTrie
}

// BuildEngine compiles routing configuration and prepares app lookup
// structures used by the dataplane.
func BuildEngine(config *model.GatewayConfig) (*Engine, error) {
	compiled, err := compiler.Compile(config)
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

// Lookup returns route candidates that match the provided request path.
// It returns nil when called on a nil engine or with a nil path.
func (engine *Engine) Lookup(path []byte) []*RouteIndexEntry {
	if engine == nil || path == nil {
		return nil
	}
	return engine.trie.Lookup(path)
}
