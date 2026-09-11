package router

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func TestBuildRadixTrie(t *testing.T) {
	t.Run("builds empty trie for empty input", func(t *testing.T) {
		trie := BuildRadixTrie(nil)

		if trie == nil {
			t.Fatal("expected non-nil trie")
		}

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		if got := flat.Lookup("/anything"); got != nil {
			t.Fatalf("lookup on empty trie = %#v", got)
		}
	})

	t.Run("indexes all provided routes", func(t *testing.T) {
		routes := []snapshot.CompiledRoute{
			{Name: "r1", Match: snapshot.CompiledMatch{PathPrefix: "/a"}},
			{Name: "r2", Match: snapshot.CompiledMatch{PathPrefix: "/b"}},
		}

		trie := BuildRadixTrie(routes)

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		gotA := flat.Lookup("/a")
		gotB := flat.Lookup("/b")

		if len(gotA) != 1 || gotA[0] != 0 {
			t.Fatalf("lookup /a = %#v, want [0]", gotA)
		}

		if len(gotB) != 1 || gotB[0] != 1 {
			t.Fatalf("lookup /b = %#v, want [1]", gotB)
		}
	})
}
