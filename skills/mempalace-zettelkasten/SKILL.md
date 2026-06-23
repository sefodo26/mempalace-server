---
name: mempalace-zettelkasten
description: Capture an idea as an atomic, self-contained note in your own words, link it to related notes, and store it in MemPalace so it resurfaces fast later. Use when the user wants to add a thought/idea/insight as a Zettelkasten note, reformulate notes atomically, or build a linked knowledge base.
---

# Zettelkasten in MemPalace

The Zettelkasten ("slip box") method turns scattered thoughts into a linked web
of small, reusable notes. This skill applies it on top of MemPalace so ideas are
**reformulated in your own words** and **found again quickly** through meaning
and links.

The four rules of a good Zettel:

1. **Atomic** — one idea per note. If it needs "and", split it.
2. **In your own words** — reformulate; never paste raw. Writing it forces
   understanding and makes it searchable by *concept*, not just keywords.
3. **Self-contained** — readable alone, months later, without the source.
4. **Linked** — every note connects to at least one other note.

## How it maps to MemPalace

| Zettelkasten | MemPalace |
| --- | --- |
| Slip box | one wing, e.g. `zettelkasten` |
| Topic / cluster | a room, e.g. `learning`, `product-ideas` |
| A single note (Zettel) | a drawer |
| Stable note ID | a slug at the top of the drawer, e.g. `z-2026-0042-spaced-repetition` |
| Link between notes | inline `[[z-id]]` refs **and** a `mempalace_create_tunnel` |
| Index / structure note | a special drawer in room `index` that lists entry points |

## Access

Use **MCP tools** (`mempalace_*`) if connected, else the **REST API**
(`/mp/api/v1`) with `curl`. Writing needs a **write** key; reading is fine with
a read-only key.

## Adding a note (the main flow)

1. **Catch the raw idea** from the user (a sentence, a paragraph, a link).

2. **Search first — always.** Run `mempalace_search` with the idea's core
   concept *before* writing. This does two jobs:
   - finds notes to **link** to,
   - reveals if the idea already exists (then *extend/refine* that note instead
     of duplicating it).

3. **Reformulate atomically.** Rewrite the idea in clear, plain words as a
   standalone claim. One idea only. If you find two, make two notes.

4. **Give it a stable ID and title.** Use a slug like
   `z-<YYYY>-<NNNN>-<short-title>`. Keep the ID forever, even if you edit the
   text — links depend on it. Ask the user for the next free number, or derive
   one (e.g. count existing notes + 1).

5. **Write the note body** in this shape:

   ```
   z-2026-0042-spaced-repetition — Spaced repetition beats cramming

   Reviewing material at growing intervals fixes it in long-term memory far
   better than one long session, because each near-forgetting reload strengthens
   the trace.

   Links: [[z-2026-0017-forgetting-curve]] [[z-2026-0031-active-recall]]
   Source: Make It Stick, ch. 3
   ```

6. **File it** with `mempalace_add_drawer`:
   - `wing` = `zettelkasten`
   - `room` = the cluster (e.g. `learning`)
   - `content` = the note body above
   - `added_by` = `zettelkasten`

7. **Create the links both ways.** For each related note found in step 2, call
   `mempalace_create_tunnel` between the rooms so the connection is queryable,
   and make sure the `[[z-id]]` ref appears in the body. Prefer 2–3 strong links
   over many weak ones.

8. **Touch the index.** If this note opens or anchors a topic, add its ID + title
   to the topic's structure note in room `index` (create it if missing). The
   index is your fast entry point into a cluster.

9. **Report** the new note's ID, room, and which notes it linked to.

## Finding notes again (fast retrieval)

Combine three moves — that is the whole point of a Zettelkasten:

- **By meaning:** `mempalace_search({query, wing: "zettelkasten"})`. Because
  notes are in your own words, concept search hits well even with different
  wording.
- **By link:** from a found note, `mempalace_follow_tunnels` / `mempalace_traverse`
  to walk to connected ideas. Following links surfaces things keyword search
  misses.
- **By index:** open the room `index` structure note for a topic and jump to the
  listed IDs.

When answering a question, gather a few linked notes, then synthesize — cite the
note IDs so the chain of thought stays traceable.

## Good practices

- **Split, don't stuff.** A note that grew two ideas → make a second note and
  link them.
- **Reformulate on revisit.** If you re-read a note and word it better, update
  the body but keep the ID.
- **No orphans.** A note with zero links is hard to refind — always connect it.
- **Index per topic, not one giant index.** Small structure notes scale better.
- **Stable IDs are sacred.** Renaming the title is fine; changing the ID breaks
  `[[refs]]` and tunnels.
