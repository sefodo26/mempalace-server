#!/usr/bin/env python3
"""
Run the LongMemEval retrieval benchmark from the original MemPalace project
through THIS Go server, so the R@5 / R@10 numbers are directly comparable to
the reference project's published figures.

Faithful to benchmarks/longmemeval_bench.py in MemPalace/mempalace (raw,
session granularity):
  * one document per haystack session = the session's USER turns, joined by "\n"
  * a FRESH store per question (their code delete+recreates the Chroma
    collection each query) — we reset the store so HNSW searches only that
    question's ~48 sessions, exactly like their per-question collection
  * gold = answer_session_ids; recall_any@k = at least one gold session in the
    top-k retrieved sessions (their headline "R@5")

Differences that are inherent to the stack under test (reported, not hidden):
  * embeddings are embeddinggemma (768d) via Ollama, not all-MiniLM-L6-v2 (384d)
  * retrieval is the Go server's hybrid vector + full-text (RRF) search, queried
    over the REST API, not ChromaDB's pure vector search
  * the server embeds query and document with the same raw text (no
    embeddinggemma task-prefixes)

So this measures "MemPalace-the-concept on a different embedding model and a
different retrieval implementation" — which is exactly the interesting question.

Usage:
    MP_KEY=... python3 longmemeval_adapter.py /path/longmemeval_s_cleaned.json [--limit N] [--workers 8]
"""

import argparse
import json
import sys
import time
from collections import Counter, defaultdict
from concurrent.futures import ThreadPoolExecutor

import mempalace_bench as mp  # reuse the HTTP plumbing / reset_store

WING = "lme"


def session_docs(entry):
    """(doc, session_id) per haystack session — user turns joined, like the ref."""
    out = []
    for session, sid in zip(entry["haystack_sessions"], entry["haystack_session_ids"]):
        user_turns = [t["content"] for t in session if t["role"] == "user"]
        if user_turns:
            out.append(("\n".join(user_turns), sid))
    return out


def index_question(docs, workers):
    """File one document per session. room=sid keeps the deterministic content
    hash unique per session (avoids cross-session ID collisions)."""
    def add(ds):
        doc, sid = ds
        mp.mp_add(WING, sid, doc, source=sid)
    with ThreadPoolExecutor(max_workers=workers) as ex:
        list(ex.map(add, docs))


def reset_wing(workers):
    """Delete every drawer currently in the store (only the previous question's
    sessions live there). Parallel deletes to keep the reset cheap."""
    ids = []
    offset = 0
    while True:
        page = mp.mp(f"/mp/api/v1/drawers?wing={WING}&limit=100&offset={offset}").get("drawers", [])
        if not page:
            break
        ids += [d["drawer_id"] for d in page]
        if len(page) < 100:
            break
        offset += 100
    if not ids:
        return
    with ThreadPoolExecutor(max_workers=workers) as ex:
        list(ex.map(lambda i: mp.mp(f"/mp/api/v1/drawers/{i}", "DELETE"), ids))


def retrieve_sessions(question, limit):
    """Return the ranked, de-duplicated list of session ids for a query."""
    res = mp.mp_search(question, limit=limit, wing=WING)
    ranked = []
    seen = set()
    for h in res.get("results", []):
        sid = h.get("metadata", {}).get("source_file") or h.get("room")
        if sid and sid not in seen:
            seen.add(sid)
            ranked.append(sid)
    return ranked


def recall_any(ranked, gold, k):
    top = set(ranked[:k])
    return 1.0 if any(g in top for g in gold) else 0.0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("dataset")
    ap.add_argument("--limit", type=int, default=0, help="first N questions (0 = all 500)")
    ap.add_argument("--workers", type=int, default=8)
    ap.add_argument("--retrieve", type=int, default=15, help="drawers to pull (>=10 for clean top-10 dedup)")
    ap.add_argument("--out", default="longmemeval_results.json")
    args = ap.parse_args()

    if not mp.MP_KEY:
        sys.exit("Set MP_KEY.")
    data = json.load(open(args.dataset))
    if args.limit:
        data = data[: args.limit]

    print(f"LongMemEval through the Go server — {len(data)} questions")
    print(f"embeddings: embeddinggemma (768d) | retrieval: Go hybrid vector+FTS (RRF)")
    print(f"metric: recall_any@k (>=1 gold session in top-k), like the reference raw mode\n")

    r5 = r10 = 0.0
    by_type = defaultdict(lambda: [0.0, 0.0, 0])  # type -> [sum_r5, sum_r10, n]
    log = []
    t0 = time.perf_counter()

    for i, entry in enumerate(data, 1):
        reset_wing(args.workers)
        docs = session_docs(entry)
        index_question(docs, args.workers)
        gold = set(entry["answer_session_ids"])
        ranked = retrieve_sessions(entry["question"], args.retrieve)
        a5 = recall_any(ranked, gold, 5)
        a10 = recall_any(ranked, gold, 10)
        r5 += a5
        r10 += a10
        qt = entry.get("question_type", "?")
        by_type[qt][0] += a5
        by_type[qt][1] += a10
        by_type[qt][2] += 1
        log.append({"qid": entry["question_id"], "type": qt, "r5": a5, "r10": a10,
                    "gold": list(gold), "top10": ranked[:10]})
        if i % 25 == 0 or i == len(data):
            el = time.perf_counter() - t0
            print(f"  [{i:>3}/{len(data)}]  R@5={r5/i:.3f}  R@10={r10/i:.3f}  "
                  f"({el:.0f}s, {el/i:.1f}s/q)")

    n = len(data)
    print("\n" + "=" * 60)
    print(f"RESULT over {n} questions")
    print(f"  Recall@5 : {r5/n:.3f}")
    print(f"  Recall@10: {r10/n:.3f}")
    print("\n  by question type:")
    print(f"  {'type':<28}{'R@5':>7}{'R@10':>7}{'n':>5}")
    for qt in sorted(by_type):
        s5, s10, c = by_type[qt]
        print(f"  {qt:<28}{s5/c:>7.3f}{s10/c:>7.3f}{c:>5}")

    json.dump({"n": n, "recall_at_5": r5 / n, "recall_at_10": r10 / n,
               "by_type": {k: {"r5": v[0]/v[2], "r10": v[1]/v[2], "n": v[2]}
                           for k, v in by_type.items()},
               "log": log}, open(args.out, "w"), indent=2)
    print(f"\n  wrote {args.out}")


if __name__ == "__main__":
    main()
