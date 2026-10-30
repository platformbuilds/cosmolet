#!/bin/bash
# Build script for Cosmolet
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --dirty --always 2>/dev/null || echo 'dev')}"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo 'unknown')}"
BUILD_DATE="${BUILD_DATE:-$(date -u +'%Y-%m-%dT%H:%M:%SZ')}"
echo "Building cosmolet ${VERSION}..."

mkdir -p bin
CGO_ENABLED=0 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
    -o bin/cosmolet \
    ./cmd/cosmolet

echo "Build complete: bin/cosmolet"
