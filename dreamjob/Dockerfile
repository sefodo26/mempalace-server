# ============================================================================
# MemPalace "Dream" consolidation job — standalone subproject image
#
# Its own Go module (mempalace/dreamjob), sharing the storage/embed/consolidate
# code with the server via the mempalace/core module (replace ../core). Runs to
# completion as a Kubernetes CronJob, scans the palace's rooms, and writes
# near-duplicate merge PROPOSALS to room_merge_candidates. It never serves HTTP
# and never mutates the palace — proposals are reviewed via MCP and applied by a
# human/LLM.
#
# Build context is the REPO ROOT (needs both core/ and dreamjob/):
#   docker build -f dreamjob/Dockerfile -t your-registry.example.com/mempalace-dreamjob:latest .
#   docker push your-registry.example.com/mempalace-dreamjob:latest
# ============================================================================

# ── Stage 1: build ──────────────────────────────────────────────────────────
FROM golang:1.27-alpine AS builder

# ca-certificates needed for TLS calls (embed API, Postgres TLS)
RUN apk add --no-cache ca-certificates git

WORKDIR /src

# core is a local module dependency (replace ../core); copy it first for caching.
COPY core/ ./core/
COPY dreamjob/go.mod dreamjob/go.sum ./dreamjob/
RUN cd dreamjob && go mod download

COPY dreamjob/ ./dreamjob/

RUN cd dreamjob && CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /dreamjob \
    .

# ── Stage 2: runtime ────────────────────────────────────────────────────────
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

# Non-root user
RUN addgroup -S app && adduser -S -G app app
USER app

COPY --from=builder /dreamjob /usr/local/bin/dreamjob

# No port — this is a batch job, not a server.
ENTRYPOINT ["/usr/local/bin/dreamjob"]
