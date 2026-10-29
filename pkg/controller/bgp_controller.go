package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cosmolet/pkg/config"
	"cosmolet/pkg/health"
	"cosmolet/pkg/logging"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// commandTimeout bounds every external command (vtysh/ip). A hung FRR or
// network namespace must not block the control loop or shutdown (Finding 6).
const commandTimeout = 10 * time.Second

// Leader-election timings (Finding 2). Kept as vars so tests can shorten them.
var (
	leaderLeaseDuration = 15 * time.Second
	leaderRenewDeadline = 10 * time.Second
	leaderRetryPeriod   = 5 * time.Second
)

// podName / podNamespace identify this controller instance for the leader
// lock identity and Lease placement. Overridable in tests.
var (
	podName      = envOr("POD_NAME", hostnameFallback())
	podNamespace = envOr("POD_NAMESPACE", "default")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func hostnameFallback() string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "unknown"
}

// randomString returns n random lowercase-hex characters (n/2 random bytes),
// used to make the leader-lock identity unique per process lifetime.
func randomString(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		// Fall back to a time-derived value; identity uniqueness is best-effort.
		return fmt.Sprintf("%08x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)[:n]
}

// CommandRunner runs an external command and returns its combined output.
// It exists so tests can substitute a fake runner instead of invoking real
// vtysh/ip binaries. Production behaviour is unchanged: execRunner runs the
// command verbatim.
type CommandRunner func(name string, args ...string) ([]byte, error)

// commandContext builds a context that is cancelled either when the parent
// context is done or when commandTimeout elapses, so a hung vtysh/ip cannot
// block the loop (Finding 6). The returned cancel must always be called.
func commandContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, commandTimeout)
}

// execRunner is the default CommandRunner: it runs the command via os/exec
// with a bounded timeout.
func execRunner(name string, args ...string) ([]byte, error) {
	cmdCtx, cancel := commandContext(context.Background())
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && cmdCtx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command %q timed out after %s", name, commandTimeout)
	}
	return out, err
}

// BGPServiceController manages BGP advertisements for Kubernetes services
type BGPServiceController struct {
	client        kubernetes.Interface
	config        *config.Config
	ctx           context.Context
	healthChecker *health.Checker
	run           CommandRunner
	podName       string
	podNamespace  string

	mu             sync.Mutex
	advertised     map[string]bool
	lastAdvertised map[string]bool
}

// NewBGPServiceController creates a new BGP service controller
func NewBGPServiceController(cfg *config.Config, ctx context.Context) (*BGPServiceController, error) {
	kubeConfig, err := GetKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	return &BGPServiceController{
		client:        clientset,
		config:        cfg,
		ctx:           ctx,
		healthChecker: health.NewChecker(),
		run:           execRunner,
		podName:       podName,
		podNamespace:  podNamespace,
		advertised:    make(map[string]bool),
	}, nil
}

// newControllerForTest constructs a controller with an injected client and
// command runner, bypassing kubeconfig discovery. It is exported for tests.
func newControllerForTest(client kubernetes.Interface, cfg *config.Config, ctx context.Context, run CommandRunner) *BGPServiceController {
	if run == nil {
		run = execRunner
	}
	return &BGPServiceController{
		client:        client,
		config:        cfg,
		ctx:           ctx,
		healthChecker: health.NewChecker(),
		run:           run,
		advertised:    make(map[string]bool),
	}
}

