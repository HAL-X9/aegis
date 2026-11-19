package balance

import (
	"net/url"
	"sync"
	"testing"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

func services(targets ...[]string) snapshot.CompiledServices {
	items := make([]snapshot.CompiledService, len(targets))
	for i, hosts := range targets {
		origins := make([]url.URL, len(hosts))
		for j, host := range hosts {
			origins[j] = url.URL{Scheme: "http", Host: host}
		}
		items[i] = snapshot.CompiledService{Targets: origins}
	}
	return snapshot.CompiledServices{Items: items}
}

// order returns the hosts that one selection tries, one per attempt.
func order(s Selection, attempts int) []string {
	hosts := make([]string, attempts)
	for i := range hosts {
		hosts[i] = s.Target(i).Host
	}
	return hosts
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPickTakesTargetsInTurn(t *testing.T) {
	b := New(services([]string{"a:1", "b:1", "c:1"}))

	want := [][]string{
		{"a:1", "b:1", "c:1", "a:1"},
		{"b:1", "c:1", "a:1", "b:1"},
		{"c:1", "a:1", "b:1", "c:1"},
		{"a:1", "b:1", "c:1", "a:1"},
	}
	for i, w := range want {
		s := b.Pick(0)
		if s.Len() != 3 {
			t.Fatalf("pick %d: Len = %d, want 3", i, s.Len())
		}
		if got := order(s, 4); !equal(got, w) {
			t.Fatalf("pick %d: order = %v, want %v", i, got, w)
		}
	}
}

func TestPickWithOneTargetAlwaysReturnsIt(t *testing.T) {
	b := New(services([]string{"only:1"}))

	for range 3 {
		if got := order(b.Pick(0), 2); !equal(got, []string{"only:1", "only:1"}) {
			t.Fatalf("order = %v, want the single target on every attempt", got)
		}
	}
}

func TestPickKeepsAServiceTurnOfItsOwn(t *testing.T) {
	b := New(services([]string{"a:1", "b:1"}, []string{"x:1", "y:1"}))

	b.Pick(0)
	if got := b.Pick(1).Target(0).Host; got != "x:1" {
		t.Fatalf("first pick of service 1 = %s, want x:1", got)
	}
	if got := b.Pick(0).Target(0).Host; got != "b:1" {
		t.Fatalf("second pick of service 0 = %s, want b:1", got)
	}
}

// TestPickSpreadsConcurrentRequestsEvenly checks that the turn is taken
// atomically: picks made concurrently still start at each target equally
// often.
func TestPickSpreadsConcurrentRequestsEvenly(t *testing.T) {
	const (
		workers = 8
		picks   = 3000
	)
	b := New(services([]string{"a:1", "b:1", "c:1"}))

	var (
		mu     sync.Mutex
		counts = map[string]int{}
		wg     sync.WaitGroup
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := map[string]int{}
			for range picks {
				local[b.Pick(0).Target(0).Host]++
			}
			mu.Lock()
			for host, n := range local {
				counts[host] += n
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	for _, host := range []string{"a:1", "b:1", "c:1"} {
		if want := workers * picks / 3; counts[host] != want {
			t.Fatalf("picks starting at %s = %d, want %d (all: %v)", host, counts[host], want, counts)
		}
	}
}

func TestPickDoesNotAllocate(t *testing.T) {
	b := New(services([]string{"a:1", "b:1"}))

	if allocs := testing.AllocsPerRun(100, func() { _ = b.Pick(0).Target(1) }); allocs != 0 {
		t.Fatalf("Pick allocates %.0f times, want 0", allocs)
	}
}
