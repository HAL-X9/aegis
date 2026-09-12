package router

import (
	"fmt"
	"math"
)

// noNode is the sentinel for "no child" — index 0 is a valid node (the
// root), so we can't use 0 as "absent" the way a nil pointer would work.
const noNode = ^uint32(0)

// FlatNode is a single pointer-free trie node living in a contiguous
// arena. Every cross-reference is an index into one of FlatTrie's own
// slices, never a pointer — so the whole trie is one GC-opaque memory
// block, and sibling/child access stays cache-friendly on the hot path.
//
// Field order and set are unchanged from the original layout deliberately:
// keeping sizeof(FlatNode) minimal maximizes how many sibling nodes share
// a cache line during findChild's scan, which matters more for real
// route-table fan-outs (typically single digits) than shaving one
// indirection per comparison would.
type FlatNode struct {
	PrefixOffset uint32 // offset into FlatTrie.prefixes
	PrefixLen    uint16

	FirstChild uint32 // index into FlatTrie.nodes; children are contiguous
	ChildCount uint16 // and sorted by first prefix byte

	ParamChild    uint32 // index into FlatTrie.nodes, or noNode
	WildcardChild uint32 // index into FlatTrie.nodes, or noNode

	FirstRoute uint32 // offset into FlatTrie.routeRefs
	RouteCount uint16
}

// FlatTrie is the immutable, pointer-free radix trie consumed on the
// request hot path. Lookup never allocates: it walks nodes by index and
// returns a window into routeRefs, which is itself a sub-slice of an
// existing arena — never a copy.
type FlatTrie struct {
	nodes     []FlatNode
	prefixes  []byte   // all edge labels, one shared blob
	routeRefs []uint32 // all candidate lists, concatenated
}

// Flatten converts a build-time RadixTrie into an immutable FlatTrie.
// This runs once per config reload (control-plane); it is free to
// allocate, sort, and use temporary maps.
func Flatten(trie *RadixTrie) (*FlatTrie, error) {
	if trie == nil || trie.root == nil {
		return &FlatTrie{}, nil
	}

	b := &flatBuilder{}
	if err := b.add(trie.root); err != nil {
		return nil, err
	}

	return &FlatTrie{
		nodes:     b.nodes,
		prefixes:  b.prefixes,
		routeRefs: b.routeRefs,
	}, nil
}

type flatBuilder struct {
	nodes     []FlatNode
	prefixes  []byte
	routeRefs []uint32
}

