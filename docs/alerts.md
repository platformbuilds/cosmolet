
# Alerts (Prometheus)
## Blackhole Risk (Local policy)
Alert if a node advertises a VIP but has no local ready endpoints.
```yaml
groups:
- name: cosmolet-alerts
  rules:
  - alert: CosmoletBlackholeRisk
    expr: |
      increase(cosmolet_vip_advertised_total[5m]) > 0
      unless on(service,namespace,node)
      (cosmolet_endpoints_ready == 0)
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "Cosmolet VIP advertised with no local ready endpoints"
      description: "Service {{ $labels.namespace }}/{{ $labels.service }} on node {{ $labels.node }}"
```
## Flapping
```yaml
- alert: CosmoletVipFlap
  expr: rate(cosmolet_vip_advertised_total[5m]) + rate(cosmolet_vip_withdrawn_total[5m]) > 5
  for: 5m
  labels: {severity: warning}
  annotations:
    summary: "Cosmolet VIP announcements flapping"
```
