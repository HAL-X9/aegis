package snapshot

import (
	"net/url"
	"time"
)

// ServiceID is a stable index into the compiled service table.
type ServiceID uint32

// CompiledRetry is the runtime retry policy. Attempts counts the first try;
// values <= 1 mean no retries. AttemptTimeout bounds each attempt, including
// its response body; zero means none.
type CompiledRetry struct {
	Attempts       int
	AttemptTimeout time.Duration
}

// CompiledService is the runtime representation of a backend service.
type CompiledService struct {
	// Name is the stable service identifier used for diagnostics and observability.
	Name string

	// Upstream is the parsed upstream origin, scheme and host only.
	Upstream *url.URL

	// Timeout bounds the whole upstream exchange, including retries and the
	// response body; zero means none.
	Timeout time.Duration

	// Retry is the service's upstream retry policy.
	Retry CompiledRetry
}

// CompiledServices contains all backend services addressable by ServiceID.
type CompiledServices struct {
	// Items stores compiled services in deterministic ServiceID order.
	Items []CompiledService
}
