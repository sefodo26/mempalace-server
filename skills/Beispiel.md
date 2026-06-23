# Example: Put your docs into MemPalace and share them with customers

This is a simple, step-by-step guide. By the end you will have:

1. Your documentation stored in a MemPalace server.
2. A **read-only** key your customers (or their AI tools) can use to search it.

You can do every step with **Claude Code**, or with any other CLI using `curl`.

---

## What you need

- A running **MemPalace server** (see the [README](../README.md) to start one).
- The server's **write key** (`MCP_API_KEY`) — lets you add documents.
- For sharing: a **read-only key** (`MCP_API_KEY_READONLY`) — read but not change.

> The two keys must be different. The read-only key is optional but recommended
> for customers, so they can never edit your docs.

---

## Step 1 — Install the two skills (Claude Code)

Copy the skills from this repo into your Claude Code skills folder:

```bash
mkdir -p ~/.claude/skills
cp -r skills/mempalace-ingest-docs ~/.claude/skills/
cp -r skills/mempalace-read-docs   ~/.claude/skills/
cp -r skills/mempalace-zettelkasten ~/.claude/skills/
```

Now Claude Code knows how to **write** docs into MemPalace, **read** them back,
and capture ideas as linked Zettelkasten notes. (Skip this step if you only want
to use raw `curl` — see Step 3b.)

---

## Step 2 — Connect Claude Code to your MemPalace server

Add the MemPalace MCP server to your client config, using your **write** key:

```json
{
  "mcpServers": {
    "mempalace": {
      "command": "npx",
      "args": [
        "mcp-remote",
        "http://localhost:8000/mp/mcp",
        "--header",
        "Authorization: Bearer YOUR_WRITE_KEY"
      ]
    }
  }
}
```

Restart Claude Code. See [MCP_USAGE.md](../MCP_USAGE.md) for details.

---

## Step 3a — Write a document into MemPalace (with Claude Code)

Just ask Claude Code in plain language. The ingest skill does the work:

```
Ingest the file docs/api-guide.md into MemPalace under the wing "acme-api-docs".
```

Claude Code will:

- read the file,
- split it into sections (wing → room → drawer),
- file each section so it can be searched by meaning,
- tell you how many sections it stored.

Repeat for every document you want to share.

---

## Step 3b — Or write it with plain `curl` (any CLI)

First switch the server on for REST (set `ENABLE_REST_API=true` and restart),
then add one piece of content per call:

```bash
KEY="YOUR_WRITE_KEY"
BASE="http://localhost:8000/mp/api/v1"

curl -sS -X POST "$BASE/drawers" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "wing": "acme-api-docs",
    "room": "getting-started",
    "content": "# Acme API > Getting started\n\nCreate an account, then ...",
    "source_file": "docs/api-guide.md"
  }'
```

Send one request per section of your document.

---

## Step 4 — Check it worked

Ask Claude Code:

```
List the wings in MemPalace, then search it for "how do I authenticate?"
```

Or with `curl`:

```bash
curl -sS "$BASE/wings" -H "Authorization: Bearer $KEY"

curl -sS -X POST "$BASE/search" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"query": "how do I authenticate?", "limit": 5}'
```

You should see your document's wing and relevant sections.

---

## Step 5 — Make it available to customers (read-only)

You do **not** give customers your write key. Instead, give them the
**read-only** key.

1. On the server, set a second key and restart:

   ```bash
   MCP_API_KEY="your-write-key"
   MCP_API_KEY_READONLY="your-readonly-key"   # different from the write key
   ```

2. Share **only** `your-readonly-key` with customers.

3. Customers can now search your docs — but cannot add, change, or delete
   anything. Any write attempt is rejected (HTTP `403` / MCP error `-32003`).

A customer using Claude Code connects exactly like Step 2, but with the
read-only key, and uses the `mempalace-read-docs` skill:

```
Search the MemPalace docs: what are the rate limits for the Acme API?
```

A customer using a plain CLI:

```bash
KEY="your-readonly-key"
BASE="https://docs.acme.example.com/mp/api/v1"

curl -sS -X POST "$BASE/search" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"query": "what are the rate limits?", "limit": 5}'
```

---

## Tips

- **One wing per document** keeps things tidy and easy to export later.
- **Update a doc:** delete its old wing's drawers, then ingest the new version,
  so customers never see stale content.
- **Keep it private:** put the server behind HTTPS and rotate keys if leaked.
- **Need facts that change** (versions, prices, limits)? Store them as
  knowledge-graph facts (`mempalace_kg_add`) so you can ask "what was true on
  date X?" later.
