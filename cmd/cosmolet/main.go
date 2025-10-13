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

var (
	configPath = flag.String("config", "/etc/cosmolet/config.yaml", "Path to configuration file")
	Version    = "dev"
	GitCommit  = "unknown"
	BuildDate  = "unknown"
)

func main() {
	flag.Parse()
	log.Printf("Starting Cosmolet BGP Controller v%s", Version)

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Auto-detect node name if not provided
	cfg.GetNodeName()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hc := health.NewChecker()
	go startHealthServer(hc)

	// ✅ FIX: remove extra argument (cfg.NodeName)
	bgp, err := controller.NewBGPServiceController(cfg, ctx)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	go func() {
		if err := bgp.Start(); err != nil {
			log.Printf("Controller error: %v", err)
			cancel()
		}
	}()

	hc.SetReady(true)
	waitForShutdown(cancel)
	log.Println("Cosmolet stopped")
}

func startHealthServer(checker *health.Checker) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", checker.LivenessHandler)
	mux.HandleFunc("/readyz", checker.ReadinessHandler)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "# Cosmolet metrics\n")
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Println("Health server running on :8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Health server error: %v", err)
	}
}

func waitForShutdown(cancel context.CancelFunc) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Printf("Received signal: %v", s)
	cancel()
	time.Sleep(2 * time.Second)
}

