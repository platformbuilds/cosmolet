// Package otel bootstraps and owns the OpenTelemetry SDK for cosmolet: a
// TracerProvider for control-loop and FRR spans, and a MeterProvider that
// feeds both the Prometheus scrape endpoint and (optionally) an OTLP/HTTP push
// exporter. Tracing is always on (low-volume per-node daemon); OTLP export of
// traces and metrics is enabled only when explicitly configured.
package otel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	metricapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config is the otel configuration block. It is intentionally independent of
// the rest of cosmolet's config so this package has no dependency on
// pkg/config.
type Config struct {
	Enabled     bool
	Endpoint    string
	SampleRatio float64
}

// Tracer names.
const (
	TracerController = "cosmolet.controller"
	TracerFRR        = "cosmolet.frr"
)

// Metric names.
const (
	MetricLoopDuration       = "cosmolet_loop_duration_seconds"
	MetricServicesDiscovered = "cosmolet_services_discovered"
	MetricBGPOperations      = "cosmolet_bgp_operations_total"
	MetricAdvertisedRoutes   = "cosmolet_advertised_routes"
)

// op labels for cosmolet_bgp_operations_total.
const (
	OpAdvertise = "advertise"
	OpWithdraw  = "withdraw"
	OpCheck     = "check"
)

type instruments struct {
	controller trace.Tracer
	frr        trace.Tracer

	loopDuration       metricapi.Float64Histogram
	servicesDiscovered metricapi.Int64Gauge
	bgpOperations      metricapi.Int64Counter
	advertisedRoutes   metricapi.Int64ObservableGauge
}

// Provider owns the SDK providers and instruments.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider

	// promReader is the prometheus reader serving /metrics. It is always
	// present (even when otel is disabled) so the /metrics endpoint can keep
	// serving the OTel metric pipeline.
	promReader *sdkmetric.ManualReader

	inst    *instruments
	mu      sync.Mutex
	enabled bool

	// advertisedRoutesProvider reports the live count of advertised routes to
	// the observable gauge callback. Defaults to a zero-value until the
	// controller wires it.
	advertisedRoutesProvider func() int64
}

// New bootstraps the SDK providers, instruments, and the Prometheus reader.
// A Prometheus manual reader is always installed so /metrics serves the OTel
// metric pipeline. The OTLP exporters (traces + metrics) are only wired when
// cfg.Enabled && cfg.Endpoint != "". version is the main.Version ldflag,
// recorded as service.version.
func New(cfg Config, version string) (*Provider, error) {
	if version == "" {
		version = "unknown"
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName("cosmolet"),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}

	promReader := sdkmetric.NewManualReader()

	mpOpts := []sdkmetric.Option{
		sdkmetric.WithReader(promReader),
		sdkmetric.WithResource(res),
	}

	p := &Provider{promReader: promReader}

	if cfg.Enabled && cfg.Endpoint != "" {
		traceExp, err := otlptracehttp.New(context.Background(),
			otlptracehttp.WithEndpoint(endpointHostPort(cfg.Endpoint)),
			otlptracehttp.WithURLPath("/v1/traces"),
		)
		if err != nil {
			return nil, fmt.Errorf("otel: build OTLP trace exporter: %w", err)
		}
		mp, err := otlpmetrichttp.New(context.Background(),
			otlpmetrichttp.WithEndpoint(endpointHostPort(cfg.Endpoint)),
			otlpmetrichttp.WithURLPath("/v1/metrics"),
		)
		if err != nil {
			return nil, fmt.Errorf("otel: build OTLP metric exporter: %w", err)
		}
		// Wrap the metric exporter in a periodic reader so it pushes on a
		// fixed cadence in addition to the manual Prometheus reader.
		mpOpts = append(mpOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mp, sdkmetric.WithInterval(30*time.Second))))

		p.tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExp),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(samplerFor(cfg.SampleRatio)),
		)
		p.meterProvider = sdkmetric.NewMeterProvider(mpOpts...)
		otel.SetTracerProvider(p.tracerProvider)
		otel.SetMeterProvider(p.meterProvider)
		p.enabled = true
	} else {
		// Tracing is always on: a real SDK provider is installed so the
		// instrumentation code paths are exercised. With no OTLP push
		// endpoint the spans are dropped (no exporter attached).
		p.tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSampler(samplerFor(cfg.SampleRatio)),
		)
		p.meterProvider = sdkmetric.NewMeterProvider(mpOpts...)
		otel.SetTracerProvider(p.tracerProvider)
		otel.SetMeterProvider(p.meterProvider)
	}

	meter := p.meterProvider.Meter(TracerController)

	loopDuration, err := meter.Float64Histogram(MetricLoopDuration,
		metricapi.WithUnit("s"),
		metricapi.WithDescription("Duration of a single control loop iteration."))
	if err != nil {
		return nil, fmt.Errorf("otel: create loop duration histogram: %w", err)
	}

	servicesDiscovered, err := meter.Int64Gauge(MetricServicesDiscovered,
		metricapi.WithDescription("Number of services discovered in the last control loop."))
	if err != nil {
		return nil, fmt.Errorf("otel: create services discovered gauge: %w", err)
	}

	bgpOperations, err := meter.Int64Counter(MetricBGPOperations,
		metricapi.WithDescription("BGP operations performed, labelled by op and outcome."))
	if err != nil {
		return nil, fmt.Errorf("otel: create bgp operations counter: %w", err)
	}

	// The observable gauge callback reads the live count from the provider,
	// which the controller wires via SetAdvertisedRoutesProvider.
	advertisedRoutes, err := meter.Int64ObservableGauge(MetricAdvertisedRoutes,
		metricapi.WithDescription("Number of routes currently advertised by this controller."),
		metricapi.WithInt64Callback(func(_ context.Context, o metricapi.Int64Observer) error {
			o.Observe(p.advertisedRoutesValue())
			return nil
		}))
	if err != nil {
		return nil, fmt.Errorf("otel: create advertised routes gauge: %w", err)
	}

	p.advertisedRoutesProvider = func() int64 { return 0 }
	p.inst = &instruments{
		controller:         p.tracerProvider.Tracer(TracerController),
		frr:                p.tracerProvider.Tracer(TracerFRR),
		loopDuration:       loopDuration,
		servicesDiscovered: servicesDiscovered,
		bgpOperations:      bgpOperations,
		advertisedRoutes:   advertisedRoutes,
	}

	return p, nil
}

