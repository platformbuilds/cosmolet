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

	"cosmolet/pkg/config"
	"cosmolet/pkg/controller"
	"cosmolet/pkg/health"
	"cosmolet/pkg/logging"
)

const (
	defaultConfigPath = "/etc/cosmolet/config.yaml"
	defaultLogLevel   = "info"
	defaultLogFormat  = "text"
)

var (
	configPath  = flag.String("config", defaultConfigPath, "Path to configuration file")
	logLevel    = flag.String("log-level", defaultLogLevel, "Log level (debug, info, warn, error)")
	logFormat   = flag.String("log-format", defaultLogFormat, "Log format (text, json)")
	version     = flag.Bool("version", false, "Print version information")

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

	// Start health check server
	healthChecker := health.NewChecker()
	go startHealthServer(healthChecker)

	// Create and start BGP controller
	bgpController, err := controller.NewBGPServiceController(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to create BGP service controller: %v", err)
	}

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

func startHealthServer(checker *health.Checker) {
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/healthz", checker.LivenessHandler)
	mux.HandleFunc("/readyz", checker.ReadinessHandler)
	mux.HandleFunc("/version", versionHandler)

	// Metrics endpoint (basic for now)
	mux.HandleFunc("/metrics", metricsHandler)

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

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	// Basic metrics endpoint - in a real implementation,
	// you would use Prometheus client library
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# HELP cosmolet_info Information about cosmolet\n")
	fmt.Fprintf(w, "# TYPE cosmolet_info gauge\n")
	fmt.Fprintf(w, "cosmolet_info{version=\"%s\",commit=\"%s\"} 1\n", Version, GitCommit)
}


