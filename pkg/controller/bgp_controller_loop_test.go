package controller

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"cosmolet/pkg/config"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// newFakeController builds a controller backed by the k8s fake clientset so the
// full control loop can be exercised without a real cluster or real vtysh/ip.
// The namespaces come from objs (a namespace is included iff an object lives in
// it) so the test author never has to keep config in sync with objects by hand.
func newFakeController(t *testing.T, run CommandRunner, ctx context.Context, objs ...runtime.Object) *BGPServiceController {
	t.Helper()
	ns := map[string]struct{}{}
	for _, o := range objs {
		if o == nil {
			continue
		}
		switch v := o.(type) {
		case *v1.Service:
			ns[v.Namespace] = struct{}{}
		case *v1.Endpoints:
			ns[v.Namespace] = struct{}{}
		}
	}
	namespaces := make([]string, 0, len(ns))
	for n := range ns {
		namespaces = append(namespaces, n)
	}
	cfg := &config.Config{
		Services:            config.ServicesConfig{Namespaces: namespaces},
		LoopIntervalSeconds: 1,
		BGP:                 config.BGPConfig{Enabled: true, ASN: 65000},
	}
	if run == nil {
		run = func(string, ...string) ([]byte, error) { return nil, nil }
	}
	return newControllerForTest(fake.NewClientset(objs...), cfg, ctx, run)
}

// svc builds a Service with a given ClusterIP ("" and "None" are allowed to
// model empty/headless services).
func svc(ns, name, clusterIP string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec:       v1.ServiceSpec{ClusterIP: clusterIP},
	}
}

// eps builds an Endpoints object with a single subset holding the given
// ready addresses (pass nil/empty for an empty subset).
func eps(ns, name string, ready ...string) *v1.Endpoints {
	addrs := make([]v1.EndpointAddress, 0, len(ready))
	for _, a := range ready {
		addrs = append(addrs, v1.EndpointAddress{IP: a})
	}
	return &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Subsets:    []v1.EndpointSubset{{Addresses: addrs}},
	}
}

// loAvailable reports whether the controller's loopback probe
// (net.InterfaceByName("lo")) succeeds in this environment. On Linux the
// loopback is "lo"; on macOS/BSD it is "lo0", so the probe errors there. The
// controller's isServiceAdvertisedByFRR relies on "lo", so the loopback-gated
// advertise/withdraw paths can only be exercised where "lo" exists.
func loAvailable() bool {
	iface, err := net.InterfaceByName("lo")
	if err != nil {
		return false
	}
	_, err = iface.Addrs()
	return err == nil
}

// skipUnlessLo skips the test when the loopback interface "lo" is unavailable,
// which is the case on macOS/BSD (loopback is "lo0" there) and any environment
// where the probe cannot succeed.
func skipUnlessLo(t *testing.T) {
	t.Helper()
	if !loAvailable() {
		t.Skip("loopback interface 'lo' not available (macOS/BSD use 'lo0'); the loopback-gated BGP path cannot be exercised here")
	}
}

// pickTestIP returns a ClusterIP that is guaranteed NOT to be on the local
// loopback interface (the normal case in tests). It prefers 10.99.99.1,
// falling back to a fixed candidate that is not present on lo.
func pickTestIP(t *testing.T) string {
	t.Helper()
	cands := []string{"10.99.99.1", "10.99.99.2", "10.99.99.3", "10.99.99.4"}
	if !loAvailable() {
		return cands[0]
	}
	iface, _ := net.InterfaceByName("lo")
	addrs, _ := iface.Addrs()
	onLo := map[string]struct{}{}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil {
			onLo[ip.String()] = struct{}{}
		}
	}
	for _, c := range cands {
		if _, ok := onLo[c]; !ok {
			return c
		}
	}
	return cands[0]
}

