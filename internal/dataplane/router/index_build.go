package router

import "github.com/HAL-X9/aegis/internal/snapshot"

// BuildRadixTrie constructs a build-time radix path index from compiled
// routes. This is a control-plane operation — not part of the request path.
func BuildRadixTrie(routes []snapshot.CompiledRoute) *RadixTrie {
	trie := &RadixTrie{}
	for i := range routes {
		trie.Insert(routes[i].Match.PathPrefix, snapshot.RouteID(i))
	}
	return trie
}
