package compile

import "github.com/HAL-X9/aegis/internal/snapshot"

// ApplyServicePolicies copies each service's timeout and retry policy onto
// the routes that target it. Call once after both Services and Routes have
// been compiled.
func ApplyServicePolicies(routes []snapshot.CompiledRoute, services snapshot.CompiledServices) {
	for i := range routes {
		id := int(routes[i].Service)
		if id < len(services.Items) {
			routes[i].Policies.Timeout = services.Items[id].Timeout
			routes[i].Policies.Retry = services.Items[id].Retry
		}
	}
}
