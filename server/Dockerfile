# ============================================================================
# MemPalace Go server image.
#
# The server module (mempalace/server) shares storage/embed/config with the
# dreamjob via the mempalace/core module (replace ../core), so the build context
# is the REPO ROOT (needs both core/ and server/):
#   docker build -f server/Dockerfile -t your-registry.example.com/mempalace-go:latest .
# ============================================================================

# ── Stage 1: build ──────────────────────────────────────────────────────────
FROM golang:1.27-alpine AS builder

# ca-certificates needed for TLS calls (embed API, Postgres TLS)
RUN apk add --no-cache ca-certificates git

WORKDIR /src

# core is a local module dependency (replace ../core); copy it first for caching.
COPY core/ ./core/
COPY server/go.mod server/go.sum ./server/
RUN cd server && go mod download

COPY server/ ./server/

RUN cd server && CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /mempalace \
    ./cmd/mempalace

# ── Stage 2: runtime ────────────────────────────────────────────────────────
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

# Non-root user
RUN addgroup -S app && adduser -S -G app app
USER app

COPY --from=builder /mempalace /usr/local/bin/mempalace

EXPOSE 8000

ENTRYPOINT ["/usr/local/bin/mempalace"]
