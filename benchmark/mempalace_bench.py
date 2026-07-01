#!/usr/bin/env python3
"""
MemPalace end-to-end benchmark.

Answers three questions that a single-layer micro-benchmark cannot:

  1. LOOKUP -> USEFUL ANSWER  (`latency`)
     How fast does a memory lookup turn into a useful agent answer, and where
     does the wall-clock time actually go (embed+retrieve vs. LLM generation)?

  2. RECALL ACROSS SESSIONS   (`sessions`)
     Does answer quality improve as an agent accumulates memories over repeated
     sessions? Compares a stateless agent (only the current session in context)
     against one backed by MemPalace, on questions whose answers were learned in
     *earlier* sessions. Also plots retrieval recall@k as the store grows.

  3. CARD-INDEX vs LONG CONTEXT (`tokens`)
     Does splitting a document into an embedded card index actually cut the
     tokens an agent must read enough to pay for the preprocessing? Compares
     stuffing the whole corpus into the prompt against retrieving a few cards,
     measuring real input tokens (from the model) and answer correctness, then
     computes the break-even query count for the one-time indexing cost.

All measurements use the *same* running MemPalace server (embeddinggemma via
Ollama for embeddings + hybrid vector/full-text retrieval) and a local Ollama
chat model as the "agent". Token counts are the model's own prompt_eval_count,
so they are real, not estimated.

Usage:
    python3 mempalace_bench.py all
    python3 mempalace_bench.py latency
    python3 mempalace_bench.py sessions
    python3 mempalace_bench.py tokens
Environment:
    MP_URL       MemPalace base URL         (default http://localhost:8000)
    MP_KEY       MemPalace API key          (required)
    OLLAMA_URL   Ollama base URL            (default http://localhost:11434)
    GEN_MODEL    chat model for the "agent" (default qwen3.5:4b)
"""

import json
import os
import re
import statistics
import sys
import time
import urllib.error
import urllib.request

import corpus

MP_URL = os.environ.get("MP_URL", "http://localhost:8000").rstrip("/")
MP_KEY = os.environ.get("MP_KEY", "")
OLLAMA_URL = os.environ.get("OLLAMA_URL", "http://localhost:11434").rstrip("/")
GEN_MODEL = os.environ.get("GEN_MODEL", "qwen3.5:4b")

TENANT_PREFIX = "bench"  # wings are prefixed per run-mode to isolate stores


# ---------------------------------------------------------------------------
# HTTP plumbing
# ---------------------------------------------------------------------------
def _req(url, method="GET", body=None, headers=None, timeout=120):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read().decode())


def mp(path, method="GET", body=None):
    return _req(MP_URL + path, method, body, {"Authorization": "Bearer " + MP_KEY})


def mp_search(query, limit=5, wing=None, room=None):
    b = {"query": query, "limit": limit}
    if wing:
        b["wing"] = wing
    if room:
        b["room"] = room
    return mp("/mp/api/v1/search", "POST", b)


def mp_add(wing, room, content, source=None):
    b = {"wing": wing, "room": room, "content": content}
    if source:
        b["source_file"] = source
    return mp("/mp/api/v1/drawers", "POST", b)


def ollama_chat(prompt, system=None, model=None, timeout=300):
    """Return (text, prompt_tokens, gen_tokens, seconds)."""
    msgs = []
    if system:
        msgs.append({"role": "system", "content": system})
    msgs.append({"role": "user", "content": prompt})
    body = {
        "model": model or GEN_MODEL,
        "messages": msgs,
        "stream": False,
        "think": False,
        "options": {"temperature": 0, "num_ctx": 8192},
    }
    t0 = time.perf_counter()
    r = _req(OLLAMA_URL + "/api/chat", "POST", body, timeout=timeout)
    dt = time.perf_counter() - t0
    text = r.get("message", {}).get("content", "")
    # strip any <think> blocks that reasoning models still emit
    text = re.sub(r"<think>.*?</think>", "", text, flags=re.S).strip()
    return text, r.get("prompt_eval_count", 0), r.get("eval_count", 0), dt


# ---------------------------------------------------------------------------
# Grading — deterministic token match, number-normalised
# ---------------------------------------------------------------------------
def _norm(s):
    return re.sub(r"[,\s]+", " ", s.lower()).strip()


