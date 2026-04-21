package router

import "github.com/aegis/internal/config/controlplane"

type Engine struct {
	trie *RadixTrie
}

func BuildEngine(cfg *controlplane.AegisManifest) (*Engine, error) {
	return nil, nil
}
