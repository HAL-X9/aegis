package router

// RadixNode is a radix trie node.
//
// Static edges use radix compression within a path segment. Once a segment
// is fully consumed, children represent the next segment. A node may contain
// candidates and still have children, allowing routes such as "/ab" and
// "/abcd" to coexist.
type RadixNode struct {
	prefix []byte

	// Static edges. Children have unique first bytes.
	children []*RadixNode

	paramChild    *RadixNode
	wildcardChild *RadixNode

	candidates []*RouteIndexEntry
}

// Lookup resolves route candidates for path.
//
// Priority: static > param > wildcard.
// The hot path performs no heap allocations.
func (t *RadixTrie) Lookup(path []byte) []*RouteIndexEntry {
	if t == nil || t.root == nil {
		return nil
	}
	return lookup(t.root, path)
}

func lookup(node *RadixNode, path []byte) []*RouteIndexEntry {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	if len(path) == 0 {
		if len(node.candidates) > 0 {
			return node.candidates
		}
		if node.wildcardChild != nil {
			return node.wildcardChild.candidates
		}
		return nil
	}

	end := 0
	for end < len(path) && path[end] != '/' {
		end++
	}
	segment, rest := path[:end], path[end:]

	if result := lookupStaticSegment(node, segment, rest); result != nil {
		return result
	}

	if node.paramChild != nil {
		if result := lookup(node.paramChild, rest); result != nil {
			return result
		}
	}

	if node.wildcardChild != nil {
		return node.wildcardChild.candidates
	}

	return nil
}

// lookupStaticSegment matches the complete segment through compressed
// static edges, then continues with the remaining path.
func lookupStaticSegment(node *RadixNode, segment, rest []byte) []*RouteIndexEntry {
	for len(segment) > 0 {
		_, child := findChildByFirstByte(node, segment[0])
		if child == nil {
			return nil
		}

		n := len(child.prefix)
		if n > len(segment) || commonPrefixLen(child.prefix, segment) != n {
			return nil
		}

		node = child
		segment = segment[n:]
	}

	return lookup(node, rest)
}
