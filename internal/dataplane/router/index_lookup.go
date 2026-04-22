package router

// RouteIndex provides route lookup by request path bytes.
type RouteIndex interface {
	Lookup(path []byte) []*RadixNodeCandidates
}

func (trie *RadixTrie) Lookup(path []byte) []*RadixNodeCandidates {

	return nil
}
