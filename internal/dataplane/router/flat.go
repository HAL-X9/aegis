package router

import (
	"fmt"
	"math"
	"strings"

	"github.com/HAL-X9/aegis/internal/snapshot"
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
	nodes []FlatNode
	// prefixes is a string, not []byte, so prefixOf can hand out
	// substrings for comparison without converting or allocating.
	prefixes  string
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
	return &FlatTrie{nodes: b.nodes, prefixes: string(b.prefixes), routeRefs: b.routeRefs}, nil
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

// Lookup calls visit, in priority order, for every candidate set whose
// route matches path; visit returns true once it accepts a route, which
// ends the lookup.
//
// path must be canonical (see request.CanonicalPath): it starts with '/'
// and has no empty or dot segments. Lookup does not canonicalize it
// itself, so the router never routes a path other than the one forwarded
// upstream: an empty segment never matches, and every other segment, dot
// segments included, is compared literally.
//
// Route paths have prefix semantics on segment boundaries: "/api" matches
// "/api", "/api/" and "/api/v1", but not "/apix". "/" therefore matches
// every path. At each depth, candidates are offered most specific first:
// static, then :param, then *wildcard, then the routes ending at the
// enclosing segment.
func (t *FlatTrie) Lookup(path string, visit func(candidates []snapshot.RouteID) bool) {
	if t == nil || len(t.nodes) == 0 || visit == nil || !strings.HasPrefix(path, "/") {
		return
	}
	t.lookup(0, path[1:], visit)
}

// lookup matches rest, the part of the request path after the '/' that
// ends nodeID's segment, against routes at or below boundary node nodeID.
func (t *FlatTrie) lookup(nodeID NodeID, rest string, visit func([]snapshot.RouteID) bool) bool {
	node := &t.nodes[nodeID]

	if rest == "" {
		// The path ends here: exact routes first, then a trailing wildcard,
		// which also matches an empty remainder ("/files/*" matches "/files").
		if t.visitRoutes(nodeID, visit) {
			return true
		}
		return node.WildcardChild != noNode && t.visitRoutes(node.WildcardChild, visit)
	}

	segment, tail := cutSegment(rest)
	if segment == "" {
		// Only a non-canonical path has an empty segment ("//").
		return false
	}

	if childID, ok := t.matchStatic(nodeID, segment); ok && t.lookup(childID, tail, visit) {
		return true
	}
	if node.ParamChild != noNode && t.lookup(node.ParamChild, tail, visit) {
		return true
	}
	if node.WildcardChild != noNode && t.visitRoutes(node.WildcardChild, visit) {
		return true
	}
	// Prefix semantics: routes ending at this boundary match every path below it.
	return t.visitRoutes(nodeID, visit)
}

// cutSegment splits path at its first '/' into the leading segment and
// the rest after that '/'. On short path segments a plain byte loop beats
// strings.Cut, whose IndexByte call overhead dominates; see LookupDeep.
func cutSegment(path string) (segment, rest string) {
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			return path[:i], path[i+1:]
		}
	}
	return path, ""
}

// visitRoutes offers nodeID's candidates to visit, if it has any.
func (t *FlatTrie) visitRoutes(nodeID NodeID, visit func([]snapshot.RouteID) bool) bool {
	candidates := t.candidatesOf(nodeID)
	return candidates != nil && visit(candidates)
}

// matchStatic follows the static edges below nodeID that spell segment
// plus its terminating '/', and returns the boundary node reached.
func (t *FlatTrie) matchStatic(nodeID NodeID, segment string) (NodeID, bool) {
	for {
		next := byte('/')
		if segment != "" {
			next = segment[0]
		}

		childID, ok := t.findChild(&t.nodes[nodeID], next)
		if !ok {
			return 0, false
		}

		prefix := t.prefixOf(childID)
		if len(prefix) > len(segment) {
			// The edge runs past the segment, so it must end exactly with
			// the terminating '/': "ab/" matches segment "ab", "abc/" does not.
			last := len(prefix) - 1
			return childID, last == len(segment) && prefix[last] == '/' && prefix[:last] == segment
		}
		if prefix != segment[:len(prefix)] {
			return 0, false
		}

		nodeID, segment = childID, segment[len(prefix):]
	}
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

func (t *FlatTrie) prefixOf(nodeID NodeID) string {
	n := &t.nodes[nodeID]
	return t.prefixes[n.PrefixOffset : n.PrefixOffset+uint32(n.PrefixLen)]
}

func (t *FlatTrie) candidatesOf(nodeID NodeID) []snapshot.RouteID {
	n := &t.nodes[nodeID]
	if n.RouteCount == 0 {
		return nil
	}
	return t.routeRefs[n.FirstRoute : n.FirstRoute+uint32(n.RouteCount)]
}
