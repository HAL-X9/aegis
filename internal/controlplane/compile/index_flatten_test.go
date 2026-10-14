package compile

import (
	"slices"
	"testing"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

func TestFlattenAndLookup(t *testing.T) {
	t.Run("nil trie flattens to empty RouteIndex", func(t *testing.T) {
		flat, err := flatten(&radixTrie{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
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
		trie := &radixTrie{}
		trie.insert("/a", snapshot.RouteID(1))
		trie.insert("/ab", snapshot.RouteID(2))
		trie.insert("/abc", snapshot.RouteID(3))

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
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
		trie := &radixTrie{}
		trie.insert("/users/:id", snapshot.RouteID(1))
		trie.insert("/users/static", snapshot.RouteID(2))

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/users/static")
		if len(got) != 1 || got[0] != snapshot.RouteID(2) {
			t.Fatalf("lookup = %#v, want [2]", got)
		}
	})

	t.Run("param route matches dynamic segment", func(t *testing.T) {
		trie := &radixTrie{}
		trie.insert("/users/:id", snapshot.RouteID(1))

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/users/42")
		if len(got) != 1 || got[0] != snapshot.RouteID(1) {
			t.Fatalf("lookup = %#v, want [1]", got)
		}
	})

	t.Run("wildcard route matches remaining path", func(t *testing.T) {
		trie := &radixTrie{}
		trie.insert("/assets/*path", snapshot.RouteID(3))

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/assets/img/logo.png")
		if len(got) != 1 || got[0] != snapshot.RouteID(3) {
			t.Fatalf("lookup = %#v, want [3]", got)
		}
	})

	t.Run("static route wins over param and wildcard", func(t *testing.T) {
		trie := &radixTrie{}
		trie.insert("/users/:id", snapshot.RouteID(1))
		trie.insert("/users/static", snapshot.RouteID(2))
		trie.insert("/users/*path", snapshot.RouteID(3))

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
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
		trie := &radixTrie{}

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
			trie.insert("/"+route.segment, route.id)
		}

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
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

func TestRouteIndexLookup(t *testing.T) {
	t.Run("nil receiver does nothing", func(t *testing.T) {
		var trie *snapshot.RouteIndex

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
		trie := snapshot.NewRouteIndex([]snapshot.IndexNode{{}}, "", nil)

		trie.Lookup("/anything", nil)
	})

	t.Run("empty flat trie does nothing", func(t *testing.T) {
		trie := snapshot.RouteIndex{}

		called := false

		trie.Lookup("/anything", func(candidates []snapshot.RouteID) bool {
			called = true
			return true
		})

		if called {
			t.Fatal("visit callback must not be called")
		}
	})

	t.Run("unmatched path returns no candidates", func(t *testing.T) {
		trie := &radixTrie{}
		trie.insert("/api", snapshot.RouteID(1))

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
		}

		got := flatLookupIDs(flat, "/other")

		if got != nil {
			t.Fatalf("lookup = %#v, want nil", got)
		}
	})

	t.Run("visit returning true stops lookup", func(t *testing.T) {
		trie := &radixTrie{}
		trie.insert("/users/:id", snapshot.RouteID(1))
		trie.insert("/users/*path", snapshot.RouteID(2))

		flat, err := flatten(trie)
		if err != nil {
			t.Fatalf("flatten failed: %v", err)
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

func TestRouteIndexLookupSemantics(t *testing.T) {
	tests := []struct {
		name   string
		routes []string
		path   string
		want   []snapshot.RouteID // nil means no match
	}{
		{name: "exact static", routes: []string{"/api"}, path: "/api", want: []snapshot.RouteID{0}},
		{name: "trailing slash", routes: []string{"/api"}, path: "/api/", want: []snapshot.RouteID{0}},
		{name: "prefix on segment boundary", routes: []string{"/api"}, path: "/api/v1/users", want: []snapshot.RouteID{0}},
		{name: "no prefix inside a segment", routes: []string{"/api"}, path: "/apix"},
		{name: "shorter path than route", routes: []string{"/api/v1"}, path: "/api"},

		{name: "split node is not a segment boundary", routes: []string{"/abc", "/abd"}, path: "/ab/c"},
		{name: "segment boundary beside split", routes: []string{"/abc", "/ab/c"}, path: "/ab/c", want: []snapshot.RouteID{1}},
		{name: "longer sibling segment", routes: []string{"/ab/c", "/abc"}, path: "/abc", want: []snapshot.RouteID{1}},
		{name: "shared prefix ending mid segment", routes: []string{"/abc", "/abd"}, path: "/ab"},

		{name: "root matches itself", routes: []string{"/"}, path: "/", want: []snapshot.RouteID{0}},
		{name: "root is a catch-all", routes: []string{"/"}, path: "/x/y", want: []snapshot.RouteID{0}},
		{name: "specific route wins over root", routes: []string{"/", "/api"}, path: "/api/v1", want: []snapshot.RouteID{1}},
		{name: "root catches what others miss", routes: []string{"/", "/api"}, path: "/other", want: []snapshot.RouteID{0}},

		{name: "param does not match empty remainder", routes: []string{"/users/:id"}, path: "/users"},
		{name: "wildcard matches empty remainder", routes: []string{"/files/*"}, path: "/files", want: []snapshot.RouteID{0}},
		{name: "wildcard matches deep remainder", routes: []string{"/files/*"}, path: "/files/a/b", want: []snapshot.RouteID{0}},

		{name: "empty segment never matches", routes: []string{"/api"}, path: "//api"},
		{name: "empty segment stops prefix match", routes: []string{"/api"}, path: "/api//v1"},
		{name: "relative path never matches", routes: []string{"/"}, path: "api"},
		{name: "empty path never matches", routes: []string{"/"}, path: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trie := &radixTrie{}
			for i, route := range tt.routes {
				trie.insert(route, snapshot.RouteID(i))
			}

			flat, err := flatten(trie)
			if err != nil {
				t.Fatalf("flatten failed: %v", err)
			}

			got := flatLookupIDs(flat, tt.path)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("routes %q: lookup(%q) = %v, want %v", tt.routes, tt.path, got, tt.want)
			}
		})
	}
}

func flatLookupIDs(
	idx snapshot.RouteIndex,
	path string,
) []snapshot.RouteID {
	var got []snapshot.RouteID

	idx.Lookup(path, func(candidates []snapshot.RouteID) bool {
		got = append(got, candidates...)
		return true
	})

	return got
}
