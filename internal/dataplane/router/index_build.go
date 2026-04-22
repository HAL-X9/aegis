package router

// BuildRadixTrie builds a path index from route candidates.
func BuildRadixTrie(routes []*RadixNodeCandidates) *RadixTrie {
	trie := &RadixTrie{}

	for _, r := range routes {
		trie.Insert(r.Route.PathPrefix, r)
	}

	return trie
}
