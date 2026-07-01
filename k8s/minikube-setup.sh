#!/usr/bin/env bash
# ============================================================================
# MemPalace — one-shot minikube setup
# ============================================================================
# Brings up a complete MemPalace stack on a local minikube cluster:
#
#   1. starts minikube (if not already running)
#   2. builds the two images straight into minikube's container runtime
#      (no registry, no push)
#   3. creates the `mempalace` namespace and a Secret with fresh credentials
#   4. applies the local kustomize overlay in k8s/minikube/
#   5. waits for both workloads to become ready
#   6. prints how to reach the server and the generated API key
#
# Usage:
#   ./k8s/minikube-setup.sh            # set up / update the stack
#   ./k8s/minikube-setup.sh --delete   # tear the stack down (keeps the cluster)
#
# Requirements: minikube, kubectl, and a Docker-compatible driver.
# An embedding API (e.g. Ollama with `embeddinggemma`) should be reachable on
# the host at port 11434, listening on 0.0.0.0 (OLLAMA_HOST=0.0.0.0 ollama serve).
# ============================================================================
set -euo pipefail

# --- Resolve paths so the script works from any working directory ----------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PATCH_DIR="$SCRIPT_DIR/minikube"

NAMESPACE="mempalace"
GO_IMAGE="mempalace-go:local"
DB_IMAGE="mempalace-postgres:local"

# --- Pretty logging --------------------------------------------------------
info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()    { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
warn()  { printf '\033[1;33m  !\033[0m %s\n' "$*"; }
die()   { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

# --- Preflight -------------------------------------------------------------
command -v minikube >/dev/null 2>&1 || die "minikube is not installed. See https://minikube.sigs.k8s.io/docs/start/"
command -v kubectl  >/dev/null 2>&1 || die "kubectl is not installed."

# --- Teardown mode ---------------------------------------------------------
if [[ "${1:-}" == "--delete" || "${1:-}" == "down" ]]; then
  info "Deleting the MemPalace stack (namespace '$NAMESPACE')…"
  kubectl delete namespace "$NAMESPACE" --ignore-not-found
  ok "Done. The minikube cluster itself is left running."
  echo "To remove the whole cluster: minikube delete"
  exit 0
fi

# --- 1. Make sure minikube is running --------------------------------------
if ! minikube status >/dev/null 2>&1; then
  info "Starting minikube…"
  minikube start
else
  ok "minikube is already running."
fi

# --- 2. Build both images directly into minikube ---------------------------
# `minikube image build` builds inside the cluster's runtime, so no registry
# and no `docker push` are needed. The kustomize overlay references these tags.
info "Building PostgreSQL image (pgvector + Apache AGE) — this is the slow one…"
minikube image build -t "$DB_IMAGE" "$SCRIPT_DIR/postgres"
ok "Built $DB_IMAGE"

info "Building MemPalace Go server image…"
minikube image build -t "$GO_IMAGE" "$REPO_ROOT/server"
ok "Built $GO_IMAGE"

# --- 3. Namespace ----------------------------------------------------------
info "Ensuring namespace '$NAMESPACE' exists…"
kubectl apply -f "$SCRIPT_DIR/namespace.yaml" >/dev/null
ok "Namespace ready."

# --- 4. Secret (generate once, keep on re-runs) ----------------------------
if kubectl -n "$NAMESPACE" get secret mempalace-secrets >/dev/null 2>&1; then
  ok "Secret 'mempalace-secrets' already exists — keeping current credentials."
  API_KEY="$(kubectl -n "$NAMESPACE" get secret mempalace-secrets -o jsonpath='{.data.api-key}' | base64 --decode)"
else
  info "Creating Secret with freshly generated credentials…"
  # hex output is URL-safe — important because the password goes into db-url.
  DB_PASS="$(openssl rand -hex 24)"
  API_KEY="$(openssl rand -hex 32)"
  DB_URL="postgres://${NAMESPACE}:${DB_PASS}@mempalace-db.${NAMESPACE}.svc.cluster.local:5432/${NAMESPACE}"

  kubectl -n "$NAMESPACE" create secret generic mempalace-secrets \
    --from-literal=api-key="$API_KEY" \
    --from-literal=db-password="$DB_PASS" \
    --from-literal=db-url="$DB_URL"
  ok "Secret created."
fi

# --- 5. Apply everything through the local kustomize overlay ----------------
# The overlay (k8s/minikube/kustomization.yaml) renders the production manifests
# with the local-only adjustments — locally built images, AGE preload, host-
# facing embedding URL, single replica, REST API on — and applies them in a
# single pass. Applying it all at once (rather than apply-then-set-image-then-
# patch) means each pod is created ONCE with the correct image, so nothing ever
# gets wedged pulling the placeholder registry image.
info "Applying manifests via the minikube overlay…"
# LoadRestrictionsNone lets the overlay reference the sibling base manifests in
# k8s/ (kustomize forbids '../' paths by default). Rendering then piping to
# apply is equivalent to `apply -k`, which has no flag to relax that.
kubectl kustomize --load-restrictor LoadRestrictionsNone "$PATCH_DIR" | kubectl apply -f -

# --- 6. Wait for readiness -------------------------------------------------
info "Waiting for PostgreSQL to become ready…"
kubectl -n "$NAMESPACE" rollout status statefulset/mempalace-db --timeout=300s

info "Waiting for the MemPalace server to become ready…"
kubectl -n "$NAMESPACE" rollout status deployment/mempalace --timeout=180s

# --- Done — print access instructions --------------------------------------
cat <<EOF

$(ok "MemPalace is up on minikube.")

Reach it with a port-forward (leave this running in its own terminal):

    kubectl -n $NAMESPACE port-forward svc/mempalace 8000:80

Then, in another terminal:

    # Health check (no auth)
    curl http://localhost:8000/mp/mcp/health

    # MCP endpoint (needs the API key)
    #   POST http://localhost:8000/mp/mcp
    #   Authorization: Bearer <API_KEY>

Your generated API key (MCP_API_KEY):

    $API_KEY

Useful commands:
    kubectl -n $NAMESPACE get pods
    kubectl -n $NAMESPACE logs deploy/mempalace -f
    ./k8s/minikube-setup.sh --delete     # tear down the stack

EOF
