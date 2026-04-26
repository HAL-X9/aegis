package router

import "bytes"

// RouteIndex provides route lookup by request path bytes.
type RouteIndex interface {
	Lookup(path []byte) []*RouteIndexEntry
}

// Lookup resolves compiled routes for the provided request path.
// The method returns all candidates that match the path shape; method-based
// filtering is expected to be applied by the caller or downstream stage.
func (trie *RadixTrie) Lookup(path []byte) []*RouteIndexEntry {
	if trie.root == nil {
		return nil
	}

	node := trie.root
	slash := byte('/')
	offset := 0

	for i := 0; i <= len(path); i++ {
		var segment []byte
		var next *RadixNode

		if i == len(path) || path[i] == slash {
			segment = path[offset:i]
			offset = i + 1

			for _, child := range node.children {
				if bytes.Equal(segment, child.prefix) {
					next = child
					break
				}
			}

			if next == nil && node.paramChild != nil && len(segment) > 0 {
				next = node.paramChild
			}

			if next == nil && node.wildcardChild != nil {
				return node.wildcardChild.candidates
			}

			if next == nil {
				return nil
			}

			node = next
		}
	}

	return node.candidates
}
