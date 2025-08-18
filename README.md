
# Cosmolet - BareMetal Kubernetes Services BGP Advertiser

## Release Version: beta
Cosmolet has been functionally tested and deemed working. 
Testing at scale is in progress.

Cosmolet is a Kubernetes controller that automatically advertises Kubernetes service IPs via BGP. It runs as a privileged DaemonSet with direct FRR (Free Range Routing) integration to enable bare-metal Kubernetes clusters to announce service IPs to network infrastructure.

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

## 📚 Documentation

Start here → [`docs/index.md`](docs/index.md)

- **Concepts & Design**: [`docs/overview.md`](docs/overview.md), [`docs/architecture.md`](docs/architecture.md)
- **Deployment Guide**: [`docs/deployment.md`](docs/deployment.md)
- **Fabric / FRR Config**: [`docs/frr-config.md`](docs/frr-config.md)
- **Helm**: [`docs/helm/quickstart.md`](docs/helm/quickstart.md), [`docs/helm/values.md`](docs/helm/values.md)
- **Operations**: [`docs/operations.md`](docs/operations.md), [`docs/metrics.md`](docs/metrics.md), [`docs/alerts.md`](docs/alerts.md), [`docs/security.md`](docs/security.md), [`docs/troubleshooting.md`](docs/troubleshooting.md)
- **Migration & Compatibility**: [`docs/migration.md`](docs/migration.md), [`docs/compatibility.md`](docs/compatibility.md)
- **📦 Examples catalog**: [`docs/examples/README.md`](docs/examples/README.md)
- **🛠️ Config Generator (Q&A)**: [`docs/tools.md`](docs/tools.md) · script: `tools/gen-values.sh`

## ⚙️ Quick Start (Q&A generator)

Generate Helm values and FRR configs interactively:
```bash
chmod +x tools/gen-values.sh
./tools/gen-values.sh
helm upgrade --install cosmolet ./charts/cosmolet -n kube-system -f generated/helm-values.yaml
# (optional) apply a sample Service from docs/examples/k8s/
```

Non‑interactive (CI-friendly) mode:
```bash
TOPOLOGY=dual-tor NODE_ASN=65101 NEIGHBOR_IPS=10.0.0.1,10.0.0.2 NEIGHBOR_ASNS=65000 \
DUAL_STACK=true ENSURE_STATIC=true IMAGE_REPO=ghcr.io/platformbuilds/cosmolet IMAGE_TAG=latest \
LOOP_INTERVAL=20 NAMESPACE=kube-system OUT_DIR=generated SERVICE_STYLE=local \
SERVICE_NAMESPACE=demo SERVICE_NAME=web \
./tools/gen-values.sh --yes
```

## 📋 Prerequisites

- Kubernetes 1.20+ cluster
- FRR (Free Range Routing) installed on nodes
- BGP-capable network infrastructure (switches/routers)
- Cluster admin permissions for installation

## Code Flowchart
![High Level Algorithm/Flowchart](./flowchart/flowchart-1.png)

---

# 🧱 Build & Release Instructions

> All common tasks are codified in the **Makefile**.

### Install dependencies
```bash
make deps
```

### Lint & security
```bash
make lint       # golangci-lint + fmt + vet
make security   # govulncheck + gosec
```

### Run tests (with race + coverage)
```bash
make test
# Optional:
make coverage       # prints summary
make coverage-html  # writes ./dist/coverage.html
```

### Build the binary
```bash
make build          # outputs ./bin/cosmolet
```

### Cross-compile release binaries
```bash
# One target
GOOS=linux GOARCH=arm64 make build-release   # -> ./dist/cosmolet-linux-arm64

# All common targets
make build-multi    # -> ./dist/cosmolet-<os>-<arch>
```

### Docker images
```bash
# Single-arch image (local)
make docker-build
# Push it
make docker-push

# Multi-arch image (Buildx + push)
IMAGE_TAGS="v1.2.3,latest" make docker-buildx
# Optionally customize platforms:
PLATFORMS="linux/amd64,linux/arm64" IMAGE_TAGS="$(git describe --tags --always)" make docker-buildx
```

### Helm helpers (optional)
```bash
make helm-lint
make helm-package  # -> ./dist/
```

### CI fast path (everything)
```bash
make ci   # deps + lint + security + test + build
```

---

## 🧪 Manual Build (advanced)

If you prefer building without the Makefile:

```bash
# Simple build (dev only)
go build -o ./bin/cosmolet ./cmd/cosmolet

# Production (static)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build   -ldflags='-w -s -extldflags "-static"'   -a -installsuffix cgo   -o ./bin/cosmolet   ./cmd/cosmolet
```

Cross‑platform compilation examples:
```bash
# Linux default
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-w -s -extldflags "-static"' -a -installsuffix cgo -o ./bin/cosmolet-linux-amd64 ./cmd/cosmolet
# macOS
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags='-w -s -extldflags "-static"' -a -installsuffix cgo -o ./bin/cosmolet-linux-darwin-amd64 ./cmd/cosmolet
# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags='-w -s -extldflags "-static"' -a -installsuffix cgo -o ./bin/cosmolet-windows-amd64 ./cmd/cosmolet
# ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags='-w -s -extldflags "-static"' -a -installsuffix cgo -o ./bin/cosmolet-linux-arm64 ./cmd/cosmolet
```

Dev build with debug info:
```bash
go build -gcflags="all=-N -l" -o ./bin/cosmolet-debug ./cmd/cosmolet
```

### Build flags explained
- `-ldflags='-w -s'` — drop debug symbols (smaller binary)
- `-extldflags "-static"` — static linking
- `CGO_ENABLED=0` — pure Go build
- `-a -installsuffix cgo` — rebuild all, isolate CGO artifacts

### Environment variables
```bash
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
go build -ldflags='-w -s' -o cosmolet ./cmd/main.go
```

## ✅ Verification
```bash
file cosmolet
ldd cosmolet          # "not a dynamic executable" for static build
./cosmolet --help
./cosmolet --version
```

## 🧯 Common Build Issues
**Dependencies**
```bash
go mod tidy
go mod verify
```

**CGO**
```bash
CGO_ENABLED=0 go build ./cmd/main.go
```

**Module path**
```bash
go mod init github.com/platformcosmo/cosmolet  # if starting fresh
```

The resulting binary will be statically linked and suitable for deployment in containers or bare-metal systems without external dependencies.

---

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the GNU Affero General Public License v3.0 — see the [LICENSE](LICENSE) file for details.

---

⭐ If this project helps you, please consider giving it a star on GitHub!
