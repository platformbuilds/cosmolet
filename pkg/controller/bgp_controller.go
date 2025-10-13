// pkg/controller/bgp_controller.go
package controller

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
	"strings"
	"net"

	"cosmolet/pkg/config"
	"cosmolet/pkg/health"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// BGPServiceController manages BGP advertisements for Kubernetes services
type BGPServiceController struct {
	client        kubernetes.Interface
	config        *config.Config
	ctx           context.Context
	healthChecker *health.Checker
	excludeMap    map[string]struct{} // IPs to exclude from removal
	nodeName      string
}

// NewBGPServiceController creates a new BGP service controller
func NewBGPServiceController(cfg *config.Config, ctx context.Context) (*BGPServiceController, error) {
	// Use kubeconfig from kubeconfig.go
	kubeConfig, err := GetKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		nodeName, _ = os.Hostname()
	}

	controller := &BGPServiceController{
		client:        clientset,
		config:        cfg,
		ctx:           ctx,
		healthChecker: health.NewChecker(),
		nodeName:      nodeName,
	}

	if err := controller.populateNodeIPs(); err != nil {
		log.Printf("Warning: could not populate node IPs for exclusion: %v", err)
	}

	return controller, nil
}

// populateNodeIPs fetches all node IPs to exclude from loopback removal
// populateNodeIPs fetches all node IPs to exclude from loopback removal
func (c *BGPServiceController) populateNodeIPs() error {
	nodes, err := c.client.CoreV1().Nodes().List(c.ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list Kubernetes nodes: %v", err)
	}

	c.excludeMap = make(map[string]struct{})
	for _, node := range nodes.Items {
		if node.Name != c.nodeName {
			continue
		}
		for _, addr := range node.Status.Addresses {
			if addr.Type == v1.NodeInternalIP || addr.Type == v1.NodeExternalIP {
				c.excludeMap[addr.Address] = struct{}{}
			}
		}
		break
	}

	// Always protect localhost
	c.excludeMap["127.0.0.1"] = struct{}{}
	c.excludeMap["::1"] = struct{}{}

	// Append exclusions from config.yaml
	if c.config != nil && len(c.config.BGP.ExcludedIPs) > 0 {
		for _, ip := range c.config.BGP.ExcludedIPs {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				c.excludeMap[ip] = struct{}{}
			}
		}
	}

	// Log the protected IPs
	var protected []string
	for ip := range c.excludeMap {
		protected = append(protected, ip)
	}
	log.Printf("Protected IPs (won’t be deleted): %v", protected)

	return nil
}

// Start begins the main control loop
func (c *BGPServiceController) Start() error {
	log.Println("Starting BGP Service Controller...")

	if err := c.testKubernetesAPI(); err != nil {
		c.healthChecker.CheckKubernetesAPI(false, err.Error())
		return fmt.Errorf("kubernetes API connectivity test failed: %v", err)
	}
	c.healthChecker.CheckKubernetesAPI(true, "Connected")

	if c.config.BGPMode == "dynamic" {
		if err := c.testFRRConnectivity(); err != nil {
			c.healthChecker.CheckFRRStatus(false, err.Error())
			log.Printf("Warning: FRR connectivity test failed: %v", err)
		} else {
			c.healthChecker.CheckFRRStatus(true, "Connected")
		}
	}

	for {
		select {
		case <-c.ctx.Done():
			log.Println("Received shutdown signal, stopping controller")
			return nil
		default:
			c.runControlLoop()
		}
	}
}