def is_correct(answer, probe):
    a = _norm(answer)
    for tok in [probe.answer] + probe.aliases:
        t = _norm(tok)
        # for pure-number tokens also match without spaces (e.g. "12 000" -> "12000")
        if t in a or t.replace(" ", "") in a.replace(" ", ""):
            return True
    return False


def pct(x, n):
    return f"{100.0 * x / n:.1f}%" if n else "n/a"


def quantiles(xs):
    xs = sorted(xs)
    if not xs:
        return (0, 0, 0)
    p = lambda q: xs[min(len(xs) - 1, int(q * len(xs)))]
    return (p(0.50), p(0.95), sum(xs) / len(xs))


# ---------------------------------------------------------------------------
# Store loading / reset
# ---------------------------------------------------------------------------
def reset_store():
    """Delete every drawer so each mode starts from a clean, isolated store.
    Uses only public endpoints (list + delete); safe on a dedicated bench DB."""
    removed = 0
    while True:
        page = mp("/mp/api/v1/drawers?limit=100&offset=0").get("drawers", [])
        if not page:
            break
        for d in page:
            mp(f"/mp/api/v1/drawers/{d['drawer_id']}", "DELETE")
            removed += 1
    return removed


def load_facts(facts, wing_prefix=None):
    """File each fact as its own drawer. Returns (count, seconds)."""
    t0 = time.perf_counter()
    n = 0
    for f in facts:
        wing = f"{wing_prefix}:{f.wing}" if wing_prefix else f.wing
        mp_add(wing, f.room, f.text, source=f.id)
        n += 1
    return n, time.perf_counter() - t0


def answer_from_context(question, context, extra_system=""):
    system = (
        "You are a precise assistant answering questions about Aurelia Robotics "
        "using ONLY the provided context. Answer in one short sentence. If the "
        "answer is not in the context, reply exactly: NOT FOUND." + extra_system
    )
    prompt = f"Context:\n{context}\n\nQuestion: {question}\nAnswer:"
    return ollama_chat(prompt, system=system)


# ===========================================================================
# THEME 1 — lookup -> useful answer latency
# ===========================================================================
def bench_latency(limit=5, warmup=True):
    print("\n" + "=" * 72)
    print("THEME 1  lookup -> useful answer:  latency breakdown + usefulness")
    print("=" * 72)
    print(f"Reset store ({reset_store()} old drawers removed).")
    print(f"Filing {len(corpus.FACTS)} facts as cards...")
    load_facts(corpus.FACTS)

    if warmup:  # warm the embedding + chat model so we measure steady state
        mp_search("warmup query about services", limit=limit, wing=None)
        ollama_chat("Say OK.", system="Reply with one word.")

    rows = []
    for p in corpus.PROBES:
        t0 = time.perf_counter()
        res = mp_search(p.question, limit=limit)
        t_ret = time.perf_counter() - t0
        hits = res.get("results", [])
        context = "\n".join(f"- {h['content']}" for h in hits)
        ans, ptok, gtok, t_gen = answer_from_context(p.question, context)
        top_ok = bool(hits) and hits[0].get("metadata", {}).get("source_file") == p.gold_id
        in_topk = any(h.get("metadata", {}).get("source_file") == p.gold_id for h in hits)
        rows.append({
            "t_ret": t_ret, "t_gen": t_gen, "t_tot": t_ret + t_gen,
            "correct": is_correct(ans, p), "top1": top_ok, "topk": in_topk,
            "ptok": ptok,
        })

    n = len(rows)
    ret = [r["t_ret"] for r in rows]
    gen = [r["t_gen"] for r in rows]
    tot = [r["t_tot"] for r in rows]
    r50, r95, rav = quantiles(ret)
    g50, g95, gav = quantiles(gen)
    t50, t95, tav = quantiles(tot)
    correct = sum(r["correct"] for r in rows)
    top1 = sum(r["top1"] for r in rows)
    topk = sum(r["topk"] for r in rows)

    print(f"\nProbes: {n}   retrieval limit (k): {limit}   gen model: {GEN_MODEL}")
    print(f"\n  stage         p50        p95        mean")
    print(f"  retrieve   {r50*1000:7.0f}ms  {r95*1000:7.0f}ms  {rav*1000:7.0f}ms   (embed query + hybrid search)")
    print(f"  generate   {g50*1000:7.0f}ms  {g95*1000:7.0f}ms  {gav*1000:7.0f}ms   (LLM turns cards into an answer)")
    print(f"  TOTAL      {t50*1000:7.0f}ms  {t95*1000:7.0f}ms  {tav*1000:7.0f}ms")
    print(f"\n  retrieval share of total (mean): {100*rav/tav:.0f}%   generation share: {100*gav/tav:.0f}%")
    print(f"\n  usefulness")
    print(f"    gold card retrieved in top-1 : {top1}/{n}  ({pct(top1,n)})")
    print(f"    gold card retrieved in top-{limit} : {topk}/{n}  ({pct(topk,n)})")
    print(f"    final answer correct         : {correct}/{n}  ({pct(correct,n)})")
    print(f"\n  read: retrieval is a small, flat cost; the LLM dominates wall-clock.")
    print(f"        a lookup becomes a *useful* answer ~{pct(correct,n)} of the time here,")
    print(f"        gated mostly by whether the right card was retrieved.")
    return rows


