package compile

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
)

// TestServicesCompilesEveryTargetInOrder checks that each target becomes an
// origin of the service's scheme, in configuration order, with an IPv6
// address bracketed so that its port stays unambiguous.
func TestServicesCompilesEveryTargetInOrder(t *testing.T) {
	compiled, ids := Services(ir.Services{
		"pool": {
			Upstream: ir.Upstream{
				Scheme: "https",
				Targets: []ir.Target{
					{Host: "b.internal", Port: 443},
					{Host: "a.internal", Port: 8443},
					{Host: "2001:db8::1", Port: 443},
				},
			},
			Retries: ir.Retries{Attempts: 1},
		},
	})

	service := compiled.Items[ids["pool"]]
	want := []string{"https://b.internal:443", "https://a.internal:8443", "https://[2001:db8::1]:443"}
	if len(service.Targets) != len(want) {
		t.Fatalf("targets = %v, want %v", service.Targets, want)
	}
	for i, origin := range want {
		if got := service.Targets[i].String(); got != origin {
			t.Fatalf("target %d = %s, want %s", i, got, origin)
		}
	}
}
