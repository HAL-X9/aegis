package policy

import (
	"net/http"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func resolveHeaderName(id snapshot.HeaderID) string {
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
		return ""
	}
}

func ExecuteMutations(h http.Header, plan *snapshot.CompiledHeadersPlan) {
	if plan == nil || len(plan.Ops) == 0 {
		return
	}

	for i := range plan.Ops {
		op := &plan.Ops[i]

		name := resolveHeaderName(op.HeaderID)
		if name == "" {
			continue
		}

		switch op.Op {
		case snapshot.HeaderOpRemove:
			h.Del(name)
		case snapshot.HeaderOpSet:
			h.Set(name, op.Value)
		case snapshot.HeaderOpAddIfAbsent:
			if h.Get(name) == "" {
				h.Set(name, op.Value)
			}
		}
	}
}
