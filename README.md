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

# Cosmolet - Go Build Instructions

This guide provides comprehensive instructions for building the Cosmolet BGP Service Controller from source.

## Prerequisites

### Go Installation

**Minimum Required Version:** Go 1.21+

#### Install Go

**Linux/macOS:**
```bash
# Download and install Go 1.21
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc or ~/.zshrc)
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export GOBIN=$GOPATH/bin
```

**macOS (using Homebrew):**
```bash
brew install go@1.21
```

**Windows:**
Download from https://golang.org/dl/ and run the installer.

#### Verify Installation
```bash
go version
# Should output: go version go1.21.x
```

### Development Tools

```bash
# Install development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

### System Dependencies

**For Local Development:**
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y git make curl

# macOS
brew install git make curl

# For FRR integration (optional for build, required for runtime)
sudo apt-get install -y frr frr-pythontools  # Ubuntu/Debian
```

## Project Setup

### 1. Clone Repository

```bash
git clone https://github.com/your-org/cosmolet.git
cd cosmolet
```

### 2. Verify Go Module

```bash
# Check go.mod exists and is valid
cat go.mod

# Download dependencies
go mod download
go mod tidy
```

### 3. Verify Project Structure

```bash
tree -L 3
# Should show:
# ├── cmd/cosmolet/
# ├── pkg/
# │   ├── config/
# │   ├── controller/
# │   └── health/
# ├── go.mod
# └── go.sum
```

## Building the Application

### 1. Basic Build

```bash
# Simple build (development)
go build ./cmd/cosmolet

# Or use Make (recommended)
make build
```

### 2. Build with Version Information

```bash
# Set version variables
export VERSION="v1.0.0"
export GIT_COMMIT=$(git rev-parse HEAD)
export BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# Build with ldflags
go build \
  -ldflags="-w -s -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
  -o bin/cosmolet \
  ./cmd/cosmolet
```

### 3. Optimized Production Build

```bash
# Production build with optimizations
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-w -s -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
  -trimpath \
  -o bin/cosmolet-linux-amd64 \
  ./cmd/cosmolet
```

### 4. Cross-Platform Builds

```bash
# Build for multiple platforms
make build-all

# Or manually:
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o bin/cosmolet-linux-amd64 ./cmd/cosmolet

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o bin/cosmolet-linux-arm64 ./cmd/cosmolet

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -o bin/cosmolet-darwin-amd64 ./cmd/cosmolet

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o bin/cosmolet-darwin-arm64 ./cmd/cosmolet

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/cosmolet-windows-amd64.exe ./cmd/cosmolet
```

### 5. Using Makefile

The provided Makefile includes several build targets:

```bash
# Show all available targets
make help

# Basic build
make build

# Build for all platforms
make build-all

# Clean build artifacts
make clean

# Build and run tests
make test

# Run linter
make lint

# Development build and run
make dev
```

## Docker Builds

### 1. Basic Docker Build

```bash
# Build Docker image
docker build -t cosmolet:latest .

# Or use Make
make docker-build
```

### 2. Multi-Platform Docker Build

```bash
# Set up buildx (if not already done)
docker buildx create --name cosmolet-builder --use

# Build for multiple platforms
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=${VERSION} \
  --build-arg GIT_COMMIT=${GIT_COMMIT} \
  --build-arg BUILD_DATE=${BUILD_DATE} \
  -t cosmolet/cosmolet:${VERSION} \
  -t cosmolet/cosmolet:latest \
  --push .
```

### 3. Debug Docker Build

```bash
# Build debug version with additional tools
docker build -f Dockerfile.debug -t cosmolet:debug .
```

## Testing

### 1. Run Unit Tests

```bash
# Basic test run
go test ./...

# With verbose output
go test -v ./...

# With coverage
go test -v -race -coverprofile=coverage.out ./...

# Generate coverage report
go tool cover -html=coverage.out -o coverage.html
```

### 2. Using Make for Testing

```bash
# Run tests with coverage
make test

# Run tests with coverage report
make test-coverage

# Run benchmarks
RUN_BENCHMARKS=true make test

# Run integration tests (if available)
RUN_INTEGRATION=true make test
```

### 3. Linting and Code Quality

```bash
# Run golangci-lint
golangci-lint run

# Or use Make
make lint

# Format code
go fmt ./...
goimports -w .

# Or use Make
make fmt

# Run vulnerability check
govulncheck ./...
```

## Development Workflow

### 1. Development Setup

```bash
# Set up development environment
./scripts/dev-setup.sh

# Or manually:
go mod download
go mod tidy
make build
```

### 2. Local Development

```bash
# Run with development config
go run ./cmd/cosmolet --config examples/dev-config.yaml --log-level debug

# Or use Make
make dev
```

### 3. Pre-commit Checks

```bash
# Run all checks before committing
make check

# This runs:
# - go fmt
# - go vet  
# - golangci-lint
# - tests
```

