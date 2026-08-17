package router

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func TestRadixTrieLookup(t *testing.T) {
	t.Run("returns nil when trie is empty", func(t *testing.T) {
		trie := &RadixTrie{}
		if got := trie.Lookup([]byte("/a")); got != nil {
			t.Fatalf("got %#v, want nil", got)
		}
	})

	t.Run("prefers exact static route over dynamic", func(t *testing.T) {
		trie := &RadixTrie{}
		staticEntry := &RouteIndexEntry{Route: &snapshot.CompiledRoute{Name: "static"}}
		paramEntry := &RouteIndexEntry{Route: &snapshot.CompiledRoute{Name: "param"}}

		trie.Insert("/users/me", staticEntry)
		trie.Insert("/users/:id", paramEntry)

		got := trie.Lookup([]byte("/users/me"))
		if len(got) != 1 || got[0] != staticEntry {
			t.Fatalf("lookup = %#v", got)
		}
	})

	t.Run("uses param child when static edge missing", func(t *testing.T) {
		trie := &RadixTrie{}
		entry := &RouteIndexEntry{Route: &snapshot.CompiledRoute{Name: "param"}}
		trie.Insert("/users/:id", entry)

		got := trie.Lookup([]byte("/users/123"))
		if len(got) != 1 || got[0] != entry {
			t.Fatalf("lookup = %#v", got)
		}
	})

	t.Run("uses wildcard child for suffix", func(t *testing.T) {
		trie := &RadixTrie{}
		entry := &RouteIndexEntry{Route: &snapshot.CompiledRoute{Name: "wild"}}
		trie.Insert("/files/*path", entry)

		got := trie.Lookup([]byte("/files/a/b/c"))
		if len(got) != 1 || got[0] != entry {
			t.Fatalf("lookup = %#v", got)
		}
	})

	t.Run("returns nil when no matching edges", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/api/v1", &RouteIndexEntry{Route: &snapshot.CompiledRoute{Name: "v1"}})

		if got := trie.Lookup([]byte("/api/v2")); got != nil {
			t.Fatalf("got %#v, want nil", got)
		}
	})
}
