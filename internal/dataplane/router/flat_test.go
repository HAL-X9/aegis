package router

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func TestFlattenAndLookup(t *testing.T) {
	t.Run("nil trie flattens to empty non-nil FlatTrie", func(t *testing.T) {
		flat, err := Flatten(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if flat == nil {
			t.Fatal("expected non-nil FlatTrie")
		}

		var got []snapshot.RouteID
		flat.Lookup("/anything", func(candidates []snapshot.RouteID) bool {
			got = append(got, candidates...)
			return true
		})

		if got != nil {
			t.Fatalf("lookup result = %#v, want nil", got)
		}
	})

	t.Run("static routes resolve to correct route IDs", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/a", snapshot.RouteID(1))
		trie.Insert("/ab", snapshot.RouteID(2))
		trie.Insert("/abc", snapshot.RouteID(3))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		tests := []struct {
			path string
			want snapshot.RouteID
		}{
			{path: "/a", want: snapshot.RouteID(1)},
			{path: "/ab", want: snapshot.RouteID(2)},
			{path: "/abc", want: snapshot.RouteID(3)},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				got := flatLookupIDs(flat, tt.path)

				if len(got) != 1 || got[0] != tt.want {
					t.Fatalf(
						"lookup(%q) = %#v, want [%d]",
						tt.path,
						got,
						tt.want,
					)
				}
			})
		}
	})

	t.Run("static route has priority over param route", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/users/:id", snapshot.RouteID(1))
		trie.Insert("/users/static", snapshot.RouteID(2))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/users/static")
		if len(got) != 1 || got[0] != snapshot.RouteID(2) {
			t.Fatalf("lookup = %#v, want [2]", got)
		}
	})

	t.Run("param route matches dynamic segment", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/users/:id", snapshot.RouteID(1))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/users/42")
		if len(got) != 1 || got[0] != snapshot.RouteID(1) {
			t.Fatalf("lookup = %#v, want [1]", got)
		}
	})

	t.Run("wildcard route matches remaining path", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/assets/*path", snapshot.RouteID(3))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/assets/img/logo.png")
		if len(got) != 1 || got[0] != snapshot.RouteID(3) {
			t.Fatalf("lookup = %#v, want [3]", got)
		}
	})

	t.Run("static route wins over param and wildcard", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/users/:id", snapshot.RouteID(1))
		trie.Insert("/users/static", snapshot.RouteID(2))
		trie.Insert("/users/*path", snapshot.RouteID(3))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		tests := []struct {
			path string
			want snapshot.RouteID
		}{
			{
				path: "/users/static",
				want: snapshot.RouteID(2),
			},
			{
				path: "/users/42",
				want: snapshot.RouteID(1),
			},
			{
				path: "/users/foo/bar",
				want: snapshot.RouteID(1),
			},
		}

		for _, tt := range tests {
			t.Run(tt.path, func(t *testing.T) {
				got := flatLookupIDs(flat, tt.path)

				if len(got) != 1 || got[0] != tt.want {
					t.Fatalf(
						"lookup(%q) = %#v, want [%d]",
						tt.path,
						got,
						tt.want,
					)
				}
			})
		}
	})

	t.Run("static children are sorted by first byte", func(t *testing.T) {
		trie := &RadixTrie{}

		routes := []struct {
			segment string
			id      snapshot.RouteID
		}{
			{segment: "gg", id: snapshot.RouteID(0)},
			{segment: "cc", id: snapshot.RouteID(1)},
			{segment: "aa", id: snapshot.RouteID(2)},
			{segment: "ff", id: snapshot.RouteID(3)},
			{segment: "bb", id: snapshot.RouteID(4)},
			{segment: "ee", id: snapshot.RouteID(5)},
			{segment: "dd", id: snapshot.RouteID(6)},
		}

		for _, route := range routes {
			trie.Insert("/"+route.segment, route.id)
		}

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		for _, route := range routes {
			got := flatLookupIDs(flat, "/"+route.segment)

			if len(got) != 1 || got[0] != route.id {
				t.Fatalf(
					"lookup(/%s) = %#v, want [%d]",
					route.segment,
					got,
					route.id,
				)
			}
		}
	})
}

func TestFlatTrieLookup(t *testing.T) {
	t.Run("nil receiver does nothing", func(t *testing.T) {
		var trie *FlatTrie

		called := false

		trie.Lookup("/anything", func(candidates []snapshot.RouteID) bool {
			called = true
			return true
		})

		if called {
			t.Fatal("visit callback must not be called")
		}
	})

	t.Run("nil visitor does nothing", func(t *testing.T) {
		trie := &FlatTrie{
			nodes: []FlatNode{
				{},
			},
		}

		trie.Lookup("/anything", nil)
	})

	t.Run("empty flat trie does nothing", func(t *testing.T) {
		trie := &FlatTrie{}

		called := false

		trie.Lookup("/anything", func(candidates []snapshot.RouteID) bool {
			called = true
			return true
		})

		if called {
			t.Fatal("visit callback must not be called")
		}
	})

	t.Run("leading slashes are ignored", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/api", snapshot.RouteID(42))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		for _, path := range []string{"/api", "//api", "///api"} {
			got := flatLookupIDs(flat, path)

			if len(got) != 1 || got[0] != snapshot.RouteID(42) {
				t.Fatalf(
					"lookup(%q) = %#v, want [42]",
					path,
					got,
				)
			}
		}
	})

	t.Run("unmatched path returns no candidates", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/api", snapshot.RouteID(1))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/other")

		if got != nil {
			t.Fatalf("lookup = %#v, want nil", got)
		}
	})

	t.Run("visit returning true stops lookup", func(t *testing.T) {
		trie := &RadixTrie{}
		trie.Insert("/users/:id", snapshot.RouteID(1))
		trie.Insert("/users/*path", snapshot.RouteID(2))

		flat, err := Flatten(trie)
		if err != nil {
			t.Fatalf("Flatten failed: %v", err)
		}

		calls := 0

		flat.Lookup("/users/42", func(candidates []snapshot.RouteID) bool {
			calls++
			return true
		})

		if calls != 1 {
			t.Fatalf("visit called %d times, want 1", calls)
		}
	})
}

func flatLookupIDs(
	trie *FlatTrie,
	path string,
) []snapshot.RouteID {
	var got []snapshot.RouteID

	trie.Lookup(path, func(candidates []snapshot.RouteID) bool {
		got = append(got, candidates...)
		return true
	})

	return got
}
