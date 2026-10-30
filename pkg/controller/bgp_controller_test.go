package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"cosmolet/pkg/config"
	"cosmolet/pkg/health"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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

// lastCall returns the most recent command recorded, or "" if none.
func (f *fakeRunner) lastCall() string {
	if len(f.calls) == 0 {
		return ""
	}
	return f.calls[len(f.calls)-1]
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

func TestWithdrawStaleRoutes_RemovesUnconfirmedIPs(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.advertised["10.0.0.1"] = true
	c.advertised["10.0.0.2"] = true

	// 10.0.0.1 confirmed healthy; 10.0.0.2 in neither set → withdraw.
	c.withdrawStaleRoutes(map[string]bool{"10.0.0.1": true}, map[string]bool{})

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

func TestWithdrawStaleRoutes_PreservesTransientErrorRoute(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.advertised["10.0.0.1"] = true
	c.advertised["10.0.0.2"] = true

	// 10.0.0.1 hit a transient API error (kept); 10.0.0.2 is stale (neither).
	c.withdrawStaleRoutes(map[string]bool{}, map[string]bool{"10.0.0.1": true})

	if !c.advertised["10.0.0.1"] {
		t.Error("10.0.0.1 (transient error) should remain in advertised set")
	}
	if _, ok := c.advertised["10.0.0.2"]; ok {
		t.Error("10.0.0.2 (stale) should have been removed from advertised set")
	}
	if fr.hasCall("no network 10.0.0.1/32") {
		t.Errorf("must NOT withdraw the route hit by a transient API error, got: %v", fr.calls)
	}
	if !fr.hasCall("no network 10.0.0.2/32") {
		t.Errorf("expected withdrawal of the stale 10.0.0.2, got: %v", fr.calls)
	}
}

func TestWithdrawStaleRoutes_NoOpWhenAllConfirmed(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.advertised["10.0.0.1"] = true

	c.withdrawStaleRoutes(map[string]bool{"10.0.0.1": true}, map[string]bool{})

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
	c.withdrawStaleRoutes(map[string]bool{"10.0.0.1": true}, map[string]bool{})

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

func TestWithdrawServiceViaBGP_RunsNetworkNoDel(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)

	c.withdrawServiceViaBGP("10.1.2.3")

	if !fr.hasCall("no network 10.1.2.3/32") {
		t.Errorf("expected 'no network' command, got: %v", fr.calls)
	}
	if !fr.hasCall("ip addr del 10.1.2.3/32 dev lo") {
		t.Errorf("expected 'ip addr del' command, got: %v", fr.calls)
	}
	// Persistence is done once per loop by persistFRRConfig, not per-withdraw
	// (Finding 12). A single withdraw must not rewrite frr.conf.
	if fr.hasCall("write memory") {
		t.Errorf("withdraw must not run 'write memory' per call (Finding 12), got: %v", fr.calls)
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
	// Persistence is done once per loop by persistFRRConfig, not per-advertise
	// (Finding 12). A single advertise must not rewrite frr.conf.
	if fr.hasCall("write memory") {
		t.Errorf("advertise must not run 'write memory' per call (Finding 12), got: %v", fr.calls)
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

func TestPersistFRRConfig_RunsWriteMemory(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)

	c.persistFRRConfig()

	if !fr.hasCall("write memory") {
		t.Errorf("expected persistFRRConfig to run 'write memory', got: %v", fr.calls)
	}
}

func TestRouteSetChanged_DetectsChange(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)

	// Baseline: nothing advertised.
	c.snapshotAdvertised()
	if c.routeSetChanged() {
		t.Error("no change expected with an empty baseline")
	}

	// Add a route: change should be detected.
	c.advertised["10.0.0.1"] = true
	if !c.routeSetChanged() {
		t.Error("change expected after adding a route")
	}

	// Snapshot again, then remove it: change detected.
	c.snapshotAdvertised()
	if c.routeSetChanged() {
		t.Error("no change expected after re-snapshot")
	}
	delete(c.advertised, "10.0.0.1")
	if !c.routeSetChanged() {
		t.Error("change expected after removing a route")
	}
}

// Finding 4: a failed `ip addr add` must abort the advertisement (fail hard)
// rather than advertising a route for an IP this node cannot receive.
func TestAdvertiseServiceViaBGP_FailsHardOnIPAddrAdd(t *testing.T) {
	fr := &fakeRunner{failFor: []string{"ip addr add"}}
	c := newTestController(fr.run)

	if err := c.advertiseServiceViaBGP("10.9.9.9"); err == nil {
		t.Fatal("expected an error when 'ip addr add' fails")
	}
	if fr.hasCall("network 10.9.9.9/32") {
		t.Errorf("must not advertise a route after 'ip addr add' failed, got: %v", fr.calls)
	}
	if _, ok := c.advertised["10.9.9.9"]; ok {
		t.Error("must not mark the route advertised when ip addr add failed")
	}
}

// Finding 11: a configured FRR socket path must be passed to vtysh via -s.
func TestAdvertiseServiceViaBGP_PassesVtyshSocketArg(t *testing.T) {
	fr := &fakeRunner{}
	c := newTestController(fr.run)
	c.config.FRR.SocketPath = "/run/frr/vty"

	if err := c.advertiseServiceViaBGP("10.4.4.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fr.hasCall("vtysh -c configure terminal") || !fr.hasCall("-s /run/frr/vty") {
		t.Errorf("expected vtysh to be called with -s /run/frr/vty, got: %v", fr.calls)
	}
}

// Finding 35: the loopback must be found by flag, so both "lo" (Linux) and
// "lo0" (macOS) are returned by loopbackInterface() rather than hardcoding one.
func TestLoopbackInterface_FoundByFlag(t *testing.T) {
	iface, err := loopbackInterface()
	if err != nil {
		t.Fatalf("loopbackInterface() error: %v", err)
	}
	if iface.Flags&net.FlagLoopback == 0 {
		t.Fatalf("loopbackInterface() returned %s which is not flagged loopback", iface.Name)
	}
}

// Finding 5: a transient API error (not a 404) must not withdraw a healthy
// route. performHealthCheck returns errored=true, processService returns
// outcomeKeep, and the control loop leaves the advertised route up.
func TestProcessService_TransientAPIErrorKeepsRoute(t *testing.T) {
	objs := []runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web"},
			Spec:       v1.ServiceSpec{ClusterIP: "10.0.0.50"},
		},
	}
	client := fake.NewSimpleClientset(objs...)
	client.PrependReactor("get", "endpoints", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection reset by peer (transient)")
	})

	fr := &fakeRunner{}
	c := &BGPServiceController{
		client:        client,
		config:        &config.Config{BGP: config.BGPConfig{Enabled: true, ASN: 65000}},
		ctx:           context.Background(),
		healthChecker: health.NewChecker(),
		run:           fr.run,
		advertised:    map[string]bool{"10.0.0.50": true},
	}

	if got := c.processService(*objs[0].(*v1.Service)); got != outcomeKeep {
		t.Fatalf("expected outcomeKeep on transient API error, got %v", got)
	}
	if !c.advertised["10.0.0.50"] {
		t.Error("advertised route must be preserved on a transient API error (Finding 5)")
	}
	if len(fr.calls) != 0 {
		t.Errorf("no FRR commands expected on a transient API error, got: %v", fr.calls)
	}
}

// Finding 5: a service whose endpoints are gone (404) is withdrawn.
func TestProcessService_ServiceDeletedWithdraws(t *testing.T) {
	objs := []runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "gone"},
			Spec:       v1.ServiceSpec{ClusterIP: "10.0.0.51"},
		},
	}
	// No Endpoints object in the clientset → Get returns IsNotFound.
	client := fake.NewSimpleClientset(objs...)

	fr := &fakeRunner{}
	c := &BGPServiceController{
		client:        client,
		config:        &config.Config{BGP: config.BGPConfig{Enabled: true, ASN: 65000}},
		ctx:           context.Background(),
		healthChecker: health.NewChecker(),
		run:           fr.run,
		advertised:    map[string]bool{"10.0.0.51": true},
	}

	if got := c.processService(*objs[0].(*v1.Service)); got != outcomeWithdraw {
		t.Fatalf("expected outcomeWithdraw for a deleted service, got %v", got)
	}
}
