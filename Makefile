.PHONY: build test clean help \
	lint fmt vet lint-install \
	security vuln vuln-install gosec-install \
	coverage coverage-html \
	readme-regen \
	build-release build-multi \
	docker-build docker-push docker-buildx docker-load \
	helm-lint helm-package \
	deps ci

# -------- Repo / Build Vars --------
BINARY_NAME        ?= cosmolet
CMD_PATH           ?= ./cmd/cosmolet
PKG                ?= ./...
DIST               ?= dist
BIN_DIR            ?= bin

DOCKER_REGISTRY    ?= docker.io
DOCKER_REPOSITORY  ?= cosmolet/cosmolet
IMAGE              := $(DOCKER_REGISTRY)/$(DOCKER_REPOSITORY)

VERSION            ?= $(shell git describe --tags --dirty --always)
GIT_COMMIT         ?= $(shell git rev-parse HEAD)
BUILD_DATE         ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Multi-arch (binaries & containers)
OS_ARCHES          ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
PLATFORMS          ?= linux/amd64,linux/arm64
IMAGE_TAGS         ?= $(VERSION)               # comma-separated allowed (e.g. v1.2.3,latest)

# Linker flags
LDFLAGS            ?= -w -s -X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)

# Tool detection
GOLANGCI_LINT      := $(shell command -v golangci-lint 2>/dev/null)
GOVULNCHECK        := $(shell command -v govulncheck 2>/dev/null)
GOSEC              := $(shell command -v gosec 2>/dev/null)

# =========================================================
# Core (kept from your original Makefile, unchanged behavior)
# =========================================================

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME) $(VERSION)"
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build \
		-ldflags="$(LDFLAGS)" \
		-o $(BIN_DIR)/$(BINARY_NAME) \
		$(CMD_PATH)

## test: Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=$(DIST)/coverage.out $(PKG)

## docker-build: Build Docker image (single-arch)
docker-build:
	@echo "Building Docker image"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE):$(VERSION) \
		.

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR)/ $(DIST)/

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

# ==================
# New/Enhanced Tasks
# ==================

deps:
	go mod download

fmt:
	@echo "Formatting code..."
	@files=$$(gofmt -s -l .); if [ -n "$$files" ]; then echo "$$files" | xargs -r gofmt -s -w; fi

vet:
	go vet $(PKG)

lint-install:
ifndef GOLANGCI_LINT
	@echo "Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
endif

## lint: Run linters (golangci-lint + fmt/vet)
lint: lint-install
	golangci-lint run ./...
	$(MAKE) fmt
	$(MAKE) vet

vuln-install:
ifndef GOVULNCHECK
	@echo "Installing govulncheck..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
endif

gosec-install:
ifndef GOSEC
	@echo "Installing gosec..."
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
endif

## security: Security checks (govulncheck + gosec)
security: vuln-install gosec-install
	govulncheck $(PKG)
	gosec -severity medium -confidence medium ./...

coverage:
	@go tool cover -func=$(DIST)/coverage.out | tail -n 1

coverage-html:
	@mkdir -p $(DIST)
	@go tool cover -html=$(DIST)/coverage.out -o $(DIST)/coverage.html
	@echo "Wrote $(DIST)/coverage.html"

