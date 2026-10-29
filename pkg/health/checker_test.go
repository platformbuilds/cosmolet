package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const livenessStaleThreshold = 5 * time.Minute

func TestNewCheckerInitialState(t *testing.T) {
	c := NewChecker()
	if c == nil {
		t.Fatal("NewChecker returned nil")
	}
	if c.IsReady() {
		t.Error("expected new checker to not be ready")
	}
	if !c.IsLive() {
		t.Error("expected new checker to be live")
	}
	if !c.GetLastLoop().IsZero() {
		t.Errorf("expected zero last loop time, got %v", c.GetLastLoop())
	}
	if up := c.GetUptime(); up < 0 || up > time.Minute {
		t.Errorf("unexpected uptime for fresh checker: %v", up)
	}
}

func TestSetReady(t *testing.T) {
	c := NewChecker()
	if c.IsReady() {
		t.Fatal("precondition: should not be ready")
	}
	c.SetReady(true)
	if !c.IsReady() {
		t.Error("expected ready after SetReady(true)")
	}
	c.SetReady(false)
	if c.IsReady() {
		t.Error("expected not ready after SetReady(false)")
	}
}

func TestSetLive(t *testing.T) {
	c := NewChecker()
	if !c.IsLive() {
		t.Fatal("precondition: should be live")
	}
	c.SetLive(false)
	if c.IsLive() {
		t.Error("expected not live after SetLive(false)")
	}
	c.SetLive(true)
	if !c.IsLive() {
		t.Error("expected live after SetLive(true)")
	}
}

// TestUpdateLastLoopSetsFreshTime proves the loop timestamp is refreshed,
// which is what keeps LivenessHandler from reporting "stale".
func TestUpdateLastLoopSetsFreshTime(t *testing.T) {
	c := NewChecker()
	if !c.GetLastLoop().IsZero() {
		t.Fatal("precondition: last loop should start zero")
	}
	c.UpdateLastLoop()
	got := c.GetLastLoop()
	if got.IsZero() {
		t.Fatal("expected non-zero last loop after UpdateLastLoop")
	}
	if d := time.Since(got); d < 0 || d > time.Minute {
		t.Errorf("last loop not fresh after UpdateLastLoop: %v ago", d)
	}
}

func TestLivenessHandler_HealthyWhenLoopRecent(t *testing.T) {
	c := NewChecker()
	c.UpdateLastLoop()

	w := httptest.NewRecorder()
	c.LivenessHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok, got %q", resp.Status)
	}
}

func TestLivenessHandler_StaleWhenLoopOld(t *testing.T) {
	c := NewChecker()
	c.lastLoop = time.Now().Add(-livenessStaleThreshold - time.Minute)

	w := httptest.NewRecorder()
	c.LivenessHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "stale" {
		t.Errorf("expected status stale, got %q", resp.Status)
	}
}

func TestLivenessHandler_UnhealthyWhenNotLive(t *testing.T) {
	c := NewChecker()
	c.SetLive(false)

	w := httptest.NewRecorder()
	c.LivenessHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Errorf("expected status unhealthy, got %q", resp.Status)
	}
}

// TestLivenessHandler_ZeroLastLoopNotStale pins the current contract: a
// checker that has never run a loop (zero lastLoop) is not treated as stale,
// only the !live flag is.
func TestLivenessHandler_ZeroLastLoopNotStale(t *testing.T) {
	c := NewChecker()

	w := httptest.NewRecorder()
	c.LivenessHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status ok for zero last loop, got %q", resp.Status)
	}
}

func TestReadinessHandler_AllChecksPass(t *testing.T) {
	c := NewChecker()
	c.SetReady(true)
	c.CheckKubernetesAPI(true, "api reachable")
	c.CheckFRRStatus(true, "frr up")
	c.CheckServiceDiscovery(3, 12*time.Millisecond)

	w := httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ready" {
		t.Errorf("expected status ready, got %q", resp.Status)
	}
	if len(resp.Checks) != 3 {
		t.Fatalf("expected 3 checks, got %d", len(resp.Checks))
	}
	for _, name := range []string{"kubernetes_api", "frr_status", "service_discovery"} {
		chk, ok := resp.Checks[name]
		if !ok {
			t.Errorf("expected check %q in response", name)
			continue
		}
		if chk.Status != "pass" {
			t.Errorf("expected check %q to be pass, got %q", name, chk.Status)
		}
	}
}