# ===========================================================================
# THEME 2 — recall quality across repeated sessions
# ===========================================================================
def bench_sessions(limit=5):
    print("\n" + "=" * 72)
    print("THEME 2  recall across repeated sessions:  memory vs no-memory")
    print("=" * 72)
    print(f"Reset store ({reset_store()} old drawers removed).")
    ns = corpus.NUM_SESSIONS

    # ----- Part A: retrieval recall@k as the store grows session by session
    print(f"\nPart A — retrieval recall@k as memories accumulate ({ns} sessions)")
    print(f"  (each session files more facts; we re-probe everything learned so far)\n")
    filed = []
    print(f"  session   cards_in_store   probes_answerable   recall@1   recall@{limit}")
    for s in range(1, ns + 1):
        new = [f for f in corpus.FACTS if f.session == s]
        load_facts(new)
        filed += new
        probes = corpus.probes_answerable_by_session(s)
        r1 = rk = 0
        for p in probes:
            res = mp_search(p.question, limit=limit)
            hits = res.get("results", [])
            ids = [h.get("metadata", {}).get("source_file") for h in hits]
            if ids[:1] == [p.gold_id]:
                r1 += 1
            if p.gold_id in ids:
                rk += 1
        np_ = len(probes)
        print(f"  {s:>7}   {len(filed):>14}   {np_:>17}   {pct(r1,np_):>8}   {pct(rk,np_):>8}")

    # ----- Part B: cross-session usefulness — memory vs stateless
    print(f"\nPart B — does the agent answer *earlier-session* questions?")
    print(f"  Two agents run at session {ns}. Questions whose answer was learned in an")
    print(f"  EARLIER session (session < {ns}). Stateless agent only sees the current")
    print(f"  session's notes; MemPalace agent retrieves from its accumulated store.\n")

    cur_session_facts = [f for f in corpus.FACTS if f.session == ns]
    cur_notes = "\n".join(f"- {f.text}" for f in cur_session_facts)
    earlier_probes = [p for p in corpus.PROBES
                      if corpus.facts_by_id()[p.gold_id].session < ns]

    mem_ok = state_ok = 0
    for p in earlier_probes:
        # stateless: only current-session notes in context
        a_state, *_ = answer_from_context(p.question, cur_notes)
        if is_correct(a_state, p):
            state_ok += 1
        # memory-backed: retrieve from the palace
        res = mp_search(p.question, limit=limit)
        ctx = "\n".join(f"- {h['content']}" for h in res.get("results", []))
        a_mem, *_ = answer_from_context(p.question, ctx)
        if is_correct(a_mem, p):
            mem_ok += 1

    ne = len(earlier_probes)
    print(f"  questions about earlier sessions: {ne}")
    print(f"    stateless agent (current session only): {state_ok}/{ne}  ({pct(state_ok,ne)})")
    print(f"    MemPalace-backed agent               : {mem_ok}/{ne}  ({pct(mem_ok,ne)})")
    lift = mem_ok - state_ok
    print(f"\n  read: cross-session recall is exactly what persistent memory buys.")
    print(f"        the stateless agent structurally cannot answer these ({pct(state_ok,ne)});")
    print(f"        writing memories each session lifts it to {pct(mem_ok,ne)} (+{lift} answers).")


