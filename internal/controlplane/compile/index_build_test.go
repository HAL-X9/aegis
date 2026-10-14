package compile

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

func TestIndex(t *testing.T) {
	t.Run("builds empty trie for empty input", func(t *testing.T) {
		idx, err := Index(nil)
		if err != nil {
			t.Fatalf("Index failed: %v", err)
		}

		if got := flatLookupIDs(idx, "/anything"); got != nil {
			t.Fatalf("lookup on empty trie = %#v", got)
		}
	})

	t.Run("indexes all provided routes", func(t *testing.T) {
		routes := []snapshot.CompiledRoute{
			{Name: "r1", Match: snapshot.CompiledMatch{PathPrefix: "/a"}},
			{Name: "r2", Match: snapshot.CompiledMatch{PathPrefix: "/b"}},
		}

		idx, err := Index(routes)
		if err != nil {
			t.Fatalf("Index failed: %v", err)
		}

		gotA := flatLookupIDs(idx, "/a")
		gotB := flatLookupIDs(idx, "/b")

		if len(gotA) != 1 || gotA[0] != 0 {
			t.Fatalf("lookup /a = %#v, want [0]", gotA)
		}

		if len(gotB) != 1 || gotB[0] != 1 {
			t.Fatalf("lookup /b = %#v, want [1]", gotB)
		}
	})
}
