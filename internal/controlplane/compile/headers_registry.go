package compile

import "github.com/HAL-X9/aegis/internal/snapshot"

// wellKnownHeaderIDs assigns the fixed, statically-known snapshot.HeaderID
// values used throughout the compiler. Any header name not listed here is
// a custom header and gets a dynamic ID from HeaderRegistryBuilder.
var wellKnownHeaderIDs = map[string]snapshot.HeaderID{
	"Host":                   snapshot.HeaderHost,
	"Content-Type":           snapshot.HeaderContentType,
	"Content-Length":         snapshot.HeaderContentLength,
	"Authorization":          snapshot.HeaderAuthorization,
	"X-Forwarded-For":        snapshot.HeaderXForwardedFor,
	"X-Forwarded-Proto":      snapshot.HeaderXForwardedProto,
	"X-Request-ID":           snapshot.HeaderXRequestID,
	"X-Request-Id":           snapshot.HeaderXRequestID,
	"Server":                 snapshot.HeaderServer,
	"X-Content-Type-Options": snapshot.HeaderXContentTypeOptions,
	"X-Frame-Options":        snapshot.HeaderXFrameOptions,
	"X-XSS-Protection":       snapshot.HeaderXXSSProtection,
	"X-Xss-Protection":       snapshot.HeaderXXSSProtection,
}

// HeaderRegistryBuilder assigns snapshot.HeaderID values to header names
// seen while compiling routes.
//
// Well-known names use fixed IDs. Custom names receive sequential dynamic IDs
// starting at snapshot.HeaderDynamicStart.
//
// One builder must be shared by every compile.Routes call belonging to the
// same config build.
type HeaderRegistryBuilder struct {
	dynamic map[string]snapshot.HeaderID
	names   [][]byte
}

// NewHeaderRegistryBuilder returns an empty builder, ready to be passed to
// compile.Routes.
func NewHeaderRegistryBuilder() *HeaderRegistryBuilder {
	return &HeaderRegistryBuilder{
		dynamic: make(map[string]snapshot.HeaderID),
	}
}

func (b *HeaderRegistryBuilder) resolve(name string) snapshot.HeaderID {
	if id, ok := wellKnownHeaderIDs[name]; ok {
		return id
	}

	if id, ok := b.dynamic[name]; ok {
		return id
	}

	id := snapshot.HeaderDynamicStart + snapshot.HeaderID(len(b.names))
	b.dynamic[name] = id
	b.names = append(b.names, []byte(name))

	return id
}

// Registry returns the compiled header-name table for every dynamic ID
// handed out so far.
func (b *HeaderRegistryBuilder) Registry() snapshot.HeaderRegistry {
	return snapshot.HeaderRegistry{
		Names: b.names,
	}
}
