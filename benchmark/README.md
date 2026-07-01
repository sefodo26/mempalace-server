# MemPalace end-to-end benchmark

A single-layer micro-benchmark (e.g. "the Go server does X ops/sec") can't
tell you whether MemPalace is *worth it*. The interesting questions are
end-to-end:

1. **Lookup → useful answer.** How fast does a memory lookup turn into a useful
   agent answer, and where does the wall-clock time actually go?
2. **Recall across sessions.** Does answer quality improve as an agent
   accumulates memories over repeated sessions?
3. **Card-index vs long context.** Does splitting a document into an embedded
   card index cut the tokens an agent must read *enough to pay for* the
   preprocessing?

This harness measures all three against a **real running stack**: the MemPalace
Go server on minikube, PostgreSQL (pgvector + Apache AGE), `embeddinggemma` for
embeddings and hybrid vector/full-text retrieval, and a local Ollama chat model
as the "agent". Token counts are the model's own `prompt_eval_count`, so they
are measured, not estimated.

## Files

| File | Purpose |
| --- | --- |
| `corpus.py` | Hand-authored synthetic knowledge base: 37 atomic facts + 40 probe questions, each with a deterministic gold answer token and the id of the fact that answers it. Facts carry a `session` number for the cross-session test. |
| `mempalace_bench.py` | The harness: three modes (`latency`, `sessions`, `tokens`) plus `all`. |

The corpus is deliberately hand-written (not LLM-generated) so gold answers are
known with certainty and the run is reproducible. Questions are paraphrased away
from the fact wording, and every topic has near-duplicate distractor facts (same
subject, different number/name) so top-1 retrieval has to discriminate. A few
probes are in German to exercise `embeddinggemma`'s multilingual claim.

## Running

Bring the stack up first (see [`../k8s/MINIKUBE.md`](../k8s/MINIKUBE.md)) and
port-forward it:

```bash
kubectl -n mempalace port-forward svc/mempalace 8000:80
```

Then:

```bash
export MP_KEY="$(kubectl -n mempalace get secret mempalace-secrets \
  -o jsonpath='{.data.api-key}' | base64 --decode)"

python3 mempalace_bench.py all        # or: latency | sessions | tokens
```

No third-party Python packages are required (standard library only).

| Env var | Default | Meaning |
| --- | --- | --- |
| `MP_URL` | `http://localhost:8000` | MemPalace base URL |
| `MP_KEY` | — (required) | MemPalace API key |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama base URL |
| `GEN_MODEL` | `qwen3.5:4b` | chat model that plays the "agent" |

Each mode calls `reset_store()` first (delete every drawer via the public REST
API) so the three measurements are isolated and the cross-session recall curve
starts from empty. **Point it at a throwaway benchmark database, not a store you
care about.**

## What each mode measures

### `latency` — lookup → useful answer
Files the whole corpus as cards, then for every probe: times the retrieval
(`POST /search` = embed query + hybrid search), feeds the top-k cards to the
agent, times the generation, and grades the answer. Reports p50/p95/mean for
each stage, the retrieval-vs-generation split, and three usefulness rates:
gold card in top-1, gold card in top-k, final answer correct.

### `sessions` — recall across repeated sessions
*Part A*: files facts session by session and re-probes everything answerable so
far, reporting recall@1 and recall@k as the store grows. *Part B*: the payoff —
at the last session, two agents answer questions whose answers were learned in
*earlier* sessions. The **stateless** agent sees only the current session's
notes; the **MemPalace-backed** agent retrieves from its accumulated store. The
gap is exactly what persistent memory buys.

### `tokens` — card-index vs long context
Answers every probe two ways: (A) stuff the **entire corpus** into the prompt
every query; (B) retrieve a few **cards** and prompt with only those. Reports
average input tokens and correctness for each, tokens saved per query, and a
break-even query count that amortises the one-time indexing cost against the
per-query token saving.

## Interpreting the numbers honestly

- The corpus is small and semantically well-separated, so retrieval recall is
  near-perfect. Treat recall figures as an **upper bound**; a noisy real corpus
  with overlapping facts will score lower. The *shape* of the results (retrieval
  is a small flat cost; memory enables cross-session recall a stateless agent
  cannot achieve; a card index shrinks per-query context) is what generalises.
- Token savings scale with document size. With a 37-fact corpus the full-context
  prompt is already modest; the gap between "stuff everything" and "retrieve a
  few cards" widens with every extra document and every extra query.
- Absolute latencies are laptop-and-model specific (minikube + Ollama on the
  same host). The **breakdown** (retrieval % vs generation %) is the portable
  takeaway.

See [`RESULTS.md`](RESULTS.md) for a captured run with commentary.

## Apples-to-apples vs. the reference project

The end-to-end suite above answers "is it worth it operationally". To compare
retrieval quality directly against the Python reference project
([`MemPalace/mempalace`](https://github.com/MemPalace/mempalace)), we also run
**their** headline benchmark — LongMemEval (500 questions), raw session mode —
through this Go server with the identical metric (`recall_any@5`), via
[`longmemeval_adapter.py`](longmemeval_adapter.py).

**Result: R@5 = 0.972 / R@10 = 0.988**, reproducing (and marginally beating)
the reference's published non-LLM 0.966 / 0.982 — from an independent
implementation on a different embedding model. Full write-up in
[`COMPARISON.md`](COMPARISON.md).
