// cmd/cosmolet/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cosmolet/pkg/config"
	"cosmolet/pkg/controller"
	"cosmolet/pkg/health"
	"cosmolet/pkg/logging"
	"cosmolet/pkg/otel"
)

const (
	defaultConfigPath = "/etc/cosmolet/config.yaml"
	defaultLogLevel   = "info"
	defaultLogFormat  = "text"
)

var (
	configPath = flag.String("config", defaultConfigPath, "Path to configuration file")
	logLevel   = flag.String("log-level", defaultLogLevel, "Log level (debug, info, warn, error)")
	logFormat  = flag.String("log-format", defaultLogFormat, "Log format (text, json)")
	version    = flag.Bool("version", false, "Print version information")

	// Build information (set via ldflags)
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	flag.Parse()

	if *version {
		printVersion()
		return
	}

	// Load configuration first so the config's log level can be applied
	// before any logging happens. The -log-level flag wins over the config
	// value when it has been explicitly set.
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	applyLogLevel(cfg, *logLevel != defaultLogLevel)
	applyLogFormat(cfg, *logFormat != defaultLogFormat)

	logging.Info("Starting Cosmolet BGP Service Controller")
	logging.Info("Version: %s, Commit: %s, Build Date: %s", Version, GitCommit, BuildDate)

	logging.Info("Configuration loaded from: %s", *configPath)
	logging.Info("Monitoring namespaces: %v", cfg.Services.Namespaces)
	logging.Info("Loop interval: %d seconds", cfg.LoopIntervalSeconds)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bootstrap the observability provider (OTel SDK + Prometheus reader).
	// Tracing is always on; OTLP push exporters are wired only when
	// otel.enabled and otel.endpoint are both set. The provider's manual
	// reader backs the /metrics endpoint.
	otelCfg := otel.Config{
		Enabled:     cfg.OTEL.Enabled,
		Endpoint:    cfg.OTEL.Endpoint,
		SampleRatio: cfg.OTEL.SampleRatio,
	}
	otelProvider, err := otel.New(otelCfg, Version)
	if err != nil {
		log.Fatalf("Failed to bootstrap OpenTelemetry: %v", err)
	}
	if otelProvider.Enabled() {
		logging.Info("OpenTelemetry enabled: pushing traces/metrics to %s (sample ratio %g)", cfg.OTEL.Endpoint, cfg.OTEL.SampleRatio)
	} else {
		logging.Info("OpenTelemetry tracing enabled (no OTLP push endpoint configured; /metrics still served)")
	}

	// Start health check server (now wired to the OTel metric reader)
	healthChecker := health.NewChecker()
	go startHealthServer(healthChecker, otelProvider)

	// Create and start BGP controller
	bgpController, err := controller.NewBGPServiceController(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to create BGP service controller: %v", err)
	}
	bgpController.SetOTELProvider(otelProvider)

	// Mark as ready
	healthChecker.SetReady(true)

	// Run the controller on this goroutine so shutdown blocks until it has
	// finished withdrawing routes. Start returns only once the context is
	// cancelled AND withdrawAllRoutes has completed (Findings 2, 3) — no more
	// fixed 5 s sleep that could let the process exit before withdrawal.
	startErr := make(chan error, 1)
	go func() { startErr <- bgpController.Start() }()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logging.Info("Received signal: %s — waiting for the controller to withdraw routes and stop", sig)

	cancel()

	err = <-startErr
	if err != nil {
		logging.Error("BGP controller error during shutdown: %v", err)
	}

	// Flush and stop the observability providers so any buffered spans and
	// metric deltas are exported before the process exits.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if shErr := otelProvider.Shutdown(shutdownCtx); shErr != nil {
		logging.Warn("OpenTelemetry shutdown error: %v", shErr)
	}
	shutdownCancel()

	logging.Info("Shutting down Cosmolet BGP Service Controller")
}

// applyLogLevel sets the active log level. The -log-level flag wins when it
// was explicitly set; otherwise the config's logging.level is used. Both the
// flag and the config field are documented knobs, so they must take effect.
func applyLogLevel(cfg *config.Config, flagSet bool) {
	level := cfg.Logging.Level
	if flagSet {
		level = *logLevel
	}
	if level == "" {
		level = defaultLogLevel
	}
	logging.SetLevel(level)
}

// applyLogFormat sets the active log format (text or json). The -log-format
// flag wins when it was explicitly set; otherwise the config's
// logging.format is used. Both are documented knobs and must take effect
// (Finding 34).
func applyLogFormat(cfg *config.Config, flagSet bool) {
	format := cfg.Logging.Format
	if flagSet {
		format = *logFormat
	}
	if format == "" {
		format = defaultLogFormat
	}
	logging.SetFormat(format)
}

func printVersion() {
	fmt.Printf("Cosmolet BGP Service Controller\n")
	fmt.Printf("Version: %s\n", Version)
	fmt.Printf("Git Commit: %s\n", GitCommit)
	fmt.Printf("Build Date: %s\n", BuildDate)
}

func startHealthServer(checker *health.Checker, otelProvider *otel.Provider) {
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/healthz", checker.LivenessHandler)
	mux.HandleFunc("/readyz", checker.ReadinessHandler)
	mux.HandleFunc("/version", versionHandler)

	// Metrics endpoint: serves the OTel metric pipeline (control-loop duration,
	// services discovered, BGP operations, advertised routes). The provider's
	// manual reader backs promhttp so /metrics reflects real instrumented
	// values rather than a static stub.
	mux.Handle("/metrics", otelProvider.MetricsHandler())

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	logging.Info("Starting health check server on :8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logging.Error("Health server error: %v", err)
	}
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
		"version": "%s",
		"gitCommit": "%s",
		"buildDate": "%s"
	}`, Version, GitCommit, BuildDate)
}
