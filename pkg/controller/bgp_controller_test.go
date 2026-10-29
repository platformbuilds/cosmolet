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

func TestRoutePrefix_FamilyAware(t *testing.T) {
	cases := map[string]string{
		"10.0.0.5":      "10.0.0.5/32",
		"fd00::1":       "fd00::1/128",
		"2001:db8::42":  "2001:db8::42/128",
	}
	for in, want := range cases {
		if got := routePrefix(in); got != want {
			t.Errorf("routePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBgpAddressFamily_FamilyAware(t *testing.T) {
	if got := bgpAddressFamily("10.0.0.5"); got != "address-family ipv4 unicast" {
		t.Errorf("expected ipv4 address-family, got %q", got)
	}
	if got := bgpAddressFamily("fd00::1"); got != "address-family ipv6 unicast" {
		t.Errorf("expected ipv6 address-family, got %q", got)
	}
}

func TestBgpShowCommand_FamilyAware(t *testing.T) {
	if got := bgpShowCommand("10.0.0.5"); got != "show ip bgp 10.0.0.5" {
		t.Errorf("expected ipv4 show command, got %q", got)
	}
	if got := bgpShowCommand("fd00::1"); got != "show bgp ipv6 unicast fd00::1" {
		t.Errorf("expected ipv6 show command, got %q", got)
	}
}

func TestAdvertiseServiceViaBGP_IPv6UsesFamilyAndPrefix(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)

	if err := c.advertiseServiceViaBGP("fd00:1234::42"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fr.hasCall("ip addr add fd00:1234::42/128 dev lo") {
		t.Errorf("expected ipv6 /128 loopback add, got: %v", fr.calls)
	}
	if !fr.hasCall("address-family ipv6 unicast") {
		t.Errorf("expected ipv6 address-family, got: %v", fr.calls)
	}
	if !fr.hasCall("network fd00:1234::42/128") {
		t.Errorf("expected ipv6 network command, got: %v", fr.calls)
	}
	if fr.hasCall("address-family ipv4 unicast") {
		t.Errorf("must not use ipv4 address-family for an ipv6 cluster ip, got: %v", fr.calls)
	}
}

func TestWithdrawServiceViaBGP_IPv6UsesFamilyAndPrefix(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)

	c.withdrawServiceViaBGP("fd00:1234::42")

	if !fr.hasCall("no network fd00:1234::42/128") {
		t.Errorf("expected ipv6 /128 withdrawal, got: %v", fr.calls)
	}
	if !fr.hasCall("address-family ipv6 unicast") {
		t.Errorf("expected ipv6 address-family on withdraw, got: %v", fr.calls)
	}
	if !fr.hasCall("ip addr del fd00:1234::42/128 dev lo") {
		t.Errorf("expected ipv6 loopback del, got: %v", fr.calls)
	}
}
