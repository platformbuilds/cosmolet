package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	VIPAdvertised = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cosmolet_vip_advertised_total",
		Help: "Number of VIP advertisements issued by this node.",
	}, []string{"service", "namespace", "ipfamily", "node"})

	VIPWithdrawn = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cosmolet_vip_withdrawn_total",
		Help: "Number of VIP withdrawals issued by this node.",
	}, []string{"service", "namespace", "ipfamily", "node"})

	EndpointsReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cosmolet_endpoints_ready",
		Help: "Ready endpoints for a service on this node.",
	}, []string{"service", "namespace", "node"})

	ReconcileErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cosmolet_reconcile_errors_total",
		Help: "Total number of reconcile errors.",
	})
)

func init() {
	prometheus.MustRegister(VIPAdvertised, VIPWithdrawn, EndpointsReady, ReconcileErrors)
}
