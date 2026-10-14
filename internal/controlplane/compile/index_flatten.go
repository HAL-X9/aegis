package compile

import (
	"fmt"
	"math"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Index builds the path index over routes. Route i is registered under its
// PathPrefix with ID i, so the route IDs the index returns are positions in
// routes.
//
// This is a control-plane operation: it runs once per config build and its
// result is the only index the request path reads.
func Index(routes []snapshot.CompiledRoute) (snapshot.RouteIndex, error) {
	trie := &radixTrie{}
	for i := range routes {
		trie.insert(routes[i].Match.PathPrefix, snapshot.RouteID(i))
	}
	return flatten(trie)
}

// flatten converts the build-time tree into the immutable arena: a
// breadth-first traversal producing contiguous node, prefix, and
// route-reference slices addressed by index.
func flatten(trie *radixTrie) (snapshot.RouteIndex, error) {
	if trie.root == nil {
		return snapshot.RouteIndex{}, nil
	}
	b := &flatBuilder{}
	if err := b.add(trie.root); err != nil {
		return snapshot.RouteIndex{}, err
	}
	return snapshot.NewRouteIndex(b.nodes, string(b.prefixes), b.routeRefs), nil
}

type flatBuilder struct {
	nodes     []snapshot.IndexNode
	prefixes  []byte
	routeRefs []snapshot.RouteID
}

func (b *flatBuilder) add(root *radixNode) error {
	type queued struct {
		raw *radixNode
		id  snapshot.IndexNodeID
	}

	b.nodes = append(b.nodes, snapshot.IndexNode{})
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

		firstChild := snapshot.NoIndexNode
		if len(sorted) > 0 {
			firstChild = snapshot.IndexNodeID(len(b.nodes))
			for _, c := range sorted {
				childID := snapshot.IndexNodeID(len(b.nodes))
				b.nodes = append(b.nodes, snapshot.IndexNode{})
				queue = append(queue, queued{c, childID})
			}
		}

		paramChild, wildcardChild := snapshot.NoIndexNode, snapshot.NoIndexNode
		if n.paramChild != nil {
			paramChild = snapshot.IndexNodeID(len(b.nodes))
			b.nodes = append(b.nodes, snapshot.IndexNode{})
			queue = append(queue, queued{n.paramChild, paramChild})
		}
		if n.wildcardChild != nil {
			wildcardChild = snapshot.IndexNodeID(len(b.nodes))
			b.nodes = append(b.nodes, snapshot.IndexNode{})
			queue = append(queue, queued{n.wildcardChild, wildcardChild})
		}

		b.nodes[cur.id] = snapshot.IndexNode{
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
func sortedChildren(children []*radixNode) []*radixNode {
	if len(children) == 0 {
		return nil
	}
	out := append([]*radixNode(nil), children...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].prefix[0] > out[j].prefix[0]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
