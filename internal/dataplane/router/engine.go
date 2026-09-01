package router

import (
	"fmt"
	"net/url"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
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
		route := &cfg.Routes[i]

		if int(route.Service) >= len(cfg.Services.Items) {
			return nil, fmt.Errorf(
				"route %q references invalid service ID %d",
				route.Name,
				route.Service,
			)
		}

		service := &cfg.Services.Items[route.Service]

		upstreamURL, err := url.Parse(service.Upstream)
		if err != nil {
			return nil, fmt.Errorf(
				"route %q references invalid upstream %q: %w",
				route.Name,
				service.Upstream,
				err,
			)
		}

		entries = append(entries, &RouteIndexEntry{
			Route:       route,
			UpstreamURL: upstreamURL,
		})
	}

	trie := BuildRadixTrie(entries)

	return &Engine{trie: trie}, nil
}

// Lookup returns route candidates that match the provided request path.
// It returns nil when called on a nil engine or with a nil path.
func (engine *Engine) Lookup(path string) []*RouteIndexEntry {
	if engine == nil || path == "" {
		return nil
	}
	return engine.trie.Lookup(path)
}
