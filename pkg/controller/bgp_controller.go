// pkg/controller/bgp_controller.go
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
	//"k8s.io/client-go/rest"
)

// BGPServiceController manages BGP advertisements for Kubernetes services
type BGPServiceController struct {
	client        kubernetes.Interface
	config        *config.Config
	ctx           context.Context
	healthChecker *health.Checker
}

// ServiceInfo contains information about a discovered service
type ServiceInfo struct {
	Name      string
	Namespace string
	ClusterIP string
	Healthy   bool
}

// NewBGPServiceController creates a new BGP service controller
func NewBGPServiceController(cfg *config.Config, ctx context.Context) (*BGPServiceController, error) {
	// Create in-cluster config (since we're running as a DaemonSet)
	// kubeConfig, err := rest.InClusterConfig()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create in-cluster config: %v", err)
	// }
	
	// Get Kubernetes config (in-cluster or local)
	kubeConfig, err := GetKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Create Kubernetes client
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

	// Test Kubernetes API connectivity
	if err := c.testKubernetesAPI(); err != nil {
		c.healthChecker.CheckKubernetesAPI(false, err.Error())
		return fmt.Errorf("kubernetes API connectivity test failed: %v", err)
	}
	c.healthChecker.CheckKubernetesAPI(true, "Connected")

	// Test FRR connectivity
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

	// Update health checker
	c.healthChecker.UpdateLastLoop()

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

	// Step 2: Process each service
	for _, service := range services {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.processService(service)
		}
	}

	// Step 4: Sleep and restart loop
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

		// Filter for services with ClusterIP
		for _, service := range services.Items {
			if service.Spec.ClusterIP != "" && service.Spec.ClusterIP != "None" {
				allServices = append(allServices, service)
			}
		}
	}

	return allServices, nil
}

// processService processes a single service through the health check and BGP advertisement flow
func (c *BGPServiceController) processService(service v1.Service) {
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	log.Printf("Processing service: %s (ClusterIP: %s)", serviceKey, service.Spec.ClusterIP)

	// Step 2: Perform health check
	isHealthy, err := c.performHealthCheck(service)
	if err != nil {
		log.Printf("Error performing health check for service %s: %v", serviceKey, err)
		return
	}

	// Step 3: Decision - Service ClusterIP is healthy?
	if !isHealthy {
		log.Printf("LOG and Stop: Service %s ClusterIP is unhealthy", serviceKey)
		return
	}

	log.Printf("Service %s is healthy", serviceKey)

	// Step 4: Check if service ClusterIP is already advertised by FRR via BGP
	isAdvertised, err := c.isServiceAdvertisedByFRR(service.Spec.ClusterIP)
	if err != nil {
		log.Printf("Error checking BGP advertisement status for service %s: %v", serviceKey, err)
		return
	}

	// Step 5: Decision - Service ClusterIP is already advertised?
	if isAdvertised {
		log.Printf("LOG and Stop: Service %s ClusterIP is healthy and already advertised", serviceKey)
		return
	}

	// Step 6: Advertise the Service ClusterIP using FRR
	log.Printf("Service %s ClusterIP is not advertised. Advertising via FRR...", serviceKey)
	err = c.advertiseServiceViaBGP(service.Spec.ClusterIP)
	if err != nil {
		log.Printf("Error advertising service %s via BGP: %v", serviceKey, err)
		return
	}

	log.Printf("Successfully advertised service %s ClusterIP via BGP", serviceKey)
}

// performHealthCheck checks the health of a service by examining its endpoints
func (c *BGPServiceController) performHealthCheck(service v1.Service) (bool, error) {
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)

	// Get endpoints for the service
	endpoints, err := c.client.CoreV1().Endpoints(service.Namespace).Get(c.ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get endpoints for service %s: %v", serviceKey, err)
	}

	// Check if there are any ready endpoints
	readyEndpoints := 0
	for _, subset := range endpoints.Subsets {
		readyEndpoints += len(subset.Addresses)
	}

	isHealthy := readyEndpoints > 0
	log.Printf("Health check for service %s: %d ready endpoints, healthy: %t", serviceKey, readyEndpoints, isHealthy)

	return isHealthy, nil
}

// isServiceAdvertisedByFRR checks if a ClusterIP is already advertised by FRR via BGP
func (c *BGPServiceController) isServiceAdvertisedByFRR(clusterIP string) (bool, error) {
	// Use vtysh to check if the route is already advertised
	cmd := exec.Command("vtysh", "-c", "show ip route bgp")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute vtysh command: %v", err)
	}

	// Check if the ClusterIP appears in BGP routes
	outputStr := string(output)
	isAdvertised := strings.Contains(outputStr, clusterIP)

	log.Printf("BGP route check for %s: advertised=%t", clusterIP, isAdvertised)
	return isAdvertised, nil
}

// advertiseServiceViaBGP advertises a ClusterIP via FRR BGP
func (c *BGPServiceController) advertiseServiceViaBGP(clusterIP string) error {
	if !c.config.IsBGPEnabled() {
		log.Printf("BGP is disabled in configuration, skipping advertisement")
		return nil
	}

	// Create BGP route advertisement commands
	route := fmt.Sprintf("%s/32", clusterIP)

	// Construct vtysh commands to advertise the route
	commands := []string{
		"configure terminal",
		fmt.Sprintf("ip route %s Null0", route),
		"router bgp",
		"redistribute static",
		"exit",
		"exit",
	}

	// Execute vtysh commands
	for _, command := range commands {
		cmd := exec.Command("vtysh", "-c", command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to execute vtysh command '%s': %v, output: %s", command, err, string(output))
		}
		log.Printf("Executed FRR command: %s", command)
	}

	log.Printf("Successfully advertised route %s via FRR BGP", route)
	return nil
}

// testKubernetesAPI tests connectivity to the Kubernetes API
func (c *BGPServiceController) testKubernetesAPI() error {
	_, err := c.client.CoreV1().Namespaces().List(c.ctx, metav1.ListOptions{Limit: 1})
	return err
}

// testFRRConnectivity tests connectivity to FRR
func (c *BGPServiceController) testFRRConnectivity() error {
	cmd := exec.Command("vtysh", "-c", "show version")
	_, err := cmd.Output()
	return err
}

// sleep pauses execution for the configured interval
func (c *BGPServiceController) sleep() {
	select {
	case <-c.ctx.Done():
		return
	case <-time.After(time.Duration(c.config.GetLoopInterval()) * time.Second):
		return
	}
}
