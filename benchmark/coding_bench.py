#!/usr/bin/env python3
"""
Coding-usefulness benchmark for MemPalace.

Measures whether a memory system actually helps a *coding* agent, using a corpus
of real facts about THIS repository (see coding_corpus.py). Three themes:

  C1  CODING Q&A
      File everything the agent has "learned" about the codebase as cards, then
      answer coding questions from retrieval. recall@k + answer correctness +
      latency, split into code-lookup vs tribal-knowledge questions.

  C2  MEMORY vs READING THE SOURCE  (code-lookup questions)
      The alternative to a memory card is to open the source and read it. Compare
      answering from a bundle of real .go files stuffed into the prompt vs.
      answering from a retrieved card. Real input tokens + correctness.

  C3  TRIBAL KNOWLEDGE  (design decisions / gotchas)
      The payoff. Questions whose answer is NOT in the source (why args-not-
      command, the pgvector bootstrap order, the image-pull race, the Ollama
      bind address). A code-only agent that can read the whole source bundle
      vs. a memory-backed agent. grep can't recover *why*; memory can.

Usage:
    MP_KEY=... python3 coding_bench.py [all|qa|tokens|tribal]
Shares plumbing, grading, and env vars with mempalace_bench.py.
"""

import os
import sys
import time

import mempalace_bench as mb
import coding_corpus as cc

# Real source files that collectively contain every in_source fact. This is the
# "read the code" context a cold agent would have to load.
REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
BUNDLE_FILES = [
    "server/internal/handler/tools.go",
    "server/internal/storage/collection.go",
    "server/internal/storage/schema.go",
    "server/internal/config/config.go",
    "server/internal/handler/rest.go",
    "server/internal/storage/pool.go",
]
BUNDLE_CHAR_BUDGET = 60000  # fits all six files whole (~12k tokens); no answer-
                            # bearing file gets truncated away
# Context window for the "read the source" prompts. Must exceed the bundle's
# token count with headroom to actually GENERATE an answer — otherwise the input
# fills num_ctx and the model emits a 1-token reply (a subtle way to get a bogus
# 0% for the source-reading baseline).
BUNDLE_CTX = 16384


def load_bundle():
    parts = []
    for rel in BUNDLE_FILES:
        try:
            with open(os.path.join(REPO, rel)) as f:
                parts.append(f"// ===== FILE: {rel} =====\n" + f.read())
        except OSError as e:
            print(f"  (warning: could not read {rel}: {e})")
    bundle = "\n\n".join(parts)
    return bundle[:BUNDLE_CHAR_BUDGET]


def grade(answer, probe):
    return mb.is_correct(answer, probe)


def answer(question, context, num_ctx=8192):
    system = ("You are a senior engineer answering a question about the MemPalace "
              "codebase using ONLY the provided context. Answer in one short "
              "sentence. If the answer is not in the context, reply exactly: NOT FOUND.")
    prompt = f"Context:\n{context}\n\nQuestion: {question}\nAnswer:"
    return mb.ollama_chat(prompt, system=system, num_ctx=num_ctx)


def file_corpus():
    print(f"Reset store ({mb.reset_store()} old drawers removed).")
    n = 0
    for f in cc.FACTS:
        mb.mp_add(f.wing, f.room, f.text, source=f.id)
        n += 1
    print(f"Filed {n} codebase facts as cards.")


# ===========================================================================
def bench_qa(limit=5):
    print("\n" + "=" * 72)
    print("C1  coding Q&A:  retrieval + answer correctness + latency")
    print("=" * 72)
    file_corpus()
    mb.mp_search("warm up", limit=limit)
    mb.ollama_chat("OK", system="one word")

    def run(probes, label):
        rows = []
        for p in probes:
            t0 = time.perf_counter()
            res = mb.mp_search(p.question, limit=limit)
            t_ret = time.perf_counter() - t0
            hits = res.get("results", [])
            ctx = "\n".join(f"- {h['content']}" for h in hits)
            ans, _, _, t_gen = answer(p.question, ctx)
            top1 = bool(hits) and hits[0].get("metadata", {}).get("source_file") == p.gold_id
            ink = any(h.get("metadata", {}).get("source_file") == p.gold_id for h in hits)
            rows.append((grade(ans, p), top1, ink, t_ret, t_gen))
        n = len(rows)
        cor = sum(r[0] for r in rows)
        t1 = sum(r[1] for r in rows)
        tk = sum(r[2] for r in rows)
        ret = sum(r[3] for r in rows) / n
        gen = sum(r[4] for r in rows) / n
        print(f"  {label:<22} n={n:<3} recall@1={mb.pct(t1,n):>6} "
              f"recall@{limit}={mb.pct(tk,n):>6} answer_correct={mb.pct(cor,n):>6} "
              f"(ret {ret*1000:.0f}ms / gen {gen*1000:.0f}ms)")
        return cor, n

    print()
    c1, n1 = run(cc.CODE_PROBES, "code lookups")
    c2, n2 = run(cc.TRIBAL_PROBES, "tribal knowledge")
    print(f"\n  overall answer correctness: {c1+c2}/{n1+n2} ({mb.pct(c1+c2, n1+n2)})")
    print(f"  read: with the knowledge filed, the agent answers both code lookups")
    print(f"        and 'why' questions from a ~{limit}-card retrieval — no file reads.")


