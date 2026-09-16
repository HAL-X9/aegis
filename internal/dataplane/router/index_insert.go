package router

import "github.com/HAL-X9/aegis/internal/controlplane/snapshot"

// RadixNode is a build-time radix trie node. It exists only while the
// control plane constructs the routing table; the request hot path never
// touches it — see FlatTrie in flat.go.
type RadixNode struct {
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

// Insert registers routeID under the provided normalized path.
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
			remaining = ""
		default:
			node = insertStaticSegment(node, segment)
		}
	}

	node.candidates = append(node.candidates, routeID)
}

// insertStaticSegment / splitChild / findChildByFirstByte / nextSegment /
// commonPrefixLen — unchanged from your original index_insert.go.
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
			node = child
			segment = segment[common:]
			continue
		}

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

func nextSegment(path string) (segment, rest string) {
	for i, b := range path {
		if b == '/' {
			return path[:i], path[i:]
		}
	}
	return path, ""
}

func commonPrefixLen(a, b string) int {
	n := min(len(b), len(a))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}