// ipBOther returns a second ClusterIP distinct from ipA that is also not on
// loopback.
func ipBOther(t *testing.T, ipA string) string {
	t.Helper()
	cands := []string{"10.99.99.11", "10.99.99.12", "10.99.99.13"}
	if !loAvailable() {
		for _, c := range cands {
			if c != ipA {
				return c
			}
		}
		return cands[0]
	}
	iface, _ := net.InterfaceByName("lo")
	addrs, _ := iface.Addrs()
	onLo := map[string]struct{}{}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil {
			onLo[ip.String()] = struct{}{}
		}
	}
	for _, c := range cands {
		if c == ipA {
			continue
		}
		if _, ok := onLo[c]; !ok {
			return c
		}
	}
	return cands[0]
}

func TestFetchServicesFromNamespaces(t *testing.T) {
	fr := &fakeRunner{}
	objs := []runtime.Object{
		svc("ns-a", "web", "10.0.0.1"),
		svc("ns-a", "headless", "None"),
		svc("ns-a", "emptyip", ""),
		svc("ns-b", "api", "10.0.0.2"),
	}
	c := newFakeController(t, fr.run, context.Background(), objs...)

	got, err := c.fetchServicesFromNamespaces()
	if err != nil {
		t.Fatalf("fetchServicesFromNamespaces() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 services (headless + empty ClusterIP filtered), got %d: %v", len(got), names(got))
	}
	ips := map[string]bool{}
	for _, s := range got {
		ips[s.Spec.ClusterIP] = true
	}
	if !ips["10.0.0.1"] || !ips["10.0.0.2"] {
		t.Errorf("expected ClusterIPs 10.0.0.1 and 10.0.0.2, got %v", ips)
	}
	if len(fr.calls) != 0 {
		t.Errorf("fetchServicesFromNamespaces should not issue commands, got %v", fr.calls)
	}
}

func TestPerformHealthCheck_ReadyEndpointsHealthy(t *testing.T) {
	fr := &fakeRunner{}
	c := newFakeController(t, fr.run, context.Background(),
		svc("ns", "web", "10.0.0.1"),
		eps("ns", "web", "1.2.3.4", "5.6.7.8"),
	)

	healthy, err := c.performHealthCheck(*svc("ns", "web", "10.0.0.1"))
	if err != nil {
		t.Fatalf("performHealthCheck() error: %v", err)
	}
	if !healthy {
		t.Error("expected healthy=true with ready endpoints, got false")
	}
}

func TestPerformHealthCheck_NoReadyEndpointsUnhealthy(t *testing.T) {
	fr := &fakeRunner{}
	c := newFakeController(t, fr.run, context.Background(),
		svc("ns", "web", "10.0.0.1"),
		eps("ns", "web"), // empty subset, no ready addresses
	)

	healthy, err := c.performHealthCheck(*svc("ns", "web", "10.0.0.1"))
	if err != nil {
		t.Fatalf("performHealthCheck() error: %v", err)
	}
	if healthy {
		t.Error("expected healthy=false with no ready endpoints, got true")
	}
}

func TestPerformHealthCheck_MissingEndpointsError(t *testing.T) {
	fr := &fakeRunner{}
	// Service exists but no Endpoints object -> Get returns not found -> error.
	c := newFakeController(t, fr.run, context.Background(),
		svc("ns", "web", "10.0.0.1"),
	)

	healthy, err := c.performHealthCheck(*svc("ns", "web", "10.0.0.1"))
	if err == nil {
		t.Fatal("expected an error when endpoints are missing")
	}
	if healthy {
		t.Error("expected healthy=false on error, got true")
	}
}

func TestProcessService_UnhealthySkipsAdvertise(t *testing.T) {
	fr := &fakeRunner{}
	ip := pickTestIP(t)
	c := newFakeController(t, fr.run, context.Background(),
		svc("ns", "web", ip),
		eps("ns", "web"), // no ready endpoints -> unhealthy
	)

	healthy := c.processService(*svc("ns", "web", ip))
	if healthy {
		t.Error("expected processService to return false for an unhealthy service")
	}
	// processService returns false at the health-check step, BEFORE the
	// isServiceAdvertisedByFRR / loopback probe is ever reached, so this holds
	// on both lo and no-lo environments.
	if len(fr.calls) != 0 {
		t.Errorf("expected no commands for an unhealthy service, got %v", fr.calls)
	}
	if _, ok := c.advertised[ip]; ok {
		t.Errorf("unhealthy service %s must not be marked advertised", ip)
	}
}

