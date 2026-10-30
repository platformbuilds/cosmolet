.PHONY: build test clean docker-build docker-push helm-lint helm-package help

# Build variables
BINARY_NAME := cosmolet
DOCKER_REGISTRY := docker.io
DOCKER_REPOSITORY := cosmolet/cosmolet
# VERSION: prefer the nearest git tag (semver release). `git describe --tags
# --dirty` is empty when the tree has no tags at all, so fall back to an
# explicit "unreleased-<commit>" marker rather than a bare short hash that
# could be mistaken for a release version (Finding 32).
GIT_DESCRIBE := $(shell git describe --tags --dirty 2>/dev/null)
ifneq ($(GIT_DESCRIBE),)
  VERSION := $(GIT_DESCRIBE)
else
  VERSION := unreleased-$(shell git rev-parse --short HEAD 2>/dev/null)
endif
GIT_COMMIT := $(shell git rev-parse HEAD)
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME) $(VERSION)"
	CGO_ENABLED=0 go build \
		-ldflags="-w -s -X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildDate=$(BUILD_DATE)" \
		-o bin/$(BINARY_NAME) \
		./cmd/cosmolet

## test: Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image"
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_REGISTRY)/$(DOCKER_REPOSITORY):$(VERSION) \
		.

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
