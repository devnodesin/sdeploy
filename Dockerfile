# Multi-stage Dockerfile for SDeploy
# Builds a minimal, production-ready container for the sdeploy app.

# ---- Build stage ----
# Use the official Go image to compile a static binary.
FROM golang:1.24 AS builder

WORKDIR /build

# Copy Go module files first for better layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Copy application source. All packages currently live under cmd/.
COPY cmd/ ./cmd/

# Build a fully static binary.
#   CGO_ENABLED=0      -> static binary, no libc dependency at runtime
#   -ldflags="-w -s"   -> strip debug info to shrink the image
#   -a                 -> force rebuild of all packages for a clean static build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -a \
    -o sdeploy \
    ./cmd/sdeploy

# ---- Runtime stage ----
# Debian slim: git/ssh compatibility without Alpine's musl quirks.
FROM debian:bookworm-slim

# Core runtime dependencies:
#   ca-certificates : HTTPS (SMTP, webhooks, git over https)
#   git             : clone / pull repositories
#   openssh-client  : git over ssh
#   wget            : (kept for debugging / fallback)
#   rsync           : sync deployed files
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
        openssh-client \
        wget \
        rsync && \
    rm -rf /var/lib/apt/lists/*

# Install Node.js 22.x LTS. Project configs can run build steps such as
# `npm install && npm run build`, so Node is required at runtime.
# Uses the signed NodeSource apt repository instead of piping a script
# straight into bash.
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl gnupg && \
    install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
        | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
        > /etc/apt/sources.list.d/nodesource.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends nodejs && \
    apt-get purge -y --auto-remove gnupg && \
    rm -rf /var/lib/apt/lists/*

# Use UID/GID 1000 for the sdeploy runtime user/group. This is the standard
# non-root ID used in many dev/preview environments (e.g. host-mounted volumes
# and CI runners), so files written by SDeploy keep consistent ownership.
#   - group sdeploy  (GID 1000)
#   - user  sdeploy  (UID 1000, primary group sdeploy)
RUN groupadd -g 1000 -r sdeploy && \
    useradd  -u 1000 -r -g sdeploy -s /usr/sbin/nologin -d /home/sdeploy sdeploy

# Prepare directories and give the runtime user ownership.
# Default log path is /var/log/sdeploy (see config.go Defaults.LogPath).
RUN mkdir -p /var/log/sdeploy /home/sdeploy && \
    chown -R sdeploy:sdeploy /var/log/sdeploy /home/sdeploy

# Copy the static binary from the build stage.
COPY --from=builder /build/sdeploy /usr/local/bin/sdeploy
RUN chmod 0755 /usr/local/bin/sdeploy

# Entrypoint applies a permissive umask (022) so generated site files are
# created world/group readable (0644 / 0755) for Caddy to serve.
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh

WORKDIR /home/sdeploy

# Default webhook daemon port (see config.go Defaults.Port).
EXPOSE 8080

# Health check: the daemon must be listening and answering on 8080.
# Any HTTP response (the handler returns 405 for GET /) proves the service
# is up; only a connection failure is treated as unhealthy.
# curl is installed above; -f is intentionally omitted so error statuses
# still count as "responding".
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -s -o /dev/null --max-time 2 http://localhost:8080/ || exit 1

# Run as the unprivileged sdeploy user (UID/GID 1000).
USER sdeploy

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["-c", "/etc/sdeploy.conf"]
