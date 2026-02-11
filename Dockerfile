# Multi-stage Dockerfile for SDeploy
# This Dockerfile builds a minimal, production-ready container for the sdeploy app

# Build stage: Use official Go image to compile the binary
FROM golang:1.24 AS builder

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/

# Build static binary with all optimizations
# CGO_ENABLED=0 creates a fully static binary
# -ldflags="-w -s" strips debug information to reduce size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -a \
    -installsuffix cgo \
    -o sdeploy \
    ./cmd/sdeploy

# Runtime stage: Use Debian slim for compatibility with git
# Alpine has network issues in some CI environments, and distroless doesn't include git
FROM debian:bookworm-slim

# Install runtime dependencies
# ca-certificates: Required for HTTPS connections (SMTP, webhooks)
# git: Required for git operations (clone, pull)
# wget: Required for health check
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates git wget openssh-client && \
    rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN groupadd -g 1000 sdeploy && \
    useradd -m -u 1000 -g sdeploy -s /bin/bash sdeploy

# Create directories with proper permissions
RUN mkdir -p /var/log/sdeploy && \
    chown -R sdeploy:sdeploy /var/log/sdeploy

# Copy binary from builder
COPY --from=builder /build/sdeploy /usr/local/bin/sdeploy

# Set ownership and make executable
RUN chown sdeploy:sdeploy /usr/local/bin/sdeploy && \
    chmod +x /usr/local/bin/sdeploy

# Switch to non-root user
USER sdeploy

# Set working directory
WORKDIR /home/sdeploy

# Expose default port
EXPOSE 8080

# Health check to ensure the service is responding
# Use wget with server response option to check if the port is accessible
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --timeout=2 --server-response http://localhost:8080/ -O /dev/null 2>&1 | grep -q "HTTP" || exit 1

# Default command
# Users should mount a config file at /etc/sdeploy.conf or override CMD
ENTRYPOINT ["/usr/local/bin/sdeploy"]
CMD ["-c", "/etc/sdeploy.conf"]
