package router

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func TestRadixTrieInsert(t *testing.T) {
	t.Run("insert creates root and terminal candidate", func(t *testing.T) {
		trie := &RadixTrie{}
		entry := &RouteIndexEntry{
			Route: &snapshot.CompiledRoute{Name: "route-a"},
		}

		trie.Insert("/api/v1", entry)

		if trie.root == nil {
			t.Fatal("root should be initialized")
		}

		got := trie.Lookup("/api/v1")
		if len(got) != 1 || got[0] != entry {
			t.Fatalf("lookup result = %#v", got)
		}
	})

	t.Run("insert reuses dynamic edges for parameter and wildcard", func(t *testing.T) {
		trie := &RadixTrie{}

		param := &RouteIndexEntry{
			Route: &snapshot.CompiledRoute{Name: "param"},
		}
		wild := &RouteIndexEntry{
			Route: &snapshot.CompiledRoute{Name: "wild"},
		}

		trie.Insert("/users/:id", param)
		trie.Insert("/assets/*path", wild)

		if got := trie.Lookup("/users/42"); len(got) != 1 || got[0] != param {
			t.Fatalf("param lookup = %#v", got)
		}

		if got := trie.Lookup("/assets/img/logo.png"); len(got) != 1 || got[0] != wild {
			t.Fatalf("wild lookup = %#v", got)
		}
	})

	t.Run("insert appends candidates on same terminal node", func(t *testing.T) {
		trie := &RadixTrie{}

		first := &RouteIndexEntry{
			Route: &snapshot.CompiledRoute{Name: "first"},
		}
		second := &RouteIndexEntry{
			Route: &snapshot.CompiledRoute{Name: "second"},
		}

		trie.Insert("/same", first)
		trie.Insert("/same", second)

		got := trie.Lookup("/same")

		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}

		if got[0] != first || got[1] != second {
			t.Fatalf("order mismatch: %#v", got)
		}
	})
}
