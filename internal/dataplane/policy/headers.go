package policy

import (
	"net/http"

	"github.com/aegis/internal/controlplane/snapshot"
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
		return "X-Request-ID"
	case snapshot.HeaderServer:
		return "Server"
	case snapshot.HeaderXContentTypeOptions:
		return "X-Content-Type-Options"
	case snapshot.HeaderXFrameOptions:
		return "X-Frame-Options"
	case snapshot.HeaderXXSSProtection:
		return "X-XSS-Protection"
	default:
		return ""
	}
}

func ExecuteMutations(h http.Header, plan *snapshot.CompiledHeadersPlan) {
	if plan == nil || len(plan.Ops) == 0 {
		return
	}

	for i := 0; i < len(plan.Ops); i++ {
		op := plan.Ops[i]

		name := resolveHeaderName(op.HeaderID)
		if name == "" {
			continue
		}

		switch op.Op {
		case snapshot.HeaderOpRemove:
			h.Del(name)
		case snapshot.HeaderOpSet:
			valueBytes := plan.Values[op.ValueOffset : op.ValueOffset+uint32(op.ValueLength)]
			h.Set(name, string(valueBytes))
		case snapshot.HeaderOpAddIfAbsent:
			if h.Get(name) == "" {
				valueBytes := plan.Values[op.ValueOffset : op.ValueOffset+uint32(op.ValueLength)]
				h.Set(name, string(valueBytes))
			}
		}
	}
}