// Start begins the main control loop, gated by Lease-based leader election
// (Finding 2). Only the elected leader mutates BGP state; all pods keep
// monitoring health/readiness. Returns once the context is cancelled and all
// previously advertised routes have been withdrawn, so callers can wait for a
// clean shutdown (Finding 3).
func (c *BGPServiceController) Start() error {
	logging.Info("Starting BGP Service Controller...")

	if err := c.testKubernetesAPI(); err != nil {
		c.healthChecker.CheckKubernetesAPI(false, err.Error())
		return fmt.Errorf("kubernetes API connectivity test failed: %v", err)
	}
	c.healthChecker.CheckKubernetesAPI(true, "Connected")

	if err := c.testFRRConnectivity(); err != nil {
		c.healthChecker.CheckFRRStatus(false, err.Error())
		logging.Warn("FRR connectivity test failed: %v", err)
	} else {
		c.healthChecker.CheckFRRStatus(true, "Connected")
	}

	// Build the leader-election lock. A coordination.k8s.io Lease is the
	// standard, idempotent lock type for controllers.
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      "cosmolet-leader",
			Namespace: c.podNamespace,
		},
		Client: c.client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: c.podName + "_" + randomString(5),
		},
	}

	leaderelection.RunOrDie(c.ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaderLeaseDuration,
		RenewDeadline:   leaderRenewDeadline,
		RetryPeriod:     leaderRetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				logging.Info("Acquired leader lease; running BGP control loop")
				c.runLeaderLoop()
			},
			OnStoppedLeading: func() {
				logging.Warn("Lost leader lease; ceasing route management")
			},
		},
	})

	// On shutdown, withdraw any routes we still advertise so we do not leave
	// stale routes behind (Finding 3). RunOrDie returns when c.ctx is
	// cancelled and the leader lock is released.
	if len(c.advertisedKeys()) > 0 {
		logging.Info("Shutting down: withdrawing all advertised routes")
		c.withdrawAllRoutes()
	}
	return nil
}

// runLeaderLoop is the control-loop body, executed only by the elected
// leader. It blocks until the context is cancelled.
func (c *BGPServiceController) runLeaderLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.runControlLoop()
		}
	}
}

// advertisedKeys returns a snapshot of the currently advertised IPs.
func (c *BGPServiceController) advertisedKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ips := make([]string, 0, len(c.advertised))
	for ip := range c.advertised {
		ips = append(ips, ip)
	}
	return ips
}

// runControlLoop executes one iteration of the control loop
func (c *BGPServiceController) runControlLoop() {
	start := time.Now()
	logging.Debug("=== Starting new loop iteration ===")

	c.healthChecker.UpdateLastLoop()
	c.snapshotAdvertised()

	// Step 1: Fetch all running services in configured namespaces
	services, err := c.fetchServicesFromNamespaces()
	if err != nil {
		log.Printf("Error fetching services: %v", err)
		c.healthChecker.CheckServiceDiscovery(0, time.Since(start))
		c.sleep()
		return
	}

	log.Printf("Found %d services to process", len(services))
	c.healthChecker.CheckServiceDiscovery(len(services), time.Since(start))

	// Step 2: Process each service and track which are currently healthy
	healthyIPs := make(map[string]bool)
	for _, service := range services {
		select {
		case <-c.ctx.Done():
			return
		default:
			isHealthy := c.processService(service)
			if isHealthy {
				healthyIPs[service.Spec.ClusterIP] = true
			}
		}
	}

	// Step 3: Withdraw routes for services no longer healthy or deleted
	c.withdrawStaleRoutes(healthyIPs)

	// Step 4: Persist FRR state once per iteration, and only when the
	// advertised route set changed, so we do not rewrite frr.conf on every
	// loop (Finding 12).
	if c.config.IsBGPEnabled() && c.routeSetChanged() {
		c.persistFRRConfig()
	}

	duration := time.Since(start)
	logging.Debug("Loop finished in %v. Sleeping for %d seconds...", duration, c.config.GetLoopInterval())
	c.sleep()
}

// routeSetChanged reports whether the set of advertised IPs differs from the
// snapshot taken at the start of this iteration. Used to persist FRR state
// only on actual change (Finding 12).
func (c *BGPServiceController) routeSetChanged() bool {
	current := c.advertisedKeys()
	c.mu.Lock()
	last := make(map[string]bool, len(c.lastAdvertised))
	for ip := range c.lastAdvertised {
		last[ip] = true
	}
	c.mu.Unlock()

	if len(current) != len(last) {
		return true
	}
	cur := make(map[string]bool, len(current))
	for _, ip := range current {
		cur[ip] = true
	}
	for ip := range last {
		if !cur[ip] {
			return true
		}
	}
	return false
}

