package policy

import (
	"net/http"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// resolveHeaderName maps a HeaderID back to its wire name.
//
// Well-known IDs are resolved by snapshot.WellKnownHeaderName, a plain array
// index. Anything >= snapshot.HeaderDynamicStart is a custom header assigned
// during compilation (see internal/controlplane/compile.HeaderRegistryBuilder)
// and is resolved through names, the registry published alongside the route it
// belongs to (router.Engine.HeaderNames). An ID that neither resolves yields "".
func resolveHeaderName(id snapshot.HeaderID, names *snapshot.HeaderRegistry) string {
	if id < snapshot.HeaderDynamicStart {
		return snapshot.WellKnownHeaderName(id)
	}

	i := int(id) - int(snapshot.HeaderDynamicStart)
	if names == nil || i >= len(names.Names) {
		return ""
	}
	return names.Names[i]
}

// ExecuteMutations applies plan's header operations, in compiled order
// (remove, then set, then add-if-absent), to h.
//
// names resolves any dynamic (non-well-known) header ID plan references;
// pass router.Engine.HeaderNames() for the snapshot the route was
// compiled from. names may be nil if plan is known to reference only
// well-known headers.
//
// Operations write h directly. For an outbound request, writing "Host" has no
// effect, because net/http sends http.Request.Host and ignores
// Header["Host"].
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
