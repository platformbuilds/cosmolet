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

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
	// Create in-cluster config
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %v", err)
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

// Start begins the main control loop (implements the flow chart)
func (c *BGPServiceController) Start() error {
	log.Println("Starting BGP Service Controller...")

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

	// Step 1: Fetch all running services in configured namespaces
	services, err := c.fetchServicesFromNamespaces()
	if err != nil {
		log.Printf("Error fetching services: %v", err)
		c.sleep()
		return
	}

	log.Printf("Found %d services to process", len(services))

	// Step 2: Process each service through the flow chart logic
	for _, service := range services {
		c.processService(service)
	}

	// Step 4: Sleep and restart loop
	duration := time.Since(start)
	log.Printf("Loop finished in %v. Sleeping for %d seconds...", duration, c.config.GetLoopInterval())
	c.sleep()
}

// Implement other methods (fetchServicesFromNamespaces, processService, etc.)
// ... (full implementation in the artifacts above)

func (c *BGPServiceController) fetchServicesFromNamespaces() ([]v1.Service, error) {
	// Implementation here
	return nil, nil
}

func (c *BGPServiceController) processService(service v1.Service) {
	// Implementation here
}

func (c *BGPServiceController) sleep() {
	time.Sleep(time.Duration(c.config.GetLoopInterval()) * time.Second)
}
