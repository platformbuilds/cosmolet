
# Metrics

Expose on `:8080/metrics`. For Prometheus Operator, use: [servicemonitor.yaml](examples/monitoring/servicemonitor.yaml).

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `cosmolet_vip_advertised_total` | Counter | `service`,`namespace`,`ipfamily`,`node` | VIP advertisements issued by this node |
| `cosmolet_vip_withdrawn_total` | Counter | `service`,`namespace`,`ipfamily`,`node` | VIP withdrawals issued by this node |
| `cosmolet_endpoints_ready` | Gauge | `service`,`namespace`,`node` | Ready endpoints on this node |
| `cosmolet_reconcile_errors_total` | Counter | — | Reconcile errors |
