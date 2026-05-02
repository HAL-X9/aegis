package compiler

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/aegis/internal/controlplane/model"
)

// Compile transforms control-plane configuration into a routing manifest that
// is optimized for deterministic dataplane lookup.
func Compile(cfg *model.GatewayConfig) (*CompiledGatewayConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("compile routing configuration: manifest is nil")
	}

	compiledRoute := make([]CompiledRoute, 0, len(cfg.Routes))
	for _, route := range cfg.Routes {
		compiledRoute = append(compiledRoute, CompiledRoute{
			PathPrefix: route.Match.PathPrefix,
			Upstream:   BuildUpstreamOriginURL(route),
		})
	}

	// TODO: BitMash for Method and Header Predicates

	/*
		routes = append(routes, CompiledRoute{

		})
	*/

	return &CompiledGatewayConfig{Routes: compiledRoute}, nil
}

func BuildUpstreamOriginURL(route model.Route) string {
	u := &url.URL{
		Scheme: route.Upstream.Scheme,
		Host:   net.JoinHostPort(route.Upstream.Host, strconv.Itoa(route.Upstream.Port)),
	}

	return u.String()
}
