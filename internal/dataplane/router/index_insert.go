package router

import (
	"strings"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// RadixNode is a build-time radix trie node. It exists only while the
// control plane constructs the routing table; the request hot path never
// touches it — see FlatTrie in flat.go.
//
// Every static segment is stored together with its terminating '/', so
// "/abc" and "/ab/c" diverge at "c" versus "/" instead of sharing a node.
// A node therefore sits on a segment boundary exactly when it is the root,
// a param or wildcard node, or its prefix ends in '/'. Only boundary nodes
// carry candidates, paramChild, or wildcardChild.
type RadixNode struct {
	// prefix is a run of literal path bytes. It contains '/' only as its
	// last byte, and only on a segment boundary.
	prefix string

	// Static edges. Children have unique first bytes.
	children []*RadixNode

	paramChild    *RadixNode
	wildcardChild *RadixNode

	// candidates holds indices into snapshot.CompiledConfig.Routes, not
	// pointers — there is nothing here for Flatten to translate.
	candidates []snapshot.RouteID
}

// RadixTrie is a build-time radix path index for compiled routes.
type RadixTrie struct {
	root *RadixNode
}

// Insert registers routeID under path, a validated and normalized route
// path: "/" or "/"-separated segments, each a literal, a ":name" parameter,
// or a trailing "*" / "*name" wildcard.
//
// Insert is a setup-time operation (route table construction), so it
// favors correctness and real radix compression over avoiding
// allocations. The request hot path runs entirely against the flattened
// arena produced by Flatten — see flat.go.
func (t *RadixTrie) Insert(path string, routeID snapshot.RouteID) {
	if t.root == nil {
		t.root = &RadixNode{}
	}
	node := t.root

	for rest := strings.TrimPrefix(path, "/"); rest != ""; {
		var segment string
		segment, rest = cutSegment(rest)

		switch {
		case strings.HasPrefix(segment, ":"):
			if node.paramChild == nil {
				node.paramChild = &RadixNode{}
			}
			node = node.paramChild
		case strings.HasPrefix(segment, "*"):
			if node.wildcardChild == nil {
				node.wildcardChild = &RadixNode{}
			}
			node = node.wildcardChild
			// A wildcard consumes the rest of the request path, so nothing
			// can follow it; validation rejects such routes upfront.
			rest = ""
		default:
			node = insertStatic(node, segment+"/")
		}
	}

	node.candidates = append(node.candidates, routeID)
}

// insertStatic inserts key below node with radix compression and returns
// the node at which key ends.
func insertStatic(node *RadixNode, key string) *RadixNode {
	for len(key) > 0 {
		idx, child := findChildByFirstByte(node, key[0])
		if child == nil {
			leaf := &RadixNode{prefix: key}
			node.children = append(node.children, leaf)
			return leaf
		}

		common := commonPrefixLen(child.prefix, key)

		if common == len(child.prefix) {
			node = child
			key = key[common:]
			continue
		}

		split := splitChild(node, idx, child, common)

		key = key[common:]
		if len(key) == 0 {
			return split
		}

		leaf := &RadixNode{prefix: key}
		split.children = append(split.children, leaf)
		return leaf
	}

	return node
}

func splitChild(parent *RadixNode, idx int, child *RadixNode, common int) *RadixNode {
	split := &RadixNode{
		prefix:   child.prefix[:common],
		children: []*RadixNode{child},
	}

	child.prefix = child.prefix[common:]
	parent.children[idx] = split

	return split
}

func findChildByFirstByte(node *RadixNode, b byte) (int, *RadixNode) {
	for i, child := range node.children {
		if child.prefix[0] == b {
			return i, child
		}
	}
	return -1, nil
}

func commonPrefixLen(a, b string) int {
	n := min(len(b), len(a))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}
