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

		if got := trie.Lookup("/anything"); got != nil {
			t.Fatalf("lookup on empty trie = %#v", got)
		}
	})

	t.Run("indexes all provided routes", func(t *testing.T) {
		r1 := &RouteIndexEntry{
			Route: &snapshot.CompiledRoute{
				Name: "r1",
				Match: snapshot.CompiledMatch{
					PathPrefix: "/a",
				},
			},
		}

		r2 := &RouteIndexEntry{
			Route: &snapshot.CompiledRoute{
				Name: "r2",
				Match: snapshot.CompiledMatch{
					PathPrefix: "/b",
				},
			},
		}

		trie := BuildRadixTrie([]*RouteIndexEntry{r1, r2})

		gotA := trie.Lookup("/a")
		gotB := trie.Lookup("/b")

		if len(gotA) != 1 || gotA[0] != r1 {
			t.Fatalf("lookup /a = %#v", gotA)
		}

		if len(gotB) != 1 || gotB[0] != r2 {
			t.Fatalf("lookup /b = %#v", gotB)
		}
	})
}