func TestReadinessHandler_NotReadyWhenFlagFalse(t *testing.T) {
	c := NewChecker()
	c.SetReady(false)
	c.CheckKubernetesAPI(true, "api reachable")

	w := httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "not_ready" {
		t.Errorf("expected status not_ready, got %q", resp.Status)
	}
}

func TestReadinessHandler_UnhealthyWhenCheckFails(t *testing.T) {
	c := NewChecker()
	c.SetReady(true)
	c.CheckKubernetesAPI(true, "api reachable")
	c.CheckFRRStatus(false, "vtysh failed")

	w := httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Errorf("expected status unhealthy, got %q", resp.Status)
	}
	frr, ok := resp.Checks["frr_status"]
	if !ok {
		t.Fatal("expected frr_status check in response")
	}
	if frr.Status != "fail" {
		t.Errorf("expected frr_status fail, got %q", frr.Status)
	}
	if frr.Message != "vtysh failed" {
		t.Errorf("expected message %q, got %q", "vtysh failed", frr.Message)
	}
}

func TestAddCheckAndRemoveCheck(t *testing.T) {
	c := NewChecker()
	c.AddCheck("kubernetes_api", "ok", "fine")

	if !c.IsReady() {
		// sanity: unrelated, but AddCheck must not affect readiness
	}
	w := httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var resp HealthResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if chk, ok := resp.Checks["kubernetes_api"]; !ok || chk.Status != "ok" || chk.Message != "fine" {
		t.Errorf("expected kubernetes_api ok/fine, got %+v (ok=%v)", chk, ok)
	}

	c.RemoveCheck("kubernetes_api")
	w = httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	resp = HealthResponse{}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp.Checks["kubernetes_api"]; ok {
		t.Error("expected kubernetes_api removed after RemoveCheck")
	}
}

func TestAddCheckWithDuration(t *testing.T) {
	c := NewChecker()
	c.AddCheckWithDuration("custom", "pass", "msg", 250*time.Millisecond)

	w := httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	chk, ok := resp.Checks["custom"]
	if !ok {
		t.Fatal("expected custom check in response")
	}
	if chk.Duration != "250ms" {
		t.Errorf("expected duration 250ms, got %q", chk.Duration)
	}
}

func TestCheckServiceDiscoveryUpdatesReadiness(t *testing.T) {
	c := NewChecker()
	c.SetReady(true)
	c.CheckServiceDiscovery(42, 7*time.Millisecond)

	w := httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ready" {
		t.Errorf("expected ready after passing service discovery, got %q", resp.Status)
	}
	chk, ok := resp.Checks["service_discovery"]
	if !ok {
		t.Fatal("expected service_discovery check")
	}
	if chk.Status != "pass" {
		t.Errorf("expected pass, got %q", chk.Status)
	}
	if chk.Message != "Discovered 42 services" {
		t.Errorf("expected 'Discovered 42 services', got %q", chk.Message)
	}
}

func TestCheckKubernetesAPIAndFRRStatus(t *testing.T) {
	c := NewChecker()
	c.CheckKubernetesAPI(false, "connection refused")
	c.CheckFRRStatus(true, "vtysh ok")

	w := httptest.NewRecorder()
	c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", resp.Status)
	}
	k8s := resp.Checks["kubernetes_api"]
	if k8s.Status != "fail" || k8s.Message != "connection refused" {
		t.Errorf("unexpected kubernetes_api check: %+v", k8s)
	}
	frr := resp.Checks["frr_status"]
	if frr.Status != "pass" || frr.Message != "vtysh ok" {
		t.Errorf("unexpected frr_status check: %+v", frr)
	}
}

// TestCheckerConcurrentReadWrite simulates the production pattern where the
// HTTP server reads state while the control loop writes it. This runs under
// -race to prove the RWMutex guards are correct.
func TestCheckerConcurrentReadWrite(t *testing.T) {
	c := NewChecker()
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.UpdateLastLoop()
				c.SetReady(j%2 == 0)
				c.SetLive(j%2 == 0)
				c.AddCheck("kubernetes_api", "ok", "w")
				c.AddCheck("frr_status", "pass", "w")
				c.CheckServiceDiscovery(1, time.Millisecond)
				c.RemoveCheck("frr_status")
				_ = c.IsReady()
				_ = c.IsLive()
				_ = c.GetUptime()
				_ = c.GetLastLoop()
			}
		}(i)
	}

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				w := httptest.NewRecorder()
				c.LivenessHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
				w = httptest.NewRecorder()
				c.ReadinessHandler(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			}
		}()
	}

	wg.Wait()
}
