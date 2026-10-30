package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newTestProvider builds a Provider with no OTLP exporters (cfg.Enabled false),
// so no network is touched. The provider's built-in manual reader is reused to
// collect metrics synchronously.
func newTestProvider(t *testing.T, cfg Config) *Provider {
	t.Helper()

	p, err := New(cfg, "test-version")
	if err != nil {
		t.Fatalf("New(%+v) returned error: %v", cfg, err)
	}
	return p
}

func TestNewDisabled(t *testing.T) {
	p := newTestProvider(t, Config{})

	if p.Enabled() {
		t.Fatal("expected Enabled()==false when cfg.Enabled is false")
	}
	if p.ControllerTracer() == nil {
		t.Fatal("expected non-nil controller tracer")
	}
	if p.FRRTracer() == nil {
		t.Fatal("expected non-nil FRR tracer")
	}
}

func TestNewEnabledWithoutEndpointFallsBack(t *testing.T) {
	// Enabled but no endpoint → no exporters, same as disabled.
	p := newTestProvider(t, Config{Enabled: true})
	if p.Enabled() {
		t.Fatal("expected Enabled()==false when Enabled is true but Endpoint is empty")
	}
}

// collectByMetricName invokes Collect and returns a map of metric name → the
// metricdata.Aggregation, so tests can assert on individual instruments.
func collectByMetricName(t *testing.T, p *Provider) map[string]metricdata.Aggregation {
	t.Helper()
	ctx := context.Background()
	var rm metricdata.ResourceMetrics
	if err := p.promReader.Collect(ctx, &rm); err != nil {
		t.Fatalf("promReader.Collect returned error: %v", err)
	}
	out := make(map[string]metricdata.Aggregation)
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			out[m.Name] = m.Data
		}
	}
	return out
}

func TestMetricsAreRecorded(t *testing.T) {
	p := newTestProvider(t, Config{})
	ctx := context.Background()

	p.RecordLoopDuration(ctx, 150*time.Millisecond)
	p.RecordServicesDiscovered(ctx, 42)
	p.RecordBGPOperation(ctx, OpAdvertise, "success")
	p.RecordBGPOperation(ctx, OpWithdraw, "failure")
	p.SetAdvertisedRoutesProvider(func() int64 { return 7 })

	data := collectByMetricName(t, p)

	// Loop duration: a single histogram data point at 0.15s.
	if hist, ok := data[MetricLoopDuration].(metricdata.Histogram[float64]); ok {
		if len(hist.DataPoints) != 1 {
			t.Fatalf("expected 1 loop duration data point, got %d", len(hist.DataPoints))
		}
		if hist.DataPoints[0].Count != 1 {
			t.Errorf("expected 1 loop duration observation, got count %d", hist.DataPoints[0].Count)
		}
		if got := hist.DataPoints[0].Sum; got != 150*time.Millisecond.Seconds() {
			t.Errorf("expected loop duration 0.15s, got %v", got)
		}
	} else {
		t.Fatalf("loop duration missing or wrong type: %T", data[MetricLoopDuration])
	}

	// Services discovered gauge = 42.
	if g, ok := data[MetricServicesDiscovered].(metricdata.Gauge[int64]); ok {
		if len(g.DataPoints) != 1 || g.DataPoints[0].Value != 42 {
			t.Errorf("expected services discovered 42, got %v", g.DataPoints)
		}
	} else {
		t.Fatalf("services discovered missing or wrong type: %T", data[MetricServicesDiscovered])
	}

	// BGP operations counter: two distinct label sets → two data points, value 1.
	if s, ok := data[MetricBGPOperations].(metricdata.Sum[int64]); ok {
		if len(s.DataPoints) != 2 {
			t.Fatalf("expected 2 bgp operations data points, got %d", len(s.DataPoints))
		}
		for _, dp := range s.DataPoints {
			if dp.Value != 1 {
				t.Errorf("expected each bgp operation count to be 1, got %v", dp.Value)
			}
		}
	} else {
		t.Fatalf("bgp operations missing or wrong type: %T", data[MetricBGPOperations])
	}

	// Advertised routes observable gauge reads from the wired provider = 7.
	if g, ok := data[MetricAdvertisedRoutes].(metricdata.Gauge[int64]); ok {
		if len(g.DataPoints) != 1 || g.DataPoints[0].Value != 7 {
			t.Errorf("expected advertised routes 7, got %v", g.DataPoints)
		}
	} else {
		t.Fatalf("advertised routes missing or wrong type: %T", data[MetricAdvertisedRoutes])
	}
}

func TestMetricsHandlerServesOTelPipeline(t *testing.T) {
	p := newTestProvider(t, Config{})
	p.RecordLoopDuration(context.Background(), time.Second)

	handler := p.MetricsHandler()
	if handler == nil {
		t.Fatal("MetricsHandler() returned nil")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") == "" {
		t.Error("expected a Content-Type header on /metrics")
	}
}
