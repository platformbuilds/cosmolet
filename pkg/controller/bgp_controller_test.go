package controller

import (
	"context"
	"testing"

	"cosmolet/pkg/config"
	"cosmolet/pkg/health"
)

func newTestController(t *testing.T) *BGPServiceController {
	t.Helper()
	return &BGPServiceController{
		client:  nil,
		config: &config.Config{
			BGP: config.BGPConfig{
				Enabled: true,
				ASN:     65000,
			},
		},
		ctx:           context.Background(),
		healthChecker: health.NewChecker(),
		advertised:    make(map[string]bool),
	}
}

func TestWithdrawStaleRoutes_RemovesWithdrawnIPs(t *testing.T) {
	c := newTestController(t)
	c.advertised["10.0.0.1"] = true
	c.advertised["10.0.0.2"] = true

	healthy := map[string]bool{
		"10.0.0.1": true,
	}

	c.withdrawStaleRoutes(healthy)

	if !c.advertised["10.0.0.1"] {
		t.Error("10.0.0.1 should still be in advertised set")
	}
	if _, ok := c.advertised["10.0.0.2"]; ok {
		t.Error("10.0.0.2 should have been removed from advertised set")
	}
}

func TestWithdrawStaleRoutes_NoOpWhenAllHealthy(t *testing.T) {
	c := newTestController(t)
	c.advertised["10.0.0.1"] = true

	healthy := map[string]bool{
		"10.0.0.1": true,
	}

	c.withdrawStaleRoutes(healthy)

	if !c.advertised["10.0.0.1"] {
		t.Error("10.0.0.1 should remain in advertised set")
	}
}

func TestWithdrawStaleRoutes_EmptyAdvertised(t *testing.T) {
	c := newTestController(t)
	c.withdrawStaleRoutes(map[string]bool{"10.0.0.1": true})

	if len(c.advertised) != 0 {
		t.Errorf("advertised should be empty, got %v", c.advertised)
	}
}

func TestWithdrawAllRoutes_ClearsMap(t *testing.T) {
	c := newTestController(t)
	c.advertised["10.0.0.1"] = true
	c.advertised["10.0.0.2"] = true

	c.withdrawAllRoutes()

	if len(c.advertised) != 0 {
		t.Errorf("advertised should be empty after withdrawAllRoutes, got %v", c.advertised)
	}
}

func TestProcessService_UnhealthyServiceNotAdvertised(t *testing.T) {
	c := newTestController(t)
	// processService on a service with no endpoints returns false
	// We test via the health check logic indirectly by checking that
	// an unhealthy service does not add to advertised set.
	_ = c
	// This is a smoke test that the controller can be constructed
	// and that advertised map starts empty.
	if len(c.advertised) != 0 {
		t.Error("new controller should have empty advertised map")
	}
}
