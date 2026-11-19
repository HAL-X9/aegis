// Package balance spreads the requests of each service across its upstream
// targets.
package balance

import (
	"net/url"
	"sync/atomic"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Balancer chooses the upstream targets of each request. Every service cycles
// through its targets in configuration order (round robin), independently of
// every other service. Balancer is safe for concurrent use.
//
// The turn counters are the only mutable state. They live here rather than
// in the snapshot, which stays immutable.
type Balancer struct {
	services []service
}

type service struct {
	targets []url.URL
	turn    atomic.Uint64
}

// New returns a Balancer over the services of a compiled snapshot. It shares
// their target slices rather than copying them.
func New(services snapshot.CompiledServices) *Balancer {
	b := &Balancer{services: make([]service, len(services.Items))}
	for i := range services.Items {
		b.services[i].targets = services.Items[i].Targets
	}
	return b
}

// Pick returns the order in which one request to the service tries its
// targets, starting with the next target in turn. id must identify a service
// of the snapshot New was given. Pick never allocates.
func (b *Balancer) Pick(id snapshot.ServiceID) Selection {
	s := &b.services[id]
	if len(s.targets) == 1 {
		return Selection{targets: s.targets}
	}

	turn := s.turn.Add(1) - 1
	return Selection{targets: s.targets, first: int(turn % uint64(len(s.targets)))}
}

// Selection is the order in which one request tries the targets of a
// service: the picked target first, then each following target in
// configuration order, wrapping around.
type Selection struct {
	targets []url.URL
	first   int
}

// Target returns the origin for attempt, counted from zero. An attempt past
// the last distinct target wraps around to the first one. The result is
// shared by every request and must not be modified.
func (s Selection) Target(attempt int) *url.URL {
	return &s.targets[(s.first+attempt)%len(s.targets)]
}

// Len returns the number of distinct targets in the selection.
func (s Selection) Len() int {
	return len(s.targets)
}
