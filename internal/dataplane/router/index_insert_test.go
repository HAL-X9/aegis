package router

import "testing"

// lookupViaFlatten flattens trie and looks up path against the resulting
// FlatTrie. RadixTrie itself has no Lookup method — it's a build-time-only
// structure; the request-serving lookup logic lives entirely in FlatTrie
// (see flat.go), so that's what these Insert tests exercise.
func lookupViaFlatten(t *testing.T, trie *RadixTrie, path string) []uint32 {
	t.Helper()
	flat, err := Flatten(trie)
	if err != nil {
		t.Fatalf("Flatten failed: %v", err)
	}
	return flatLookupIDs(flat, path)
}

func TestRadixTrieInsert(t *testing.T) {
	t.Run("insert creates root and terminal candidate", func(t *testing.T) {
		trie := &RadixTrie{}

		trie.Insert("/api/v1", 42)

		if trie.root == nil {
			t.Fatal("root should be initialized")
		}

		got := lookupViaFlatten(t, trie, "/api/v1")
		if len(got) != 1 || got[0] != 42 {
			t.Fatalf("lookup result = %#v, want [42]", got)
		}
	})

	t.Run("insert reuses dynamic edges for parameter and wildcard", func(t *testing.T) {
		trie := &RadixTrie{}

		trie.Insert("/users/:id", 1)
		trie.Insert("/assets/*path", 2)

		if got := lookupViaFlatten(t, trie, "/users/42"); len(got) != 1 || got[0] != 1 {
			t.Fatalf("param lookup = %#v, want [1]", got)
		}

		if got := lookupViaFlatten(t, trie, "/assets/img/logo.png"); len(got) != 1 || got[0] != 2 {
			t.Fatalf("wild lookup = %#v, want [2]", got)
		}
	})

	t.Run("insert appends candidates on same terminal node", func(t *testing.T) {
		trie := &RadixTrie{}

		trie.Insert("/same", 10)
		trie.Insert("/same", 20)

		got := lookupViaFlatten(t, trie, "/same")

		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}

		if got[0] != 10 || got[1] != 20 {
			t.Fatalf("order mismatch: %#v, want [10 20]", got)
		}
	})
}
