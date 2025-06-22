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
)

const (
	defaultConfigPath = "/etc/cosmolet/config.yaml"
	defaultLogLevel   = "info"
)

var (
	configPath = flag.String("config", defaultConfigPath, "Path to configuration file")
	logLevel   = flag.String("log-level", defaultLogLevel, "Log level (debug, info, warn, error)")
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

	log.Printf("Starting Cosmolet BGP Service Controller")
	log.Printf("Version: %s, Commit: %s, Build Date: %s", Version, GitCommit, BuildDate)

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded from: %s", *configPath)
	log.Printf("Monitoring namespaces: %v", cfg.Services.Namespaces)
	log.Printf("Loop interval: %d seconds", cfg.LoopIntervalSeconds)

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

	// Start controller in goroutine
	go func() {
		if err := bgpController.Start(); err != nil {
			log.Printf("BGP controller error: %v", err)
			cancel()
		}
	}()

	// Mark as ready
	healthChecker.SetReady(true)

	// Wait for shutdown signal
	waitForShutdown(cancel)

	log.Println("Shutting down Cosmolet BGP Service Controller")
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

	log.Println("Starting health check server on :8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Health server error: %v", err)
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

func waitForShutdown(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	sig := <-sigChan
	log.Printf("Received signal: %s", sig)
	
	// Give some time for graceful shutdown
	cancel()
	time.Sleep(5 * time.Second)
}
