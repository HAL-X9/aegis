package router

import (
	"fmt"

	"github.com/aegis/internal/controlplane/snapshot"
)

// Engine encapsulates compiled routing structures required at request time.
type Engine struct {
	trie *RadixTrie
}

// BuildEngine prepares lookup structures from an already-compiled control-plane snapshot.
func BuildEngine(cfg *snapshot.CompiledConfig) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	entries := make([]*RouteIndexEntry, 0, len(cfg.Routes))
	for i := range cfg.Routes {
		entries = append(entries, &RouteIndexEntry{
			Route: &cfg.Routes[i],
		})
	}

	trie := BuildRadixTrie(entries)

	return &Engine{trie: trie}, nil
}

// Lookup returns route candidates that match the provided request path.
// It returns nil when called on a nil engine or with a nil path.
func (engine *Engine) Lookup(path []byte) []*RouteIndexEntry {
	if engine == nil || path == nil {
		return nil
	}
	return engine.trie.Lookup(path)
}
