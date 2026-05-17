package compile

import (
	"fmt"

	"github.com/aegis/internal/controlplane/ir"
	"github.com/aegis/internal/controlplane/snapshot"
)

func Policy(policies *ir.NormalizedPolicies) (*snapshot.CompiledPolicies, error) {
	if policies == nil {
		return nil, fmt.Errorf("compile policies configuration: config is nil")
	}

	// Preallocate slice to avoid dynamic growth during compilation.
	// compiledHeaders := make([]CompiledHeaders, 0, len(policies.Headers))

	return nil, nil
}

func resolveHeaderID(name string) snapshot.HeaderID {
	switch name {
	case "Host":
		return snapshot.HeaderHost
	case "Content-Type":
		return snapshot.HeaderContentType
	case "Content-Length":
		return snapshot.HeaderContentLength
	case "Authorization":
		return snapshot.HeaderAuthorization
	case "X-Forwarded-For":
		return snapshot.HeaderXForwardedFor
	case "X-Forwarded-Proto":
		return snapshot.HeaderXForwardedProto
	case "X-Request-ID":
		return snapshot.HeaderXRequestID
	case "Server":
		return snapshot.HeaderServer
	case "X-Content-Type-Options":
		return snapshot.HeaderXContentTypeOptions
	case "X-Frame-Options":
		return snapshot.HeaderXFrameOptions
	case "X-XSS-Protection":
		return snapshot.HeaderXXSSProtection
	default:
		return snapshot.HeaderUnknown
	}
}

type headerValueBuilder struct {
	buf []byte
}

func newValueBuilder() *headerValueBuilder {
	return nil
}

func compileHeaderOps(
	ops *ir.NormalizedHeadersOps,
	b *headerValueBuilder,
) []snapshot.HeaderOp {
	return nil
}

func compileHeader() *snapshot.CompiledHeaders {

	return nil
}
