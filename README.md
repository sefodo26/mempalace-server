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

---

## Configuration

All settings come from environment variables.

| Variable | What it does | Default |
| --- | --- | --- |
| `MEMPALACE_DB_URL` | PostgreSQL connection string (**required**) | – |
| `MCP_API_KEY` | API key clients must send | – |
| `EMBED_API_URL` | OpenAI-compatible embedding API | `http://localhost:11434/v1` |
| `EMBED_API_KEY` | API key for the embedding API (if needed) | empty |
| `EMBED_MODEL` | Embedding model name | `embeddinggemma` |
| `EMBED_DIM` | Embedding size — **must match the model** | `768` |
| `MEMPALACE_TENANT_ID` | Keeps data separate per tenant | `default` |
| `MEMPALACE_HNSW_EF_SEARCH` | Search quality (higher = better, slower) | `100` |
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
