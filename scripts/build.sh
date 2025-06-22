#!/bin/bash
# Build script for Cosmolet
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --dirty --always 2>/dev/null || echo 'dev')}"
echo "Building cosmolet ${VERSION}..."

mkdir -p bin
CGO_ENABLED=0 go build \
    -ldflags="-w -s -X main.Version=${VERSION}" \
    -o bin/cosmolet \
    ./cmd/cosmolet

echo "Build complete: bin/cosmolet"