func TestProcessService_HealthyAdvertises(t *testing.T) {
	skipUnlessLo(t)
	fr := &fakeRunner{}
	ip := pickTestIP(t)
	c := newFakeController(t, fr.run, context.Background(),
		svc("ns", "web", ip),
		eps("ns", "web", "1.2.3.4"),
	)

	healthy := c.processService(*svc("ns", "web", ip))
	if !healthy {
		t.Error("expected processService to return true for a healthy service")
	}
	if !fr.hasCall("ip addr add "+ip+"/32 dev lo") {
		t.Errorf("expected 'ip addr add %s/32', got %v", ip, fr.calls)
	}
	if !fr.hasCall("network "+ip+"/32") {
		t.Errorf("expected 'network %s/32', got %v", ip, fr.calls)
	}
	if !fr.hasCall("write memory") {
		t.Errorf("expected 'write memory', got %v", fr.calls)
	}
	if _, ok := c.advertised[ip]; !ok {
		t.Errorf("expected %s to be marked advertised", ip)
	}
}

func TestProcessService_HealthyButAdvertiseFailsNotMarked(t *testing.T) {
	// If the vtysh network command fails, advertiseServiceViaBGP returns an
	// error and processService must NOT mark the IP advertised (so the next
	// loop retries). It still returns true (healthy) because health is
	// independent of advertise success.
	skipUnlessLo(t)
	ip := pickTestIP(t)
	fr := &fakeRunner{failFor: []string{"network " + ip + "/32"}}
	c := newFakeController(t, fr.run, context.Background(),
		svc("ns", "web", ip),
		eps("ns", "web", "1.2.3.4"),
	)

	healthy := c.processService(*svc("ns", "web", ip))
	if !healthy {
		t.Error("expected processService to return true (healthy) even when advertise fails")
	}
	if _, ok := c.advertised[ip]; ok {
		t.Errorf("advertised map must not contain %s after a failed advertise", ip)
	}
}

func TestProcessService_AlreadyAdvertisedOnLoopback(t *testing.T) {
	// Exercises the "isServiceAdvertisedByFRR returns already-advertised" path
	// by using a real loopback address (127.0.0.1, always present on "lo") as
	// the ClusterIP, with the runner returning the FRR best-path marker "*>".
	// When isServiceAdvertisedByFRR reports already-advertised, processService
	// must skip advertiseServiceViaBGP entirely (no ip addr add / network
	// commands) and simply record the IP as advertised.
	skipUnlessLo(t)
	ip := "127.0.0.1"
	fr := &fakeRunner{}
	c := newFakeController(t, func(name string, args ...string) ([]byte, error) {
		// Record the call so we can assert what (if anything) was issued, and
		// make the BGP show command report a locally-sourced best path.
		fr.run(name, args...)
		if name == "vtysh" {
			return []byte("*> " + ip + "/32 0.0.0.0 0 0 ?"), nil
		}
		return []byte("ok"), nil
	}, context.Background(),
		svc("ns", "web", ip),
		eps("ns", "web", "1.2.3.4"),
	)

	healthy := c.processService(*svc("ns", "web", ip))
	if !healthy {
		t.Error("expected processService to return true for an already-advertised healthy service")
	}
	if _, ok := c.advertised[ip]; !ok {
		t.Errorf("expected %s to be in the advertised map", ip)
	}
	// No advertise commands should have been issued.
	if fr.hasCall("ip addr add") {
		t.Errorf("already-advertised path must not run 'ip addr add', got %v", fr.calls)
	}
	if fr.hasCall("network ") {
		t.Errorf("already-advertised path must not run a 'network' advertise, got %v", fr.calls)
	}
}

func TestRunControlLoop_NoServicesNoOp(t *testing.T) {
	fr := &fakeRunner{}
	c := newFakeController(t, fr.run, context.Background())

	c.runControlLoop()

	if len(fr.calls) != 0 {
		t.Errorf("expected no commands with no services, got %v", fr.calls)
	}
	if len(c.advertised) != 0 {
		t.Errorf("expected advertised to be empty, got %v", c.advertised)
	}
}

