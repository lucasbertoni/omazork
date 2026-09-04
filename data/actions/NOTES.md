# Action duration tables — what lives here

The action-wait data the casual-mode mediation layer prices turns from, per
[docs/action-waits.md](../../docs/action-waits.md) §4. Every file carries
`"schemaVersion": 1` and the wrapper asserts an exact match at load.

| File | Role | Written by |
|---|---|---|
| `<game>.json` | Generated duration table: rooms map, edge rows, (verb, object) rows, vocab | `go run ./cmd/actiongen` — never hand-edited |
| `<game>.llm.json` | LLM cache: one row per key with `inputHash`, `class`, `minutes`, `rationale` | `go run ./cmd/actiongen -llm` — never hand-edited |
| `<game>.overlay.json` | Hand-curated overrides, per-game verb defaults, `acknowledged` map | humans only |
| `verbs.json` | Shared verb-default table (§10) | hand-authored |
| `calibration.json` | Shared calibration thresholds (§6) | hand-authored |

The per-game `<game>.calibration.json` report (§5.4) lands with the calibration
slice ([#36](https://github.com/lucasbertoni/omazork/issues/36)).

## Regenerating

```
go run ./cmd/actiongen            # rewrite all three tables from data/extract/ + the committed cache
go run ./cmd/actiongen -llm       # infer the rows the cache is missing or stale on, then rewrite
go run ./cmd/actiongen -llm -batch 100   # same, but at most 100 rows per game this sitting
scripts/validate.sh               # what CI runs: regen-and-diff, then validate
```

Generation is deterministic: the table is a pure function of
`data/extract/<game>.json` and `<game>.llm.json`, so a regeneration that changes
a table is a reviewable diff. Only `-llm` ever reaches a model; a plain run and
`-check` never do.

`-llm` goes through the `claude` CLI in headless mode, on the logged-in Claude
subscription — one `claude -p` process per row, tools off, one turn, settings
ignored, a fixed system prompt, run from an empty scratch directory so no
CLAUDE.md or memory is picked up. Setting `ANTHROPIC_API_KEY` switches it to the
direct Messages API instead. A full pass is ~1300 rows; `-batch N` stops each
game after N rows and writes the partly-warm cache, and the next run continues
from there. Don't commit between batches: a cache that exists but is incomplete
fails `-check`, by design.

## Rule pricing

Edges are priced by the §2 formula — a 2-minute base plus mechanical modifiers
(up +2, down +1, water +3, door or conditional passage +1, dark destination +1),
clamped to the Movement band. Every modifier is spelled out in the row's `note`,
so a curator reads the arithmetic and not just the total. Multiple exits joining
the same `(from, to)` collapse to the cheapest, with the collision named in the
note; a pair any of whose exits drifts stays `kind: "drift"`, so collapsing
never revokes drift-set membership. Blocked exits and self-loops are not rows
at all — they never change the room, and PER-routine exits have no static
destination to key a row on: 7 / 11 / 59 of them (Zork I / II / III) stay
unpriced, and `actiongen` prints the count on every run.

"Water" means a room that is neither solid ground nor open air — the games hang
`NONLANDBIT` on the volcano airspace and on balloon-reachable ledges too, so
both are excluded. Zork III's lake rooms carry `RLANDBIT` and so read as land
here; that one is for the LLM nudge or an overlay row.

## The LLM pass

Static edges are rule-priced and then nudged inside the Movement band around
that baseline; `(verb, object)` handler pairs are classified outright, since
nothing about a handler is mechanically priceable. Every answer is read from the
committed `<game>.llm.json` cache, keyed by row with
`inputHash = sha256(modelId + renderedPrompt)`. The rendered prompt embeds both
the template and the payload, so editing one of the three Go template constants
invalidates exactly that kind's rows and re-pinning the model id invalidates
everything — either way the re-inference lands as a reviewable diff.

A row whose cached answer is missing or stale is simply not refined: the edge
keeps its rule price and the handler pair gets no row at all, resolving through
`verbs.json` instead. Nothing is invented and nothing is clamped — an
out-of-band answer gets one retry with the band restated and then hard-fails the
run. A Dramatic nomination is cached raw and written to the table at the 30-min
uncurated cap with an `overlay candidate` note — on any row kind, edge or
handler — because drama needs a human signature (§2); a nomination that already
sits at the cap still carries the note, so it still reaches the curation queue.
The model's own one-sentence rationale is the row's `note`.

Until a game's cache is committed, `-check` reports how many rows await a first
pass instead of failing on all of them; the moment a cache exists it must cover
every row, or `-check` fails rather than let a regeneration reach for the API.

Which rows the pass covers: every collapsed edge row, and every `(verb, object)`
pair an object's ACTION routine dispatches on. Pairs are dropped when the
runtime could never look them up — a verb neither `verbs.json` nor the generated
vocab can address, an object no noun names — or when a wrapper hard class
outranks the row: fast verbs (except PRAY, which §10 prices at the altar as a
handler pair) and melee verbs the shared table declines to price.

The three games leave 7 / 11 / 59 PER-routine exits unpriced. Those exits resolve
their destination at run time, so there is no `(from, to)` key to hang either a
rule price or an LLM answer on; they fall to the 1-minute global fallback, and a
single overlay row can adjust any that turn out to matter. The LLM pass does
classify a routine-typed exit that *does* carry a static destination — none of
the three games currently has one.

Until a cache is committed the `actions` array stays empty and edge medians sit
at the low end of the Movement band; the §6 histogram gates run on the finished
tables.

## Curating

Overlay rows override a base row, add one it lacks, and are the only place a
duration may exceed the 30-minute uncurated cap; above 60 minutes a row must
carry a `reason`. `minutes: 0` forces instant. Overlay keys are orphan-checked:
a row or `acknowledged` key that no longer names a live room, verb, or object
fails validation rather than sitting silently inert. Layering happens in memory
at load — there is no merged artifact, and regeneration never touches curation.
