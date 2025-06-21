package controller

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// Controller manages BGP route announcements for Kubernetes services
type Controller struct {
	client             kubernetes.Interface
	logger             logr.Logger
	nodeName           string
	leaderElection     bool
	isLeader           bool
	
	informerFactory    informers.SharedInformerFactory
	serviceInformer    cache.SharedIndexInformer
	endpointInformer   cache.SharedIndexInformer
	
	advertisedRoutes   map[string]*RouteInfo
	mutex              sync.RWMutex
	
	workqueue          chan WorkItem
	stopCh             chan struct{}
}

// RouteInfo contains information about an advertised route
type RouteInfo struct {
	ServiceKey      string
	CIDR            string
	NextHop         string
	ServiceIP       string
	LastUpdated     time.Time
	HealthStatus    string
}

// WorkItem represents work to be processed
type WorkItem struct {
	Type       WorkItemType
	ServiceKey string
	Service    *corev1.Service
	Endpoints  *corev1.Endpoints
}

// WorkItemType represents the type of work item
type WorkItemType string

const (
	WorkItemServiceAdd    WorkItemType = "service-add"
	WorkItemServiceUpdate WorkItemType = "service-update"
	WorkItemServiceDelete WorkItemType = "service-delete"
	WorkItemEndpointUpdate WorkItemType = "endpoint-update"
)

// New creates a new controller
func New(client kubernetes.Interface, logger logr.Logger, nodeName string) (*Controller, error) {
	if client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}

	// Create informer factory
	informerFactory := informers.NewSharedInformerFactory(client, 30*time.Second)

	controller := &Controller{
		client:             client,
		logger:             logger,
		nodeName:           nodeName,
		leaderElection:     true,
		informerFactory:    informerFactory,
		serviceInformer:    informerFactory.Core().V1().Services().Informer(),
		endpointInformer:   informerFactory.Core().V1().Endpoints().Informer(),
		advertisedRoutes:   make(map[string]*RouteInfo),
		workqueue:          make(chan WorkItem, 100),
		stopCh:             make(chan struct{}),
	}

	// Setup informer event handlers
	controller.setupEventHandlers()

	return controller, nil
}

// Run starts the controller
func (c *Controller) Run(ctx context.Context) error {
	c.logger.Info("Starting controller", "node", c.nodeName, "leader_election", c.leaderElection)

	// Start informers
	c.informerFactory.Start(ctx.Done())

	// Wait for cache sync
	if !cache.WaitForCacheSync(ctx.Done(), 
		c.serviceInformer.HasSynced,
		c.endpointInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer caches")
	}

	c.logger.Info("Informer caches synced")

	return c.runController(ctx)
}

// runController runs the main controller loop
func (c *Controller) runController(ctx context.Context) error {
	c.logger.Info("Starting controller workers")

	// Start worker goroutines
	for i := 0; i < 3; i++ {
		go c.worker(ctx)
	}

	<-ctx.Done()
	return nil
}

// worker processes work items from the queue
func (c *Controller) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-c.workqueue:
			c.processWorkItem(ctx, item)
		}
	}
}

// processWorkItem processes a work item
func (c *Controller) processWorkItem(ctx context.Context, item WorkItem) {
	c.logger.V(1).Info("Processing work item", "type", item.Type, "key", item.ServiceKey)

	switch item.Type {
	case WorkItemServiceAdd, WorkItemServiceUpdate:
		c.handleServiceUpdate(ctx, item.Service, item.Endpoints)
	case WorkItemServiceDelete:
		c.handleServiceDelete(ctx, item.ServiceKey)
	case WorkItemEndpointUpdate:
		c.handleEndpointUpdate(ctx, item.ServiceKey, item.Endpoints)
	}
}

// handleServiceUpdate handles service add/update events
func (c *Controller) handleServiceUpdate(ctx context.Context, service *corev1.Service, endpoints *corev1.Endpoints) {
	if service == nil {
		return
	}

	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)

	// Only handle LoadBalancer services with external IPs
	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return
	}

	// Get endpoints if not provided
	if endpoints == nil {
		var err error
		endpoints, err = c.client.CoreV1().Endpoints(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if err != nil {
			c.logger.Error(err, "Failed to get endpoints", "service", serviceKey)
			return
		}
	}

	// Check if service is healthy
	if !c.isServiceHealthy(endpoints) {
		c.logger.V(1).Info("Service is not healthy, withdrawing routes", "service", serviceKey)
		c.withdrawServiceRoutes(ctx, serviceKey)
		return
	}

	// Announce routes for external IPs
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			c.announceServiceRoute(ctx, serviceKey, ingress.IP, service, endpoints)
		}
	}
}

// handleServiceDelete handles service delete events
func (c *Controller) handleServiceDelete(ctx context.Context, serviceKey string) {
	c.withdrawServiceRoutes(ctx, serviceKey)
}

// handleEndpointUpdate handles endpoint update events
func (c *Controller) handleEndpointUpdate(ctx context.Context, serviceKey string, endpoints *corev1.Endpoints) {
	// Get the service
	namespace, name, err := cache.SplitMetaNamespaceKey(serviceKey)
	if err != nil {
		c.logger.Error(err, "Failed to split service key", "key", serviceKey)
		return
	}

	service, err := c.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		c.logger.Error(err, "Failed to get service", "key", serviceKey)
		return
	}

	c.handleServiceUpdate(ctx, service, endpoints)
}

