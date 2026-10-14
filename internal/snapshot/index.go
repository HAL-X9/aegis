package snapshot

import (
	"math"
	"slices"
	"strings"
)

// IndexNodeID indexes RouteIndex.Nodes. It is deliberately distinct from
// RouteID: the two index different arenas.
type IndexNodeID uint32

// NoIndexNode marks an absent child. Index 0 is the root, so 0 cannot serve
// as the "absent" marker the way a nil pointer would.
const NoIndexNode = IndexNodeID(math.MaxUint32)

// IndexNode is one node of a RouteIndex.
type IndexNode struct {
	PrefixOffset uint32
	PrefixLen    uint16

	FirstChild IndexNodeID
	ChildCount uint16

	ParamChild    IndexNodeID
	WildcardChild IndexNodeID

	// FirstRoute/RouteCount describe an offset+length window into
	// RouteIndex.RouteRefs — an arena slice, not an identifier, so it stays a
	// plain uint32/uint16 rather than a typed ID.
	FirstRoute uint32
	RouteCount uint16
}

// RouteIndex is an immutable, flattened path index over CompiledConfig.Routes.
// The control plane builds it once with NewRouteIndex; the request path only
// reads it. All cross-references are indices into its slices, so it holds no
// pointers. Its fields are unexported, so nothing outside this package can
// change an index after it is built.
type RouteIndex struct {
	nodes []IndexNode

	// prefixes is a string, not []byte, so edge prefixes can be compared as
	// substrings without converting or allocating.
	prefixes  string
	routeRefs []RouteID
}

// NewRouteIndex builds a RouteIndex from its arena. It copies nodes and
// routeRefs, so the caller keeps no way to modify the index afterward.
func NewRouteIndex(nodes []IndexNode, prefixes string, routeRefs []RouteID) RouteIndex {
	return RouteIndex{
		nodes:     slices.Clone(nodes),
		prefixes:  prefixes,
		routeRefs: slices.Clone(routeRefs),
	}
}

// Nodes returns a copy of the node arena. It exists for diagnostics and
// tests; the request path never calls it.
func (r *RouteIndex) Nodes() []IndexNode {
	return slices.Clone(r.nodes)
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
func (r *RouteIndex) Lookup(path string, visit func(candidates []RouteID) bool) {
	if r == nil || len(r.nodes) == 0 || visit == nil || !strings.HasPrefix(path, "/") {
		return
	}
	r.lookup(0, path[1:], visit)
}

// lookup matches rest, the part of the request path after the '/' that
// ends nodeID's segment, against routes at or below boundary node nodeID.
func (r *RouteIndex) lookup(nodeID IndexNodeID, rest string, visit func([]RouteID) bool) bool {
	node := &r.nodes[nodeID]

	if rest == "" {
		// The path ends here: exact routes first, then a trailing wildcard,
		// which also matches an empty remainder ("/files/*" matches "/files").
		if r.visitRoutes(nodeID, visit) {
			return true
		}
		return node.WildcardChild != NoIndexNode && r.visitRoutes(node.WildcardChild, visit)
	}

	segment, tail := cutSegment(rest)
	if segment == "" {
		// Only a non-canonical path has an empty segment ("//").
		return false
	}

	if childID, ok := r.matchStatic(nodeID, segment); ok && r.lookup(childID, tail, visit) {
		return true
	}
	if node.ParamChild != NoIndexNode && r.lookup(node.ParamChild, tail, visit) {
		return true
	}
	if node.WildcardChild != NoIndexNode && r.visitRoutes(node.WildcardChild, visit) {
		return true
	}
	// Prefix semantics: routes ending at this boundary match every path below it.
	return r.visitRoutes(nodeID, visit)
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
func (r *RouteIndex) visitRoutes(nodeID IndexNodeID, visit func([]RouteID) bool) bool {
	candidates := r.candidatesOf(nodeID)
	return candidates != nil && visit(candidates)
}

// matchStatic follows the static edges below nodeID that spell segment
// plus its terminating '/', and returns the boundary node reached.
func (r *RouteIndex) matchStatic(nodeID IndexNodeID, segment string) (IndexNodeID, bool) {
	for {
		next := byte('/')
		if segment != "" {
			next = segment[0]
		}

		childID, ok := r.findChild(&r.nodes[nodeID], next)
		if !ok {
			return 0, false
		}

		prefix := r.prefixOf(childID)
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

func (r *RouteIndex) findChild(node *IndexNode, b byte) (IndexNodeID, bool) {
	first := node.FirstChild
	count := uint32(node.ChildCount)

	for i := range count {
		idx := IndexNodeID(uint32(first) + i)
		fb := r.prefixes[r.nodes[idx].PrefixOffset]
		if fb == b {
			return idx, true
		}
		if fb > b {
			break
		}
	}
	return 0, false
}

func (r *RouteIndex) prefixOf(nodeID IndexNodeID) string {
	n := &r.nodes[nodeID]
	return r.prefixes[n.PrefixOffset : n.PrefixOffset+uint32(n.PrefixLen)]
}

func (r *RouteIndex) candidatesOf(nodeID IndexNodeID) []RouteID {
	n := &r.nodes[nodeID]
	if n.RouteCount == 0 {
		return nil
	}
	return r.routeRefs[n.FirstRoute : n.FirstRoute+uint32(n.RouteCount)]
}