// snapshotAdvertised records the current advertised set as the baseline for
// change detection in this iteration.
func (c *BGPServiceController) snapshotAdvertised() {
	c.mu.Lock()
	c.lastAdvertised = make(map[string]bool, len(c.advertised))
	for ip := range c.advertised {
		c.lastAdvertised[ip] = true
	}
	c.mu.Unlock()
}

// fetchServicesFromNamespaces fetches all services from configured namespaces
func (c *BGPServiceController) fetchServicesFromNamespaces() ([]v1.Service, error) {
	var allServices []v1.Service

	for _, namespace := range c.config.GetNamespaces() {
		log.Printf("Fetching services from namespace: %s", namespace)

		services, err := c.client.CoreV1().Services(namespace).List(c.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list services in namespace %s: %v", namespace, err)
		}

		for _, service := range services.Items {
			if service.Spec.ClusterIP != "" && service.Spec.ClusterIP != "None" {
				allServices = append(allServices, service)
			}
		}
	}

	return allServices, nil
}

// processService handles health and BGP advertisement for one service.
// Returns true if the service is healthy (regardless of whether advertisement succeeded).
func (c *BGPServiceController) processService(service v1.Service) bool {
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	clusterIP := service.Spec.ClusterIP

	log.Printf("Processing service: %s (ClusterIP: %s)", serviceKey, clusterIP)

	isHealthy, err := c.performHealthCheck(service)
	if err != nil {
		log.Printf("Error performing health check for service %s: %v", serviceKey, err)
		return false
	}
	// Decision: Service ClusterIP is healthy?
	if !isHealthy {
		log.Printf("Service %s marked unhealthy — skipping", serviceKey)
		return false
	}

	log.Printf("Service %s is healthy", serviceKey)

	// Check if service ClusterIP is already advertised by FRR via BGP
	isAdvertised, err := c.isServiceAdvertisedByFRR(clusterIP)
	if err != nil {
		log.Printf("Error checking BGP advertisement status for service %s: %v", serviceKey, err)
		return true
	}
	// Decision: Service ClusterIP is already advertised?
	if isAdvertised {
		log.Printf("Service %s already advertised — nothing to do", serviceKey)
		c.mu.Lock()
		c.advertised[clusterIP] = true
		c.mu.Unlock()
		return true
	}

	// Advertise the Service ClusterIP using FRR. Only mark as advertised on
	// success; on failure leave it out of the map so the next loop retries.
	log.Printf("Advertising service %s (ClusterIP: %s) via BGP", serviceKey, clusterIP)
	if err := c.advertiseServiceViaBGP(clusterIP); err != nil {
		log.Printf("Error advertising service %s via BGP: %v", serviceKey, err)
		return true
	}
	c.mu.Lock()
	c.advertised[clusterIP] = true
	c.mu.Unlock()
	log.Printf("Successfully advertised service %s", serviceKey)
	return true
}

// performHealthCheck checks if service has at least one ready endpoint
func (c *BGPServiceController) performHealthCheck(service v1.Service) (bool, error) {
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)

	endpoints, err := c.client.CoreV1().Endpoints(service.Namespace).Get(c.ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get endpoints for service %s: %v", serviceKey, err)
	}

	readyEndpoints := 0
	for _, subset := range endpoints.Subsets {
		readyEndpoints += len(subset.Addresses)
	}

	isHealthy := readyEndpoints > 0
	log.Printf("Health check for service %s: %d ready endpoints, healthy: %t", serviceKey, readyEndpoints, isHealthy)

	return isHealthy, nil
}

