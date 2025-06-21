# Cosmolet Project - Complete File Structure

## 📁 Project Overview
Cosmolet is a production-ready Kubernetes BGP service controller that automatically advertises LoadBalancer service IPs via BGP. It runs as a privileged DaemonSet with direct FRR (Free Range Routing) integration.

## 🗂️ Complete File Structure

### Root Level
```
cosmolet/
├── README.md                     # Project overview and documentation
├── CONTRIBUTING.md               # Contribution guidelines
├── go.mod                        # Go module dependencies
├── go.sum                        # Go module checksums
├── Makefile                      # Build and development commands
├── Dockerfile                    # Docker build configuration
├── LICENSE                       # Project license (AGPL-3.0)
└── PROJECT                       # Kubebuilder project configuration
```

### Source Code (`/pkg`)
```
pkg/
├── bgp/
│   ├── manager.go               # BGP route management through FRR
│   ├── client.go                # FRR client implementation
│   └── config.go                # BGP configuration structures
├── controller/
│   ├── controller.go            # Main Kubernetes controller logic
│   └── controller_test.go       # Controller unit tests
├── config/
│   ├── config.go                # Configuration loading and validation
│   └── config_test.go           # Configuration tests
├── metrics/
│   ├── metrics.go               # Prometheus metrics definitions
│   └── metrics_test.go          # Metrics tests
├── health/
│   ├── checker.go               # Health check implementation
│   └── checker_test.go          # Health checker tests
└── utils/
    ├── conditions.go            # Kubernetes condition utilities
    ├── finalizers.go            # Finalizer management
    └── predicates.go            # Event filtering predicates
```

### Main Application (`/cmd`)
```
cmd/
└── cosmolet/
    └── main.go                  # Application entry point
```

### Kubernetes Configurations (`/config`)
```
config/
├── base/
│   ├── daemonset.yaml           # Core DaemonSet configuration
│   ├── configmap.yaml           # Configuration and FRR settings
│   ├── rbac.yaml                # RBAC permissions
│   └── kustomization.yaml       # Kustomize configuration
├── examples/
│   ├── simple-production.yaml   # Basic production setup
│   ├── admin-config.yaml        # Advanced administrator config
│   └── development.yaml         # Development environment config
├── crd/
│   └── bases/
│       ├── bgp.cosmolet.io_bgpconfigs.yaml
│       └── bgp.cosmolet.io_bgppeers.yaml
├── manager/
│   ├── kustomization.yaml
│   ├── manager.yaml
│   └── controller_manager_config.yaml
├── default/
│   ├── kustomization.yaml
│   ├── manager_auth_proxy_patch.yaml
│   └── manager_config_patch.yaml
├── samples/
│   ├── bgp_v1_bgpconfig.yaml
│   └── bgp_v1_bgppeer.yaml
└── prometheus/
    ├── kustomization.yaml
    └── monitor.yaml
```

### Helm Charts (`/charts`)
```
charts/
└── cosmolet/
    ├── Chart.yaml               # Helm chart metadata
    ├── values.yaml              # Default Helm values
    └── templates/
        ├── daemonset.yaml       # DaemonSet template
        ├── rbac.yaml            # RBAC template
        ├── configmap.yaml       # ConfigMap template
        └── servicemonitor.yaml  # Prometheus ServiceMonitor
```

### Documentation (`/docs`)
```
docs/
├── configuration-guide.md       # Comprehensive configuration guide
├── api-reference.md             # API documentation
├── installation.md              # Installation instructions
├── troubleshooting.md           # Troubleshooting guide
└── development.md               # Development setup guide
```

### Testing (`/test`)
```
test/
├── e2e/
│   ├── suite_test.go           # E2E test suite
│   ├── bgp_test.go             # BGP functionality tests
│   └── service_discovery_test.go # Service discovery tests
└── fixtures/
    ├── services.yaml           # Test service fixtures
    └── nodes.yaml              # Test node fixtures
```

### Build and Deployment (`/build`)
```
build/
├── Dockerfile                  # Multi-stage Docker build
└── frr-defaults.conf          # Default FRR configuration
```

### Development Tools (`/hack`)
```
hack/
├── boilerplate.go.txt         # Go file header template
├── install-tools.sh           # Development tool installer
└── update-codegen.sh          # Code generation script
```

