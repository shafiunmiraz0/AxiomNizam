# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.25 AS builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    postgresql-client \
    libpq-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Set Go environment variables
ENV GOFLAGS=-mod=mod
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=off
ENV GO111MODULE=on
ENV CGO_ENABLED=1

# Copy go.mod first for better layer caching
COPY go.mod go.sum* ./

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

# Copy source code
COPY . .

# Run go mod tidy to ensure consistency
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod tidy

# Build the application with version info
RUN --mount=type=cache,target=/go/pkg/mod \
    go build -o axiomnizam . 2>&1 || (echo "Build failed with exit code $?" && exit 1)

# Build CLI tool (static binary)
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o axiomnizamctl \
    ./cmd/axiomnizamctl 2>&1 || \
    (echo "CLI build failed" && exit 1)

# ── Runtime stage ────────────────────────────────────────────────────────────
# Phase 12: Hardened runtime — non-root user, no shell, minimal attack surface
FROM debian:bookworm-slim

# OCI image labels (Phase 12 — supply chain metadata)
LABEL org.opencontainers.image.title="AxiomNizam"
LABEL org.opencontainers.image.description="AxiomNizam Data Control Plane"
LABEL org.opencontainers.image.vendor="AxiomNizam"
LABEL org.opencontainers.image.source="https://github.com/AxiomNizam/AxiomNizam"

WORKDIR /app

# Install only runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libpq5 \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user (Phase 12 — principle of least privilege)
RUN groupadd --gid 1000 axiomnizam \
    && useradd --uid 1000 --gid axiomnizam --shell /bin/false --create-home axiomnizam

# Copy binaries from builder
COPY --from=builder /app/axiomnizam /app/axiomnizam
COPY --from=builder /app/axiomnizamctl /usr/local/bin/axiomnizamctl

# Copy frontend templates and static assets
COPY --from=builder /app/internal/frontend/templates /app/internal/frontend/templates

# Set ownership and permissions
RUN chown -R axiomnizam:axiomnizam /app \
    && chmod +x /app/axiomnizam /usr/local/bin/axiomnizamctl

# Create data directories with proper permissions
RUN mkdir -p /data/certs /data/storage /data/raft /data/query_logs \
    && chown -R axiomnizam:axiomnizam /data

# Switch to non-root user
USER axiomnizam

EXPOSE 8000

# Health check (uses curl -sk for self-signed TLS certs)
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -sk -f https://localhost:8000/health || exit 1

CMD ["/app/axiomnizam"]
