package router

import "bytes"

// RadixNode represents a single trie node in the radix index.
// The node stores fixed-segment edges in children and dedicated edges for
// parameter and wildcard matches.
type RadixNode struct {
	// prefix stores a static path segment for this edge.
	prefix []byte

	// children contains static-segment descendants.
	children []*RadixNode
	// paramChild stores the descendant for named parameters (for example, :id).
	paramChild *RadixNode
	// wildcardChild stores the descendant for wildcard captures (for example, *rest).
	wildcardChild *RadixNode

	// candidates contains routes that terminate at this node.
	candidates []*RouteIndexEntry
}

// RadixTrie is a radix-based path index for compiled routes.
type RadixTrie struct {
	root *RadixNode
}

// RouteIndexEntry groups route candidates and their method mask.
type RouteIndexEntry struct {
	Route *CompiledRoute

	/*
		TODO:
		MethodMask MethodMask

		HeaderMatcher CompiledHeaderMatcher
		QueryMatcher  CompiledQueryMatcher
		etc
	*/
}

// Insert registers a route entry under the provided normalized path.
func (trie *RadixTrie) Insert(path string, entry *RouteIndexEntry) {
	if trie.root == nil {
		trie.root = &RadixNode{}
	}

	node := trie.root
	offset := 0
	slash := byte('/')

	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == slash {
			// Extract one path segment and advance to the next segment start.
			segment := []byte(path[offset:i])
			offset = i + 1

			var next *RadixNode

			// Prefer an existing static edge for deterministic lookup behavior.
			for _, child := range node.children {
				if bytes.Equal(child.prefix, segment) {
					next = child
					break
				}
			}

			if next == nil {
				// Dynamic edges are keyed by segment type (:param or *wildcard).
				if len(segment) > 0 && segment[0] == ':' {
					if node.paramChild == nil {
						node.paramChild = &RadixNode{}
					}
					next = node.paramChild
				} else if len(segment) > 0 && segment[0] == '*' {
					if node.wildcardChild == nil {
						node.wildcardChild = &RadixNode{}
					}
					next = node.wildcardChild
				} else {
					next = &RadixNode{
						prefix: segment,
					}
					node.children = append(node.children, next)
				}
			}

			// Descend into the selected edge and continue processing.
			node = next
		}
	}

	// Attach the route to the terminal node for this path.
	node.candidates = append(node.candidates, entry)
}
