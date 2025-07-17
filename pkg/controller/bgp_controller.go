package controller

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

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
	}, nil
}

// Start begins the main control loop
func (c *BGPServiceController) Start() error {
	log.Println("Starting BGP Service Controller...")

	if err := c.testKubernetesAPI(); err != nil {
		c.healthChecker.CheckKubernetesAPI(false, err.Error())
		return fmt.Errorf("kubernetes API connectivity test failed: %v", err)
	}
	c.healthChecker.CheckKubernetesAPI(true, "Connected")

	if err := c.testFRRConnectivity(); err != nil {
		c.healthChecker.CheckFRRStatus(false, err.Error())
		log.Printf("Warning: FRR connectivity test failed: %v", err)
	} else {
		c.healthChecker.CheckFRRStatus(true, "Connected")
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

	for _, service := range services {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.processService(service)
		}
	}

	duration := time.Since(start)
	log.Printf("Loop finished in %v. Sleeping for %d seconds...", duration, c.config.GetLoopInterval())
	c.sleep()
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

// processService handles health and BGP advertisement for one service
func (c *BGPServiceController) processService(service v1.Service) {
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	clusterIP := service.Spec.ClusterIP

	log.Printf("Processing service: %s (ClusterIP: %s)", serviceKey, clusterIP)

	isHealthy, err := c.performHealthCheck(service)
	if err != nil {
		log.Printf("Error performing health check for service %s: %v", serviceKey, err)
		return
	}
	if !isHealthy {
		log.Printf("Service %s marked unhealthy — skipping", serviceKey)
		return
	}

	log.Printf("Service %s is healthy", serviceKey)

	isAdvertised, err := c.isServiceAdvertisedByFRR(clusterIP)
	if err != nil {
		log.Printf("Error checking BGP advertisement status for service %s: %v", serviceKey, err)
		return
	}
	if isAdvertised {
		log.Printf("Service %s already advertised — nothing to do", serviceKey)
		return
	}

	log.Printf("Advertising service %s (ClusterIP: %s) via BGP", serviceKey, clusterIP)
	if err := c.advertiseServiceViaBGP(clusterIP); err != nil {
		log.Printf("Error advertising service %s via BGP: %v", serviceKey, err)
		return
	}
	log.Printf("Successfully advertised service %s", serviceKey)
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

// isServiceAdvertisedByFRR checks BGP table for given IP
func (c *BGPServiceController) isServiceAdvertisedByFRR(clusterIP string) (bool, error) {
	cmd := exec.Command("vtysh", "-c", "show ip route bgp")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute vtysh command: %v", err)
	}

	outputStr := string(output)
	isAdvertised := strings.Contains(outputStr, clusterIP)

	log.Printf("BGP route check for %s: advertised=%t", clusterIP, isAdvertised)
	return isAdvertised, nil
}

// advertiseServiceViaBGP adds loopback route and configures FRR
func (c *BGPServiceController) advertiseServiceViaBGP(clusterIP string) error {
	if !c.config.IsBGPEnabled() {
		log.Printf("BGP is disabled in configuration, skipping advertisement")
		return nil
	}

	route := fmt.Sprintf("%s/32", clusterIP)
	asn := c.config.GetBGPASN()
	log.Printf("Advertising route %s via BGP ASN %d", route, asn)

	assignCmd := exec.Command("ip", "addr", "add", route, "dev", "lo")
	if output, err := assignCmd.CombinedOutput(); err != nil {
		log.Printf("Warning: failed to assign IP to loopback: %v\nOutput: %s", err, output)
	}

	cmd := exec.Command(
		"vtysh",
		"-c", "configure terminal",
		"-c", fmt.Sprintf("router bgp %d", asn),
		"-c", "address-family ipv4 unicast",
		"-c", fmt.Sprintf("network %s", route),
		"-c", "exit-address-family",
		"-c", "exit",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to advertise route via BGP: %v\nOutput: %s", err, output)
	}
	log.Printf("vtysh route advertisement successful: %s", output)

	writeCmd := exec.Command("vtysh", "-c", "write memory")
	if output, err := writeCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to persist config to /etc/frr/frr.conf: %v\nOutput: %s", err, output)
	}

	log.Printf("Successfully advertised %s via BGP and saved config to /etc/frr/frr.conf", route)
	return nil
}

// testKubernetesAPI tests Kubernetes API access
func (c *BGPServiceController) testKubernetesAPI() error {
	_, err := c.client.CoreV1().Namespaces().List(c.ctx, metav1.ListOptions{Limit: 1})
	return err
}

// testFRRConnectivity tests FRR CLI availability
func (c *BGPServiceController) testFRRConnectivity() error {
	cmd := exec.Command("vtysh", "-c", "show version")
	_, err := cmd.Output()
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
