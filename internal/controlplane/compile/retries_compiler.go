package compile

import "github.com/HAL-X9/aegis/internal/snapshot"

// ApplyServiceRetries copies each service's retry policy onto the routes that
// target it. Call once after both Services and Routes have been compiled.
//
// It is a separate step so compile.Routes keeps its current signature.
func ApplyServiceRetries(routes []snapshot.CompiledRoute, services snapshot.CompiledServices) {
	for i := range routes {
		id := int(routes[i].Service)
		if id < len(services.Items) {
			routes[i].Policies.Retry = services.Items[id].Retry
		}
	}
}
