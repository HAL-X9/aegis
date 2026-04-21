package router

import "bytes"

// PathIndex provides route lookup by request path bytes.
type PathIndex interface {
	Lookup(path []byte) []*RadixNodeCandidates
}

// RadixNode stores a compressed path segment and links to child nodes.
type RadixNode struct {
	prefix []byte

	children      []*RadixNode
	paramChild    *RadixNode
	wildcardChild *RadixNode

	candidates []*RadixNodeCandidates
}

// RadixTrie is a radix-based path index for compiled routes.
type RadixTrie struct {
	root *RadixNode
}

// RadixNodeCandidates groups route candidates and their method mask.
type RadixNodeCandidates struct {
	Routes     *CompileRoute
	MethodMask uint32
}

// Insert adds a route candidate for the provided path into the trie.
func (trie *RadixTrie) Insert(path string, candidate *RadixNodeCandidates) {
	if trie.root == nil {
		trie.root = &RadixNode{}
	}

	node := trie.root
	offset := 0
	slash := byte('/')

	for i := 0; i <= len(path); i++ {

		if i == len(path) || path[i] == slash {
			segment := []byte(path[offset:i])
			offset = i + 1

			var next *RadixNode

			for _, child := range node.children {
				if bytes.Equal(child.prefix, segment) {
					next = child
					break
				}
			}

			if next == nil {
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

			node = next
		}
	}

	node.candidates = append(node.candidates, candidate)
}
