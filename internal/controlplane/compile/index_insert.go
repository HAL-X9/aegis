package compile

import (
	"strings"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// radixNode is a build-time radix trie node. It exists only while the
// control plane constructs the route index; index_flatten.go turns the
// finished tree into the snapshot.RouteIndex that the request path reads.
//
// Every static segment is stored together with its terminating '/', so
// "/abc" and "/ab/c" diverge at "c" versus "/" instead of sharing a node.
// A node therefore sits on a segment boundary exactly when it is the root,
// a param or wildcard node, or its prefix ends in '/'. Only boundary nodes
// carry candidates, paramChild, or wildcardChild.
type radixNode struct {
	// prefix is a run of literal path bytes. It contains '/' only as its
	// last byte, and only on a segment boundary.
	prefix string

	// Static edges. Children have unique first bytes.
	children []*radixNode

	paramChild    *radixNode
	wildcardChild *radixNode

	// candidates holds indices into snapshot.CompiledConfig.Routes, not
	// pointers — there is nothing here for flatten to translate.
	candidates []snapshot.RouteID
}

// radixTrie is a build-time radix path index for compiled routes.
type radixTrie struct {
	root *radixNode
}

// insert registers routeID under path, a validated and normalized route
// path: "/" or "/"-separated segments, each a literal, a ":name" parameter,
// or a trailing "*" / "*name" wildcard.
//
// insert runs at setup time only, so it favors correctness and real radix
// compression over avoiding allocations. The request path never sees this
// tree; it runs against the arena produced by flatten.
func (t *radixTrie) insert(path string, routeID snapshot.RouteID) {
	if t.root == nil {
		t.root = &radixNode{}
	}
	node := t.root

	for rest := strings.TrimPrefix(path, "/"); rest != ""; {
		var segment string
		segment, rest, _ = strings.Cut(rest, "/")

		switch {
		case strings.HasPrefix(segment, ":"):
			if node.paramChild == nil {
				node.paramChild = &radixNode{}
			}
			node = node.paramChild
		case strings.HasPrefix(segment, "*"):
			if node.wildcardChild == nil {
				node.wildcardChild = &radixNode{}
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
func insertStatic(node *radixNode, key string) *radixNode {
	for len(key) > 0 {
		idx, child := findChildByFirstByte(node, key[0])
		if child == nil {
			leaf := &radixNode{prefix: key}
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

		leaf := &radixNode{prefix: key}
		split.children = append(split.children, leaf)
		return leaf
	}

	return node
}

func splitChild(parent *radixNode, idx int, child *radixNode, common int) *radixNode {
	split := &radixNode{
		prefix:   child.prefix[:common],
		children: []*radixNode{child},
	}

	child.prefix = child.prefix[common:]
	parent.children[idx] = split

	return split
}

func findChildByFirstByte(node *radixNode, b byte) (int, *radixNode) {
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
