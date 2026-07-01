# Coding-usefulness benchmark

Does a memory system actually help a **coding agent**? The other benchmarks use
a fictional handbook; this one uses a corpus of **real facts about this very
repository** (the MemPalace Go server) — the situation a coding agent is
actually in: it accumulates knowledge about a codebase and either recalls it or
re-derives it every session.

Corpus ([`coding_corpus.py`](coding_corpus.py)): 17 facts / 17 probes, each with
a deterministic gold answer, split into two kinds:

- **code lookups** (13) — discoverable by reading the Go source (index
  parameters, defaults, function behaviour). A grep/read agent *can* answer
  these; the question is at what cost.
- **tribal knowledge** (4) — design decisions and gotchas that are **not in the
  source**: why the postgres container uses `args` not `command`, the pgvector
  bootstrap ordering, the image-pull race the kustomize overlay avoids, the
  Ollama bind address. The reasons behind the code, which normally live only in
  an engineer's head.

Run: `MP_KEY=… python3 coding_bench.py all`. Captured output:
[`coding-run.txt`](coding-run.txt). Environment as in [`RESULTS.md`](RESULTS.md)
(minikube stack, embeddinggemma, `qwen3.5:4b` agent).

---

## C1 — Coding Q&A

| question kind | recall@1 | recall@5 | answer correct |
|---|---|---|---|
| code lookups (13) | 100% | 100% | 92.3% |
| tribal knowledge (4) | 100% | 100% | 100% |
| **overall (17)** | **100%** | **100%** | **94.1%** |

Retrieval ~100 ms, generation ~1 s. With the knowledge filed as cards, the agent
answers both "where/what" code lookups and "why" questions from a 5-card
retrieval — no file reads at all.

## C2 — Memory vs. reading the source (code lookups)

The alternative to a memory card is to open the source and read it. Both were
given to the same agent; the source bundle is **6 real `.go` files** from this
repo (`tools.go`, `collection.go`, `schema.go`, `config.go`, `rest.go`,
`pool.go`).

| | read-the-source | memory card (k=5) |
|---|---|---|
| avg input tokens / query | 15,981 | **211** |
| answer correctness | 92.3% | 92.3% |

- **Same correctness, ~76× fewer tokens (99% smaller prompt).**

This is the honest, model-independent result: even when the agent already knows
which files hold the answer, reading them costs ~76× the tokens of a retrieved
card **at equal accuracy**. Over a coding session, re-reading unchanged source to
re-establish known facts is where the context budget goes; a card replaces it
for ~1% of the tokens. (A larger agent model would raise both correctness
figures but not the token ratio — the 15,981 tokens still have to be read.)

> Methodology note: the source-reading prompt needs a context window larger than
> the bundle *plus* room to generate. If `num_ctx` only just fits the input, the
> model emits a 1-token reply and scores a bogus 0% — an easy way to accidentally
> flatter the memory side. We size `num_ctx` to 16k (bundle ≈ 16k tokens) so the
> baseline is measured fairly; see `BUNDLE_CTX` in `coding_bench.py`.

## C3 — Tribal knowledge: the part grep can't recover

Four "why"/gotcha questions whose answers are **not in the source**. The
code-only agent is handed the entire 6-file bundle; the memory agent retrieves
cards.

| agent | correct |
|---|---|
| code-only (reads the whole source) | **1 / 4 (25%)** |
| memory-backed | **4 / 4 (100%)** |

Three of the four were answerable **only** from memory; the code-only agent got
one (the Ollama bind address) by generic guessing, not from the code. The
reasons behind the code — why `args` not `command`, the pgvector bootstrap
order, the image-pull race — are decisions, not statements in the source. grep
cannot recover them; a memory of the decision can.

---

## Takeaway

For a coding agent, MemPalace's value shows up on two axes the source tree
itself can't cover:

1. **Cost.** Recalling an established code fact from a card is ~76× cheaper in
   context tokens than re-reading the file that contains it, at equal accuracy.
2. **Knowledge that isn't in the code.** Design rationale and hard-won gotchas —
   exactly what a new session (or a new teammate) lacks — are recovered at 100%
   from memory vs. 25% from reading the code. This is the institutional memory a
   codebase never stores about itself.

Same caveats as the other benchmarks: small, clean corpus and one small local
model, so treat the correctness rates as an upper bound and the *ratios*
(≈76× token saving; 100% vs 25% on rationale) as the portable result.