// routePrefix returns the host route for a cluster IP, with a prefix length
// appropriate to its family: /32 for IPv4, /128 for IPv6.
func routePrefix(clusterIP string) string {
	if strings.Contains(clusterIP, ":") {
		return clusterIP + "/128"
	}
	return clusterIP + "/32"
}

// bgpAddressFamily returns the FRR address-family command for the IP family.
func bgpAddressFamily(clusterIP string) string {
	if strings.Contains(clusterIP, ":") {
		return "address-family ipv6 unicast"
	}
	return "address-family ipv4 unicast"
}

// bgpShowCommand builds the vtysh command to look up a route for the family.
func bgpShowCommand(clusterIP string) string {
	if strings.Contains(clusterIP, ":") {
		return "show bgp ipv6 unicast " + clusterIP
	}
	return "show ip bgp " + clusterIP
}

// loopbackInterface finds the loopback interface by its flag rather than a
// hardcoded name, so it works on Linux ("lo"), macOS ("lo0"), and BSDs
// (Finding 35).
func loopbackInterface() (net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			return iface, nil
		}
	}
	return net.Interface{}, fmt.Errorf("no loopback interface found")
}

// isServiceAdvertisedByFRR checks if the ClusterIP is locally assigned and advertised via BGP
func (c *BGPServiceController) isServiceAdvertisedByFRR(clusterIP string) (bool, error) {

	iface, err := loopbackInterface()
	if err != nil {
		return false, fmt.Errorf("failed to get loopback interface: %v", err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return false, fmt.Errorf("failed to get addresses on loopback: %v", err)
	}

	found := false
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		if ip.String() == clusterIP {
			found = true
			break
		}
	}

	if !found {
		log.Printf("ClusterIP %s is NOT on loopback interface", clusterIP)
		return false, nil
	}

	log.Printf("ClusterIP %s is on loopback interface", clusterIP)

	// Step 2: Check if BGP is advertising this IP and sourced locally.
	// FRR marks a locally-originated (network-announced) route with "*>";
	// the old substring match on "sourced"/"valid" was unreliable.
	output, err := c.run("vtysh", "-c", bgpShowCommand(clusterIP))
	if err != nil {
		return false, fmt.Errorf("failed to check BGP advertisement for %s: %v\nOutput: %s", clusterIP, err, output)
	}

	outStr := string(output)
	isLocal := strings.Contains(outStr, "*>")

	log.Printf("BGP advertisement check for %s: locally advertised = %v", clusterIP, isLocal)
	return isLocal, nil
}

// advertiseServiceViaBGP adds the loopback address and configures FRR.
// Returns an error if any FRR command fails; callers must NOT mark the route
// as advertised in that case (so the next loop can retry).
func (c *BGPServiceController) advertiseServiceViaBGP(clusterIP string) error {
	if !c.config.IsBGPEnabled() {
		log.Printf("BGP is disabled in configuration, skipping advertisement")
		return nil
	}

	route := routePrefix(clusterIP)
	asn := c.config.GetBGPASN()
	log.Printf("Advertising route %s via BGP ASN %d", route, asn)

	// Fail hard if the IP cannot be assigned: advertising a route for an IP
	// this node cannot receive would blackhole traffic (Finding 4).
	if output, err := c.run("ip", "addr", "add", route, "dev", "lo"); err != nil {
		return fmt.Errorf("failed to assign IP to loopback: %v\nOutput: %s", err, output)
	}

	args := []string{
		"-c", "configure terminal",
		"-c", fmt.Sprintf("router bgp %d", asn),
		"-c", bgpAddressFamily(clusterIP),
		"-c", fmt.Sprintf("network %s", route),
		"-c", "exit-address-family",
		"-c", "exit",
	}
	args = append(args, c.config.GetVtyshSocketArgs()...)
	output, err := c.run("vtysh", args...)
	if err != nil {
		return fmt.Errorf("failed to advertise route via BGP: %v\nOutput: %s", err, output)
	}
	logging.Debug("vtysh route advertisement successful: %s", output)

	// Persistence to frr.conf is handled once per loop by persistFRRConfig,
	// not on every advertise call (Finding 12).
	logging.Debug("Successfully advertised %s via BGP", route)
	return nil
}

