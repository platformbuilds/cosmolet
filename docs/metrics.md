
# Metrics
| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `cosmolet_vip_advertised_total` | Counter | `service`,`namespace`,`ipfamily`,`node` | VIP announcements issued by this node |
| `cosmolet_vip_withdrawn_total` | Counter | `service`,`namespace`,`ipfamily`,`node` | VIP withdrawals issued by this node |
| `cosmolet_endpoints_ready` | Gauge | `service`,`namespace`,`node` | Count of ready endpoints on this node for the service |
| `cosmolet_reconcile_errors_total` | Counter | *none* | Reconcile errors encountered |

**Scrape**: `http://<podIP>:8080/metrics` (hostNetwork).
