
# Helm Quickstart
```bash
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system   --set config.bgp.asn=65001   --set securityContext.privileged=true
```
- Metrics are exposed at `:8080/metrics` in‑pod (hostNetwork=true). Create a Service/ServiceMonitor if needed.
- Customize values in [values.md](values.md).