// runControlLoop executes one iteration of the control loop
func (c *BGPServiceController) runControlLoop() {
	start := time.Now()
	log.Println("=== Starting new loop iteration ===")
	c.healthChecker.UpdateLastLoop()

	services, err := c.fetchServicesFromNamespaces()
	if err != nil {
		log.Printf("Error fetching services: %v", err)
		c.healthChecker.CheckServiceDiscovery(0, time.Since(start))
		c.sleep()
		return
	}

	log.Printf("Found %d services to process", len(services))
	c.healthChecker.CheckServiceDiscovery(len(services), time.Since(start))

	activeIPs := make(map[string]struct{})

	for _, svc := range services {
		select {
		case <-c.ctx.Done():
			return
		default:
			podIPs, _, err := c.getNodeLocalPodIPsAndHealthURLs(svc)
			serviceIP := svc.Spec.ClusterIP

			if err != nil {
				log.Printf("Error getting pods for %s/%s: %v", svc.Namespace, svc.Name, err)
				continue
			}

			if len(podIPs) == 0 {
				// No pods on this node: withdraw BGP advertisement if dynamic
				if c.config.GetBGPMode() == "dynamic" {
					if err := c.withdrawServiceFromBGP(serviceIP); err != nil {
						log.Printf("Failed to withdraw service IP %s from BGP: %v", serviceIP, err)
					} else {
						log.Printf("Withdrawn service IP %s from BGP", serviceIP)
					}
				} else {
					log.Printf("Service %s/%s has no pods on this node, skipping FRR withdrawal (connected mode)", svc.Namespace, svc.Name)
				}
				continue
			}

			// Call your existing processService logic
			c.processService(svc, activeIPs)
		}
	}

	// Remove stale loopback IPs (except excluded)
	log.Printf("Remove stale loopback IPs (except excluded) %v ...", activeIPs)
	c.cleanupLoopbackIPs(activeIPs)

	duration := time.Since(start)
	log.Printf("Loop finished in %v. Sleeping for %d seconds...", duration, c.config.LoopIntervalSeconds)
	c.sleep()
}

// fetchServicesFromNamespaces fetches all services from configured namespaces
func (c *BGPServiceController) fetchServicesFromNamespaces() ([]v1.Service, error) {
	var allServices []v1.Service
	for _, ns := range c.config.Services.Namespaces {
		svcs, err := c.client.CoreV1().Services(ns).List(c.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list services in %s failed: %v", ns, err)
		}
		for _, svc := range svcs.Items {
			if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
				allServices = append(allServices, svc)
			}
		}
	}
	return allServices, nil
}

// processService handles one service (health check + loopback + optional FRR)
func (c *BGPServiceController) processService(svc v1.Service, activeIPs map[string]struct{}) {
	clusterIP := svc.Spec.ClusterIP
	serviceKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)

	podIPs, healthURLs, err := c.getNodeLocalPodIPsAndHealthURLs(svc)
	if err != nil {
		log.Printf("Error listing pods for service %s: %v", serviceKey, err)
		return
	}


	// If no pods on this node, remove from FRR if dynamic
	if len(podIPs) == 0 {
		log.Printf("Service %s has no pods on this node", serviceKey)
		if c.config.BGPMode == "dynamic" {
			if err := c.withdrawServiceFromBGP(clusterIP); err != nil {
				log.Printf("Failed to withdraw %s from BGP: %v", clusterIP, err)
			} else {
				log.Printf("Withdrew %s from BGP", clusterIP)
			}
		}
		return
	}

	isHealthy := false

	for _, url := range healthURLs {
		if c.checkHTTPHealth(url) {
			isHealthy = true
			break
		}
	}

	if !isHealthy {
		log.Printf("Service %s unhealthy on this node, skipping. Pod IPs: %v, Health URL(s): %v", serviceKey, podIPs, healthURLs)
		return
	}

	log.Printf("Service %s healthy. Pod IPs: %v, Health URL(s): %v", serviceKey, podIPs, healthURLs)

	// Step 1: Add service ClusterIP to loopback
	if err := c.addIPToLoopback(clusterIP); err != nil {
		log.Printf("Failed to add service IP %s to loopback: %v", clusterIP, err)
	} else {
		log.Printf("Added service IP %s to loopback", clusterIP)
	}
	activeIPs[clusterIP] = struct{}{}

	// Step 2: Advertise in FRR if dynamic
	if c.config.BGPMode == "dynamic" {
		isAdvertised, err := c.isServiceAdvertisedByFRR(clusterIP)
		if err != nil {
			log.Printf("Error checking BGP advertisement for %s: %v", clusterIP, err)
		}
		if !isAdvertised {
			if err := c.advertiseServiceViaBGP(clusterIP); err != nil {
				log.Printf("Failed to advertise %s via FRR: %v", clusterIP, err)
			} else {
				log.Printf("Advertised %s via FRR", clusterIP)
			}
		}
	} else {
		log.Printf("Connected mode: %s added to loopback only, skipping FRR", clusterIP)
	}
}

