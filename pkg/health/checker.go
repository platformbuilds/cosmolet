// pkg/health/checker.go
package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Checker manages the health state of the application
type Checker struct {
	mu       sync.RWMutex
	ready    bool
	live     bool
	started  time.Time
	lastLoop time.Time
	checks   map[string]HealthCheck
}

// HealthCheck represents a single health check
type HealthCheck struct {
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	Message  string    `json:"message,omitempty"`
	LastRun  time.Time `json:"last_run"`
	Duration string    `json:"duration,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Uptime    string                 `json:"uptime"`
	Checks    map[string]HealthCheck `json:"checks,omitempty"`
}

// NewChecker creates a new health checker
func NewChecker() *Checker {
	return &Checker{
		ready:   false,
		live:    true,
		started: time.Now(),
		checks:  make(map[string]HealthCheck),
	}
}

// SetReady sets the readiness state
func (h *Checker) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// SetLive sets the liveness state
func (h *Checker) SetLive(live bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.live = live
}

// LivenessHandler handles liveness probe requests
func (h *Checker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := "ok"
	httpStatus := http.StatusOK

	if !h.live {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    time.Since(h.started).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// ReadinessHandler handles readiness probe requests
func (h *Checker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := "ready"
	httpStatus := http.StatusOK

	if !h.ready {
		status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    time.Since(h.started).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}
