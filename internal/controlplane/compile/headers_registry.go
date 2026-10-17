package compile

import "github.com/HAL-X9/aegis/internal/snapshot"

// HeaderRegistryBuilder assigns snapshot.HeaderID values to header names
// seen while compiling routes.
//
// Well-known names use the fixed IDs defined in snapshot. Custom names receive sequential dynamic IDs
// starting at snapshot.HeaderDynamicStart.
//
// One builder must be shared by every compile.Routes call belonging to the
// same config build.
type HeaderRegistryBuilder struct {
	dynamic map[string]snapshot.HeaderID
	names   []string
}

// NewHeaderRegistryBuilder returns an empty builder, ready to be passed to
// compile.Routes.
func NewHeaderRegistryBuilder() *HeaderRegistryBuilder {
	return &HeaderRegistryBuilder{
		dynamic: make(map[string]snapshot.HeaderID),
	}
}

func (b *HeaderRegistryBuilder) resolve(name string) snapshot.HeaderID {
	if id, ok := snapshot.WellKnownHeaderID(name); ok {
		return id
	}

	if id, ok := b.dynamic[name]; ok {
		return id
	}

	id := snapshot.HeaderDynamicStart + snapshot.HeaderID(len(b.names))
	b.dynamic[name] = id
	b.names = append(b.names, name)

	return id
}

// Registry returns the compiled header-name table for every dynamic ID
// handed out so far.
func (b *HeaderRegistryBuilder) Registry() snapshot.HeaderRegistry {
	return snapshot.HeaderRegistry{
		Names: b.names,
	}
}