// add performs a breadth-first flattening: for every node it reserves
// contiguous placeholder slots for all of that node's direct static
// children *before* descending into any of their subtrees. That's what
// keeps FirstChild, FirstChild+ChildCount a valid contiguous range even
// though each child's own descendants are appended later, out of order
// relative to its siblings.
func (b *flatBuilder) add(root *RadixNode) error {
	type queued struct {
		raw *RadixNode
		id  uint32
	}

	b.nodes = append(b.nodes, FlatNode{})
	queue := []queued{{root, 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		n := cur.raw

		if len(n.prefix) > math.MaxUint16 {
			return fmt.Errorf("flatten: path segment %q exceeds max edge length", n.prefix)
		}
		if len(n.candidates) > math.MaxUint16 {
			return fmt.Errorf("flatten: node has %d candidates, exceeds uint16 range", len(n.candidates))
		}
		if len(n.children) > math.MaxUint16 {
			return fmt.Errorf("flatten: node has %d children, exceeds uint16 range", len(n.children))
		}

		prefixOffset := uint32(len(b.prefixes))
		b.prefixes = append(b.prefixes, n.prefix...)

		firstRoute := uint32(len(b.routeRefs))
		b.routeRefs = append(b.routeRefs, n.candidates...)

		sorted := sortedChildren(n.children)

		firstChild := noNode
		if len(sorted) > 0 {
			firstChild = uint32(len(b.nodes))
			for _, c := range sorted {
				childID := uint32(len(b.nodes))
				b.nodes = append(b.nodes, FlatNode{})
				queue = append(queue, queued{c, childID})
			}
		}

		paramChild, wildcardChild := noNode, noNode
		if n.paramChild != nil {
			paramChild = uint32(len(b.nodes))
			b.nodes = append(b.nodes, FlatNode{})
			queue = append(queue, queued{n.paramChild, paramChild})
		}
		if n.wildcardChild != nil {
			wildcardChild = uint32(len(b.nodes))
			b.nodes = append(b.nodes, FlatNode{})
			queue = append(queue, queued{n.wildcardChild, wildcardChild})
		}

		b.nodes[cur.id] = FlatNode{
			PrefixOffset:  prefixOffset,
			PrefixLen:     uint16(len(n.prefix)),
			FirstChild:    firstChild,
			ChildCount:    uint16(len(sorted)),
			ParamChild:    paramChild,
			WildcardChild: wildcardChild,
			FirstRoute:    firstRoute,
			RouteCount:    uint16(len(n.candidates)),
		}
	}

	return nil
}

// sortedChildren orders static children by first prefix byte. The radix
// invariant (findChildByFirstByte) already guarantees uniqueness; sorting
// gives findChild a predictable order to scan and lets it exit as soon as
// it passes the target byte, and keeps physically adjacent trie levels
// adjacent in memory.
func sortedChildren(children []*RadixNode) []*RadixNode {
	if len(children) == 0 {
		return nil
	}
	out := append([]*RadixNode(nil), children...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].prefix[0] > out[j].prefix[0]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Lookup resolves route-ID candidates for path.
//
// Priority: static > param > wildcard, matching Insert's compression.
// Zero heap allocations: the returned slice is a window into the trie's
// own routeRefs arena, not a copy, and must not be retained past the
// snapshot's lifetime.
//
// This stays recursive deliberately: recursion depth here is bounded by
// the number of path segments in a URL (single digits in practice), and
// Go's call overhead for that is cheaper than the fixed cost of setting
// up any explicit backtrack structure on every call, even for paths that
// never need to backtrack at all.
func (t *FlatTrie) Lookup(path string) []uint32 {
	if t == nil || len(t.nodes) == 0 {
		return nil
	}
	return t.lookup(0, path)
}

func (t *FlatTrie) lookup(nodeID uint32, path string) []uint32 {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	node := &t.nodes[nodeID]

	if len(path) == 0 {
		if node.RouteCount > 0 {
			return t.candidatesOf(nodeID)
		}
		if node.WildcardChild != noNode {
			return t.candidatesOf(node.WildcardChild)
		}
		return nil
	}

	end := 0
	for end < len(path) && path[end] != '/' {
		end++
	}
	segment, rest := path[:end], path[end:]

	if childID, ok := t.matchStaticSegment(nodeID, segment); ok {
		if result := t.lookup(childID, rest); result != nil {
			return result
		}
	}

	if node.ParamChild != noNode {
		if result := t.lookup(node.ParamChild, rest); result != nil {
			return result
		}
	}

	if node.WildcardChild != noNode {
		return t.candidatesOf(node.WildcardChild)
	}

	return nil
}

// matchStaticSegment walks the compressed static edges under nodeID that
// together spell out segment exactly — mirroring insertStaticSegment at
// build time. Keeping the two in lockstep is what makes the compression
// safe to match against.
func (t *FlatTrie) matchStaticSegment(nodeID uint32, segment string) (uint32, bool) {
	for len(segment) > 0 {
		node := &t.nodes[nodeID]

		childID, ok := t.findChild(node, segment[0])
		if !ok {
			return 0, false
		}
		if !t.hasPrefix(childID, segment) {
			return 0, false
		}

		nodeID = childID
		segment = segment[t.nodes[childID].PrefixLen:]
	}
	return nodeID, true
}

// findChild scans node's static children for one starting with b. Children
// are sorted by first prefix byte at build time (sortedChildren), so the
// scan exits as soon as it passes b. For the small fan-outs typical of
// real route tables (a handful of siblings per node), a linear scan over
// a sorted, contiguous, cache-resident range beats a binary search: the
// comparisons are sequential and mostly-not-taken, which branch predictors
// handle far better than binary search's data-dependent jumps.
func (t *FlatTrie) findChild(node *FlatNode, b byte) (uint32, bool) {
	first := node.FirstChild
	count := uint32(node.ChildCount)

	for i := range count {
		idx := first + i
		fb := t.prefixes[t.nodes[idx].PrefixOffset]

		if fb == b {
			return idx, true
		}
		if fb > b {
			break
		}
	}

	return 0, false
}

// hasPrefix reports whether node id's edge label is a full prefix of s.
func (t *FlatTrie) hasPrefix(id uint32, s string) bool {
	n := &t.nodes[id]
	pl := int(n.PrefixLen)
	if pl > len(s) {
		return false
	}
	off := n.PrefixOffset
	for i := range pl {
		if t.prefixes[off+uint32(i)] != s[i] {
			return false
		}
	}
	return true
}

func (t *FlatTrie) candidatesOf(nodeID uint32) []uint32 {
	n := &t.nodes[nodeID]
	if n.RouteCount == 0 {
		return nil
	}
	return t.routeRefs[n.FirstRoute : n.FirstRoute+uint32(n.RouteCount)]
}
