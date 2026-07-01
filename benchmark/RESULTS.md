# Benchmark results — captured run

One full run of `python3 mempalace_bench.py all`. Raw console output is in
[`sample-run.txt`](sample-run.txt).

**Environment.** MemPalace Go server on minikube (Docker driver) + PostgreSQL 16
with pgvector and Apache AGE, all on one Apple-silicon laptop (Mac16,7, 14
cores). Embeddings: `embeddinggemma` (768-dim) served by Ollama on the host.
Agent / judge model: `qwen3.5:4b` via Ollama, `temperature=0`. Corpus: 37
atomic facts, 40 probe questions with deterministic gold answers.

> These numbers are a single machine's single run, not a leaderboard. The
> **shape** of each result is the point, not the third significant figure.

---

## 1. Lookup → useful answer

| stage | p50 | p95 | mean | what it is |
| --- | --- | --- | --- | --- |
| retrieve | 96 ms | 106 ms | 95 ms | embed query + hybrid vector/FTS search |
| generate | 780 ms | 917 ms | 787 ms | LLM turns the retrieved cards into an answer |
| **total** | **879 ms** | **1016 ms** | **883 ms** | |

- Retrieval is **~11%** of wall-clock; the LLM is **~89%**.
- Gold card retrieved in **top-1: 40/40 (100%)**, top-5: 40/40 (100%).
- Final answer **correct: 39/40 (97.5%)**.

**Read.** The memory layer is a small, flat cost (sub-100 ms and roughly
constant). What makes a lookup *useful* is (a) whether the right card is
retrieved — here effectively always, on a clean corpus — and (b) the LLM, which
dominates both the latency and the one wrong answer. Optimising the storage/Go
layer moves ~10% of the end-to-end time; the retrieval *quality* and the model
move the other 90% and the correctness.

## 2. Recall across repeated sessions

**Part A — retrieval recall@k as memories accumulate.**

| session | cards in store | probes answerable | recall@1 | recall@5 |
| --- | --- | --- | --- | --- |
| 1 | 21 | 24 | 100% | 100% |
| 2 | 32 | 35 | 100% | 100% |
| 3 | 37 | 40 | 100% | 100% |

Adding more memories did **not** degrade retrieval of the earlier ones — recall
stayed at ceiling as the store grew. (On this small, well-separated corpus 100%
is expected; read it as "no degradation with growth", not "retrieval is
perfect".)

**Part B — the payoff: answering *earlier-session* questions at session 3.**

| agent | correct on earlier-session questions |
| --- | --- |
| stateless (current session context only) | **0 / 35 (0.0%)** |
| MemPalace-backed (retrieves from its store) | **34 / 35 (97.1%)** |

**Read.** This is the crux the user asked about — "whether recall quality
improves across repeated sessions." A stateless agent *structurally cannot*
answer questions whose answers were established in earlier sessions: 0%. An agent
that writes what it learns to MemPalace and retrieves it later answers 97.1% of
them. The improvement across sessions isn't a marginal quality bump — it's the
difference between impossible and routine.

## 3. Card-index preprocessing vs long context

| | full-context (whole corpus in prompt) | card-index (retrieve k=5) |
| --- | --- | --- |
| avg input tokens / query | 888 | **181** |
| answer correctness | 95.0% | **97.5%** |

- Tokens saved per query: **706 (80% smaller prompt)**.
- One-time indexing cost: ~827 embedding tokens (≈ 3.6 s, 98 ms/card).
- **Break-even: ~1.2 queries.**

**Read.** Two things fall out, and the second is the interesting one:

1. The card index cuts per-query context by **80%** on a corpus that is already
   small; the saving grows with document size. The one-time preprocessing pays
   for itself after roughly the *first* query, because generation input tokens
   (paid every query, forever) dwarf the embedding tokens (paid once).
2. Retrieval was **more** accurate than stuffing everything in (97.5% vs 95.0%),
   not less. Dumping the whole corpus into the prompt hands the small model more
   distractors to trip over; focused retrieval gives it just the relevant card.
   So the card index is not a quality-for-cost trade here — it improved both.

---

## Caveats worth stating

- **Small, clean corpus.** 37 semantically distinct facts. Retrieval recall is
  therefore an upper bound; a large corpus with overlapping, contradictory, or
  time-varying facts will score lower and is where reranking, `max_distance`
  tuning, and the knowledge-graph layer would start to matter.
- **One local model.** `qwen3.5:4b` is a small agent. A stronger model would
  likely close the one wrong answer in Theme 1 and might tolerate the noisier
  full-context prompt better in Theme 3 (shrinking, not eliminating, the
  accuracy gap).
- **Latencies are host-specific.** The portable numbers are the *ratios*
  (retrieval ≈ 11% of wall-clock; 80% token reduction; ~1-query break-even), not
  the millisecond counts.
- **Token accounting is real, cost is modelled.** Input/output token counts come
  from the model's own `prompt_eval_count`/`eval_count`. The break-even converts
  the one-time index cost to tokens with a ~4-chars/token estimate; the
  per-query generation tokens it is compared against are measured exactly.
