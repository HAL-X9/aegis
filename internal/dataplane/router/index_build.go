package router

// BuildRadixTrie constructs a radix path index from compiled route candidates.
func BuildRadixTrie(routes []*RouteIndexEntry) *RadixTrie {
	trie := &RadixTrie{}

	for _, route := range routes {
		trie.Insert(route.Route.Match.PathPrefix, route)
	}

	return trie
}
