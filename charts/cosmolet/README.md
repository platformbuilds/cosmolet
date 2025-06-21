# Cosmolet Helm Chart

A Helm chart for deploying Cosmolet, a BareMetal Kubernetes BGP Service Controller.

## Prerequisites

- Kubernetes 1.20+
- Helm 3.0+
- FRR (Free Range Routing) installed on nodes
- BGP-capable network infrastructure

## Installation

### Add Helm Repository (if using from repo)
```bash
helm repo add cosmolet https://cosmolet.github.io/cosmolet
helm repo update
```

### Install Chart
```bash
# Install with default values
helm install cosmolet ./cosmolet -n network-system --create-namespace

# Install with custom values
helm install cosmolet ./cosmolet -f values-production.yaml -n network-system --create-namespace
```

## Configuration

The following table lists the configurable parameters of the Cosmolet chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `bgp.asn` | BGP Autonomous System Number | `65001` |
| `bgp.neighbors` | List of BGP neighbors | `["10.0.1.1:65000", "10.0.1.2:65000"]` |
| `bgp.enableBfd` | Enable BFD for fast convergence | `true` |
| `serviceAdvertisement.enabled` | Enable service advertisement | `true` |
| `serviceAdvertisement.defaultAction` | Default action for services | `"ignore"` |
| `metrics.enabled` | Enable Prometheus metrics | `true` |
| `serviceMonitor.enabled` | Create ServiceMonitor for Prometheus | `false` |

## Examples

### Production Deployment
```yaml
# values-production.yaml
bgp:
  asn: 65001
  neighbors:
    - "10.0.1.1:65000"
    - "10.0.1.2:65000"

serviceAdvertisement:
  rules:
    - name: "production-services"
      action: "advertise"
      serviceSelector:
        types: ["LoadBalancer"]
        namespaces: ["production"]

serviceMonitor:
  enabled: true
```

### Development Deployment
```yaml
# values-dev.yaml
bgp:
  asn: 65002
  neighbors:
    - "10.1.1.1:65000"

serviceAdvertisement:
  defaultAction: "advertise"

logging:
  level: "debug"
  format: "text"
```

## Upgrade

```bash
helm upgrade cosmolet ./cosmolet -f values-production.yaml -n network-system
```

## Uninstall

```bash
helm uninstall cosmolet -n network-system
```

## Troubleshooting

1. Check pod status:
```bash
kubectl get pods -n network-system -l app.kubernetes.io/name=cosmolet
```

2. Check logs:
```bash
kubectl logs -n network-system -l app.kubernetes.io/name=cosmolet
```

3. Verify BGP sessions:
```bash
kubectl exec -n network-system <cosmolet-pod> -- vtysh -c "show bgp summary"
```
