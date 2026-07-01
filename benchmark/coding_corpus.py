"""
Coding-usefulness corpus for the MemPalace benchmark.

Unlike corpus.py (a fictional company handbook), every fact here is a REAL fact
about *this* repository — the MemPalace Go server — so the benchmark measures
the actual use case: a coding agent that accumulates knowledge about a codebase
across sessions and recalls it instead of re-deriving it.

Two kinds of fact, tagged by `in_source`:

  * in_source=True  — "code lookups": discoverable by reading the Go source
    (function behaviour, index parameters, defaults). A grep/read agent CAN
    answer these; the question is whether memory does it cheaper.

  * in_source=False — "tribal knowledge": design decisions and operational
    gotchas that are NOT in the Go source — the hard-won reasons and pitfalls
    that normally live only in an engineer's head (or a fixed commit). This is
    where a memory system earns its keep: grep cannot recover *why*.

Each probe has a deterministic gold answer token so grading is exact.
"""

from dataclasses import dataclass, field


@dataclass
class CFact:
    id: str
    wing: str
    room: str
    text: str
    in_source: bool  # discoverable by reading the Go source bundle?


@dataclass
class CProbe:
    question: str
    answer: str
    gold_id: str
    aliases: list = field(default_factory=list)


FACTS = [
    # --- code lookups (in the Go source) --------------------------------
    CFact("drawer-id", "mempalace-server", "storage",
          "A drawer's ID is the first 16 hex characters of sha256(\"wing/room/content[:500]\"), which makes add_drawer idempotent.", True),
    CFact("rrf", "mempalace-server", "search",
          "Hybrid search fuses the vector-similarity and full-text result lists with Reciprocal Rank Fusion (RRF).", True),
    CFact("rrf-k", "mempalace-server", "search",
          "The RRF merge uses a constant k of 60 when combining the two rank lists.", True),
    CFact("hnsw", "mempalace-server", "storage",
          "The pgvector HNSW index is created with m=16 and ef_construction=64 as RAG defaults.", True),
    CFact("efsearch", "mempalace-server", "search",
          "Each vector query sets hnsw.ef_search to 100 via SET LOCAL inside the search transaction.", True),
    CFact("fts", "mempalace-server", "search",
          "Full-text search runs over a generated tsvector column built with to_tsvector('simple', document), so it is language-agnostic.", True),
    CFact("hybrid-fetch", "mempalace-server", "search",
          "QueryHybrid fetches three times the requested number of results (minimum 10) from each leg before the RRF merge.", True),
    CFact("embed-model", "mempalace-server", "config",
          "The default embedding model is embeddinggemma at 768 dimensions.", True),
    CFact("bullets", "mempalace-server", "storage",
          "add_drawer splits a pure bullet list into one drawer per bullet item for more precise retrieval.", True),
    CFact("rest-base", "mempalace-server", "api",
          "The optional REST API is mounted at base path /mp/api/v1 and only when ENABLE_REST_API=true; MCP is always on.", True),
    CFact("tenant-schema", "mempalace-server", "storage",
          "Each tenant's data lives in a PostgreSQL schema named mp_<sanitized-tenant-id>.", True),
    CFact("readonly", "mempalace-server", "api",
          "A read-only API key may call only non-mutating tools; a write attempt returns HTTP 403.", True),
    CFact("diary", "mempalace-server", "api",
          "Diary entries are stored as ordinary drawers with type=diary_entry under wing=<agent>, room=diary.", True),

    # --- tribal knowledge (NOT in the Go source) ------------------------
    CFact("k8s-args", "mempalace-server", "ops",
          "On Kubernetes the postgres container must use args, not command: command overrides the image entrypoint that drops root privileges, so postgres refuses to start as root.", False),
    CFact("vector-bootstrap", "mempalace-server", "ops",
          "The pool registers the pgvector type on its first connection, before CREATE EXTENSION runs, so the vector extension must be created at initdb or the server cannot boot.", False),
    CFact("overlay-race", "mempalace-server", "ops",
          "The minikube manifests are applied through a single kustomize overlay to avoid an image-pull race that wedged pods when using apply-then-set-image-then-patch.", False),
    CFact("ollama-bind", "mempalace-server", "ops",
          "Ollama must listen on 0.0.0.0 so cluster pods can reach it via host.minikube.internal; binding only to 127.0.0.1 breaks embedding from inside minikube.", False),
]


PROBES = [
    # code lookups
    CProbe("How is a drawer's ID derived from its content?", "16", "drawer-id", ["sha256"]),
    CProbe("How does hybrid search combine vector and keyword results?", "RRF", "rrf", ["Reciprocal Rank Fusion"]),
    CProbe("What constant k does the RRF merge use?", "60", "rrf-k"),
    CProbe("What parameters is the HNSW vector index built with?", "16", "hnsw", ["m=16", "ef_construction"]),
    CProbe("What value is hnsw.ef_search set to for a query?", "100", "efsearch"),
    CProbe("Which text-search configuration does full-text search use?", "simple", "fts", ["to_tsvector"]),
    CProbe("How many candidates does QueryHybrid pull before merging?", "three times", "hybrid-fetch", ["3", "minimum 10"]),
    CProbe("What embedding model and dimension are the defaults?", "768", "embed-model", ["embeddinggemma"]),
    CProbe("What does add_drawer do with a bullet-point list?", "one drawer per bullet", "bullets", ["per bullet"]),
    CProbe("At what path is the optional REST API served, and when?", "/mp/api/v1", "rest-base", ["ENABLE_REST_API"]),
    CProbe("How are different tenants isolated in the database?", "mp_", "tenant-schema", ["schema"]),
    CProbe("What happens when a read-only key tries to write?", "403", "readonly", ["read-only"]),
    CProbe("How are diary entries stored under the hood?", "diary_entry", "diary"),
    # tribal knowledge — gold token deliberately NOT present in the question
    # (so a question-echoing answer can't score) and NOT in the Go source
    # bundle (so the code-only baseline in theme C3 gets no free credit).
    CProbe("Why does the postgres container break when its launch command is overridden on Kubernetes?", "entrypoint", "k8s-args", ["drops root", "as root"]),
    CProbe("Why can the server fail to boot against a brand-new database?", "initdb", "vector-bootstrap", ["first connection"]),
    CProbe("Why are the minikube manifests applied in a single pass rather than patched afterwards?", "race", "overlay-race", ["image-pull"]),
    CProbe("On which address must Ollama listen so minikube pods can reach it?", "0.0.0.0", "ollama-bind", ["host.minikube.internal"]),
]


def facts_by_id():
    return {f.id: f for f in FACTS}


CODE_PROBES = [p for p in PROBES if facts_by_id()[p.gold_id].in_source]
TRIBAL_PROBES = [p for p in PROBES if not facts_by_id()[p.gold_id].in_source]


if __name__ == "__main__":
    ids = facts_by_id()
    bad = 0
    for p in PROBES:
        f = ids[p.gold_id]
        toks = [p.answer] + p.aliases
        if not any(t.lower() in f.text.lower() for t in toks):
            print(f"answer {toks} not in fact {p.gold_id}: {f.text!r}")
            bad += 1
    print(f"{len(FACTS)} facts ({sum(f.in_source for f in FACTS)} in-source, "
          f"{sum(not f.in_source for f in FACTS)} tribal), "
          f"{len(PROBES)} probes ({len(CODE_PROBES)} code, {len(TRIBAL_PROBES)} tribal), {bad} problems")
