package policy

import (
	"net/http"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// resolveHeaderName maps a HeaderID back to its wire name.
//
// Well-known IDs are a plain branch — no allocation, no lookup. Anything
// >= snapshot.HeaderDynamicStart is a custom header assigned during
// compilation (see internal/controlplane/compile.HeaderRegistryBuilder)
// and is resolved through names, the registry published alongside the
// route it belongs to (router.Engine.HeaderNames).
func resolveHeaderName(id snapshot.HeaderID, names *snapshot.HeaderRegistry) string {
	switch id {
	case snapshot.HeaderHost:
		return "Host"
	case snapshot.HeaderContentType:
		return "Content-Type"
	case snapshot.HeaderContentLength:
		return "Content-Length"
	case snapshot.HeaderAuthorization:
		return "Authorization"
	case snapshot.HeaderXForwardedFor:
		return "X-Forwarded-For"
	case snapshot.HeaderXForwardedProto:
		return "X-Forwarded-Proto"
	case snapshot.HeaderXRequestID:
		return "X-Request-Id"
	case snapshot.HeaderServer:
		return "Server"
	case snapshot.HeaderXContentTypeOptions:
		return "X-Content-Type-Options"
	case snapshot.HeaderXFrameOptions:
		return "X-Frame-Options"
	case snapshot.HeaderXXSSProtection:
		return "X-Xss-Protection"
	default:
		i := int(id) - int(snapshot.HeaderDynamicStart)
		if names == nil || i < 0 || i >= len(names.Names) {
			return ""
		}
		return string(names.Names[i])
	}
}

// ExecuteMutations applies plan's header operations, in compiled order
// (remove, then set, then add-if-absent), to h.
//
// names resolves any dynamic (non-well-known) header ID plan references;
// pass router.Engine.HeaderNames() for the snapshot the route was
// compiled from. names may be nil if plan is known to reference only
// well-known headers.
func ExecuteMutations(h http.Header, plan *snapshot.CompiledHeadersPlan, names *snapshot.HeaderRegistry) {
	if plan == nil || len(plan.Ops) == 0 {
		return
	}

	for i := range plan.Ops {
		op := &plan.Ops[i]

		name := resolveHeaderName(op.HeaderID, names)
		if name == "" {
			continue
		}

		switch op.Op {
		case snapshot.HeaderOpRemove:
			delete(h, name)
		case snapshot.HeaderOpSet:
			h[name] = []string{op.Value}
		case snapshot.HeaderOpAddIfAbsent:
			if _, exists := h[name]; !exists {
				h[name] = []string{op.Value}
			}
		}
	}
}
