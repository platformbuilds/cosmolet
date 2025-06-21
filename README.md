# Cosmolet - BareMetal Kubernetes BGP Service Controller

## Release Version: alpha
Please note cosmolet needs extensive testing is not ready for production deployments yet.

Cosmolet is a Kubernetes controller that automatically advertises LoadBalancer service IPs via BGP. It runs as a privileged DaemonSet with direct FRR (Free Range Routing) integration to enable bare-metal Kubernetes clusters to announce service IPs to network infrastructure.

## 🚀 Features

Reference Network Architecture
![Reference Network Architecture - HLD](./cosmolet.png)

- **Automatic Service Discovery**: Monitors all Kubernetes services across the cluster
- **BGP Route Advertisement**: Integrates with FRR to advertise ClusterIP and LoadBalancer service IPs
- **Health-based Routing**: Only advertises routes for healthy services with ready endpoints
- **Leader Election**: Ensures only one instance manages BGP routes while maintaining monitoring on all nodes
- **High Availability**: DaemonSet deployment with graceful failover
- **Comprehensive Monitoring**: Prometheus metrics and health checks
- **Security Hardened**: Minimal privileges with proper RBAC configuration

## 📋 Prerequisites

- Kubernetes 1.20+ cluster
- FRR (Free Range Routing) installed on nodes
- BGP-capable network infrastructure (switches/routers)
- Cluster admin permissions for installation

## 🔧 Installation

### Quick Start

```bash
# Install using kubectl
kubectl apply -f https://github.com/platformbuilds/cosmolet/releases/latest/download/cosmolet.yaml

# Or using Helm
helm repo add cosmolet https://github.com/platformbuilds/cosmolet.git
helm install cosmolet cosmolet/cosmolet
```

### Manual Installation

```bash
# Clone the repository
git clone https://github.com/platformbuilds/cosmolet.git
cd cosmolet

# Deploy using kustomize
kubectl apply -k config/base/

# Or build and deploy
make deploy
```

## ⚙️ Configuration

### Basic Configuration

Create a ConfigMap with your BGP settings:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cosmolet-config
  namespace: network-system
data:
  config.yaml: |
    bgp:
      asn: 65001
      router_id: "10.0.0.1"
      neighbors:
        - "10.0.1.1:65000"  # Leaf switch 1
        - "10.0.1.2:65000"  # Leaf switch 2
      enable_bfd: true
    
    service_selector:
      types:
        - "LoadBalancer"
        - "ClusterIP"  # Optional
```

## 🔍 Monitoring

### Prometheus Metrics

Cosmolet exports metrics on port `8080`:

- `cosmolet_bgp_routes_advertised_total` - Total routes advertised
- `cosmolet_bgp_routes_withdrawn_total` - Total routes withdrawn
- `cosmolet_bgp_sessions_up` - BGP session status
- `cosmolet_service_health_status` - Service health status
- `cosmolet_controller_info` - Controller version info

### Health Checks

- **Liveness Probe**: `/healthz` on port `8081`
- **Readiness Probe**: `/readyz` on port `8081`

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the GNU Affero General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

---

⭐ If this project helps you, please consider giving it a star on GitHub!
