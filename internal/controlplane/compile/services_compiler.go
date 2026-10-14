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
			Name:     name,
			Upstream: upstreamOriginURL(service.Upstream),
			Timeout:  service.Timeout,
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

func upstreamOriginURL(upstream ir.Upstream) *url.URL {
	return &url.URL{
		Scheme: upstream.Scheme,
		Host: net.JoinHostPort(
			upstream.Host,
			strconv.Itoa(upstream.Port),
		),
	}
}
