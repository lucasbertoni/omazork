# Achievement tables — curation notes

Per-game achievement tables for the Console's achievements list (decided in
[Casual extras: achievements and stats](https://github.com/lucasbertoni/omazork/issues/10),
authored for [Achievement tables](https://github.com/lucasbertoni/omazork/issues/13)).
Sibling to the wait tables in `data/waits/` — score-event triggers reuse those tables'
event ids so delta+room matching is implemented once.

## Semantics (from #10)

- **Lifetime persistence, per game**: unlocks survive new playthroughs (unlike play
  stats). Both modes surface them; in Casual, an achievement on a withheld turn
  unlocks at the **reveal**, folded into the recap.
- **`hidden: true`** renders as "???" (name and description) until unlocked. Policy:
  mid/late-game puzzle payoffs and ending beats are hidden; early beats, generic
  milestones, and score caps are visible. Grues are not a spoiler — the game itself
  warns about them in the second dark room.
- Names describe in-game events in our own words — no Infocom/Zork branding beyond
  naming the games (trademark note in #10).

## Table format

```jsonc
{
  "game": "zork1",
  "achievements": [
    {
      "id": "into-the-dark",         // stable key, ours; unlock state is stored by (game, id)
      "name": "Into the Dark",
      "description": "Descend into the Great Underground Empire.",
      "hidden": false,
      "trigger": { ... }              // one of the three types below
    }
  ]
}
```

### Trigger types

1. `{ "type": "score-event", "eventId": "<id>" }` — fires when the wait table's
   event of that id matches a turn's (delta, room). One matcher serves both tables.
2. `{ "type": "score-reaches", "score": N }` — fires the first time the score global
   (G1) is ≥ N. Trivial to detect from the Quetzal peek; used for the max-score
   achievement in each game.
3. `{ "type": "milestone", "kind": "<kind>" }` — wrapper-detected non-scoring events:

   | kind | detection |
   |---|---|
   | `first-death` | Zork I/II: the −10 death delta suffices. **Zork III: no score penalty on death** (#11), so the wrapper must detect death directly — the `JIGS-UP` death text followed by the teleport back to the Endless Stair. Detect once, emit a `death` event per game; don't key on score. |
   | `grue-death` | A death whose turn output contains "grue" (all grue deaths across the trilogy say so: "eaten by a grue" / "den of hungry grues"). |
   | `first-checkpoint` | First intercepted in-game SAVE (the wrapper already intercepts SAVE/RESTORE as checkpoints, per #6). |
   | `game-won` | The winning ending: engine halts after victory text (Zork I: entering the stone barrow; Zork II: past the crypt; Zork III: becoming the Dungeon Master). Distinguish from quit/death halts by the preceding turn's output. |
   | `final-death` | Zork III only: the fourth death, which hard-QUITs the story ("Good night, oh worthy adventurer!"). The wrapper must already recognize this halt to tolerate it (#11); the achievement rides on that detection. |

## Known collisions and caveats

1. **Zork III shadow events collide by delta+room** (both +1 in "Land of Shadow", as
   documented in `data/waits/NOTES.md`). For waits it's harmless (both 0); for
   achievements it matters which unlocks. Resolution: the appearance necessarily
   precedes the strike, so the matcher should treat the **first** Land-of-Shadow +1 of
   a playthrough as `shadow-appears` and the **second** as `shadow-struck`. Since
   achievements are lifetime and both usually happen in the same encounter, even a
   misattribution self-heals within one playthrough.
2. **Zork II `demon-paid` is repeatable** (up to 10×, per #11) — deliberately not an
   achievement; a repeating trigger fits a wait, not a milestone. The tribute arc is
   covered by `the-wand` instead.
3. **`score-reaches` and case removals**: Zork I's trophy-case score is recomputed on
   removal, so the score can dip after touching 350. "First time ≥ N" latches on the
   touch, which is the right behavior.
4. **Zork I "first treasure" was considered and dropped**: no generic "any treasure"
   key exists in delta+room terms, and adding one means a second matching system for
   one achievement.
5. **Thief-slaying, Cyclops, Wizard spells, the earthquake** — memorable but
   score-neutral and text-only; detecting them means grepping transcript prose, which
   is fragile across phrasings. Left out on purpose: every trigger above rests on
   state the wrapper already reads (score global, room, death/halt handling, SAVE
   interception), except the single word "grue".

## Curation shape

Zork I: 13 (8 score-keyed, 1 score-cap, 4 milestones). Zork II: 12 (7, 1, 4).
Zork III: 13 (7 — all seven potential points, 1, 5 incl. `final-death`).
