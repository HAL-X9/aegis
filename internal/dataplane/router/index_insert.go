package router

import (
	"net/url"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

// RadixTrie is a radix-based path index for compiled routes.
type RadixTrie struct {
	root *RadixNode
}

// RouteIndexEntry groups route candidates and their method mask.
type RouteIndexEntry struct {
	Route *snapshot.CompiledRoute

	// UpstreamURL is parsed once during route compilation.
	// It must be treated as immutable after the route index is built.
	UpstreamURL *url.URL
}

// Insert registers a route entry under the provided normalized path.
//
// Insert is a setup-time operation (route table construction), so it
// favors correctness and real radix compression over avoiding
// allocations. Lookup, which runs on the request hot path, does not
// allocate at all — see lookup.go.
func (t *RadixTrie) Insert(path string, entry *RouteIndexEntry) {
	if t.root == nil {
		t.root = &RadixNode{}
	}

	node := t.root
	remaining := path

	for len(remaining) > 0 {
		if remaining[0] == '/' {
			remaining = remaining[1:]
			continue
		}

		segment, rest := nextSegment(remaining)
		remaining = rest

		switch segment[0] {
		case ':':
			if node.paramChild == nil {
				node.paramChild = &RadixNode{}
			}
			node = node.paramChild

		case '*':
			if node.wildcardChild == nil {
				node.wildcardChild = &RadixNode{}
			}
			node = node.wildcardChild
			remaining = "" // wildcard consumes the rest of the path

		default:
			node = insertStaticSegment(node, segment)
		}
	}

	node.candidates = append(node.candidates, entry)
}

// insertStaticSegment inserts one static path segment under node,
// creating or splitting compressed edges as needed, and returns the node
// that represents the end of that segment (i.e. the node the next path
// segment, or the route's candidates, should be attached to).
//
// This is the mirror image of lookupStaticSegment in lookup.go: both
// walk node.children byte-range by byte-range until segment is fully
// consumed. Keeping the two in lockstep is what makes compression safe.
func insertStaticSegment(node *RadixNode, segment string) *RadixNode {
	for len(segment) > 0 {
		idx, child := findChildByFirstByte(node, segment[0])
		if child == nil {
			leaf := &RadixNode{prefix: segment}
			node.children = append(node.children, leaf)
			return leaf
		}

		common := commonPrefixLen(child.prefix, segment)

		if common == len(child.prefix) {
			// The existing edge is fully consumed (possibly with
			// segment fully consumed too, in which case the loop
			// simply exits on the next check). Keep walking its
			// children with whatever remains of segment.
			node = child
			segment = segment[common:]
			continue
		}

		// The existing edge only partially matches: split it at the
		// common boundary so the old suffix and the new suffix become
		// siblings under a shared, newly created parent.
		split := splitChild(node, idx, child, common)

		segment = segment[common:]
		if len(segment) == 0 {
			return split
		}

		leaf := &RadixNode{prefix: segment}
		split.children = append(split.children, leaf)
		return leaf
	}

	return node
}

// splitChild splits child's prefix at byte offset common, replacing it
// in place under parent with a new intermediate node:
//
//	before:  parent -[prefix]-------------> child
//	after:   parent -[prefix[:common]]-> split -[prefix[common:]]-> child
func splitChild(parent *RadixNode, idx int, child *RadixNode, common int) *RadixNode {
	split := &RadixNode{
		prefix:   child.prefix[:common],
		children: []*RadixNode{child},
	}

	child.prefix = child.prefix[common:]
	parent.children[idx] = split

	return split
}

// findChildByFirstByte returns the static child edge starting with b, if
// any. The radix invariant maintained by insertStaticSegment/splitChild
// guarantees at most one such child exists, which is what lets Lookup do
// a single linear scan per level with no backtracking between static
// children.
func findChildByFirstByte(node *RadixNode, b byte) (int, *RadixNode) {
	for i, child := range node.children {
		if child.prefix[0] == b {
			return i, child
		}
	}
	return -1, nil
}

// nextSegment splits off the next '/'-delimited segment from path.
// path must be non-empty and must not start with '/'.
func nextSegment(path string) (segment, rest string) {
	for i, b := range path {
		if b == '/' {
			return path[:i], path[i:]
		}
	}
	return path, ""
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}
