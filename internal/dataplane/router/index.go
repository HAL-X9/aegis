package router

type PathIndex interface {
	Lookup(path []byte) []*RadixNodeCandidates
}

type RadixNodeCandidates struct {
	Routes     *CompileRoute
	MethodMask uint32
}

type RadixNode struct {
	prefix []byte

	children      []RadixNode
	paramChild    *RadixNode
	wildcardChild *RadixNode

	candidates []RadixNodeCandidates
}

type RadixTrie struct {
	root string
}

func (trie *RadixTrie) Lookup() []*RadixNodeCandidates {
	return nil
}