### Deployment Manifests (`/deploy`)
```
deploy/
├── helm/
│   └── cosmolet/              # Helm chart (symlink to /charts)
└── manifests/
    ├── namespace.yaml         # Namespace definition
    ├── daemonset.yaml         # Raw DaemonSet manifest
    ├── rbac.yaml              # Raw RBAC manifest
    └── configmap.yaml         # Raw ConfigMap manifest
```

### Monitoring (`/monitoring`)
```
monitoring/
├── grafana/
│   └── cosmolet-dashboard.json # Grafana dashboard
└── alerts/
    └── cosmolet-alerts.yaml   # Prometheus alerting rules
```

### GitHub Workflows (`/.github`)
```
.github/
├── workflows/
│   ├── ci.yml                 # Continuous integration
│   ├── release.yml            # Release automation
│   └── security.yml           # Security scanning
├── ISSUE_TEMPLATE/
│   ├── bug_report.md          # Bug report template
│   └── feature_request.md     # Feature request template
└── pull_request_template.md   # PR template
```

### API Definitions (`/api`)
```
api/
└── v1/
    ├── groupversion_info.go    # API group version info
    ├── bgpconfig_types.go      # BGPConfig CRD types
    ├── bgppeer_types.go        # BGPPeer CRD types
    └── zz_generated.deepcopy.go # Generated deepcopy methods
```

### Internal Packages (`/internal`)
```
internal/
├── controller/
│   ├── suite_test.go          # Controller test suite
│   ├── bgpconfig_controller.go # BGPConfig controller
│   ├── bgpconfig_controller_test.go
│   ├── bgppeer_controller.go   # BGPPeer controller
│   ├── bgppeer_controller_test.go
│   └── service_controller.go   # Service controller
├── frr/
│   ├── client.go              # FRR client implementation
│   ├── client_test.go         # FRR client tests
│   └── config.go              # FRR configuration
├── metrics/
│   ├── metrics.go             # Internal metrics
│   └── metrics_test.go        # Metrics tests
└── config/
    ├── config.go              # Internal config utilities
    └── config_test.go         # Config utility tests
```

## 🔧 Key Configuration Files

### Core Configurations
- **`config/base/daemonset.yaml`** - Main DaemonSet with privileged networking access
- **`config/base/configmap.yaml`** - Cosmolet and FRR configuration
- **`config/base/rbac.yaml`** - Minimal required RBAC permissions

### Example Configurations
- **`config/examples/simple-production.yaml`** - Basic production setup
- **`config/examples/admin-config.yaml`** - Advanced configuration with traffic engineering
- **`config/examples/development.yaml`** - Development environment setup

### Helm Chart
- **`charts/cosmolet/Chart.yaml`** - Helm chart metadata
- **`charts/cosmolet/values.yaml`** - Configurable Helm values

## 🏗️ Key Source Files

### Core Components
- **`pkg/controller/controller.go`** - Main Kubernetes controller with leader election
- **`pkg/bgp/manager.go`** - BGP route management through FRR
- **`pkg/config/config.go`** - Configuration parsing and validation
- **`pkg/metrics/metrics.go`** - Prometheus metrics definitions
- **`pkg/health/checker.go`** - Health check and readiness probes

### Application Entry
- **`cmd/cosmolet/main.go`** - Application initialization and startup

## 📊 Features Implemented

### BGP Integration
- Direct FRR integration via vtysh CLI
- Route advertisement and withdrawal
- BGP session monitoring
- Health-based route management

### Kubernetes Integration
- Service and Endpoint watching
- Leader election for HA
- RBAC with minimal permissions
- Custom Resource Definitions (CRDs)

### Observability
- Comprehensive Prometheus metrics
- Health and readiness probes
- Grafana dashboard
- Structured logging

### Security
- Security-hardened container with minimal privileges
- Read-only root filesystem
- Specific capability requirements (NET_ADMIN, NET_RAW)
- RBAC principle of least privilege

### Production Features
- Multi-stage Docker builds
- Helm chart for easy deployment
- E2E testing framework
- CI/CD pipelines
- Comprehensive documentation

This project structure follows modern Kubernetes controller patterns and provides a complete, production-ready BGP service controller for bare-metal clusters.