// getNodeLocalPodIPsAndHealthURLs returns pod IPs on this node and their liveness probe URLs
func (c *BGPServiceController) getNodeLocalPodIPsAndHealthURLs(svc v1.Service) ([]string, []string, error) {
	// Convert selector map to string
	labelSelector := ""
	for k, v := range svc.Spec.Selector {
		if labelSelector != "" {
			labelSelector += ","
		}
		labelSelector += fmt.Sprintf("%s=%s", k, v)
	}

	podList, err := c.client.CoreV1().Pods(svc.Namespace).List(c.ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, nil, err
	}

	var podIPs []string
	var healthURLs []string
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != c.nodeName {
			continue
		}
		podIPs = append(podIPs, pod.Status.PodIP)
		for _, ctn := range pod.Spec.Containers {
			if ctn.LivenessProbe != nil && ctn.LivenessProbe.HTTPGet != nil {
				healthURLs = append(healthURLs, fmt.Sprintf("http://%s:%d%s", pod.Status.PodIP, ctn.LivenessProbe.HTTPGet.Port.IntVal, ctn.LivenessProbe.HTTPGet.Path))
			}
		}
	}

	return podIPs, healthURLs, nil
}

// checkHTTPHealth performs a simple HTTP GET and returns true if 200 OK
func (c *BGPServiceController) checkHTTPHealth(url string) bool {
	// Placeholder: implement actual HTTP GET logic if needed
	return true
}

func (c *BGPServiceController) addIPToLoopback(ip string) error {
	// Check if IP is already present on lo
	checkCmd := exec.Command("ip", "-o", "addr", "show", "dev", "lo")
	out, err := checkCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check loopback IPs: %v", err)
	}

	if strings.Contains(string(out), ip) {
		log.Printf("IP %s already present on loopback, skipping", ip)
		return nil
	}

	// Add the IP
	cmd := exec.Command("ip", "addr", "add", fmt.Sprintf("%s/32", ip), "dev", "lo")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add IP %s to loopback: %v", ip, err)
	}

	log.Printf("Added IP %s to loopback", ip)
	return nil
}

// cleanupLoopbackIPs removes stale IPs not in active set or exclude map
func (c *BGPServiceController) cleanupLoopbackIPs(activeIPs map[string]struct{}) {
	// List current IPs on loopback
	out, err := exec.Command("ip", "-o", "addr", "show", "dev", "lo").Output()
	if err != nil {
		log.Printf("Error listing loopback IPs: %v", err)
		return
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ipWithMask := fields[3]
		ip, _, err := net.ParseCIDR(ipWithMask)
		if err != nil || ip == nil {
			continue
		}
		ipStr := ip.String()

		// Only remove if not in active set AND not excluded
		if _, keep := activeIPs[ipStr]; !keep {
			if _, exclude := c.excludeMap[ipStr]; !exclude {
				log.Printf("Removing stale IP %s from loopback", ipStr)
				_ = exec.Command("ip", "addr", "del", fmt.Sprintf("%s/32", ipStr), "dev", "lo").Run()
			}
		}
	}
}

// sleep pauses for loop interval
func (c *BGPServiceController) sleep() {
	time.Sleep(time.Duration(c.config.LoopIntervalSeconds) * time.Second)
}

// Placeholder FRR methods
func (c *BGPServiceController) isServiceAdvertisedByFRR(ip string) (bool, error) { return false, nil }

func (c *BGPServiceController) advertiseServiceViaBGP(ip string) error {
    cmdStr := fmt.Sprintf(
        `configure terminal
	router bgp %d
	network %s/32
	end
	write memory`,
        c.config.BGP.ASN, ip,
    )

    cmd := exec.Command("vtysh", "-c", cmdStr)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("failed to advertise %s: %v (%s)", ip, err, string(out))
    }
    log.Printf("Advertised %s via FRR: %s", ip, string(out))
    return nil
}

func (c *BGPServiceController) withdrawServiceFromBGP(ip string) error {
    cmdStr := fmt.Sprintf(
        `configure terminal
	router bgp %d
	no network %s/32
	end
	write memory`,
        c.config.BGP.ASN, ip,
    )

    cmd := exec.Command("vtysh", "-c", cmdStr)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("failed to withdraw %s: %v (%s)", ip, err, string(out))
    }
    log.Printf("Withdrawn %s from FRR: %s", ip, string(out))
    return nil
}


// Placeholder tests
func (c *BGPServiceController) testKubernetesAPI() error { return nil }
func (c *BGPServiceController) testFRRConnectivity() error { return nil }

