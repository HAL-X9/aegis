package compile

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// lookupViaIndex flattens the build-time trie and performs the lookup
// against the request-time representation.
func lookupViaIndex(
	t *testing.T,
	trie *radixTrie,
	path string,
) []snapshot.RouteID {
	t.Helper()

	flat, err := flatten(trie)
	if err != nil {
		t.Fatalf("flatten failed: %v", err)
	}

	return flatLookupIDs(flat, path)
}

func TestRadixTrieInsert(t *testing.T) {
	t.Run("insert creates root and terminal candidate", func(t *testing.T) {
		trie := &radixTrie{}

		trie.insert("/api/v1", snapshot.RouteID(42))

		if trie.root == nil {
			t.Fatal("root should be initialized")
		}

		got := lookupViaIndex(t, trie, "/api/v1")

		if len(got) != 1 || got[0] != snapshot.RouteID(42) {
			t.Fatalf(
				"lookup result = %#v, want [%d]",
				got,
				snapshot.RouteID(42),
			)
		}
	})

	t.Run("insert reuses dynamic edges for parameter and wildcard", func(t *testing.T) {
		trie := &radixTrie{}

		trie.insert("/users/:id", snapshot.RouteID(1))
		trie.insert("/assets/*path", snapshot.RouteID(2))

		if got := lookupViaIndex(t, trie, "/users/42"); len(got) != 1 ||
			got[0] != snapshot.RouteID(1) {
			t.Fatalf("param lookup = %#v, want [1]", got)
		}

		if got := lookupViaIndex(t, trie, "/assets/img/logo.png"); len(got) != 1 ||
			got[0] != snapshot.RouteID(2) {
			t.Fatalf("wild lookup = %#v, want [2]", got)
		}
	})

	t.Run("insert appends candidates on same terminal node", func(t *testing.T) {
		trie := &radixTrie{}

		trie.insert("/same", snapshot.RouteID(10))
		trie.insert("/same", snapshot.RouteID(20))

		got := lookupViaIndex(t, trie, "/same")

		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}

		want := []snapshot.RouteID{
			snapshot.RouteID(10),
			snapshot.RouteID(20),
		}

		for i := range want {
			if got[i] != want[i] {
				t.Fatalf(
					"candidate[%d] = %d, want %d",
					i,
					got[i],
					want[i],
				)
			}
		}
	})
}
