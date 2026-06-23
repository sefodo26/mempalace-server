# MemPalace Server

A fast, self-hosted memory server for AI agents — written in Go.

This is the **server** for [MemPalace](https://github.com/MemPalace/mempalace).
It gives your AI agents a long-term memory they can search, grow, and connect
over time. Agents talk to it over the [MCP](https://modelcontextprotocol.io)
protocol via simple HTTP.

> Looking for the main project, docs, and clients?
> 👉 **https://github.com/MemPalace/mempalace**

---

## What it does

MemPalace stores memories like a real memory palace:

- **Wings** → broad areas (e.g. a project)
- **Rooms** → topics inside a wing
- **Drawers** → the actual memories

On top of that, it builds connections between memories:

- **Semantic search** — find memories by meaning, not just keywords.
- **Knowledge graph** — facts with time (what was true, and when).
- **Entity graph** — people, things, and how they relate.
- **Tunnels** — links between related memories.

Agents use all of this through ready-made MCP tools (search, add, recall,
traverse, and more).

---

## Why use it

- **Fast & small** — a single Go binary, low memory, quick startup.
- **Self-hosted & private** — your data stays in your own PostgreSQL.
- **Multilingual** — uses the `embeddinggemma` model (100+ languages out of the box).
- **Bring your own embeddings** — works with any OpenAI-compatible API
  (Ollama, LM Studio, LocalAI, OpenAI, …).
- **Multi-tenant** — keep many users or projects fully separated.
- **Production-ready** — Docker Compose for local use, Kubernetes manifests for deploy.
- **Secure by default** — every request needs an API key.

---

## How it works

```
┌─────────────┐     MCP over HTTP      ┌──────────────────┐
│  AI agent   │ ─────────────────────► │  MemPalace Server│
│ (MCP client)│                        │      (Go)        │
└─────────────┘                        └────────┬─────────┘
                                                 │
                          ┌──────────────────────┼───────────────────────┐
                          ▼                       ▼                       ▼
                   ┌────────────┐         ┌──────────────┐        ┌──────────────┐
                   │ PostgreSQL │         │  Embedding   │        │ Apache AGE   │
                   │ + pgvector │         │  API (Ollama)│        │ (entity graph)│
                   └────────────┘         └──────────────┘        └──────────────┘
```

---

## Requirements

- **Docker** and **Docker Compose** (easiest path), or **Go 1.26+** to build from source.
- An **embedding API**. The default config expects [Ollama](https://ollama.com)
  with the `embeddinggemma` model:

  ```bash
  ollama pull embeddinggemma
  ```

---

## Quick start (Docker Compose)

This is the fastest way to try it. It starts PostgreSQL (with pgvector + AGE)
and the MemPalace server together.

**1. Get the code**

```bash
git clone https://github.com/sefodo26/mempalace-server.git
cd mempalace-server
```

**2. Make sure your embedding API is running**

```bash
ollama pull embeddinggemma
ollama serve
```

**3. Point the server at your embedding API**

Open `docker-compose.yml` and set `EMBED_API_URL`.

- Ollama runs on your host machine → use `http://host.docker.internal:11434/v1`
  (macOS / Windows). On Linux, use your host IP.
- Ollama runs somewhere else → use that address.

While you're there, **change `MCP_API_KEY`** to your own secret.

**4. Start everything**

```bash
docker compose up --build
```

The server is now live at **http://localhost:8000**.

**5. Check it works**

```bash
curl http://localhost:8000/mp/mcp/health
```

---

## Connect an AI agent

The MCP endpoint is:

```
POST http://localhost:8000/mp/mcp
Authorization: Bearer <your MCP_API_KEY>
```

Point your MCP client (for example the [MemPalace](https://github.com/MemPalace/mempalace)
client) at this URL with your API key. See the main project for client setup.

👉 For a full guide — client config, the protocol step by step, and every
available tool — see **[MCP_USAGE.md](MCP_USAGE.md)**.

---

## Access control: read vs write keys

The server supports two kinds of API key:

| Key | Env variable | Can do |
| --- | --- | --- |
| **Full** | `MCP_API_KEY` | Everything — read **and** write |
| **Read-only** | `MCP_API_KEY_READONLY` | Read only — search, list, get, … |

The read-only key is **optional**. Set it to give some clients (dashboards,
read-only agents, monitoring) safe access that cannot change anything.

- A read-only key may call non-mutating operations only. Any write — add,
  update, delete, create tunnel, add/invalidate facts, change settings — is
  rejected (`403` for REST, JSON-RPC error `-32003` for MCP).
- The two keys **must be different**; the server refuses to start otherwise.
- This applies to **both** the MCP endpoint and the optional REST API.

```bash
# Full access
MCP_API_KEY="super-secret-write-key"
# Optional read-only access
MCP_API_KEY_READONLY="another-secret-read-key"
```

---

## Optional: plain REST/JSON API

Most users only need MCP. But if you want to talk to the palace from a normal
script, a `curl` command, or a non-MCP app, you can turn on a simple REST API.

It is **off by default**. Enable it with:

```
ENABLE_REST_API=true
```

It uses the **same `MCP_API_KEY`** for auth and lives under `/mp/api/v1`.
It is a thin wrapper over the same logic as MCP — same validation, same storage.

| Method | Path | What it does |
| --- | --- | --- |
| `GET` | `/mp/api/v1/health` | Health check |
| `GET` | `/mp/api/v1/status` | Palace overview (counts) |
| `GET` | `/mp/api/v1/wings` | List wings |
| `GET` | `/mp/api/v1/rooms?wing=` | List rooms |
| `GET` | `/mp/api/v1/taxonomy` | Full wing → room tree |
| `POST` | `/mp/api/v1/search` | Semantic search |
| `GET` | `/mp/api/v1/drawers?wing=&room=&limit=&offset=` | List drawers |
| `POST` | `/mp/api/v1/drawers` | Add a drawer |
| `GET` | `/mp/api/v1/drawers/{id}` | Get one drawer |
| `PATCH` | `/mp/api/v1/drawers/{id}` | Update a drawer |
| `DELETE` | `/mp/api/v1/drawers/{id}` | Delete a drawer |

Examples:

```bash
KEY="your-secret-key"

# Search
curl -X POST http://localhost:8000/mp/api/v1/search \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"query": "what did we decide about auth?", "limit": 5}'

# Add a memory
curl -X POST http://localhost:8000/mp/api/v1/drawers \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"wing": "project-x", "room": "decisions", "content": "We use JWT auth."}'

# List memories
curl http://localhost:8000/mp/api/v1/drawers?wing=project-x \
  -H "Authorization: Bearer $KEY"
```

### Try it with Bruno

A ready-to-use [Bruno](https://www.usebruno.com) collection for all REST
endpoints is in the [`bruno/`](bruno/) folder. Bruno is a free, open-source
API client (like Postman) that stores requests as plain files in your repo.

1. Install Bruno, then **Open Collection** and pick the `bruno/` folder.
2. Select the **Local** environment (top-right) and set your values:
   - `baseUrl` — e.g. `http://localhost:8000`
   - `apiKey` — your `MCP_API_KEY`
   - `drawerId` — a real drawer ID (copy one from *Add Drawer* / *List Drawers*)
3. Send any request. Bearer auth is applied automatically to all of them.

---

## Configuration

All settings come from environment variables.

| Variable | What it does | Default |
| --- | --- | --- |
| `MEMPALACE_DB_URL` | PostgreSQL connection string (**required**) | – |
| `MCP_API_KEY` | Full-access API key (read + write) | – |
| `MCP_API_KEY_READONLY` | Optional read-only API key (see below) | empty |
| `EMBED_API_URL` | OpenAI-compatible embedding API | `http://localhost:11434/v1` |
| `EMBED_API_KEY` | API key for the embedding API (if needed) | empty |
| `EMBED_MODEL` | Embedding model name | `embeddinggemma` |
| `EMBED_DIM` | Embedding size — **must match the model** | `768` |
| `MEMPALACE_TENANT_ID` | Keeps data separate per tenant | `default` |
| `MEMPALACE_HNSW_EF_SEARCH` | Search quality (higher = better, slower) | `100` |
| `ENABLE_REST_API` | Turn on the optional REST/JSON API (see below) | `false` |
| `PORT` | Port the server listens on | `8000` |

### A note on `EMBED_DIM`

`EMBED_DIM` must equal the embedding size your model actually returns — it
**cannot be larger**. `embeddinggemma` returns **768** values. It also supports
Matryoshka truncation *down* to **512 / 256 / 128** (smaller = faster and less
storage, with a small quality drop). For bigger vectors you need a different
model (e.g. OpenAI `text-embedding-3-large` = 3072).

---

## Run from source (without Docker)

You need a PostgreSQL with the **pgvector** extension (and optionally
**Apache AGE** for the entity graph).

```bash
cd server

export MEMPALACE_DB_URL="postgres://user:pass@localhost:5432/mempalace"
export MCP_API_KEY="your-secret-key"
export EMBED_API_URL="http://localhost:11434/v1"
export EMBED_MODEL="embeddinggemma"
export EMBED_DIM="768"

go run ./cmd/mempalace
```

The server creates all the tables it needs on first start.

---

## Deploy to Kubernetes

Ready-to-use manifests are in the [`k8s/`](k8s/) folder (namespace, PostgreSQL
StatefulSet, server Deployment, Service, Ingress, and Secret).

```bash
kubectl apply -f k8s/
```

Edit `k8s/secret.yaml` and `k8s/deployment.yaml` for your own database, API key,
and embedding API before applying.

---

## License

[MIT](LICENSE)
