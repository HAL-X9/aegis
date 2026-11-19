package compile

import (
	"net"
	"net/url"
	"sort"
	"strconv"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

func Services(services ir.Services) (snapshot.CompiledServices, map[string]snapshot.ServiceID) {
	serviceName := make([]string, 0, len(services))
	for name := range services {
		serviceName = append(serviceName, name)
	}
	sort.Strings(serviceName)

	compiledServices := make([]snapshot.CompiledService, 0, len(services))
	serviceIDs := make(map[string]snapshot.ServiceID)

	for i, name := range serviceName {
		service := services[name]

		serviceIDs[name] = snapshot.ServiceID(i)

		compiledServices = append(compiledServices, snapshot.CompiledService{
			Name:    name,
			Targets: targetOrigins(service.Upstream),
			Timeout: service.Timeout,
			Retry: snapshot.CompiledRetry{
				Attempts:       max(1, service.Retries.Attempts),
				AttemptTimeout: service.Retries.AttemptTimeout,
			},
		})
	}

	return snapshot.CompiledServices{
		Items: compiledServices,
	}, serviceIDs
}

// targetOrigins returns the origin (scheme and host) of every target of
// upstream, in order.
func targetOrigins(upstream ir.Upstream) []url.URL {
	origins := make([]url.URL, len(upstream.Targets))
	for i, target := range upstream.Targets {
		origins[i] = url.URL{
			Scheme: upstream.Scheme,
			Host:   net.JoinHostPort(target.Host, strconv.Itoa(target.Port)),
		}
	}
	return origins
}
