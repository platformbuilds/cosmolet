# Multi-stage build for production efficiency
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /src

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o cosmolet \
    ./cmd/cosmolet/

# Production image
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    frr \
    frr-pythontools \
    iproute2 \
    iptables \
    bash \
    curl \
    tcpdump \
    && rm -rf /var/cache/apk/*

# Create frr user and directories
RUN addgroup -S frr && adduser -S -G frr frr \
    && mkdir -p /var/run/frr /var/log/frr /etc/frr \
    && chown -R frr:frr /var/run/frr /var/log/frr /etc/frr

# Create cosmolet directories
RUN mkdir -p /etc/cosmolet /var/lib/cosmolet

# Copy binary from builder
COPY --from=builder /src/cosmolet /usr/local/bin/cosmolet

# Set proper permissions
RUN chmod +x /usr/local/bin/cosmolet

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8081/healthz || exit 1

# Expose ports
EXPOSE 8080 8081 179

# Labels for metadata
LABEL maintainer="cosmolet-team" \
      version="1.0.0" \
      description="BareMetal Kubernetes BGP Service Controller"

# Default command
ENTRYPOINT ["/usr/local/bin/cosmolet"]
CMD ["--config=/etc/cosmolet/config.yaml"]