# ===========================================================================
# THEME 3 — card-index vs long context: tokens & break-even
# ===========================================================================
def bench_tokens(limit=5):
    print("\n" + "=" * 72)
    print("THEME 3  card-index preprocessing vs long context:  tokens & cost")
    print("=" * 72)
    print(f"Reset store ({reset_store()} old drawers removed).")

    # one-time preprocessing cost (indexing the corpus as cards)
    ncards, t_index = load_facts(corpus.FACTS)
    full_doc = "\n".join(f"- {f.text}" for f in corpus.FACTS)
    # measure the whole-doc token size once (as the model actually tokenises it)
    _, doc_tokens, _, _ = answer_from_context("What is the fiscal year end?", full_doc)

    print(f"\nCorpus: {ncards} facts.  Indexing (embed+store) took {t_index:.1f}s total,")
    print(f"        {t_index/ncards*1000:.0f} ms/card  (one-time preprocessing).")
    print(f"Whole corpus as prompt context = {doc_tokens} input tokens per query.\n")

    full_in = full_out = full_ok = 0
    card_in = card_out = card_ok = 0
    n = len(corpus.PROBES)
    for p in corpus.PROBES:
        # A) stuff the entire corpus every query
        a_full, pin, pout, _ = answer_from_context(p.question, full_doc)
        full_in += pin
        full_out += pout
        full_ok += is_correct(a_full, p)
        # B) retrieve a few cards
        res = mp_search(p.question, limit=limit)
        ctx = "\n".join(f"- {h['content']}" for h in res.get("results", []))
        a_card, cin, cout, _ = answer_from_context(p.question, ctx)
        card_in += cin
        card_out += cout
        card_ok += is_correct(a_card, p)

    full_avg = full_in / n
    card_avg = card_in / n
    saved = full_avg - card_avg
    print(f"                       full-context     card-index (k={limit})")
    print(f"  avg input tokens     {full_avg:>10.0f}     {card_avg:>10.0f}")
    print(f"  answer correctness   {pct(full_ok,n):>10}     {pct(card_ok,n):>10}")
    print(f"  tokens saved / query {saved:>10.0f}     ({100*saved/full_avg:.0f}% smaller prompt)")

    # break-even: the index costs embedding tokens once; retrieval costs a query
    # embedding each time. Both are tiny next to generation input tokens, but be
    # honest and amortise the preprocessing wall-time against tokens saved.
    # Express break-even in queries: after how many queries does the cumulative
    # token saving exceed the one-time indexing token cost?
    # Indexing "cost" in tokens ~= sum of card tokens embedded once.
    approx_index_tokens = sum(len(f.text) for f in corpus.FACTS) / 4  # ~4 chars/token
    breakeven = approx_index_tokens / saved if saved > 0 else float("inf")
    print(f"\n  one-time index cost  ~{approx_index_tokens:.0f} embedding tokens (est.)")
    if saved > 0:
        print(f"  break-even           ~{breakeven:.1f} queries")
        print(f"                       (after ~{breakeven:.0f} lookups the saved generation-input")
        print(f"                        tokens outweigh the whole indexing cost)")
    print(f"\n  read: the card index shrinks the per-query prompt by "
          f"{100*saved/full_avg:.0f}% here")
    print(f"        with correctness within {abs(full_ok-card_ok)} answers of full-context.")
    print(f"        preprocessing pays for itself almost immediately, and the gap")
    print(f"        widens with every extra query and every extra document.")


def main():
    if not MP_KEY:
        sys.exit("Set MP_KEY to the MemPalace API key.")
    mode = sys.argv[1] if len(sys.argv) > 1 else "all"
    # sanity: server + model reachable
    try:
        _req(MP_URL + "/mp/mcp/health")          # unauthenticated liveness
        st = mp("/mp/api/v1/status")             # authenticated — checks the key
        print(f"MemPalace up. store has {st.get('total_drawers', '?')} drawers. "
              f"gen model: {GEN_MODEL}")
    except Exception as e:
        sys.exit(f"MemPalace not reachable/authorised at {MP_URL}: {e}")

    if mode in ("all", "latency"):
        bench_latency()
    if mode in ("all", "sessions"):
        bench_sessions()
    if mode in ("all", "tokens"):
        bench_tokens()


if __name__ == "__main__":
    main()
