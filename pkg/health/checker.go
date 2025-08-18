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

// UpdateLastLoop updates the last loop execution time
func (h *Checker) UpdateLastLoop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastLoop = time.Now()
}

// AddCheck adds or updates a health check
func (h *Checker) AddCheck(name, status, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.checks[name] = HealthCheck{
		Name:    name,
		Status:  status,
		Message: message,
		LastRun: time.Now(),
	}
}

// AddCheckWithDuration adds or updates a health check with duration
func (h *Checker) AddCheckWithDuration(name, status, message string, duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.checks[name] = HealthCheck{
		Name:     name,
		Status:   status,
		Message:  message,
		LastRun:  time.Now(),
		Duration: duration.String(),
	}
}

// RemoveCheck removes a health check
func (h *Checker) RemoveCheck(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checks, name)
}

// IsReady returns the readiness state
func (h *Checker) IsReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready
}

// IsLive returns the liveness state
func (h *Checker) IsLive() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.live
}

// GetUptime returns the uptime duration
func (h *Checker) GetUptime() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return time.Since(h.started)
}

// GetLastLoop returns the time of the last loop execution
func (h *Checker) GetLastLoop() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastLoop
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

	// Check if the last loop was too long ago (indicates stuck controller)
	if !h.lastLoop.IsZero() && time.Since(h.lastLoop) > 5*time.Minute {
		status = "stale"
		httpStatus = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    time.Since(h.started).String(),
	}

	// Marshal first so we can set the correct status even if encoding fails
	data, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to encode liveness response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	if _, err := w.Write(data); err != nil {
		// Best effort: cannot change status after headers are sent; surface as 500 body for observability
		http.Error(w, "failed to write liveness response: "+err.Error(), http.StatusInternalServerError)
	}
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

	// Copy checks for response
	checks := make(map[string]HealthCheck)
	for k, v := range h.checks {
		checks[k] = v
	}

	// Check for any failed health checks
	for _, check := range checks {
		if check.Status != "ok" && check.Status != "pass" {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			break
		}
	}

	response := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    time.Since(h.started).String(),
		Checks:    checks,
	}

	// Marshal first so we can set the correct status even if encoding fails
	data, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to encode readiness response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	if _, err := w.Write(data); err != nil {
		http.Error(w, "failed to write readiness response: "+err.Error(), http.StatusInternalServerError)
	}
}

// CheckKubernetesAPI checks if Kubernetes API is accessible
func (h *Checker) CheckKubernetesAPI(accessible bool, message string) {
	status := "pass"
	if !accessible {
		status = "fail"
	}
	h.AddCheck("kubernetes_api", status, message)
}

// CheckFRRStatus checks if FRR is accessible
func (h *Checker) CheckFRRStatus(accessible bool, message string) {
	status := "pass"
	if !accessible {
		status = "fail"
	}
	h.AddCheck("frr_status", status, message)
}

// CheckServiceDiscovery updates service discovery health
func (h *Checker) CheckServiceDiscovery(serviceCount int, duration time.Duration) {
	message := fmt.Sprintf("Discovered %d services", serviceCount)
	h.AddCheckWithDuration("service_discovery", "pass", message, duration)
}