func TestRunControlLoop_WithdrawsUnhealthy(t *testing.T) {
	skipUnlessLo(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ipA := pickTestIP(t)
	ipB := ipBOther(t, ipA)

	// Loop 1: both healthy -> both advertised.
	fr1 := &fakeRunner{}
	c := newFakeController(t, fr1.run, ctx,
		svc("ns", "web", ipA),
		eps("ns", "web", "1.1.1.1"),
		svc("ns", "api", ipB),
		eps("ns", "api", "2.2.2.2"),
	)
	c.runControlLoop()

	if !c.advertised[ipA] || !c.advertised[ipB] {
		t.Fatalf("expected both IPs advertised after loop 1, got %v", c.advertised)
	}

	// Loop 2: 'api' goes unhealthy (endpoints empty) -> its route withdrawn,
	// 'web' stays.
	fr2 := &fakeRunner{}
	c2 := newFakeController(t, fr2.run, ctx,
		svc("ns", "web", ipA),
		eps("ns", "web", "1.1.1.1"),
		svc("ns", "api", ipB),
		eps("ns", "api"), // now empty
	)
	// Carry the advertised state over from loop 1 to model convergence.
	c2.advertised = c.advertised
	c2.runControlLoop()

	if !c2.advertised[ipA] {
		t.Errorf("expected %s (still healthy) to remain advertised", ipA)
	}
	if _, ok := c2.advertised[ipB]; ok {
		t.Errorf("expected %s (now unhealthy) to be removed from advertised", ipB)
	}
	if !fr2.hasCall("no network "+ipB+"/32") {
		t.Errorf("expected withdrawal of %s/32, got %v", ipB, fr2.calls)
	}
	if fr2.hasCall("no network "+ipA+"/32") {
		t.Errorf("must not withdraw the still-healthy %s, got %v", ipA, fr2.calls)
	}
}

func TestRunControlLoop_RespectsContextCancel(t *testing.T) {
	// A large loop interval (1 hour) would make the trailing sleep() block
	// forever if ctx is not respected. With a cancelled ctx the loop must
	// return promptly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fr := &fakeRunner{}
	// A fixed ClusterIP is fine here: the test proves ctx-cancel is respected
	// by the loop (sleep() returns via ctx.Done()), independent of the IP's
	// relationship to loopback.
	ip := "10.99.99.5"
	c := newFakeController(t, fr.run, ctx,
		svc("ns", "web", ip),
		eps("ns", "web", "1.2.3.4"),
	)
	// Override the loop interval to a huge value: if sleep() honored only the
	// timer (not ctx.Done()), this would hang.
	c.config.LoopIntervalSeconds = 3600

	done := make(chan struct{})
	go func() {
		c.runControlLoop()
		close(done)
	}()

	select {
	case <-done:
		// Returned promptly as expected.
	case <-time.After(3 * time.Second):
		t.Fatal("runControlLoop did not return promptly after context cancellation")
	}
}

func TestGetKubeConfig_KubeconfigFallbackError(t *testing.T) {
	// In this test environment InClusterConfig() fails (no in-cluster env), so
	// GetKubeConfig falls back to KUBECONFIG. Pointing it at a nonexistent
	// path must return an error mentioning that path.
	prev, hadPrev := os.LookupEnv("KUBECONFIG")
	defer func() {
		if hadPrev {
			os.Setenv("KUBECONFIG", prev)
		} else {
			os.Unsetenv("KUBECONFIG")
		}
	}()
	bad := "/nonexistent/kubeconfig/does-not-exist"
	os.Setenv("KUBECONFIG", bad)

	cfg, err := GetKubeConfig()
	if err == nil {
		t.Fatalf("expected an error for a nonexistent KUBECONFIG, got config %+v", cfg)
	}
	if want := bad; !strings.Contains(err.Error(), want) {
		t.Errorf("error should mention %q, got %q", want, err.Error())
	}
}

// names returns the "ns/name" of each service for readable failure messages.
func names(svcs []v1.Service) []string {
	out := make([]string, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, fmt.Sprintf("%s/%s", s.Namespace, s.Name))
	}
	return out
}
