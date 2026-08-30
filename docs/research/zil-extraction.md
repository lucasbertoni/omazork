# Research: ZIL extraction of room graph and action lines

Resolves [#18](https://github.com/lucasbertoni/omazork/issues/18), part of the wayfinder map
[#17](https://github.com/lucasbertoni/omazork/issues/17).

**Question.** Can the historicalsource ZIL source for Zork I/II/III be parsed to extract
(a) the full directed room graph — every exit, including conditional, door, and blocked exits —
and (b) every action line (verb + object handlers, room ACTION routines, SYNTAX definitions)?

**Answer: yes.** The room graph is almost entirely declarative — five fixed exit shapes inside
`<ROOM …>` forms — and the action surface is fully enumerable (SYNTAX lines, `(ACTION …)`
properties, and the routines they name). Roughly 90–95% of directed edges fall out of a
straightforward S-expression parse; the remainder (`PER` routine exits and in-routine `GOTO`
teleports) is a small, closed set — about 17 distinct exit routines across all three games —
that the charter's LLM pass plus hand-curated overlay is sized for. A hand-rolled reader in Go
is the right tool; ZILF is not needed. Details and quotes below.

## Sources

All quotes are from shallow clones of the primary sources (fetched 2026-08-30):

| Repo | Commit | License |
|---|---|---|
| [historicalsource/zork1](https://github.com/historicalsource/zork1) | `97b7b3d` | MIT (LICENSE file, © 2025 Microsoft) |
| [historicalsource/zork2](https://github.com/historicalsource/zork2) | `3da9661` | MIT |
| [historicalsource/zork3](https://github.com/historicalsource/zork3) | `3ec9ed4` | MIT |

Each repo's README describes the code as a snapshot of Infocom's development directory at
shutdown, "canonical, but not necessarily the exact source code arrangement for production."
These are the same repos this project's `.z3` story files come from, so extracted data will
match shipped behavior modulo that caveat.

ZIL runtime semantics were confirmed against the games' own interpreter-side code
(`gverbs.zil` `V-WALK`, `gmain.zil` `PERFORM`) rather than secondary documentation.

## Repo layout: what to parse

Each repo's root file (`zork1.zil` / `zork2.zil` / `zork3.zil`) is the compilation manifest —
a sequence of `<INSERT-FILE "…" T>` forms naming the files ZILCH actually compiled. This list
is authoritative; **globbing `*.zil` is wrong**, especially for Zork III.

- **Zork I** compiles `GMACROS, GSYNTAX, 1DUNGEON, GGLOBALS, GCLOCK, GMAIN, GPARSER, GVERBS, 1ACTIONS`.
- **Zork II** is identical with `2DUNGEON` / `2ACTIONS`.
- **Zork III** compiles `GSYNTAX, GMACROS, GCLOCK, GMAIN, GPARSER, 3DUNGEON, GGLOBALS, GVERBS, 3ACTIONS`.
  The repo also carries **legacy development files that are NOT compiled** — `dungeon.zil`,
  `actions.zil`, `shadow.zil`, `tm.zil`, `demons.zil`, `main.zil`, `parser.zil`, `syntax.zil`,
  `verbs.zil`, `macros.zil`, `clock.zil` — whose room definitions overlap the shipping ones
  (e.g. `shadow.zil` defines 39 rooms that also exist in `3actions.zil`). `.zap`/`.xzap` files
  are compiler output; `*.chart` files are build statistics, not maps.

A second layout gotcha: **rooms are not confined to the "dungeon" files.** Zork III's main
map (54 of its 89 rooms, including all of the Land of Shadow) is defined in `3actions.zil`;
`3dungeon.zil` holds only the 35 endgame/mirror-box rooms. The extractor must scan every
inserted file for `<ROOM …>` forms.

Room counts (by `<ROOM` in compiled files): **Zork I 110, Zork II 86, Zork III 89** (35 in
`3dungeon.zil` + 54 in `3actions.zil`) — matching the games' known room counts, a good
built-in validation check.

## (a) The room graph: five exit shapes

Every exit is a property on a `<ROOM>` form whose property name is a direction from that
game's `<DIRECTIONS …>` declaration (`NORTH EAST WEST SOUTH NE NW SE SW UP DOWN IN OUT LAND`
in Zork I, plus `CROSS` in II/III). There are exactly five value shapes. `V-WALK` in
`gverbs.zil` dispatches on them by property size (`UEXIT`/`NEXIT`/`FEXIT`/`CEXIT`/`DEXIT`
branches), which confirms the taxonomy is closed — no sixth kind exists at runtime.

All examples below are verbatim from the sources.

**1. UEXIT — unconditional** (`1dungeon.zil`, WEST-OF-HOUSE):

```zil
(NORTH TO NORTH-OF-HOUSE)
```

**2. NEXIT — blocked, with refusal text** (same room). These are non-edges but carry the
canonical "why not" copy:

```zil
(EAST "The door is boarded and you can't remove the boards.")
```

**3. CEXIT — conditional on a global flag**, optional ELSE string (WEST-OF-HOUSE and
LIVING-ROOM, `1dungeon.zil`):

```zil
(SW TO STONE-BARROW IF WON-FLAG)
(WEST TO STRANGE-PASSAGE IF MAGIC-FLAG ELSE "The door is nailed shut.")
```

**4. DEXIT — conditional on a door object being open** (`IF <obj> IS OPEN`; the runtime
checks `OPENBIT`). From `1dungeon.zil` and `3dungeon.zil`:

```zil
(UP TO LIVING-ROOM IF TRAP-DOOR IS OPEN)
(NORTH TO BEHIND-DOOR IF DUNGEON-DOOR IS OPEN)
```

**5. FEXIT — computed by a routine** (`PER`). The routine returns a room or false; false
means the routine printed its own refusal:

```zil
(DOWN PER TRAP-DOOR-EXIT)
(SOUTH PER MRGO)
```

Approximate per-game exit-shape counts (regex classification of compiled dungeon/room files;
±1–2 on multi-line properties):

| Game | UEXIT | NEXIT | CEXIT | DEXIT | FEXIT (PER) | total |
|---|---|---|---|---|---|---|
| Zork I | 277 | 37 | 25 | 6 | 7 | 352 |
| Zork II | 163 | 21 | 12 | 12 | 11 | 219 |
| Zork III | 195 | 48 | 8 | 29 | 59 | 339 |

So UEXIT+NEXIT+CEXIT+DEXIT — fully static, machine-readable edges with their conditions and
refusal text — cover **98% of Zork I, 95% of Zork II, and 83% of Zork III** exits. Zork III's
FEXIT bulge is almost entirely the endgame (mirror box `MRGO`/`MIRIN`/`MIROUT`, Royal Puzzle
`CPEXIT`/`CPENTER`, `BRONZE-DOOR-EXIT`); the distinct `PER` routines across all three games
number about 17 (Zork I: `GRATING-EXIT`, `MAZE-DIODES`, `SHUFFLING`, `TRAP-DOOR-EXIT`,
`UP-CHIMNEY-FUNCTION`; Zork II: `BKLEAVEE`, `BKLEAVEW`, `GUARDIAN`, `MAGNET-ROOM-EXIT`,
`NEWSPAPER`, `PIECE`; Zork III: the six above plus `CPENTER` in `3actions.zil`).

Many are trivially resolvable — `MAZE-DIODES` (`1actions.zil`) is a pure lookup:

```zil
<ROUTINE MAZE-DIODES ()
	 <TELL "You won't be able to get back up ..." CR CR>
	 <COND (<EQUAL? ,HERE ,MAZE-2> ,MAZE-4)
	       (<EQUAL? ,HERE ,MAZE-7> ,DEAD-END-1)
	       (<EQUAL? ,HERE ,MAZE-9> ,MAZE-11)
	       (<EQUAL? ,HERE ,MAZE-12> ,MAZE-5)>>
```

One quirk to warn on: `3dungeon.zil` has `(ENTER TO BEHIND-DOOR IF DUNGEON-DOOR IS OPEN)`
even though `ENTER` is not in the `<DIRECTIONS>` list (it's a verb; the parser reaches these
rooms via `IN`). The extractor should normalize property names against the game's
`<DIRECTIONS>` declaration and flag unknowns rather than silently dropping or inventing edges.

## (b) Action lines: three fully enumerable layers

**SYNTAX definitions** (`gsyntax.zil`, ~269/275/275 `<SYNTAX` forms per game) map a verb
pattern to a handler and optional pre-handler:

```zil
<SYNTAX TAKE OBJECT (FIND TAKEBIT) (ON-GROUND IN-ROOM MANY) = V-TAKE PRE-TAKE>
<SYNTAX TAKE IN OBJECT (FIND VEHBIT) (ON-GROUND IN-ROOM) = V-BOARD PRE-BOARD>
<SYNTAX WALK OBJECT = V-WALK>
```

The grammar is regular: `VERB [PREP] [OBJECT [(FIND bit)] [(scope…)]] [PREP OBJECT …] = handler [pre-handler]`,
plus `<SYNONYM …>` and `<VERB-SYNONYM …>` alias lines. Trivial to parse; gives the complete
`(verb, object-slot)` vocabulary the charter's `(verb, object)` wait keys need.

**Object and room ACTION properties**: `(ACTION ROUTINE-NAME)` on `<OBJECT>`/`<ROOM>` forms
(~124/149/167 per game, counting both). Room action routines receive a phase argument
(`RARG`): `M-LOOK` (describe), `M-BEG` (intercept before the verb runs), `M-END` (after),
`M-ENTER` (on entry). E.g. `LIVING-ROOM-FCN (RARG)` in `1actions.zil`.

**Dispatch order** is explicit in `gmain.zil`'s `PERFORM`, which tries, in order: the actor's
action, the current room's action with `M-BEG`, the verb's preaction, the indirect object's
action, the containing vehicle's `CONTFCN`, the direct object's action, then the verb default:

```zil
<COND (<SET V <DD-APPLY "Actor" ,WINNER <GETP ,WINNER ,P?ACTION>>> .V)
      (<SET V <D-APPLY "Room (M-BEG)" <GETP <LOC ,WINNER> ,P?ACTION> ,M-BEG>> .V)
      (<SET V <D-APPLY "Preaction" <GET ,PREACTIONS .A>>> .V)
      ...
```

So "every action line" is recoverable as: the SYNTAX table (verb grammar → handler) plus the
ACTION/CONTFCN property tables (object/room → routine) plus the routine bodies themselves.
Routine bodies are ZIL code, not data — the extractor should capture them verbatim (name,
file, line, raw text) for the rule-based/LLM duration-inference pass, not try to interpret
them.

## Hard cases

| Case | Mechanism in source | Extraction impact |
|---|---|---|
| **Zork I mazes** | Ordinary UEXITs between `MAZE-1…15` — the confusion is all in identical descriptions. Only the four one-way "diode" drops go through `PER MAZE-DIODES` (pure lookup, above). | Static edges. Trivial. |
| **Zork I Frigid River** | Boat is an object: `INFLATED-BOAT … (FLAGS … VEHBIT …) (VTYPE NONLANDBIT)`. River rooms are real rooms (`RIVER-1…5`, `NONLANDBIT`); the current is a clock demon `I-RIVER` stepping through pure tables `RIVER-NEXT` / `RIVER-SPEEDS` (`1actions.zil`); launching maps shore→river via `RIVER-LAUNCH`. | Drift edges live in named tables, extractable mechanically. Timed drift is turn-driven, not walk-driven — per the charter, edge waits gate on the room changing, so drift turns can key off `(verb,object)` instead. |
| **Zork I misc.** | `GRATING-EXIT`, `TRAP-DOOR-EXIT`, `UP-CHIMNEY-FUNCTION`, `SHUFFLING` (Loud Room), cyclops wall / magic-word teleports via `<GOTO …>` in action routines (16 call sites). | Handful of routines; overlay-sized. |
| **Zork II Carousel Room** | Room has eight normal UEXITs; `CAROUSEL-ROOM-FCN` at `M-BEG` **rewrites `PRSO` to a random direction** until the room is stopped (`CAROUSEL-FLIP-FLAG`). | Static edges are correct post-fix; pre-fix the *chosen* edge is random but still one of the eight extracted edges. Mark the room "randomized until flag". |
| **Zork II balloon** | `BALLOON` object (`VEHBIT`, `ACTION BALLOON-FCN`); vertical movement is the clock demon `I-BALLOON` over `VAIR-1…4` rooms; ledge attachment via a table (`LTABLE LEDGE-1 VAIR-2 LEDGE-2 VAIR-4`). | Rooms and tables are static; the demon's timing is code. Small curated overlay for balloon edges. |
| **Zork II Bank of Zork** | `PER MAGNET-ROOM-EXIT` — destination depends on direction and, when disoriented, `<PROB 50>` between two rooms. `BKLEAVEE/W` gate on carrying loot. | Genuinely conditional/random exits; emit as routine-edges with candidate destinations. |
| **Zork II Wizard** | `I-WIZARD` demon; spells (`FILCH`, `FALL`, …) can relocate the player via `GOTO` (19 call sites in `2actions.zil`). | Random events, not map edges. Out of scope for the edge table; wrapper's room-change gate handles them. |
| **Zork III Land of Shadow** | `SHADOW-1…8` in `3actions.zil` are **plain UEXITs**. The drama is the room ACTION `SHADOW-ROOMS`: at `M-BEG` it blocks the one direction the hooded figure occupies (`,PRSO = ,BLOCKED-DIR`); at `M-END` `<PROB 30>` events pick `BLOCKED-DIR` via `PICK-DIRECTION`. | Graph is fully static; the figure is a temporary per-direction block. No bespoke edges needed — matches the existing `data/waits` Land-of-Shadow score rule's territory. |
| **Zork III endgame** | Mirror box: `MRGO`/`MIRIN`/`MIROUT` compute destinations from mirror position/orientation globals (`MLOC`, `MDIR`). Royal Puzzle: `CPEXIT`/`CPENTER` compute movement from `CPTABLE` (a mutable 8×8 grid of sandstone/marble). | The genuinely hard ~50 FEXITs, all in two puzzle subsystems of a small endgame area. Recommend: extract as routine-edges, hand-curate durations for the whole endgame (it's one contiguous, short section). |
| **Zork III ocean / aqueduct** | `FLATHEAD-OCEAN` is a room; the Viking ship is a timed `INVISIBLE`-flag event, not movement. Aqueduct rooms are normal. | No special handling. |
| **Teleports generally** | Always `<GOTO ,ROOM>` inside a routine (16/19/26 call sites in 1/2/3ACTIONS). | Greppable; emit as candidate non-walk transitions. Under the charter these aren't walk edges — they're `(verb,object)` actions whose room change the wrapper observes. |

Randomness never appears in exit properties themselves — `PROB`/`RANDOM` in the dungeon-file
room sections is essentially nil; all nondeterminism lives in routines (26/32/23 `<PROB`
sites in the three actions files). So the **static edge table is deterministic by
construction**, exactly what the charter's committed-and-deterministic generation step needs.

## Recommended extraction approach

**Hand-rolled reader in Go (the project's language). Do not build on ZILF.**

ZILF ([foss.heptapod.net/zilf/zilf](https://foss.heptapod.net/zilf/zilf), GPL-3.0, C#/.NET —
compiler `zilf`, assembler `zapf`, disassembler, library) is a full compiler whose output is
z-code; it has no AST-export API, is a heavyweight cross-language dependency for a one-shot
offline step, and would hand us *less* than the source does (compilation erases the
CEXIT/DEXIT condition names and refusal strings we want). Its value here is only as an
optional cross-check that our file list matches what compiles.

ZIL's surface syntax is small: `<FORM …>` angle forms, `(list …)` property lists, `"strings"`,
`;comments` (both `;"string"` and `;FORM`), `%` compile-time macros (rare outside `gmain`),
and atoms. A tokenizer + form reader is ~300–500 lines of Go. The extractor then:

1. Reads the root `zorkN.zil`, follows `<INSERT-FILE>` to build the compiled-file list.
2. Collects `<DIRECTIONS>`, all `<ROOM>` and `<OBJECT>` forms; classifies each direction
   property into the five exit shapes; validates every `TO` target against the room set and
   the total against the known room counts (110/86/89).
3. Collects `<SYNTAX>`/`<SYNONYM>`/`<VERB-SYNONYM>` from `gsyntax.zil`.
4. Indexes `<ROUTINE>` names → (file, line, raw source text); attaches raw bodies to the
   FEXIT routines, room/object ACTION routines, and `GOTO` call sites it references.

**Output shape** — one committed JSON per game (feeding the `(origin, destination)` edge
table and `(verb, object)` table the charter fixed):

```json
{
  "rooms":   [{"id": "WEST-OF-HOUSE", "desc": "West of House", "flags": ["RLANDBIT", "ONBIT"], "action": "WEST-HOUSE"}],
  "edges":   [{"from": "WEST-OF-HOUSE", "dir": "SW", "kind": "cond_flag", "to": "STONE-BARROW", "if": "WON-FLAG"},
              {"from": "WEST-OF-HOUSE", "dir": "EAST", "kind": "blocked", "text": "The door is boarded ..."},
              {"from": "CELLAR", "dir": "UP", "kind": "cond_door", "to": "LIVING-ROOM", "door": "TRAP-DOOR"},
              {"from": "FRONT-DOOR", "dir": "SOUTH", "kind": "routine", "per": "MRGO"}],
  "syntax":  [{"verb": "TAKE", "pattern": ["OBJECT"], "action": "V-TAKE", "preaction": "PRE-TAKE"}],
  "handlers": {"objects": {"INFLATED-BOAT": "RBOAT-FUNCTION"}, "rooms": {"LIVING-ROOM": "LIVING-ROOM-FCN"}},
  "routines": [{"name": "MAZE-DIODES", "file": "1actions.zil", "line": 898, "source": "<ROUTINE ..."}]
}
```

`kind: "routine"` edges plus the `routines` section are the LLM pass's input; its resolved
destinations/durations land in the same edge table, and the hand-curated overlay corrects the
residue (mirror box, Royal Puzzle, balloon — a few dozen rows total).

**Effort estimate:** tokenizer + form reader + extractor ≈ 800–1,000 LOC Go; 2–4 focused
sessions including validation (room-count check, dangling-target check, spot-check against a
published walkthrough). The LLM inference pass and overlay format are separate tickets' scope.

## Answers to the map's open "special movement" question

Extraction evidence says **static edges suffice** for: all mazes, Land of Shadow (static
graph + temporary block), Carousel Room (random choice *among* static edges). Bespoke
handling (routine-edges + overlay) is needed only for: Frigid River drift (table-driven,
mechanically extractable), the balloon (demon-driven), Bank of Zork, and the Zork III
endgame's two puzzle subsystems. Teleports and wizard spells are not walk edges at all under
the charter's room-change gate.
