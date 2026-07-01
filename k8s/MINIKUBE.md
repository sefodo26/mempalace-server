# Running MemPalace on minikube

This guide gets a full MemPalace stack — the Go server plus PostgreSQL
(pgvector + Apache AGE) — running on a local [minikube](https://minikube.sigs.k8s.io)
cluster. It's meant for local development and kicking the tires on the
Kubernetes manifests; for a real deployment see the production manifests in
[`k8s/`](.) and the [README](../README.md#deploy-to-kubernetes).

The fastest path is the bundled script:

```bash
./k8s/minikube-setup.sh
```

The rest of this document explains what that script does and how to do it by
hand.

---

## What you get

```
                 host machine
   ┌───────────────────────────────────────┐
   │  Ollama (embeddinggemma) :11434        │
   └───────────────▲───────────────────────┘
                   │ host.minikube.internal
   ┌───────────────┼───────────────────────┐
   │  minikube     │            namespace: mempalace
   │      ┌────────┴─────────┐   ┌────────────────────────┐
   │      │ Deployment       │   │ StatefulSet            │
   │      │ mempalace (Go)   │──►│ mempalace-db           │
   │      │ :8000  ▲         │   │ Postgres + pgvector    │
   │      └────────┼─────────┘   │ + Apache AGE  :5432    │
   │   Service mempalace :80     └────────────────────────┘
   └───────────────┼───────────────────────┘
                   │ kubectl port-forward
              localhost:8000
```

- **mempalace** — the stateless Go server (1 replica).
- **mempalace-db** — PostgreSQL with the `pgvector` and `age` extensions,
  backed by a 10Gi PersistentVolumeClaim.
- The embedding API is **not** run in the cluster. The server reaches an
  embedding API on your host machine (Ollama by default).

---

## Prerequisites

| Tool | Notes |
| --- | --- |
| [minikube](https://minikube.sigs.k8s.io/docs/start/) | with the Docker driver (default on most machines) |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | v1.27+ |
| [Ollama](https://ollama.com) (or any OpenAI-compatible embedding API) | on the host, port `11434` |

Pull the embedding model and serve it **on all interfaces** so the cluster can
reach it through `host.minikube.internal`:

```bash
ollama pull embeddinggemma
OLLAMA_HOST=0.0.0.0 ollama serve
```

> If Ollama only listens on `127.0.0.1`, pods inside minikube cannot connect to
> it. Binding to `0.0.0.0` is what makes `host.minikube.internal` work.

---

## Quick start (scripted)

```bash
# from the repo root
./k8s/minikube-setup.sh
```

The script is idempotent — run it again to rebuild the images and re-apply
after a code change. On success it prints your generated API key and the
port-forward command.

```bash
# expose the server on localhost (keep this running)
kubectl -n mempalace port-forward svc/mempalace 8000:80

# in another terminal — health check needs no auth
curl http://localhost:8000/mp/mcp/health
```

Tear everything down (keeps the cluster itself):

```bash
./k8s/minikube-setup.sh --delete
```

---

## What the script does (manual walkthrough)

If you'd rather run the steps yourself, here they are.

### 1. Start minikube

```bash
minikube start
```

### 2. Build both images straight into minikube

`minikube image build` builds inside the cluster's container runtime, so there
is **no registry and no `docker push`**. The PostgreSQL image compiles Apache
AGE from source, so the first build takes a few minutes. The image also ships an
init script (`k8s/postgres/init-extensions.sh`) that runs `CREATE EXTENSION
vector` (and `age`) the first time the data directory is initialised — the
server registers the `vector` type on its very first connection, so the type has
to exist before the app boots. On an already-initialised volume it is a no-op.

```bash
minikube image build -t mempalace-postgres:local k8s/postgres
minikube image build -t mempalace-go:local       server
```

### 3. Create the namespace and a Secret

The production `k8s/secret.yaml` is a template with placeholders. For local use,
generate fresh credentials and create the Secret imperatively. Hex output is
used for the password because it ends up inside the connection URL.

```bash
kubectl apply -f k8s/namespace.yaml

DB_PASS=$(openssl rand -hex 24)
API_KEY=$(openssl rand -hex 32)

kubectl -n mempalace create secret generic mempalace-secrets \
  --from-literal=api-key="$API_KEY" \
  --from-literal=db-password="$DB_PASS" \
  --from-literal=db-url="postgres://mempalace:${DB_PASS}@mempalace-db.mempalace.svc.cluster.local:5432/mempalace"

echo "Your API key: $API_KEY"
```

### 4. Apply the manifests through the local overlay

Everything — the production manifests plus the local-only adjustments — is
applied in one pass through the kustomize overlay in
[`k8s/minikube/`](minikube/). Doing it in a single apply (rather than
apply-then-`set image`-then-`patch`) is important: each pod is created **once**
with the correct locally-built image, so nothing ever gets wedged pulling the
placeholder registry image the base manifests reference.

```bash
# LoadRestrictionsNone lets the overlay reference the sibling base manifests in
# k8s/ (kustomize forbids '../' paths by default).
kubectl kustomize --load-restrictor LoadRestrictionsNone k8s/minikube \
  | kubectl apply -f -
```

The overlay ([`kustomization.yaml`](minikube/kustomization.yaml)) pulls in the
production manifests (minus the Ingress, the template Secret, and the unused
legacy `pvc.yaml`), rewrites both images to the `:local` tags, and applies two
strategic-merge patches that change only what's needed for a local cluster:

- **`statefulset-patch.yaml`** — `imagePullPolicy: IfNotPresent` and
  `args: ["postgres", "-c", "shared_preload_libraries=age"]`. **Apache AGE must
  be preloaded**, or the entity graph fails to load. Note this is `args`, not
  `command`: in Kubernetes `command` overrides the image *entrypoint*
  (`docker-entrypoint.sh`), which is what drops root privileges to the
  `postgres` user — override it and Postgres refuses to start as root. `args`
  overrides only the default CMD, so the entrypoint still runs. (docker-compose's
  `command:` maps to CMD, which is why the same flags work there verbatim.)
- **`deployment-patch.yaml`** — `imagePullPolicy: IfNotPresent`,
  `replicas: 1`, `EMBED_API_URL=http://host.minikube.internal:11434/v1`, and
  `ENABLE_REST_API=true` so you can also poke the REST API with curl.

### 5. Wait for it to come up

```bash
kubectl -n mempalace rollout status statefulset/mempalace-db
kubectl -n mempalace rollout status deployment/mempalace
```

---

## Using it

```bash
kubectl -n mempalace port-forward svc/mempalace 8000:80
```

| Endpoint | Method | Auth |
| --- | --- | --- |
| `http://localhost:8000/mp/mcp/health` | GET | none |
| `http://localhost:8000/mp/mcp` | POST | `Authorization: Bearer <API_KEY>` |
| `http://localhost:8000/mp/api/v1/...` | REST | `Authorization: Bearer <API_KEY>` (enabled in the patch) |

Retrieve the generated API key any time:

```bash
kubectl -n mempalace get secret mempalace-secrets \
  -o jsonpath='{.data.api-key}' | base64 --decode; echo
```

Point your MCP client at `http://localhost:8000/mp/mcp` with that key — see
[MCP_USAGE.md](../MCP_USAGE.md).

---

## Troubleshooting

**Server pod is `CrashLoopBackOff` or not ready**

```bash
kubectl -n mempalace logs deploy/mempalace
```

- *Embedding API errors / connection refused* — Ollama isn't reachable. Confirm
  it runs with `OLLAMA_HOST=0.0.0.0 ollama serve` and that the model is pulled
  (`ollama list` should show `embeddinggemma`).

- *`ping database: vector type not found in the database`* — the `vector`
  extension was never created in the `mempalace` database. Normally the image's
  init script handles this on a fresh volume; if you are reusing a volume that
  predates the script, create it by hand and restart the server:

  ```bash
  kubectl -n mempalace exec mempalace-db-0 -- \
    psql -U mempalace -d mempalace -c \
    "CREATE EXTENSION IF NOT EXISTS vector; CREATE EXTENSION IF NOT EXISTS age;"
  kubectl -n mempalace rollout restart deployment/mempalace
  ```

**Database pod won't start, or AGE errors in the server logs**

```bash
kubectl -n mempalace logs statefulset/mempalace-db
```

- Make sure the StatefulSet picked up the `shared_preload_libraries=age` patch:

  ```bash
  kubectl -n mempalace get statefulset mempalace-db \
    -o jsonpath='{.spec.template.spec.containers[0].command}'; echo
  ```

**`ErrImagePull` / `ImagePullBackOff`**

The local images weren't built into minikube, or the pull policy wasn't
patched. Re-run `./k8s/minikube-setup.sh`, or verify the images exist:

```bash
minikube image ls | grep mempalace
```

**Start over completely**

```bash
./k8s/minikube-setup.sh --delete   # remove just the app
minikube delete                    # or nuke the whole cluster
```