## IDE Configuration

### Visual Studio Code

Create `.vscode/settings.json`:

```json
{
    "go.toolsManagement.checkForUpdates": "local",
    "go.useLanguageServer": true,
    "go.lintOnSave": "package",
    "go.formatTool": "goimports",
    "go.testFlags": ["-v", "-race"],
    "go.buildFlags": ["-v"],
    "go.vetOnSave": "package"
}
```

Create `.vscode/launch.json` for debugging:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Launch Cosmolet",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/cmd/cosmolet",
            "args": [
                "--config",
                "${workspaceFolder}/examples/dev-config.yaml",
                "--log-level",
                "debug"
            ],
            "env": {}
        }
    ]
}
```

### GoLand/IntelliJ

1. Open the project directory
2. Go to **File → Settings → Go → Build Tags & Vendoring**
3. Set **Build tags**: `integration` (for integration tests)
4. Set **Environment**: `CGO_ENABLED=0` (for static builds)

## Build Optimization

### 1. Reduce Binary Size

```bash
# Use build flags to reduce size
go build \
  -ldflags="-w -s" \    # Remove debug info and symbol table
  -trimpath \           # Remove file system paths
  ./cmd/cosmolet

# Use UPX compression (optional)
upx --best bin/cosmolet
```

### 2. Enable Compiler Optimizations

```bash
# Build with optimizations
go build \
  -ldflags="-w -s" \
  -gcflags="-l=4" \     # Aggressive inlining
  -asmflags="-trimpath=$(PWD)" \
  ./cmd/cosmolet
```

### 3. Static Binary Build

```bash
# Build completely static binary
CGO_ENABLED=0 \
GOOS=linux \
GOARCH=amd64 \
go build \
  -a \
  -installsuffix cgo \
  -ldflags="-w -s -extldflags '-static'" \
  -o bin/cosmolet-static \
  ./cmd/cosmolet
```

## Troubleshooting

### Common Build Issues

#### 1. Module Download Issues

```bash
# Problem: "go: module ... not found"
# Solution: Check proxy settings
go env GOPROXY
go env GOPRIVATE

# Or disable proxy for private repos
export GOPRIVATE=github.com/your-org/*
```

#### 2. Version Conflicts

```bash
# Problem: "version X is not available"
# Solution: Update go.mod
go get -u ./...
go mod tidy
```

#### 3. Missing Dependencies

```bash
# Problem: "package ... not found"
# Solution: Download missing dependencies
go mod download
go mod verify
```

#### 4. Build Cache Issues

```bash
# Clear build cache
go clean -cache -modcache -i -r

# Rebuild everything
go build -a ./cmd/cosmolet
```

### Platform-Specific Issues

#### Linux

```bash
# If missing gcc for CGO (shouldn't be needed for this project)
sudo apt-get install build-essential

# If missing git
sudo apt-get install git
```

#### macOS

```bash
# If Xcode command line tools missing
xcode-select --install

# If using older macOS with Go modules issues
export GO111MODULE=on
```

#### Windows

```bash
# If using Git Bash, set proper line endings
git config --global core.autocrlf false

# If path issues
go env GOPATH
go env GOROOT
```

## Performance Considerations

### 1. Build Performance

```bash
# Use build cache
export GOCACHE=/tmp/go-build-cache

# Parallel builds
export GOMAXPROCS=4

# Use vendor directory for faster builds
go mod vendor
go build -mod=vendor ./cmd/cosmolet
```

### 2. Runtime Performance

```bash
# Build with race detector for testing
go build -race ./cmd/cosmolet

# Profile builds
go build -cpuprofile=cpu.prof ./cmd/cosmolet
```

## CI/CD Integration

### GitHub Actions

The project includes `.github/workflows/ci.yaml`:

```yaml
name: CI
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    - name: Build
      run: make build
    - name: Test
      run: make test
```

### GitLab CI

Example `.gitlab-ci.yml`:

```yaml
image: golang:1.21

stages:
  - build
  - test

build:
  stage: build
  script:
    - make build
  artifacts:
    paths:
      - bin/

test:
  stage: test
  script:
    - make test
  coverage: '/coverage: \d+\.\d+% of statements/'
```

## Summary

**Quick Start Commands:**

```bash
# 1. Setup
git clone <repo-url>
cd cosmolet
go mod download

# 2. Build
make build

# 3. Test
make test

# 4. Run locally
make dev

# 5. Build Docker image
make docker-build

# 6. Deploy
helm install cosmolet charts/cosmolet
```

For any build issues, check:
1. Go version (must be 1.21+)
2. Module dependencies (`go mod tidy`)
3. Environment variables (`go env`)
4. Build cache (`go clean -cache`)

The build process follows standard Go conventions and should work across all supported platforms.

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the GNU Affero General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

---

⭐ If this project helps you, please consider giving it a star on GitHub!
