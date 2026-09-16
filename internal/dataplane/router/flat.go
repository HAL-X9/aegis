package router

import (
	"fmt"
	"math"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

// NodeID indexes FlatTrie.nodes. Never comparable with or convertible
// from snapshot.RouteID — they index different arenas.
type NodeID uint32

// noNode is the sentinel for "no child" — index 0 is a valid node (the
// root), so we can't use 0 as "absent" the way a nil pointer would work.
const noNode = NodeID(math.MaxUint32)

type FlatNode struct {
	PrefixOffset uint32
	PrefixLen    uint16

	FirstChild NodeID
	ChildCount uint16

	ParamChild    NodeID
	WildcardChild NodeID

	// FirstRoute/RouteCount describe an offset+length window into
	// routeRefs — an arena slice, not an identifier, so it stays a plain
	// uint32/uint16 rather than a typed ID.
	FirstRoute uint32
	RouteCount uint16
}

type FlatTrie struct {
	nodes     []FlatNode
	prefixes  []byte
	routeRefs []snapshot.RouteID
}

func Flatten(trie *RadixTrie) (*FlatTrie, error) {
	if trie == nil || trie.root == nil {
		return &FlatTrie{}, nil
	}
	b := &flatBuilder{}
	if err := b.add(trie.root); err != nil {
		return nil, err
	}
	return &FlatTrie{nodes: b.nodes, prefixes: b.prefixes, routeRefs: b.routeRefs}, nil
}

type flatBuilder struct {
	nodes     []FlatNode
	prefixes  []byte
	routeRefs []snapshot.RouteID
}

func (b *flatBuilder) add(root *RadixNode) error {
	type queued struct {
		raw *RadixNode
		id  NodeID
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
			firstChild = NodeID(len(b.nodes))
			for _, c := range sorted {
				childID := NodeID(len(b.nodes))
				b.nodes = append(b.nodes, FlatNode{})
				queue = append(queue, queued{c, childID})
			}
		}

		paramChild, wildcardChild := noNode, noNode
		if n.paramChild != nil {
			paramChild = NodeID(len(b.nodes))
			b.nodes = append(b.nodes, FlatNode{})
			queue = append(queue, queued{n.paramChild, paramChild})
		}
		if n.wildcardChild != nil {
			wildcardChild = NodeID(len(b.nodes))
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

func (t *FlatTrie) Lookup(path string, visit func(candidates []snapshot.RouteID) bool) {
	if t == nil || len(t.nodes) == 0 || visit == nil {
		return
	}
	t.lookup(0, path, visit)
}

func (t *FlatTrie) lookup(nodeID NodeID, path string, visit func([]snapshot.RouteID) bool) bool {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	node := &t.nodes[nodeID]

	if len(path) == 0 {
		if node.RouteCount > 0 && visit(t.candidatesOf(nodeID)) {
			return true
		}
		if node.WildcardChild != noNode {
			return visit(t.candidatesOf(node.WildcardChild))
		}
		return false
	}

	end := 0
	for end < len(path) && path[end] != '/' {
		end++
	}
	segment, rest := path[:end], path[end:]

	if childID, ok := t.matchStaticSegment(nodeID, segment); ok {
		if t.lookupOrFallback(childID, rest, visit) {
			return true
		}
	}
	if node.ParamChild != noNode {
		if t.lookupOrFallback(node.ParamChild, rest, visit) {
			return true
		}
	}
	if node.WildcardChild != noNode {
		return visit(t.candidatesOf(node.WildcardChild))
	}
	return false
}

func (t *FlatTrie) lookupOrFallback(nodeID NodeID, rest string, visit func([]snapshot.RouteID) bool) bool {
	if t.lookup(nodeID, rest, visit) {
		return true
	}
	trimmed := rest
	for len(trimmed) > 0 && trimmed[0] == '/' {
		trimmed = trimmed[1:]
	}
	if trimmed == "" {
		return false
	}
	if cands := t.candidatesOf(nodeID); cands != nil {
		return visit(cands)
	}
	return false
}

func (t *FlatTrie) matchStaticSegment(nodeID NodeID, segment string) (NodeID, bool) {
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

func (t *FlatTrie) findChild(node *FlatNode, b byte) (NodeID, bool) {
	first := node.FirstChild
	count := uint32(node.ChildCount)

	for i := range count {
		idx := NodeID(uint32(first) + i)
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

func (t *FlatTrie) hasPrefix(id NodeID, s string) bool {
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

func (t *FlatTrie) candidatesOf(nodeID NodeID) []snapshot.RouteID {
	n := &t.nodes[nodeID]
	if n.RouteCount == 0 {
		return nil
	}
	return t.routeRefs[n.FirstRoute : n.FirstRoute+uint32(n.RouteCount)]
}