// announceServiceRoute announces a BGP route for a service
func (c *Controller) announceServiceRoute(ctx context.Context, serviceKey, externalIP string, service *corev1.Service, endpoints *corev1.Endpoints) {
	cidr := externalIP + "/32"
	nextHop := c.getNodeIP()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if route already announced
	if routeInfo, exists := c.advertisedRoutes[serviceKey+":"+externalIP]; exists {
		if routeInfo.NextHop == nextHop {
			routeInfo.LastUpdated = time.Now()
			routeInfo.HealthStatus = "healthy"
			return // Already announced with same next hop
		}
	}

	// Record route info
	c.advertisedRoutes[serviceKey+":"+externalIP] = &RouteInfo{
		ServiceKey:   serviceKey,
		CIDR:         cidr,
		NextHop:      nextHop,
		ServiceIP:    externalIP,
		LastUpdated:  time.Now(),
		HealthStatus: "healthy",
	}

	c.logger.Info("Announced BGP route", "cidr", cidr, "service", serviceKey, "next_hop", nextHop)
}

// withdrawServiceRoutes withdraws all BGP routes for a service
func (c *Controller) withdrawServiceRoutes(ctx context.Context, serviceKey string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var routesToRemove []string

	for key, routeInfo := range c.advertisedRoutes {
		if routeInfo.ServiceKey == serviceKey {
			c.logger.Info("Withdrew BGP route", "cidr", routeInfo.CIDR, "service", serviceKey)
			routesToRemove = append(routesToRemove, key)
		}
	}

	// Remove from map
	for _, key := range routesToRemove {
		delete(c.advertisedRoutes, key)
	}
}

// isServiceHealthy checks if a service is healthy based on endpoints
func (c *Controller) isServiceHealthy(endpoints *corev1.Endpoints) bool {
	if endpoints == nil {
		return false
	}

	readyAddresses := 0
	for _, subset := range endpoints.Subsets {
		readyAddresses += len(subset.Addresses)
	}

	return readyAddresses > 0
}

// getNodeIP returns the node's IP address
func (c *Controller) getNodeIP() string {
	nodeIP := os.Getenv("NODE_IP")
	if nodeIP == "" {
		return "127.0.0.1" // Fallback
	}
	return nodeIP
}

// setupEventHandlers sets up informer event handlers
func (c *Controller) setupEventHandlers() {
	c.serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onServiceAdd,
		UpdateFunc: c.onServiceUpdate,
		DeleteFunc: c.onServiceDelete,
	})

	c.endpointInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onEndpointAdd,
		UpdateFunc: c.onEndpointUpdate,
		DeleteFunc: c.onEndpointDelete,
	})
}

// Event handlers
func (c *Controller) onServiceAdd(obj interface{}) {
	service := obj.(*corev1.Service)
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	
	c.enqueueWorkItem(WorkItem{
		Type:       WorkItemServiceAdd,
		ServiceKey: serviceKey,
		Service:    service,
	})
}

func (c *Controller) onServiceUpdate(oldObj, newObj interface{}) {
	service := newObj.(*corev1.Service)
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	
	c.enqueueWorkItem(WorkItem{
		Type:       WorkItemServiceUpdate,
		ServiceKey: serviceKey,
		Service:    service,
	})
}

func (c *Controller) onServiceDelete(obj interface{}) {
	service := obj.(*corev1.Service)
	serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
	
	c.enqueueWorkItem(WorkItem{
		Type:       WorkItemServiceDelete,
		ServiceKey: serviceKey,
	})
}

func (c *Controller) onEndpointAdd(obj interface{}) {
	endpoints := obj.(*corev1.Endpoints)
	serviceKey := fmt.Sprintf("%s/%s", endpoints.Namespace, endpoints.Name)
	
	c.enqueueWorkItem(WorkItem{
		Type:       WorkItemEndpointUpdate,
		ServiceKey: serviceKey,
		Endpoints:  endpoints,
	})
}

func (c *Controller) onEndpointUpdate(oldObj, newObj interface{}) {
	endpoints := newObj.(*corev1.Endpoints)
	serviceKey := fmt.Sprintf("%s/%s", endpoints.Namespace, endpoints.Name)
	
	c.enqueueWorkItem(WorkItem{
		Type:       WorkItemEndpointUpdate,
		ServiceKey: serviceKey,
		Endpoints:  endpoints,
	})
}

func (c *Controller) onEndpointDelete(obj interface{}) {
	endpoints := obj.(*corev1.Endpoints)
	serviceKey := fmt.Sprintf("%s/%s", endpoints.Namespace, endpoints.Name)
	
	c.enqueueWorkItem(WorkItem{
		Type:       WorkItemEndpointUpdate,
		ServiceKey: serviceKey,
		Endpoints:  endpoints,
	})
}

// enqueueWorkItem adds a work item to the queue
func (c *Controller) enqueueWorkItem(item WorkItem) {
	select {
	case c.workqueue <- item:
	default:
		c.logger.V(1).Info("Work queue full, dropping item", "type", item.Type, "key", item.ServiceKey)
	}
}
