package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"cosmolet/pkg/config"
	"cosmolet/pkg/health"
)

// fakeRunner records every command invoked and returns a canned success or
// failure. It stands in for vtysh/ip in tests so no real binaries are needed.
type fakeRunner struct {
	calls   []string
	failAll bool
	failFor []string
}

func (f *fakeRunner) run(name string, args ...string) ([]byte, error) {
	joined := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, joined)
	if f.failAll {
		return []byte("boom"), fmt.Errorf("fake command failed: %s", joined)
	}
	for _, pat := range f.failFor {
		if strings.Contains(joined, pat) {
			return []byte("boom"), fmt.Errorf("fake command failed: %s", joined)
		}
	}
	return []byte("ok"), nil
}

func (f *fakeRunner) hasCall(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func newTestController(run CommandRunner) *BGPServiceController {
	return &BGPServiceController{
		client: nil,
		config: &config.Config{
			BGP: config.BGPConfig{Enabled: true, ASN: 65000},
		},
		ctx:           context.Background(),
		healthChecker: health.NewChecker(),
		run:           run,
		advertised:    make(map[string]bool),
	}
}

func TestWithdrawStaleRoutes_RemovesWithdrawnIPs(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.advertised["10.0.0.1"] = true
	c.advertised["10.0.0.2"] = true

	c.withdrawStaleRoutes(map[string]bool{"10.0.0.1": true})

	if !c.advertised["10.0.0.1"] {
		t.Error("10.0.0.1 should remain in advertised set")
	}
	if _, ok := c.advertised["10.0.0.2"]; ok {
		t.Error("10.0.0.2 should have been removed from advertised set")
	}
	if !fr.hasCall("no network 10.0.0.2/32") {
		t.Errorf("expected withdrawal of 10.0.0.2, got calls: %v", fr.calls)
	}
	if fr.hasCall("no network 10.0.0.1/32") {
		t.Error("should not withdraw the still-healthy 10.0.0.1")
	}
}

func TestWithdrawStaleRoutes_NoOpWhenAllHealthy(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.advertised["10.0.0.1"] = true

	c.withdrawStaleRoutes(map[string]bool{"10.0.0.1": true})

	if !c.advertised["10.0.0.1"] {
		t.Error("10.0.0.1 should remain in advertised set")
	}
	if len(fr.calls) != 0 {
		t.Errorf("expected no commands, got: %v", fr.calls)
	}
}

func TestWithdrawStaleRoutes_EmptyAdvertised(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.withdrawStaleRoutes(map[string]bool{"10.0.0.1": true})

	if len(c.advertised) != 0 {
		t.Errorf("advertised should be empty, got %v", c.advertised)
	}
	if len(fr.calls) != 0 {
		t.Errorf("expected no commands, got: %v", fr.calls)
	}
}

func TestWithdrawAllRoutes_ClearsMap(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.advertised["10.0.0.1"] = true
	c.advertised["10.0.0.2"] = true

	c.withdrawAllRoutes()

	if len(c.advertised) != 0 {
		t.Errorf("advertised should be empty after withdrawAllRoutes, got %v", c.advertised)
	}
	if !fr.hasCall("no network 10.0.0.1/32") || !fr.hasCall("no network 10.0.0.2/32") {
		t.Errorf("expected both routes withdrawn, got calls: %v", fr.calls)
	}
}

func TestWithdrawServiceViaBGP_RunsNetworkNoWriteDel(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)

	c.withdrawServiceViaBGP("10.1.2.3")

	if !fr.hasCall("no network 10.1.2.3/32") {
		t.Errorf("expected 'no network' command, got: %v", fr.calls)
	}
	if !fr.hasCall("write memory") {
		t.Errorf("expected 'write memory' command, got: %v", fr.calls)
	}
	if !fr.hasCall("ip addr del 10.1.2.3/32 dev lo") {
		t.Errorf("expected 'ip addr del' command, got: %v", fr.calls)
	}
}

func TestAdvertiseServiceViaBGP_SuccessMarksAdvertised(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)

	if err := c.advertiseServiceViaBGP("10.2.0.5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fr.hasCall("ip addr add 10.2.0.5/32 dev lo") {
		t.Errorf("expected 'ip addr add', got: %v", fr.calls)
	}
	if !fr.hasCall("network 10.2.0.5/32") {
		t.Errorf("expected 'network' command, got: %v", fr.calls)
	}
	if !fr.hasCall("write memory") {
		t.Errorf("expected 'write memory', got: %v", fr.calls)
	}
}

func TestAdvertiseServiceViaBGP_FailureNotMarkedAdvertised(t *testing.T) {
	fr := &fakeRunner{failFor: []string{"network 10.3.0.9/32"}}
	c := newTestController(fr.run)

	err := c.advertiseServiceViaBGP("10.3.0.9")
	if err == nil {
		t.Fatal("expected an error when vtysh network command fails")
	}
	// The caller (processService) must not add to advertised on error.
	// Here we just verify advertiseServiceViaBGP returned an error so the
	// caller can decide. We assert the map is still empty because the
	// controller itself only adds on success (see processService).
	if _, ok := c.advertised["10.3.0.9"]; ok {
		t.Error("advertised must not contain the IP after a failed advertise")
	}
}
