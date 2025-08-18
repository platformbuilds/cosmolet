package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	cosmoctrl "github.com/platformbuilds/cosmolet/pkg/controller"
)

func getenvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func getenvBool(name string, def bool) bool {
	if v := os.Getenv(name); v != "" {
		if v == "1" || v == "true" || v == "TRUE" {
			return true
		}
		if v == "0" || v == "false" || v == "FALSE" {
			return false
		}
	}
	return def
}
func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func main() {
	var kubeconfig string
	var resyncSeconds int
	var loopIntervalSeconds int

	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (in-cluster if empty)")
	flag.IntVar(&resyncSeconds, "resync-seconds", 300, "Shared informer resync period seconds")
	flag.IntVar(&loopIntervalSeconds, "loop-interval-seconds", 30, "Reconcile loop interval seconds")
	flag.Parse()

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Fatalf("NODE_NAME env var must be set via Downward API")
	}

	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Fatalf("failed building kube config: %v", err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("failed creating kube client: %v", err)
	}

	factory := informers.NewSharedInformerFactory(client, time.Duration(resyncSeconds)*time.Second)
	svcInf := factory.Core().V1().Services()
	nodeInf := factory.Core().V1().Nodes()
	epsInf := factory.Discovery().V1().EndpointSlices()

	stop := make(chan struct{})
	defer close(stop)
	factory.Start(stop)
	factory.WaitForCacheSync(stop)

	// Metrics and health HTTP server with timeouts (gosec G114 fix)
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("ok")); err != nil {
				log.Printf("error writing /healthz response: %v", err)
			}
		})
		http.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte("ready")); err != nil {
				log.Printf("error writing /readyz response: %v", err)
			}
		})

		srv := &http.Server{
			Addr:              ":8080",
			Handler:           nil, // default mux
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		log.Printf("Metrics listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	asn := getenvInt("BGP_ASN", 65001)
	ensureStatic := getenvBool("FRR_ENSURE_STATIC", true)
	vtyshPath := getenv("VTYSH_PATH", "/usr/bin/vtysh")

	ctrl, err := cosmoctrl.NewBGPController(cosmoctrl.Config{
		NodeName:              nodeName,
		LoopInterval:          time.Duration(loopIntervalSeconds) * time.Second,
		ServiceInformer:       svcInf,
		EndpointSliceInformer: epsInf,
		NodeInformer:          nodeInf,
		KubeClient:            client,
		ASN:                   asn,
		EnsureStatic:          ensureStatic,
		VTYSHPath:             vtyshPath,
	})
	if err != nil {
		log.Fatalf("failed creating controller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ctrl.Run(ctx)

	sigc := make(chan os.Signal, 2)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	<-sigc
	log.Printf("Shutdown signal received, withdrawing VIPs announced by this node...")
	ctrl.WithdrawAll(context.Background())
	log.Printf("Shutdown complete.")
}