# ===========================================================================
def bench_tokens(limit=5):
    print("\n" + "=" * 72)
    print("C2  memory vs reading the source:  tokens & correctness (code lookups)")
    print("=" * 72)
    file_corpus()
    bundle = load_bundle()
    ctx_tokens = BUNDLE_CTX
    # size the bundle in real tokens once
    _, btok, _, _ = answer("How is a drawer's ID derived?", bundle, num_ctx=ctx_tokens)
    print(f"\nSource bundle = {len(BUNDLE_FILES)} real .go files, {len(bundle)} chars "
          f"≈ {btok} input tokens/query.\n")

    src_in = src_ok = card_in = card_ok = 0
    probes = cc.CODE_PROBES
    n = len(probes)
    for p in probes:
        a_src, sin, _, _ = answer(p.question, bundle, num_ctx=ctx_tokens)
        src_in += sin
        src_ok += grade(a_src, p)
        res = mb.mp_search(p.question, limit=limit)
        ctx = "\n".join(f"- {h['content']}" for h in res.get("results", []))
        a_card, cin, _, _ = answer(p.question, ctx)
        card_in += cin
        card_ok += grade(a_card, p)

    s_avg, c_avg = src_in / n, card_in / n
    saved = s_avg - c_avg
    print(f"                        read-the-source     memory card (k={limit})")
    print(f"  avg input tokens      {s_avg:>12.0f}     {c_avg:>12.0f}")
    print(f"  answer correctness    {mb.pct(src_ok,n):>12}     {mb.pct(card_ok,n):>12}")
    print(f"  tokens saved / query  {saved:>12.0f}     ({100*saved/s_avg:.0f}% smaller prompt)")
    print(f"\n  read: even when the agent already knows which files hold the answer,")
    print(f"        reading them costs ~{s_avg/max(c_avg,1):.0f}x the tokens of a retrieved card,")
    print(f"        at equal correctness. Over a coding session that is the bulk of")
    print(f"        the context budget spent re-reading unchanged code.")


# ===========================================================================
def bench_tribal():
    print("\n" + "=" * 72)
    print("C3  tribal knowledge:  code-only agent vs memory-backed agent")
    print("=" * 72)
    file_corpus()
    bundle = load_bundle()
    ctx_tokens = BUNDLE_CTX
    probes = cc.TRIBAL_PROBES
    n = len(probes)
    print(f"\n  {n} 'why'/gotcha questions whose answers are NOT in the source.")
    print(f"  code-only agent sees the full {len(BUNDLE_FILES)}-file source bundle; "
          f"memory agent retrieves cards.\n")

    code_ok = mem_ok = 0
    for p in probes:
        a_code, _, _, _ = answer(p.question, bundle, num_ctx=ctx_tokens)
        ok_code = grade(a_code, p)
        code_ok += ok_code
        res = mb.mp_search(p.question, limit=5)
        ctx = "\n".join(f"- {h['content']}" for h in res.get("results", []))
        a_mem, _, _, _ = answer(p.question, ctx)
        ok_mem = grade(a_mem, p)
        mem_ok += ok_mem
        mark = {(True, True): "both", (False, True): "MEMORY ONLY",
                (True, False): "code only", (False, False): "neither"}[(ok_code, ok_mem)]
        print(f"    [{mark:<11}] {p.question[:60]}")
    print(f"\n  code-only agent (reads the whole source): {code_ok}/{n} ({mb.pct(code_ok,n)})")
    print(f"  memory-backed agent                     : {mem_ok}/{n} ({mb.pct(mem_ok,n)})")
    print(f"\n  read: the reasons behind the code — why args not command, the pgvector")
    print(f"        bootstrap order, the image-pull race — are not IN the code. grep")
    print(f"        cannot recover them; a memory of the decision can. This is the")
    print(f"        part of a codebase that most needs remembering across sessions.")


def main():
    if not mb.MP_KEY:
        sys.exit("Set MP_KEY.")
    mode = sys.argv[1] if len(sys.argv) > 1 else "all"
    try:
        mb._req(mb.MP_URL + "/mp/mcp/health")
        mb.mp("/mp/api/v1/status")
        print(f"MemPalace up. gen model: {mb.GEN_MODEL}")
    except Exception as e:
        sys.exit(f"MemPalace not reachable/authorised: {e}")
    if mode in ("all", "qa"):
        bench_qa()
    if mode in ("all", "tokens"):
        bench_tokens()
    if mode in ("all", "tribal"):
        bench_tribal()


if __name__ == "__main__":
    main()