// samplerFor returns a trace sampler. A ratio <= 0 or >= 1 means "always on"
// (the adopted default: always-on trace sampling for this low-volume per-node
// daemon). A value in (0,1) samples by trace ID ratio, parent-based.
func samplerFor(ratio float64) sdktrace.Sampler {
	if ratio <= 0 || ratio >= 1 {
		return sdktrace.AlwaysSample()
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

// endpointHostPort converts an otel endpoint into a host:port for the
// exporters. A full URL ("http://localhost:4318") is reduced to its host:port;
// a bare "host:port" is used as-is. The scheme is dropped; the exporter's
// default is used for transport.
func endpointHostPort(endpoint string) string {
	e := strings.TrimSpace(endpoint)
	if u, err := url.Parse(e); err == nil && u.Host != "" {
		return u.Host
	}
	// Strip a leading scheme if present.
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return e
}

// Enabled reports whether the OTLP push exporters are active.
func (p *Provider) Enabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enabled
}

// MetricsHandler returns the promhttp handler serving the OTel metric reader
// on /metrics.
func (p *Provider) MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// ControllerTracer returns the controller tracer.
func (p *Provider) ControllerTracer() trace.Tracer {
	return p.inst.controller
}

// FRRTracer returns the FRR tracer.
func (p *Provider) FRRTracer() trace.Tracer {
	return p.inst.frr
}

// SetAdvertisedRoutesProvider wires the callback that reports the live count
// of advertised routes to the gauge. The callback reads this function on each
// collection cycle, so it must be safe to call concurrently.
func (p *Provider) SetAdvertisedRoutesProvider(f func() int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.advertisedRoutesProvider = f
}

// advertisedRoutesValue reads the live count under the mutex.
func (p *Provider) advertisedRoutesValue() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.advertisedRoutesProvider == nil {
		return 0
	}
	return p.advertisedRoutesProvider()
}

// RecordLoopDuration records the duration of a control loop iteration.
func (p *Provider) RecordLoopDuration(ctx context.Context, d time.Duration) {
	p.inst.loopDuration.Record(ctx, d.Seconds())
}

// RecordServicesDiscovered records how many services were discovered.
func (p *Provider) RecordServicesDiscovered(ctx context.Context, n int) {
	p.inst.servicesDiscovered.Record(ctx, int64(n))
}

// RecordBGPOperation records a BGP operation with its outcome.
func (p *Provider) RecordBGPOperation(ctx context.Context, op, outcome string) {
	p.inst.bgpOperations.Add(ctx, 1, metricapi.WithAttributes(
		attribute.String("op", op),
		attribute.String("outcome", outcome),
	))
}

// Shutdown flushes and stops both providers.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("otel: shutdown meter provider: %w", err)
		}
	}
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			return fmt.Errorf("otel: shutdown tracer provider: %w", err)
		}
	}
	return nil
}
