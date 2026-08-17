package compile

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func Services(services ir.Services) (snapshot.CompiledServices, map[string]snapshot.ServiceID, error) {
	if services == nil {
		return snapshot.CompiledServices{}, nil, fmt.Errorf("failed to compile services: services input is nil")
	}

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
		})
	}

	return snapshot.CompiledServices{
		Items: compiledServices,
	}, serviceIDs, nil
}

func upstreamOriginURL(upstream ir.Upstream) string {
	u := &url.URL{
		Scheme: upstream.Scheme,
		Host: net.JoinHostPort(
			upstream.Host,
			strconv.Itoa(upstream.Port),
		),
	}

	return u.String()
}
