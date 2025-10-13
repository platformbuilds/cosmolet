package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Checker struct {
	mu       sync.RWMutex
	ready    bool
	live     bool
	started  time.Time
	lastLoop time.Time
	checks   map[string]HealthCheck
}

type HealthCheck struct {
	Name     string    `json:"name"`
	Status   string    `json:"status"`
	Message  string    `json:"message,omitempty"`
	LastRun  time.Time `json:"last_run"`
	Duration string    `json:"duration,omitempty"`
}

type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Uptime    string                 `json:"uptime"`
	Checks    map[string]HealthCheck `json:"checks,omitempty"`
}

func NewChecker() *Checker {
	return &Checker{
		ready:   false,
		live:    true,
		started: time.Now(),
		checks:  make(map[string]HealthCheck),
	}
}

func (h *Checker) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

func (h *Checker) SetLive(live bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.live = live
}

func (h *Checker) UpdateLastLoop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastLoop = time.Now()
}

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

func (h *Checker) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	status := "ok"
	code := http.StatusOK
	if !h.live || (!h.lastLoop.IsZero() && time.Since(h.lastLoop) > 5*time.Minute) {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	}
	resp := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    time.Since(h.started).String(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp)
}

func (h *Checker) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	status := "ready"
	code := http.StatusOK
	if !h.ready {
		status = "not_ready"
		code = http.StatusServiceUnavailable
	}
	for _, check := range h.checks {
		if check.Status != "ok" && check.Status != "pass" {
			status = "unhealthy"
			code = http.StatusServiceUnavailable
			break
		}
	}
	resp := HealthResponse{
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    time.Since(h.started).String(),
		Checks:    h.checks,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(resp)
}

func (h *Checker) CheckKubernetesAPI(accessible bool, msg string) {
	status := "pass"
	if !accessible {
		status = "fail"
	}
	h.AddCheck("kubernetes_api", status, msg)
}

func (h *Checker) CheckFRRStatus(accessible bool, msg string) {
	status := "pass"
	if !accessible {
		status = "fail"
	}
	h.AddCheck("frr_status", status, msg)
}

func (h *Checker) CheckServiceDiscovery(count int, duration time.Duration) {
	msg := fmt.Sprintf("Discovered %d services", count)
	h.AddCheckWithDuration("service_discovery", "pass", msg, duration)
}

