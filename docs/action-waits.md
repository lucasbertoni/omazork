# Action Waits — Specification

**Status: locked.** Destination artifact of the wayfinder map
[Contextual action-time waiting (#17)](https://github.com/lucasbertoni/omazork/issues/17).
Each section cites the ticket that decided it; the tickets hold the full discussion,
this document holds everything a build session needs.

Casual mode stops keying waits on score deltas. Every player action carries an
inferred real-world duration — an **action wait** — generated offline from the
historicalsource ZIL, committed as deterministic per-game tables, and layered with a
hand-curated overlay at load time. The 15–45 min random fallback is gone. Classic
mode is untouched.

## 1. Charter (binding on everything below)

- Action waits **replace** score waits entirely. In-flight pending waits on existing
  saves are honored (`session.Pending` is self-contained, `MaturesAt` is absolute);
  no other migration.
- One Pending mechanic (withhold + block + recap) is reused, with quieter copy for
  mundane waits (§8). Hour-scale waits survive, sourced from rare dramatic actions
  via per-class caps — no global ~5 min ceiling.
- Movement waits key on the **directed room edge** `(origin, destination)`. Other
  actions key on `(verb, object)` with a verb-only fallback.
- Durations are inferred **offline**: rule-based inference first, an LLM pass for
  the tail; generated tables are committed and deterministic, with a hand-curated
  overlay layered on top.
- Wrapper-enforced hard classes override any table: in-combat turns are instant; a
  fixed fast-verb list is always instant; failed/no-op turns cost nothing.
- `data/waits/*.json` become pure score-**event** tables for achievements (§9); all
  timing moves to the action tables.
- Out of scope: implementing this spec (build sessions), Classic mode, achievement
  changes beyond decoupling triggers from wait timing.

## 2. Duration model ([#20](https://github.com/lucasbertoni/omazork/issues/20))

Five classes. Values are fixed integer minutes — one duration per row, no runtime
ranges or randomness.

| Class | Examples | Band (min) | Cap (min) |
|---|---|---|---|
| Instant | look, inventory, combat turns, failed/no-op turns | 0 | 0 |
| Movement | one directed room edge | 1–10 | 15 |
| Manipulation | take, drop, open, close, put, read | 0–3 | 5 |
| Mechanism | inflate boat, tie rope, turn bolt, ring bell | 3–20 | 30 |
| Dramatic | exorcism, prayer, treasure-vault moments | 30–180 | 240 |

**Edge pricing rule**: every edge gets a 2-minute base plus mechanical modifiers —
vertical up +2, vertical down +1 (asymmetric), water travel +3, door/conditional
passage +1, dark underground +1 — then the LLM may nudge within the Movement band
(§5.3). Rules guarantee coverage; the LLM refines feel.

**Drama requires a human signature**: the LLM may *nominate* Dramatic, but uncurated
rows are written at the 30-min Mechanism cap. Only an overlay row may exceed 30 min;
above 60 min it must carry a `reason`.

### Lookup precedence (first match wins; amended by [#28](https://github.com/lucasbertoni/omazork/issues/28))

1. Wrapper hard classes (in-combat, fast verb, failed/no-op turn) → instant
2. Hand-curated overlay row
3. **Drift edge** — room changed on a non-walk turn and `(from, to)` is in the
   drift set (§3.4)
4. `(verb, object)` row
5. Edge row — consulted only for directional walk commands that changed the room
6. Verb-only default (shared `verbs.json`, §10; per-game divergence via overlay
   `verbDefaults`)
7. Global fallback: 1 minute

**Teleports**: a handler-driven room change (GOTO, death, wizard spells) is priced
as the `(verb, object)` action alone — never as an edge, never both.

## 3. Runtime rules (proven in the matcher prototype, [#22](https://github.com/lucasbertoni/omazork/issues/22))

The liftable matcher module, capture harness, transcripts, and object-table dumps
live on branch
[`prototype/action-matcher`](https://github.com/lucasbertoni/omazork/tree/prototype/action-matcher)
(`internal/actions/prototype/`).

### 3.1 Room identity

Global 0 (Quetzal snapshot peek) tracks the current room's z-machine **object
number** exactly. The generated table's `rooms` section maps ZIL id → objnum; the
wrapper resolves edge keys to objnums at load and matches on global 0. Display
names are never a key ("Maze" ×15 in Zork I, "Land of Shadow" ×8 in Zork III).

### 3.2 No-op and failure detection

- **Parser-aborted turns freeze the `moves` counter** (global 2) — an exact, free
  no-op detector. Parser errors and "You don't have the sword." cost nothing.
- **Failed-but-executed actions** ("Too late for that.") consume a move; detected by
  an output failure-phrase list (accepted heuristic; mispricing residue falls back
  to normal action pricing, never to combat).
- Edge waits are gated on the room actually changing.

### 3.3 Combat detection ([#19](https://github.com/lucasbertoni/omazork/issues/19))

Full villain/phrase/verb tables:
[docs/research/combat-inventory.md on `research/combat-inventory`](https://github.com/lucasbertoni/omazork/blob/research/combat-inventory/docs/research/combat-inventory.md).
Three games, three architectures:

- **Zork I** — real melee engine; a **state machine**. Enter on a melee-verb input
  at a villain (troll, thief, cyclops) or on any melee-table output line
  (villain-initiated combat has no banner; the first melee line is the marker).
  Exit on the black-fog death line, unconsciousness, thief flee/leave lines,
  pacification, the player death banner, or **room change** (the engine itself
  clears combat on room change). Every villain line contains the villain name or
  its weapon (4 listed exceptions in the doc).
- **Zork II** — no engine; two state windows: the **dragon** anger sequence
  (window survives room changes — the dragon follows) and the **unleashed
  three-headed dog** (whole-room hostility until the collar). The Wizard's random
  spell harassment is violent-sounding non-combat and must not false-positive
  (verified in #22).
- **Zork III** — one fight: the **hooded-figure duel** (Land of Shadow, 8 rooms),
  one state window with explicit engagement text (figure appears + sword
  materializes) and three endings.
- **Blanket rule**: a melee-verb input (ATTACK/KILL/STAB/STRIKE/SWING/KNOCK DOWN
  families) ⇒ instant turn, covering every one-shot attack response without state.

### 3.4 Dynamic and vehicle movement ([#28](https://github.com/lucasbertoni/omazork/issues/28))

- **Drift edges** (new edge `kind: "drift"`): the Frigid River `RIVER-NEXT` table
  and the Zork II balloon `VAIR-1…4` ladder, mechanically extracted (~10–15 rows
  total). A room change on a non-walk turn is priced as the traversed edge only if
  `(from, to)` is in the drift set (precedence slot 3); otherwise the teleport rule
  holds. Priced by the normal edge formula (water +3 on river rows).
- **Random-destination walk exits** (Bank of Zork `MAGNET-ROOM-EXIT`, Carousel
  Room): one plain edge row per `(from, candidate-to)` pair; the matcher keys on
  the observed destination. No new mechanics.
- **Zork III endgame**: Royal Puzzle pushes are handler `(verb, object)` actions
  (teleport rule). Mirror-box walk FEXITs are **not enumerated** — unlisted
  endgame pairs fall to the 1-min fallback; a single overlay row can adjust.

## 4. Data formats ([#21](https://github.com/lucasbertoni/omazork/issues/21))

All per-game files ×3 unless noted. Every file carries `"schemaVersion": 1`; the
wrapper asserts an **exact match** at load and refuses to start otherwise (data is
go:embedded — this guards dev-time regeneration skew).

| File | Role | Written by |
|---|---|---|
| `data/extract/<game>.json` | ZIL extraction: rooms, typed edges, syntax, handlers, raw routine text — the pinned LLM-pass input | extractor |
| `data/actions/<game>.json` | Generated duration table (edges + (verb,object) rows + rooms map + vocab). Regenerated wholesale, never hand-edited | generator |
| `data/actions/<game>.overlay.json` | Hand-curated overrides + `acknowledged` map | humans only |
| `data/actions/<game>.llm.json` | LLM cache: per row key, `inputHash` + `class` + `minutes` + `rationale` | generator |
| `data/actions/<game>.calibration.json` | Replay profile, histograms, per-threshold pass/fail, coverage, `curationCandidates` | generator |
| `data/actions/verbs.json` | Shared ~50-verb default table (one file, all games) | hand-authored (§10) |
| `data/actions/calibration.json` | Shared calibration thresholds (one file, all games) | hand-authored (§6) |
| `data/events/<game>.json` | Pure score-event tables (achievements) | extractor-era, hand-maintained (§9) |

**Generated row shape** (edges and actions): key fields + `minutes` + provenance —
`class`, `source` (`"rule"` / `"llm"` / `"rule+llm"`), optional `note`
(e.g. `"base 2 + dark 1, llm 4"` — nudge magnitude visible to curators). One row
per directed `(from, to)`; multiple exits connecting the same pair collapse to the
**cheapest** duration, collision noted in `note`. `dir` is informational, not part
of the key. Edge rows reference ZIL ids (curator-readable); the `rooms` section
(`{ "id": "MAZE-5", "obj": 142, "name": "Maze" }`) resolves them at load.

**Vocab**: the generated table's generator-owned `vocab` section carries verb
synonyms + object nouns from the extracted SYNTAX/SYNONYM lines; the wrapper does
light normalization of raw command text. Mismatches degrade to the verb-default
row. The overlay cannot touch vocab.

**Overlay semantics**: same key shapes as base (edge, `(verb, object)`) plus
`verbDefaults: { "<verb>": minutes }` for per-game divergence. An overlay row
(i) overrides duration (`0` = force instant), (ii) may add a row absent from base,
(iii) is the **only** place a duration may exceed the 30-min uncurated cap,
(iv) requires a `reason` field above 60 min. No wildcards, no deletes. The
`acknowledged` section is a map `{"<row key>": "<inputHash>"}` (§7).

**Orphan rule**: a validator (run by the generator and in CI) fails on any overlay
or acknowledged key that no longer matches base or vocab — orphaned curation is a
hard error, never silently inert.

**Layering is at load time**: the wrapper loads base + overlay + verbs and applies
§2 precedence in memory. No build-time merged artifact.

## 5. Generation pipeline

### 5.1 ZIL extraction ([#18](https://github.com/lucasbertoni/omazork/issues/18))

Full write-up:
[docs/research/zil-extraction.md on `research/zil-extraction`](https://github.com/lucasbertoni/omazork/blob/research/zil-extraction/docs/research/zil-extraction.md).

- Hand-rolled ZIL S-expression reader + extractor in **Go** (~800–1,000 LOC,
  est. 2–4 sessions). ZILF rejected (GPL-3.0 C# full compiler, no AST export;
  compilation erases condition names and refusal strings).
- Every exit is one of exactly five shapes inside `<ROOM>` forms: `(DIR TO room)`,
  `(DIR "refusal")`, `(DIR TO room IF FLAG [ELSE "…"])`,
  `(DIR TO room IF DOOR IS OPEN)`, `(DIR PER ROUTINE)` — taxonomy confirmed closed
  by `V-WALK`. Static shapes cover ~98% / ~95% / ~83% of Zork I/II/III edges; only
  ~17 distinct `PER` routines exist across all three games.
- Action lines are fully enumerable: `<SYNTAX>` grammar (~270/game), `(ACTION)`
  properties (~124/149/167), dispatch order explicit in `PERFORM`.
- Parse only files named by the root `zorkN.zil` `<INSERT-FILE>` list — Zork III's
  repo carries uncompiled legacy files with duplicate rooms, and 54 of its 89
  shipping rooms live in `3actions.zil`.
- Output: `data/extract/<game>.json` with `rooms`, typed `edges`
  (plain/blocked/cond_flag/cond_door/routine/drift), `syntax`, `handlers`, and raw
  `routines` text. The extractor owes the ZIL-id → objnum correlation (name +
  exit-structure matching against the .z3 object table; technique proven in #22).

### 5.2 Rule pricing

Deterministic: hard classes and fast verbs → instant; edges → 2-min base +
modifiers (§2); everything else defers to the LLM pass or the verb-default table.

### 5.3 LLM inference pass ([#23](https://github.com/lucasbertoni/omazork/issues/23))

- **Scope**: static edges are rule-priced then **LLM-nudged within the Movement
  band** (anchored on the rule baseline shown to it); the ~17 routine-typed edges
  and every handler `(verb, object)` pair get **full classification** (any class).
  Fast verbs / instant rows never reach the LLM.
- **Payload** (assembled deterministically from `data/extract/`, never committed —
  the hash pins it): room names + full descriptions + exit flags + rule baseline;
  routine edges add raw routine text; handler pairs get verb + object names, object
  description, raw handler text.
- **Contract**: one API call per row key; strict JSON `{class, minutes, rationale}`;
  `minutes` integer; `rationale` ≤1 sentence, stored in `llm.json` and surfaced as
  the row's `note`. Validator: `class` must be `movement` on static edges;
  `minutes` within the returned class's band.
- **Hashing**: `inputHash = sha256(modelId + renderedPrompt)`. The rendered prompt
  embeds template + payload, so editing one of the ~3 templates (Go string
  constants) auto-invalidates exactly that kind's rows; changing the pinned model
  invalidates everything. Invalidation lands as a reviewable `llm.json` diff.
- **Determinism is cache-based**: the committed `llm.json` is the source of truth;
  regeneration reuses it unless a row's `inputHash` changed; re-inference is a
  reviewed event. Pinned model id constant; temperature 0 as courtesy.
- **Failures**: out-of-band `minutes` → one retry with the band restated, then
  **hard-fail** the run (never clamp silently). Malformed JSON after one retry →
  hard-fail. Dramatic nominations: `llm.json` keeps the raw nomination; the
  generated row is capped at 30 min with a note
  (`"llm nominated dramatic 120m — overlay candidate"`) and fed to the curation
  queue (§7).

### 5.4 Calibration emission

Every regen emits `data/actions/<game>.calibration.json` (§6) and the
`curationCandidates` queue (§7).

## 6. Calibration and validation ([#27](https://github.com/lucasbertoni/omazork/issues/27))

Three-part method:

1. **Walkthrough replay** (primary): the committed `(script, seed)` fixtures (§6.1)
   replayed through the real seeded engine + the liftable matcher module, so every
   turn classifies exactly as the runtime will, then priced against the generated
   tables. Emits a cumulative wait profile.
2. **Whole-table histograms**: distribution-shape gates over all rows.
3. **Manual spot-play**: non-gating human sniff-test against the per-game hot-path
   checklists (§11), run once before implementation ships; findings land as overlay
   edits or threshold adjustments, never as a gate.

**Every threshold below is a validation error that fails generation** (same
severity as orphaned overlay keys). Thresholds live in the shared committed
`data/actions/calibration.json`, so all three games gate identically and a
threshold change is a visible diff. Gates evaluate the **layered** result (base +
overlay + verbs) everywhere — otherwise the overlay is a gate bypass.

- Replay gates, per game: cumulative walkthrough wait **15–35 hours**; median
  movement wait **1–3 min**; **≥60%** of walkthrough turns resolve <5 min.
- Histogram gates, per game: mean edge duration **2–4 min**; Dramatic-class rows
  **≤3%** of (verb,object) rows; per-class medians inside the **inner half** of
  their band.
- Hard cap: **≤5 rows ≥60 min per game** (the 30–60 band is gated per-row by the
  overlay-only rule and does not count against the cap).

The emitted `<game>.calibration.json` contains the replay profile, histograms,
per-threshold pass/fail, and a non-gating coverage stat (% of rows the walkthrough
exercised).

### 6.1 Walkthrough fixtures ([#30](https://github.com/lucasbertoni/omazork/issues/30))

Committed on `prototype/action-matcher` at 7cc2a0a under
`internal/actions/prototype/fixtures/` with `check.sh`; replay via
`go run ./internal/actions/prototype capture <game> <seed> < <script>`.

| Game | Script | Seed | Commands | Result |
|---|---|---|---|---|
| Zork I | `zork1-walkthrough.txt` | 12 | 361 | 350/350, 374 moves |
| Zork II | `zork2-walkthrough.txt` | 1 | 315 | 400/400, 345 moves |
| Zork III | `zork3-walkthrough.txt` | 11 | 264 | 7/7, 320 moves |

All halt on their true endings with zero parser rejections; seed-dependent edits
are documented as comments in the scripts.

## 7. Hand-curation workflow ([#29](https://github.com/lucasbertoni/omazork/issues/29))

- **Queue**: the `curationCandidates` section of `<game>.calibration.json` —
  (a) dramatic nominations from the LLM pass, (b) statistical outliers: top-10
  longest edges + top-10 longest (verb,object) rows + every row at its class cap,
  (c) minus acknowledged keys.
- **Acknowledged**: overlay `acknowledged` map `{"<row key>": "<inputHash>"}`;
  excluded while the pinned hash matches; a changed `inputHash` re-surfaces the
  candidate. Git blame supplies who/when. Ack keys are orphan-validated.
- **Human input**: spot-play findings and GitHub issues labeled `duration-report`
  (label joins the triage vocabulary). No in-game feedback mechanism.
- **Fixes** are direct overlay edits; a `scripts/validate`-style one-liner runs
  locally exactly what CI runs.
- **CI**: new `validate.yml` on any `data/**` change, four checks:
  1. Validator: schemaVersion exact-match, orphaned overlay/ack keys, band/cap
     rules, `reason` required ≥60 min.
  2. Calibration gates (§6) on the layered result.
  3. Offline regen-and-diff drift check: regenerate from `data/extract/` + the
     committed `llm.json`; hard-fail if any row would need an API call or output
     differs from the committed table.
  4. Calibration freshness: committed `<game>.calibration.json` must match the
     recomputed layered profile.

## 8. Reveal UX and copy ([#25](https://github.com/lucasbertoni/omazork/issues/25))

**Tiers**: presentation keys on resolved **minutes**, never inference class. One
threshold, **5 minutes**: below = **quiet**, at/above = **full** (the current
dramatic register). The same line gates the desktop notification — quiet waits
never notify.

**Duration reveal**: the transcript stays diegetic — no numbers, ever. Out-of-game
surfaces (console banner, journal card, bar tooltip) show remaining minutes under
an hour ("~4 min") and absolute clock time at an hour or more ("matures at
5:12 PM").

| Moment | Quiet (<5 min) | Full (≥5 min — current copy, unchanged) |
|---|---|---|
| Withheld response | "Time passes." | "The outcome of your action will take some time to unfold..." |
| Transcript marker | `[Time passes...]` | `[Something is unfolding...]` |
| Blocked input | "Time is still passing..." | "The outcome of your last action is still unfolding..." |
| Reveal | Plain reveal: command echo + output (+ score-delta / achievement lines), **no header** | "— While you were away… —" recap unchanged |
| Desktop notification | none | "Something has happened in the Great Underground Empire." |

Derived quiet variants (drafts — build sessions may polish wording, not register):
banner "⏳ Time passes — ~2 min. (SAVE/RESTORE wait too.)" (matured variant
unchanged); journal card "⏳ north — ~2 min"; input placeholder "time is passing —
MENU still works".

**Bar icon**: gains an in-flight state, both tiers — dim/hollow dot while a wait is
unfolding, accent dot when matured (existing). Matured-dot semantics, click action,
glyph unchanged.

**PendingInfo** gains two fields: `tier` (`"quiet"` | `"full"`) —
server-authoritative, the 5-min threshold lives only in the wrapper — and
`command`, the player's own echoed command. Existing `{maturesAt, remaining}`
unchanged.

## 9. Score-event decoupling ([#24](https://github.com/lucasbertoni/omazork/issues/24))

- **Rename** `data/waits/` → `data/events/`, same per-game filenames; `NOTES.md`
  moves with them (drop the "Tuning rationale" section).
- **Drop** `waitMinutes` and `fallbackMinutes`. **Keep** `id`, `delta`/`deltaMax`,
  `room`, `roomName`, `note`, `maxScore`. **Add** `schemaVersion`, exact-match.
  **No row pruning** — the table is the game's score-event vocabulary.
- **Delete**: `tables.Game.FallbackWait`, `tables.Event.Wait`, the
  `fallbackMin`/`fallbackMax` fields and `rand` hook, `Session.waitFor`.
- **Rename**: `tables.Load` → `LoadEvents`, `tables.Game` → `EventTable`,
  `Session.waits` → `Session.events`; package doc rewritten.
- **Unchanged**: `Match` (delta+room precedence), `HasEvent`, `matchEventID`, the
  Zork III Land-of-Shadow ordering rule (`LandOfShadowHits`), and every achievement
  `eventId`.

## 10. Shared verb-default table (canonical; ships as `data/actions/verbs.json`)

Hand-authored here, one table for all three games; per-game divergence via overlay
`verbDefaults`. Fast verbs are listed for completeness but are enforced as a
wrapper hard class (precedence slot 1), not by table lookup.

**Fast verbs (hard class, always instant)**: look, examine, read, inventory, wait,
score, diagnose, verbose, brief, superbrief, save, restore, restart, quit, script,
unscript, version, again, oops, pray*, hello, count.
(*prayer at the altar is a handler pair; the LLM/overlay prices it there.)

| Verb (synonyms resolve via vocab) | Class | Minutes |
|---|---|---|
| take / get / grab | manipulation | 1 |
| drop / put down | manipulation | 1 |
| put / insert | manipulation | 1 |
| open | manipulation | 1 |
| close | manipulation | 1 |
| move / push / pull / slide | manipulation | 2 |
| turn / twist | manipulation | 2 |
| raise / lift | manipulation | 2 |
| lower | manipulation | 2 |
| throw / hurl / chuck / toss | manipulation | 1 |
| give / hand | manipulation | 1 |
| shake | manipulation | 1 |
| rub / touch / feel | manipulation | 1 |
| squeeze | manipulation | 1 |
| smell / sniff | manipulation | 0 |
| listen | manipulation | 0 |
| knock | manipulation | 1 |
| eat / drink | manipulation | 2 |
| wear / remove (clothing) | manipulation | 1 |
| light / turn on | manipulation | 1 |
| extinguish / turn off / douse | manipulation | 1 |
| wave / brandish | manipulation | 1 |
| search | manipulation | 3 |
| enter / board / get in | movement* | 2 |
| exit / disembark / get out | movement* | 2 |
| climb (no edge matched) | movement* | 3 |
| swim / wade | movement* | 4 |
| jump / leap | manipulation | 1 |
| tie / fasten / attach | mechanism | 3 |
| untie / unfasten / detach | mechanism | 3 |
| unlock | mechanism | 3 |
| lock | mechanism | 3 |
| inflate / pump | mechanism | 8 |
| deflate | mechanism | 5 |
| fill | mechanism | 3 |
| pour / spill | manipulation | 1 |
| dig | mechanism | 10 |
| burn | mechanism | 3 |
| cut / slice | mechanism | 3 |
| ring | mechanism | 3 |
| wind | mechanism | 3 |
| press (button) | manipulation | 1 |
| plug | mechanism | 3 |
| oil / grease | mechanism | 3 |
| melt | mechanism | 5 |
| break / smash / mung / destroy | manipulation | 2 |
| kiss | manipulation | 0 |
| say / shout / answer / tell | manipulation | 0 |
| wish | manipulation | 1 |

*movement-class verb defaults apply only when no edge/drift row matched (the room
didn't change, or vehicle boarding); a room change on a walk command always resolves
through the edge row first.

Melee verbs (attack/fight/kill/stab/strike/swing/knock-down families) are absent
deliberately: the hard-class blanket rule prices any melee-verb input instant (§3.3).

## 11. Spot-play checklists (non-gating; run once before implementation ships)

**Zork I** (7): Kitchen→Cellar via trapdoor (window, rug, trapdoor cadence); the
Troll Room fight (instant turns, then normal pricing after the fog); the Maze into
the Cyclops Room (1-min-ish maze hops shouldn't compound painfully); Dam +
Reservoir sequence (bolt/gates — mechanism feel); Frigid River run (drift edges);
Hades exorcism (bell/book/candles — the dramatic centerpiece); a thief encounter
mid-route (combat window entry/exit on room change).

**Zork II** (7): Carousel Room in and out (random destinations); Wizard apparition
mid-walk (no combat false-positive, no edge mispricing on Fear-forced moves);
dragon → princess sequence (anger window, glacier resolution); balloon flight
(drift ladder + tie/untie mechanism); Bank of Zork (candidate edges + curtain);
Oddly-angled Room sequence (repeated diamond moves stay quiet-tier); riddle door +
Cerberus/collar (dog window).

**Zork III** (7): lake dive and crossing (swim + water edges); Land of Shadow duel
(scripted combat window, all-instant); Scenic Vista time travel (teleport pricing);
earthquake + Great Door timing; ship boarding + amulet (drift/board pricing);
Royal Puzzle full push sequence (handler actions, sane cumulative time); mirror box
+ endgame (1-min fallback feel in unlisted pairs).

## 12. Build plan (suggested slicing; assets ready to lift)

1. **Extractor** — Go ZIL reader emitting `data/extract/*.json` (est. 2–4
   sessions; layout gotchas in §5.1).
2. **Generator + LLM pass + calibration** — rules, cache, gates, candidates
   report; fixtures already committed (§6.1).
3. **Runtime matcher + lookup** — lift the prototype module
   (`prototype/action-matcher`), wire precedence, hard classes, drift set.
4. **Score-event decoupling** (§9) — mechanical rename/deletion pass.
5. **Reveal UX** (§8) — tiers, copy, PendingInfo, bar icon in-flight dot.
6. **CI** — `validate.yml` (§7).

Charter reminders for every build session: honor in-flight pending waits; Classic
mode untouched; `duration-report` label to add at triage-vocabulary touchpoint.
