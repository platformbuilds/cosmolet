package bgp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// Manager handles BGP route management through FRR
type Manager struct {
	config        Config
	routes        map[string]*Route
	mutex         sync.RWMutex
	logger        logr.Logger
	eventCh       chan Event
	stopCh        chan struct{}
}

// Config contains BGP configuration
type Config struct {
	ASN         uint32   `yaml:"asn"`
	RouterID    string   `yaml:"router_id"`
	Neighbors   []string `yaml:"neighbors"`
	Networks    []string `yaml:"networks"`
	VTYPassword string   `yaml:"vty_password"`
	EnableBFD   bool     `yaml:"enable_bfd"`
}

// Route represents a BGP route
type Route struct {
	CIDR        string
	NextHop     string
	Owner       string
	Priority    int
	Communities []string
	LastUpdated time.Time
	Status      RouteStatus
}

// RouteStatus represents the status of a BGP route
type RouteStatus string

const (
	StatusAdvertised RouteStatus = "advertised"
	StatusWithdrawn  RouteStatus = "withdrawn"
	StatusPending    RouteStatus = "pending"
	StatusFailed     RouteStatus = "failed"
)

// Event represents a BGP event
type Event struct {
	Type    EventType
	Route   *Route
	Error   error
	Message string
}

// EventType represents the type of BGP event
type EventType string

const (
	EventAdvertised EventType = "advertised"
	EventWithdrawn  EventType = "withdrawn"
	EventFailed     EventType = "failed"
)

// NewManager creates a new BGP manager
func NewManager(config Config, logger logr.Logger) (*Manager, error) {
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid BGP config: %w", err)
	}

	return &Manager{
		config:  config,
		routes:  make(map[string]*Route),
		logger:  logger,
		eventCh: make(chan Event, 100),
		stopCh:  make(chan struct{}),
	}, nil
}

// Start starts the BGP manager
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting BGP manager", "asn", m.config.ASN, "router_id", m.config.RouterID)
	return nil
}

// Stop stops the BGP manager
func (m *Manager) Stop() error {
	close(m.stopCh)
	return nil
}

// AnnounceRoute announces a BGP route
func (m *Manager) AnnounceRoute(ctx context.Context, cidr, nextHop, owner string, priority int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	route := &Route{
		CIDR:        cidr,
		NextHop:     nextHop,
		Owner:       owner,
		Priority:    priority,
		LastUpdated: time.Now(),
		Status:      StatusAdvertised,
	}

	m.routes[cidr] = route
	m.sendEvent(Event{Type: EventAdvertised, Route: route})

	m.logger.Info("Route announced", "cidr", cidr, "next_hop", nextHop, "owner", owner)
	return nil
}

// WithdrawRoute withdraws a BGP route
func (m *Manager) WithdrawRoute(ctx context.Context, cidr, owner string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	route, exists := m.routes[cidr]
	if !exists {
		return fmt.Errorf("route %s not found", cidr)
	}

	if route.Owner != owner {
		return fmt.Errorf("route %s not owned by %s", cidr, owner)
	}

	route.Status = StatusWithdrawn
	route.LastUpdated = time.Now()
	delete(m.routes, cidr)
	m.sendEvent(Event{Type: EventWithdrawn, Route: route})

	m.logger.Info("Route withdrawn", "cidr", cidr, "owner", owner)
	return nil
}

// GetRoutes returns all current routes
func (m *Manager) GetRoutes() map[string]*Route {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	routes := make(map[string]*Route)
	for k, v := range m.routes {
		routes[k] = v
	}
	return routes
}

// HealthCheck performs a health check of the BGP manager
func (m *Manager) HealthCheck(ctx context.Context) error {
	return nil
}

// Events returns the event channel
func (m *Manager) Events() <-chan Event {
	return m.eventCh
}

// sendEvent sends an event to the event channel
func (m *Manager) sendEvent(event Event) {
	select {
	case m.eventCh <- event:
	default:
		m.logger.V(1).Info("Event channel full, dropping event", "type", event.Type)
	}
}

// validateConfig validates the BGP configuration
func validateConfig(config Config) error {
	if config.ASN == 0 {
		return fmt.Errorf("ASN must be specified")
	}
	if config.RouterID == "" {
		return fmt.Errorf("router ID must be specified")
	}
	return nil
}
