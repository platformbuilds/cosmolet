package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BGPMetrics contains all BGP-related metrics
type BGPMetrics struct {
	RouteAdvertised    *prometheus.CounterVec
	RouteWithdrawn     *prometheus.CounterVec
	BGPOperationsFailed *prometheus.CounterVec
	ServiceHealth      *prometheus.GaugeVec
	BGPSessionsUp      *prometheus.GaugeVec
	RouteCount         *prometheus.GaugeVec
	ControllerInfo     *prometheus.GaugeVec
	LeaderElection     *prometheus.GaugeVec
}

// NewBGPMetrics creates and returns new BGP metrics
func NewBGPMetrics() *BGPMetrics {
	return &BGPMetrics{
		RouteAdvertised: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cosmolet_bgp_routes_advertised_total",
				Help: "Total number of BGP routes advertised",
			},
			[]string{"cidr", "next_hop", "node"},
		),
		RouteWithdrawn: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cosmolet_bgp_routes_withdrawn_total",
				Help: "Total number of BGP routes withdrawn",
			},
			[]string{"cidr", "node"},
		),
		BGPOperationsFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cosmolet_bgp_operations_failed_total",
				Help: "Total number of failed BGP operations",
			},
			[]string{"operation", "error_type", "node"},
		),
		ServiceHealth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cosmolet_service_health_status",
				Help: "Health status of Kubernetes services (1=healthy, 0=unhealthy)",
			},
			[]string{"namespace", "service", "service_type"},
		),
		BGPSessionsUp: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cosmolet_bgp_sessions_up",
				Help: "BGP session status (1=up, 0=down)",
			},
			[]string{"peer", "asn", "node"},
		),
		RouteCount: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cosmolet_bgp_routes_active",
				Help: "Number of active BGP routes",
			},
			[]string{"type", "node"},
		),
		ControllerInfo: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cosmolet_controller_info",
				Help: "Information about the cosmolet controller",
			},
			[]string{"version", "commit", "node"},
		),
		LeaderElection: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "cosmolet_leader_election_status",
				Help: "Leader election status (1=leader, 0=follower)",
			},
			[]string{"node"},
		),
	}
}
