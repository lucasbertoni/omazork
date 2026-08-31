# Action duration tables — what lives here

The action-wait data the casual-mode mediation layer prices turns from, per
[docs/action-waits.md](../../docs/action-waits.md) §4. Every file carries
`"schemaVersion": 1` and the wrapper asserts an exact match at load.

| File | Role | Written by |
|---|---|---|
| `<game>.json` | Generated duration table: rooms map, edge rows, (verb, object) rows, vocab | `go run ./cmd/actiongen` — never hand-edited |
| `<game>.overlay.json` | Hand-curated overrides, per-game verb defaults, `acknowledged` map | humans only |
| `verbs.json` | Shared verb-default table (§10) | hand-authored |
| `calibration.json` | Shared calibration thresholds (§6) | hand-authored |

The per-game `<game>.llm.json` cache (§5.3) and `<game>.calibration.json`
report (§5.4) land with the LLM pass
([#35](https://github.com/lucasbertoni/omazork/issues/35)) and the calibration
slice ([#36](https://github.com/lucasbertoni/omazork/issues/36)).

## Regenerating

```
go run ./cmd/actiongen            # rewrite all three tables from data/extract/
scripts/validate.sh               # what CI runs: regen-and-diff, then validate
```

Generation is deterministic: the same `data/extract/<game>.json` always renders
the same bytes, so a regeneration that changes a table is a reviewable diff.

## Rule pricing

Edges are priced by the §2 formula — a 2-minute base plus mechanical modifiers
(up +2, down +1, water +3, door or conditional passage +1, dark destination +1),
clamped to the Movement band. Every modifier is spelled out in the row's `note`,
so a curator reads the arithmetic and not just the total. Multiple exits joining
the same `(from, to)` collapse to the cheapest, with the collision named in the
note; a pair any of whose exits drifts stays `kind: "drift"`, so collapsing
never revokes drift-set membership. Blocked exits and self-loops are not rows
at all — they never change the room, and PER-routine exits have no static
destination to key a row on: 7 / 11 / 59 of them (Zork I / II / III) wait on
full LLM classification, and `actiongen` prints the count on every run.

"Water" means a room that is neither solid ground nor open air — the games hang
`NONLANDBIT` on the volcano airspace and on balloon-reachable ledges too, so
both are excluded. Zork III's lake rooms carry `RLANDBIT` and so read as land
here; that one is for the LLM nudge or an overlay row.

The `actions` array is empty in every table: nothing about a `(verb, object)`
handler pair is mechanically priceable, so those rows are the LLM pass's to
write ([#35](https://github.com/lucasbertoni/omazork/issues/35)). Until then
every action resolves through `verbs.json`. Expect edge medians to sit at the
low end of the Movement band until the LLM nudge lands — the §6 histogram gates
run on the finished tables.

## Curating

Overlay rows override a base row, add one it lacks, and are the only place a
duration may exceed the 30-minute uncurated cap; above 60 minutes a row must
carry a `reason`. `minutes: 0` forces instant. Overlay keys are orphan-checked:
a row or `acknowledged` key that no longer names a live room, verb, or object
fails validation rather than sitting silently inert. Layering happens in memory
at load — there is no merged artifact, and regeneration never touches curation.
