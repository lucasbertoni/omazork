# Zork II/III quirks the wrapper and mediation layer must account for

**Date:** 2026-08-29
**Question:** The engine choice is proven against Zork I (issue #7 spike, branch `prototype/mediation-spike`). What Zork II/III-specific behaviors must the wrapper and mediation layer account for? (Issue #11, part of #1.)

**Method:** Primary sources only: the `historicalsource/zork{1,2,3}` ZIL trees (read directly, including the compiled `.zap` assembly and the `COMPILED/*.z3` binaries); the `maloquacious/zmachine` and `maloquacious/quetzal` repos (tests and fetch scripts read); the Z-Machine Standard (§8.2 status line, §11 header, §22 Quetzal); and empirical runs — the issue #7 spike adapted into a probe harness that played the actual historicalsource `zork2.z3`/`zork3.z3` under `maloquacious/zmachine` v0.2.0, dumping per-turn `StatusLine` + Quetzal-decoded globals (G0 room, G1 score, G2 turns). Claims verified by running the games are marked *(observed)*; everything else cites a ZIL file.

## The shipped story files (what we bundle)

Header fields read directly from the `COMPILED/*.z3` binaries (all three verified: declared length = file length, header checksum matches recomputation):

| game | release | serial | checksum | size | SHA-256 (prefix) |
|---|---|---|---|---|---|
| zork1.z3 | 119 | 880429 | 0xbf44 | 86,838 | 37084966477dff67… |
| zork2.z3 | 63 | 860811 | 0x4492 | 92,524 | 3ae7d5558943e972… |
| zork3.z3 | 25 | 860811 | 0xf645 | 87,984 | b637a242865d0598… |

These are the last v3 releases. Flags1 bit 1 is **clear in all three** — all are "score games" per Standard §8.2.1; none is a time game. Caveat from the repos' READMEs: the ZIL trees are "a snapshot of the Infocom development system at time of shutdown … not necessarily the exact source code arrangement for production," so source and shipped binary may diverge in detail; every mechanism below was cross-checked empirically where it matters.

---

## 1. Status-line differences

**Mechanically there are none.** All three games:

- keep `HERE`/`SCORE`/`MOVES` in G0/G1/G2 — verified in the compiled assembly (`zork2/zork2dat.zap:5038-5040` and `zork3/zork3dat.zap:4749-4751` both open with `.GVAR HERE=0 / .GVAR SCORE=0 / .GVAR MOVES=0`; Zork I ships no `.zap` but shares the same parser/verbs code and visibly behaves the same);
- compile as plain v3 score games: the only VERSION directive in all three trees is `zork1/zork1.zil:5 <VERSION ZIP>`; no TIME directive exists anywhere, and Flags1 = 0x00 in all three binaries.

`zmachine`'s `Result.StatusLine` reflects this: for all three it returns `TimeGame=false`, `Name` = room short-name from G0, `Score`/`Turns` = G1/G2 *(observed on every turn of every probe run)*. The "Zork III shows a different right-hand status" folklore is **semantic, not mechanical**: SCORE stays in G1 and the interpreter draws it normally, but the game treats it as *potential* on a 0–7 scale (`zork3/3actions.zil:2066 <GLOBAL SCORE-MAX 7>`; `V-SCORE` prints "Your potential is N of a possible 7, in M moves" — *observed*). It sits at 0 for long stretches, which is why players remember the right-hand side as "different." No per-game special-casing is needed in the wrapper.

Quirks that do matter:

- **The status line spoils darkness.** `GOTO` sets `HERE` unconditionally before any light check (`gverbs.zil`, all three games) — only the room *description* is suppressed in the dark. So G0 always holds the real room and `StatusLine.Name` prints its real name even while the player sees "It is pitch black" *(observed: "Junction", "Damp Passage" in unlit Zork III)*. Authentic Infocom behavior, but the status line leaks what the prose withholds. The #7 rule — withhold the status line together with output, and always render the status line captured *with* a turn, never a fresher one — is exactly right and sufficient.
- **Room display names are not unique in Zork III.** The Land of Shadow is eight room objects `SHADOW-1..8` (`3actions.zil:3189+`) — the probe saw objects 30, 23, 186, 63, all named "Land of Shadow" *(observed)*; the Royal Puzzle and museum areas do the same. Anything keyed on `roomName` must expect many-to-one; anything keyed on object number must expect the name to be non-identifying. `data/waits/zork3.json`'s `roomName` matching for these regions is confirmed necessary.
- **`MOVES` is not "commands typed."** `WAIT` advances it by 3 *(observed)*, and Zork III's time machine bumps it manually (`3actions.zil:2735`). Play stats should count player inputs, not G2.

## 2. Darkness / lamp mechanics

Shared machinery (all three): the lamp burns down via a `QUEUE I-LANTERN 200` interrupt that ticks only while the lamp is on, stepping through a per-game `LAMP-TABLE` of warning stages; grue death is **probabilistic and immediate — there is no dark-turn counter**. Two triggers, both `<PROB 80>` (`gverbs.zil`, all three): walking through a nonexistent exit in the dark, and moving dark-room → dark-room. Moving from a *lit* room into darkness is always safe, and waiting/looking in darkness never kills *(both observed: first dark move from a lit room survived in both games; a dark→dark move died; long waits in dark Junction were safe)*.

Per-game differences that matter to the wrapper/mediation layer:

- **Lamp lifetimes differ:** Zork I: warnings at 200/300/370 turns-of-use, dead at **385** (`1actions.zil:2220`). Zork II and III: warnings at 200/500/600, dead at **650** (`2actions.zil:1934`, `3actions.zil:80`) *(first "a bit dimmer" warning at ~200 turns observed in Zork III)*. Zork I also has candles (75 turns), 6 matches, and a permanent torch.
- **Zork II opens lit** (Barrow through Garden/Carousel is moss-lit — *observed*, lamp off with full descriptions); **Zork III is dark from the second room onward** *(observed: Junction is grue country)*.
- **Zork II: the Wizard interferes with light and with the player, score-neutrally.** He appears at random and casts a random F-spell of twelve (`2actions.zil:3656-3669`): Filch steals a treasure to his trophy case, Feeble made the probe drop the lamp *(observed)*, Freeze roots you, etc. Two light quirks the wrapper should know: (i) casting **Fluoresce** with the captured wand sets `LIGHTBIT`+`ONBIT` on any object — a permanent portable light source (`gverbs.zil:593`); (ii) a mercy rescue: if you're in the dark with a burned-out lamp and `SCORE > 200`, the Wizard casts Fluoresce *on you* and sets `ALWAYS-LIT T`, after which `LIT?` returns true forever (`2actions.zil:3497-3506`). Any adapter that peeks "is the player in darkness" must account for `ALWAYS-LIT`, not just the room/lamp.
  - Grue repellent (Zork II): single-use spray, protection for **8 turns** (`2actions.zil:688-714`); while sprayed, both grue checks print harmless noises instead of killing.
- **Zork III: darkness is forced once, by the lake.** Entering the lake strips every carried item and *fries the lamp* ("The lamp isn't functioning (probably from having gotten wet)", `GO-ON-LAKE`, `3actions.zil:4535`). The intended path South Shore → `DARK-1` → `DARK-2` → Key Room must therefore be walked in true darkness, protected by Zork III's grue repellent — which lasts only **5 turns** here (`3actions.zil:4894-4896`) vs 8 in Zork II. In `DARK-1`/`DARK-2` the grue deaths get special "den of hungry grues" text (still 80%, still repellent-suppressed). Contrary to folklore, the Land of Shadow is *not* a dark-travel section: the shadow figure only appears while `,LIT` is true (`3actions.zil:3617`), so the player needs the lamp there.
- **Zork III: the earthquake** is queued on turn 1 as `QUEUE I-CLEFT <+ 70 <RANDOM 70>>` — it fires exactly once, on a uniformly random turn 71–140 (`gmain.zil:164-167`; *observed at ~turn 135*). It permanently rewrites topology: opens the cleft/Museum route and **collapses the aqueduct** (`AQ-FLAG` cleared; `3actions.zil:2451,4997,5010`). Score-neutral, never fatal by itself — but a casual playthrough that idles past it has a changed map between reveals. Other Zork III timers: lake drowning (warnings at 4/6 lake-turns, death at 8) and a 10%/turn roc attack while on the lake.
- **Death shapes differ:**
  - Zork I/II: `JIGS-UP` applies `<SCORE-UPD -10>` (`1actions.zil:4058`, `2actions.zil:3972`) and resurrects — the Zork II probe died entering the Room of Red Mist and was revived *in that room* with the −10 applied *(observed: 20→10)*.
  - Zork III: `JIGS-UP` has **no score penalty** (`3actions.zil:1991`); death plays the Dungeon Master prison scene and teleports the player back to the Endless Stair *(observed; potential unchanged at 0)*. Two hard-halt cases: dying in the time-machine past is an instant `<QUIT>`, and the **fourth death quits permanently** ("Good night, oh worthy adventurer!"). The engine returns `Status: Halted` with `State == nil` on quit — the wrapper's autosave-every-turn model already covers recovery, but the Console must expect a turn that ends the process-level session.

Net for the mediation layer: darkness, grues, the Wizard, and the earthquake are all **score-neutral except deaths**, so the delta-triggered pacing rule never withholds them — the right behavior (a withheld darkness warning or Wizard spell would be actively confusing).

## 3. Scoring-event shapes

Verified in the ZIL and, for the starred events, watched live as G1 deltas under the engine:

- **Zork I** (max 350): as derived in #9 — `VALUE`/`TVALUE` treasures, 4 visit rooms, the one-shot variable `<SCORE-UPD ,LIGHT-SHAFT>` (+13, then zeroed; `1actions.zil:2569,2578`), death −10, and the trophy-case **recompute** (`<SETG SCORE <+ ,BASE-SCORE <OTVAL-FROB>>>`, `1actions.zil:483`) which makes case-removals the only other negative deltas.
- **Zork II** (max 400, stated in `V-SCORE` — there is no `SCORE-MAX` global in Z2): take/visit values via `SCORE-OBJ`, plus hand-rolled awards: riddle +5 (`2actions.zil:2034`) *(observed: +5 in RIDDLE-ROOM, object 238)*, Oddly-Angled solve +5 (`:2235`), dragon/ice melt +5 (`:2541`), and the demon's `<SCORE-UPD 2>` **per treasure handed over, repeatable up to 10 treasures** (`:3315`, `TREASURES-MAX 10`) — the one genuinely repeating delta in the trilogy. Treasure take *(observed: pearl necklace +15 in PEARL-ROOM)*; death −10 *(observed)*.
- **Zork III** (max 7, "potential"): exactly seven `+1` awards, each latched one-shot (all in `3actions.zil`; the `shadow.zil`/`tm.zil` scoring sites are dead code — those files are **not in the build**, per `zork3.zil`'s insert list):

  | event | trigger | latch | site |
  |---|---|---|---|
  | Royal Puzzle | first wall push inside the puzzle | `CPPUSH-FLAG` | :376 |
  | Time machine | push button with dial set to 776 | `TM-POINT` | :2638 |
  | Shadow appears | figure materializes in Land of Shadow (random, needs light) | `SHADOW-POINT-1` | :3623 |
  | Shadow struck | **first `ATTACK FIGURE WITH SWORD`** (not "figure wounded") | `SHADOW-POINT-2` | :3421 |
  | Cliff | **first entry to Cliff Ledge** (M-ENTER — not the chest exchange) | `MAN-POINT` | :4146 |
  | Lake | first entering the lake (M-ENTER `ON-LAKE`) | `LAKE-POINT` | :4527 |
  | Scenic Vista | touching/rubbing the viewing table | `VIEW-POINT` zeroed | :4643 |

  *(observed: +1 on the shadow's appearance in room object 23, +1 on the first attack command — matching the ZIL triggers exactly)*. **Nothing ever decreases Zork III's score**: the only `(VALUE …)` properties in the build are `(VALUE 0)` on the lamp and sword, `SCORE-UPD` is never called with a nonzero argument, and death is score-neutral *(observed)*.
  **The endgame is gated on inventory, not score:** `LOOK-LIKE-DM?` (`3actions.zil:1429`) requires carrying cloak, hood, amulet, staff, ring, book, and key; `,SCORE` is never read anywhere in the Z3 build except by `V-SCORE`'s display. Potential 7 correlates with readiness but does not gate it.

Consequences for the casual-pacing rule (delta + room):

- **Zork II: the rule ports unchanged** — large distinctive deltas, −10 deaths reveal-now. Exercised end-to-end in the probe.
- **Zork III is delta-degenerate (+1 everywhere)**, as `data/waits/NOTES.md` says; rooms (by `roomName` where objects are many-to-one) do the disambiguation. Two row-note refinements, neither behavior-changing: `shadow-struck` fires on the first attack *command*, and `cliff-encounter` fires on first *entry* to the ledge, before any chest business.
- The demon's repeatable +2 means Zork II's `demon-paid` row can match up to ten times in one playthrough — the wait table's per-event `waitMinutes` applies each time, which is acceptable (all are the same beat), but "score event = fixed, published, once" phrasing in CONTEXT.md is slightly too strong for this one case.
- **Zork III deaths are invisible to a delta trigger** (delta 0). Under the current rule that's correct-by-construction (only nonzero deltas are withheld, so Zork III deaths always show live) — but any *achievement* keyed on "first death" cannot use score in Zork III; it needs the death text or the teleport-to-Endless-Stair transition. And the wrapper must handle the two `<QUIT>` deaths (time-machine past; fourth death) as engine `Halted` with no resumable state for that turn.

## 4. Do the historicalsource COMPILED files run under `maloquacious/zmachine`?

**Yes — empirically verified, and the library already ships the identical bytes as fixtures.**

- **The bundled fixtures ARE the historicalsource binaries.** `zmachine`'s `testdata/stories/{zork1-r119-880429,zork2-r63-860811,zork3-r25-860811}.z3` are byte-identical (SHA-256 match) to `historicalsource/zork{1,2,3}` `COMPILED/*.z3`, with Microsoft MIT license files alongside. What the engine ships and what omazork bundles are the same story files.
- **The "second editions" of its extra tests are** ***different*** **releases.** `local_stories_test.go` + `testdata/local/fetch.sh` pull the *Lost Treasures of Infocom* (1991) ISO from archive.org and check Zork I r88/840726 (chk 0xa129), Zork II r48/840904 (0xd899), Zork III r17/840727 (0x2e7a) — header identity, one Start→snapshot→Restore→`look` cycle, and cross-edition state rejection. So the historicalsource releases do **not** match those editions (they're later), and that test tier tells us little about our files.
- **Upstream depth is Zork-I-heavy:** the full scripted playthrough (5 seeds) and the Frotz differential tests (transcripts, status lines, state, Frotz-save restore) cover only Zork I r119. Zork II r63 appears in no Go test; Zork III r25 appears only as the "wrong story" in a Quetzal-IFhd rejection test. Upstream coverage for II/III is smoke-level.
- **Empirical verification (this research), historicalsource binaries under `zmachine` v0.2.0 + `quetzal` v0.2.4:**
  - Zork II r63: correct banner (Release 63 / Serial 860811); runs to 184+ turns across Barrow→Carousel→Riddle/Pearl; scoring (+5/+15), death/resurrection (−10), Wizard events, carousel exit randomization (deterministic per seed, varies across seeds) — no engine errors; status line and Quetzal peek correct every turn.
  - Zork III r25: correct banner (Release 25 / Serial 860811); Junction/Shadow Land/combat, grue deaths + Dungeon Master revival, and a 360-turn idle during which the earthquake and lamp-dimming interrupts fired on schedule — no engine errors.
  - Snapshot/restore round-trip on both: play N turns, rebuild a fresh machine from the mid-run `Result.State`, replay the back half — outputs byte-identical, including a mid-combat restore in Zork III.
  - Header checksums verified arithmetically (the `verify` verb isn't in Zork II/III's vocabulary).
- Engine caveats relevant here (from its docs/source): `sound_effect` is decoded-and-ignored (Zork I–III don't use it); in-story SAVE/RESTORE opcodes report failure by design — the host owns persistence, which matches omazork's checkpoint model but means in-game SAVE must be intercepted by the wrapper, not passed through; PRNG is seedable and Frotz-compatible, so carousel/Wizard/combat randomness is reproducible per seed.

## What this refines in existing map decisions

1. **Pacing rule (delta + room): holds for all three games.** Empirically exercised in Zork II (+5, +15, −10) and Zork III (+1, +1). No change.
2. **Wait tables (`data/waits/`):** shapes confirmed against ZIL and live runs. Minor note-level refinements: zork3 `shadow-struck` triggers on the first attack command; `cliff-encounter` on first entry to the ledge; zork2 `demon-paid` is repeatable (up to 10×), so that row will match multiple times — acceptable, but CONTEXT.md's "score event … fixed, published moments" wording is slightly too strong for it.
3. **Death handling:** "death = −10, reveal now" is Zork I/II-only. Zork III deaths are delta-0 (never withheld — correct by construction), but two Zork III deaths hard-quit the story (time-machine past; fourth death): the wrapper must tolerate `Halted` + nil state and lean on the previous turn's autosave. First-death achievements in Zork III need a non-score trigger.
4. **Status line:** no per-game handling needed; the #7 withhold-with-output rule suffices. Don't surface `MOVES` as "commands entered" in play stats.
5. **Room identity:** Zork III requires `roomName` matching for Land of Shadow (8 objects) and the puzzle/museum areas — already in the tables; any future room-keyed feature (achievements, recaps) inherits the same many-to-one caveat.
6. **Engine risk:** upstream II/III tests are thin, but upstream bundles our exact bytes and this research's probe runs passed cleanly; the real implementation should carry a scripted II/III smoke run in CI (this research's probe scripts are a starting point).

## Sources

- `historicalsource/zork1|zork2|zork3` — ZIL sources (`gverbs.zil`, `gparser.zil`, `gmain.zil`, `gclock.zil`, `1actions.zil`/`2actions.zil`/`3actions.zil`, `*dungeon.zil`), compiled assembly (`zork2dat.zap`, `zork3dat.zap`), and `COMPILED/*.z3` binaries (headers/checksums read directly). Line numbers cited inline.
- `maloquacious/zmachine` (`local_stories_test.go`, `testdata/local/fetch.sh`, `testdata/stories/`, `playthrough_test.go`, `differential_test.go`, `StatusLine` in `machine.go`) and `maloquacious/quetzal`.
- Z-Machine Standards Document §8.2, §11, §22.
- Empirical probe harness (adapted from `prototype/mediation-spike`), run 2026-08-29 against the historicalsource binaries under `zmachine` v0.2.0 / `quetzal` v0.2.4.