// persistFRRConfig runs `write memory` so FRR state survives a restart.
// It is called once per control-loop iteration (on route-set change) rather
// than on every advertise/withdraw (Finding 12).
func (c *BGPServiceController) persistFRRConfig() {
	args := []string{"-c", "write memory"}
	args = append(args, c.config.GetVtyshSocketArgs()...)
	if output, err := c.run("vtysh", args...); err != nil {
		logging.Warn("failed to persist FRR config: %v\nOutput: %s", err, output)
	}
}

// withdrawServiceViaBGP removes the BGP route and loopback IP for a cluster IP.
// Best-effort: each command failure is logged, not returned, because the goal
// is to converge FRR state on shutdown/withdrawal even if one step fails.
func (c *BGPServiceController) withdrawServiceViaBGP(clusterIP string) {
	route := routePrefix(clusterIP)
	asn := c.config.GetBGPASN()

	args := []string{
		"-c", "configure terminal",
		"-c", fmt.Sprintf("router bgp %d", asn),
		"-c", bgpAddressFamily(clusterIP),
		"-c", fmt.Sprintf("no network %s", route),
		"-c", "exit-address-family",
		"-c", "exit",
	}
	args = append(args, c.config.GetVtyshSocketArgs()...)
	if output, err := c.run("vtysh", args...); err != nil {
		logging.Warn("failed to withdraw BGP route %s: %v\nOutput: %s", route, err, output)
	} else {
		logging.Debug("Withdrawn BGP route %s", route)
	}

	if output, err := c.run("ip", "addr", "del", route, "dev", "lo"); err != nil {
		logging.Warn("failed to remove IP %s from loopback: %v\nOutput: %s", route, err, output)
	} else {
		logging.Debug("Removed IP %s from loopback", route)
	}
}

// withdrawStaleRoutes withdraws BGP routes for IPs that are no longer in the healthy set
func (c *BGPServiceController) withdrawStaleRoutes(healthyIPs map[string]bool) {
	c.mu.Lock()
	var toWithdraw []string
	for ip := range c.advertised {
		if !healthyIPs[ip] {
			toWithdraw = append(toWithdraw, ip)
		}
	}
	for _, ip := range toWithdraw {
		delete(c.advertised, ip)
	}
	c.mu.Unlock()

	for _, ip := range toWithdraw {
		log.Printf("Withdrawing stale route for %s", ip)
		c.withdrawServiceViaBGP(ip)
	}
}

// withdrawAllRoutes withdraws all BGP routes this controller has advertised
func (c *BGPServiceController) withdrawAllRoutes() {
	c.mu.Lock()
	ips := make([]string, 0, len(c.advertised))
	for ip := range c.advertised {
		ips = append(ips, ip)
	}
	c.advertised = make(map[string]bool)
	c.mu.Unlock()

	for _, ip := range ips {
		log.Printf("Shutting down: withdrawing route for %s", ip)
		c.withdrawServiceViaBGP(ip)
	}
}

// testKubernetesAPI tests Kubernetes API access
func (c *BGPServiceController) testKubernetesAPI() error {
	_, err := c.client.CoreV1().Namespaces().List(c.ctx, metav1.ListOptions{Limit: 1})
	return err
}

// testFRRConnectivity tests FRR CLI availability
func (c *BGPServiceController) testFRRConnectivity() error {
	_, err := c.run("vtysh", "-c", "show version")
	return err
}

// sleep for configured loop interval
func (c *BGPServiceController) sleep() {
	select {
	case <-c.ctx.Done():
		return
	case <-time.After(time.Duration(c.config.GetLoopInterval()) * time.Second):
		return
	}
}
