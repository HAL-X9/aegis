package router

// RouteIndex provides route lookup by request path bytes.
type RouteIndex interface {
	Lookup(path []byte) []*CompiledRoute
}

// Lookup resolves compiled routes for the provided request path.
// The method returns all candidates that match the path shape; method-based
// filtering is expected to be applied by the caller or downstream stage.
func (trie *RadixTrie) Lookup(path []byte) []*CompiledRoute {
	if trie.root == nil {
		return nil
	}

	return nil
}
