package router

import "testing"

func TestFlattenAndLookup(t *testing.T) {
	t.Run("nil trie flattens to empty, non-nil trie", func(t *testing.T) {
		flat, err := Flatten(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flat == nil {
			t.Fatal("expected non-nil FlatTrie")
		}
		if got := flatLookupIDs(flat, "/anything"); got != nil {
			t.Fatalf("got %#v, want nil", got)
		}
	})

	t.Run("static routes resolve to correct route IDs", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/a", 1)
		trie.Insert("/ab", 2)
		trie.Insert("/abc", 3)

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		cases := map[string]uint32{"/a": 1, "/ab": 2, "/abc": 3}
		for path, want := range cases {
			got := flatLookupIDs(flat, path)
			if len(got) != 1 || got[0] != want {
				t.Fatalf("lookup(%q) = %#v, want [%d]", path, got, want)
			}
		}
	})

	t.Run("param and wildcard priority matches insert-time semantics", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/users/:id", 1)
		trie.Insert("/users/static", 2)
		trie.Insert("/assets/*path", 3)

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		// Static edge must win over the param edge for an exact match.
		if got := flatLookupIDs(flat, "/users/static"); len(got) != 1 || got[0] != 2 {
			t.Fatalf("static-over-param lookup = %#v, want [2]", got)
		}

		if got := flatLookupIDs(flat, "/users/42"); len(got) != 1 || got[0] != 1 {
			t.Fatalf("param lookup = %#v, want [1]", got)
		}

		if got := flatLookupIDs(flat, "/assets/img/logo.png"); len(got) != 1 || got[0] != 3 {
			t.Fatalf("wildcard lookup = %#v, want [3]", got)
		}
	})

	t.Run("children with many siblings resolve via binary search", func(t *testing.T) {
		trie := &RadixTrie{}
		for i, seg := range []string{"aa", "bb", "cc", "dd", "ee", "ff", "gg"} {
			trie.Insert("/"+seg, uint32(i))
		}

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		for i, seg := range []string{"aa", "bb", "cc", "dd", "ee", "ff", "gg"} {
			got := flatLookupIDs(flat, "/"+seg)
			if len(got) != 1 || got[0] != uint32(i) {
				t.Fatalf("lookup(/%s) = %#v, want [%d]", seg, got, i)
			}
		}
	})
}

func engineLookupIDs(e *Engine, path string) []uint32 {
	var got []uint32
	e.Lookup(path, func(c []uint32) bool {
		got = c
		return true
	})
	return got
}

func flatLookupIDs(t *FlatTrie, path string) []uint32 {
	var got []uint32
	t.Lookup(path, func(c []uint32) bool {
		got = c
		return true
	})
	return got
}
