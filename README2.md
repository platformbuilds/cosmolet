# Cosmolet - BGP Service Controller

Cosmolet is a Kubernetes BGP Service Controller that automatically advertises Kubernetes Service ClusterIPs via FRR BGP.

## Features

- 🔄 **Automatic Service Discovery**: Monitors services across multiple Kubernetes namespaces
- 🏥 **Health-Based Advertisement**: Only advertises services with healthy endpoints
- 🌐 **BGP Integration**: Seamlessly integrates with FRR for BGP route advertisement
- ⚡ **High Performance**: Lightweight controller with configurable polling intervals
- 📊 **Observability**: Built-in health checks, metrics, and structured logging

## Quick Start

### Installation with Helm

```bash
helm repo add cosmolet https://your-org.github.io/cosmolet
helm install cosmolet cosmolet/cosmolet \
  --namespace cosmolet-system \
  --create-namespace
```

### Configuration

Create a values file:

```yaml
config:
  services:
    namespaces:
      - "production"
      - "staging"
  loopIntervalSeconds: 30

resources:
  requests:
    cpu: 100m
    memory: 128Mi
```

## Development

```bash
# Build
make build

# Test
make test

# Docker build
make docker-build
```

## Documentation

See the `docs/` directory for detailed documentation.

## License

Apache License 2.0
