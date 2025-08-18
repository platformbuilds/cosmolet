
# Operations & Runbook

## Scrape metrics
- Create Service: [cosmolet-service.yaml](examples/monitoring/cosmolet-service.yaml)
- ServiceMonitor: [servicemonitor.yaml](examples/monitoring/servicemonitor.yaml)

## Alerts
- Import Prometheus rules: [prometheus-rules.yaml](examples/monitoring/prometheus-rules.yaml)

## Failure drills
- Test Local/Cluster Services: [svc-lb-local.yaml](examples/k8s/svc-lb-local.yaml), [svc-lb-cluster.yaml](examples/k8s/svc-lb-cluster.yaml)
