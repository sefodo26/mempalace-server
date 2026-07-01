# Head-to-head: this Go server vs. the reference MemPalace project

The reference project [`MemPalace/mempalace`](https://github.com/MemPalace/mempalace)
(Python + ChromaDB) publishes retrieval-recall numbers on the standard AI-memory
datasets. To put **this Go server** on the same scoreboard, we ran its headline
benchmark — **LongMemEval, raw mode** — through our stack with the *same
methodology and metric*, using `longmemeval_adapter.py`.

## Methodology (identical to the reference)

- Dataset: **LongMemEval-S (cleaned), all 500 questions**, median 48 haystack
  sessions per question (23,867 session-documents total).
- Indexing: **one document per session = the session's USER turns joined** —
  exactly the reference's raw session-granularity mode.
- Isolation: a **fresh store per question** (they delete+recreate the Chroma
  collection each query; we reset the store so HNSW searches only that
  question's ~48 sessions).
- Metric: **recall_any@k** — is at least one `answer_session_id` in the top-k
  retrieved sessions. This is their published "R@5".

What differs is only the stack under test (reported honestly, not hidden):

| | Reference (published) | This run |
|---|---|---|
| Embeddings | all-MiniLM-L6-v2 (384d) | **embeddinggemma (768d)** |
| Retrieval | ChromaDB pure vector | **Go hybrid vector + full-text (RRF)** |
| Query/doc prefixes | none | none (server embeds raw text) |

## Result

| LongMemEval (raw, no LLM) | **R@5** | **R@10** |
|---|---|---|
| Reference project (all-MiniLM + Chroma) | 0.966 | 0.982 |
| **This Go server (embeddinggemma + hybrid)** | **0.972** | **0.988** |

Our stack **reproduces and marginally exceeds** the reference project's headline
non-LLM figure (+0.6pp R@5, +0.6pp R@10) — well within run-to-run noise, i.e.
**statistically the same result from an independent implementation**. Both sit
far above the academic dense-retriever baselines the reference cites (Stella
~85%, Contriever ~78%, BM25 ~70%).

### By question type (our run vs. the reference's published raw breakdown)

| Question type | Ref R@5 | Ours R@5 | Ours R@10 | n |
|---|---|---|---|---|
| knowledge-update | 0.990 | **1.000** | 1.000 | 78 |
| multi-session | 0.985 | 0.970 | 0.992 | 133 |
| temporal-reasoning | 0.962 | 0.970 | 1.000 | 133 |
| single-session-user | 0.957 | 0.957 | 0.971 | 70 |
| single-session-preference | 0.933 | **0.967** | 0.967 | 30 |
| single-session-assistant | 0.929 | **0.964** | 0.964 | 56 |

Notably we are **stronger on the reference's two weakest categories**
(single-session-preference +3.4pp, single-session-assistant +3.5pp) — the
hybrid full-text component and the higher-dimensional multilingual embedding
appear to help exactly where pure MiniLM vector search struggled, without any of
the hand-tuned heuristics the reference added later (preference-extraction
regexes, name/quote boosts) to climb from 96.6% to 99.4%.

## What this establishes

1. **The Go implementation is sound.** An independent re-implementation on a
   different embedding model lands on the same recall as the reference — the
   "raw verbatim storage + good embeddings" thesis is not an artifact of their
   specific stack.
2. **Hybrid search is a free lunch here.** We match the reference *raw* number
   and beat it on its weak spots, without an LLM in the loop and without the
   dataset-specific heuristics.
3. **Where we stop.** We did **not** reproduce their LLM-rerank ladder to
   98.4–100% (that needs a Haiku/Sonnet reranker and, by their own admission,
   the last 0.6% was tuned on three specific questions). The honest,
   generalisable, no-LLM comparison is **96.6% → 97.2%**, and that is the number
   to quote.

## Reproduce

```bash
# 1. bring up the stack (see ../k8s/MINIKUBE.md) and port-forward :8000
# 2. download the dataset (≈265 MB)
curl -fsSL -o /tmp/longmemeval_s_cleaned.json \
  https://huggingface.co/datasets/xiaowu0162/longmemeval-cleaned/resolve/main/longmemeval_s_cleaned.json
# 3. run all 500 questions through the Go server (~11 min on Apple silicon)
export MP_KEY="$(kubectl -n mempalace get secret mempalace-secrets \
  -o jsonpath='{.data.api-key}' | base64 --decode)"
python3 longmemeval_adapter.py /tmp/longmemeval_s_cleaned.json
```

Captured console output: [`longmemeval-run.txt`](longmemeval-run.txt).
Machine-readable summary: [`longmemeval-summary.json`](longmemeval-summary.json).

> Caveat on comparability: the reference's `R@5` and this run measure **retrieval
> recall** (is the gold session retrieved), not end-to-end QA accuracy. The
> reference's `BENCHMARKS.md` stresses the same distinction. Our other three
> benchmarks (`RESULTS.md`) cover the end-to-end/operational axes that retrieval
> recall alone does not.
