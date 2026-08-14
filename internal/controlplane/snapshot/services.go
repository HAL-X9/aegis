package snapshot

// ServiceID is a stable index into the compiled service table.
type ServiceID uint32

// CompiledService is the runtime representation of a backend service.
type CompiledService struct {
	// Name is the stable service identifier used for diagnostics and observability.
	Name string

	// Upstream is the precomputed upstream origin URL.
	Upstream string
}

// CompiledServices contains all backend services addressable by ServiceID.
type CompiledServices struct {
	// Items stores compiled services in deterministic ServiceID order.
	Items []CompiledService
}
