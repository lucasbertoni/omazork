# Wait tables — derivation notes

Static per-game wait tables for the casual-mode mediation layer (decided in
[Casual pacing rules](https://github.com/lucasbertoni/omazork/issues/5), authored for
[Score-event wait tables](https://github.com/lucasbertoni/omazork/issues/9)).

## Source of truth

Score events were extracted from the MIT-licensed ZIL sources at
`historicalsource/zork{1,2,3}` (the same trees whose `COMPILED/*.z3` files we bundle):

- **Zork I** (max 350): treasures carry `(VALUE n)` (awarded on first take, via
  `SCORE-OBJ`) and `(TVALUE n)` (awarded on trophy-case deposit — the case score is
  *recomputed* by `OTVAL-FROB` on every deposit/removal, so removals produce negative
  deltas). Four rooms carry `(VALUE n)` awarded on first visit. One hand-rolled award:
  `SCORE-UPD ,LIGHT-SHAFT` (+13) for arriving in the Drafty Room empty-handed with
  light. Death is `SCORE-UPD -10`.
- **Zork II** (max 400): treasures carry `(VALUE n)` on first take (no trophy case);
  five rooms carry visit values; four hand-rolled awards in `2actions.zil`
  (riddle +5, oddly-angled maze +5, dragon melts ice +5, demon paid +2 per treasure).
  Death is `SCORE-UPD -10`.
- **Zork III** (max 7): no treasure/room values; seven one-point `SETG SCORE <+ ,SCORE 1>`
  awards scattered through `3actions.zil`/`shadow.zil`/`tm.zil`, each latched by a
  one-shot flag. No death penalty.

## Table format

```jsonc
{
  "game": "zork1",
  "maxScore": 350,
  "fallbackMinutes": { "min": 15, "max": 45 },   // random wait for unmatched deltas
  "events": [
    {
      "id": "take-torch",          // stable key, ours
      "delta": 14,                  // observed score change
      "room": "TORCH-ROOM",         // ZIL room id, null = match any room
      "roomName": "Torch Room",     // the room's DESC — what the status line shows
      "waitMinutes": 45,            // 0 = never delay this event
      "note": "..."
    }
  ]
}
```

Matching: `(delta, room)` — an entry with `room: null` matches its delta in any room
(checked after exact-room entries). Anything unmatched falls back to
`fallbackMinutes` random, per #5. The wrapper observes the current room via the
Quetzal decode; `room` is the ZIL identifier and `roomName` the display string —
whichever the adapter resolves more cheaply, prefer the ZIL id where object numbers
can be mapped at build time, else match on `roomName`.

`waitMinutes: 0` means reveal immediately (combat-adjacent or punitive events).
`deltaMax: -1` (Zork I `case-removal`) matches any negative delta in that room.

## Known ambiguities (delta+room cannot disambiguate)

1. **Zork I trophy-case deposits** all happen in the Living Room, and several
   treasures share a TVALUE (e.g. delta 5 covers eight different treasures). Harmless:
   one table row per distinct delta, same wait for all colliding treasures.
2. **Zork I portable/moving treasures** — sceptre (in the coffin), emerald (in the
   buoy), canary (egg opened by the thief), bauble (dropped by the songbird) — have no
   fixed take room; their entries use `room: null`. Consequence: a delta-4/5/6 take in
   some *other* treasure's home room after the thief has relocated things will match
   the wrong row. Both rows carry similar waits, so the error is bounded.
3. **Zork I first-take-only**: `VALUE` is awarded once per treasure. Re-takes score 0
   and are invisible to the mediator by design.
4. **Zork II wandering holders** — the Wizard (wand) and unicorn (gold key) move, so
   those entries are `room: null`. The oddly-angled maze solve (+5) fires in whichever
   room completes the diamond, also `room: null`.
5. **Zork III is delta-degenerate**: every event is +1, so the room does all the work.
   The two shadow-figure points fire in variable rooms (SHADOW-1..8, all displaying
   "Land of Shadow") and collide with each other — both are combat moments with
   `waitMinutes: 0`, so the collision never matters. The Technology Museum and Royal
   Puzzle each span multiple room ids sharing one display name; match on `roomName`.
6. **Deaths** (Zork I/II, delta -10, any room) must reveal immediately — a withheld
   death would leave the player issuing commands into a corpse.

## Tuning rationale

Waits scale with narrative weight, not just delta: room-visit points and small finds
sit at 5–30 min, mid treasures 30–60, marquee puzzle payoffs (light shaft, dragon,
black sphere, Royal Puzzle) 60–90, and Zork II's climactic wand theft 120. Everything
combat-adjacent (thief's Treasure Room, Cerberus, the shadow figure) is pinned to 0
per the "combat never waits" rule from #5.
