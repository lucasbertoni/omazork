# Go Z-machine: use an existing implementation, or write a v3-only VM in-repo?

**Date:** 2026-08-29
**Question:** omazork wraps Zork 1/2/3 (`.z3`, Z-machine version 3) as a Go library/service. Should it embed an existing pure-Go Z-machine implementation, or implement a minimal v3-only VM in this repo?

**Hard requirements:**

1. Run `zork1.z3`, `zork2.z3`, `zork3.z3` correctly (full v3 opcode coverage).
2. I/O interception hooks: feed input and capture output programmatically, not tied to a terminal.
3. Read internal machine state: object table (objects/attributes/properties), global variables, flags.
4. Snapshot/restore full machine state in memory for save/resume (not just Quetzal files on disk).

## Survey method

GitHub repository search (`zmachine language:go`, `z-machine language:go` via the GitHub API, 2026-08-29), web search, pkg.go.dev, and direct reading of each candidate's source tree. Several names floated as starting points (`toolkitchen/zmachine`, `edmccard/zork`, `DeedleFake/zmachine`, `sussman/zvm`, `beevik/zmachine`) do not exist as Go Z-machine repos and were discarded; the candidates below are the ones that actually exist.

## Candidates

### 1. `github.com/maloquacious/zmachine` — headless v3 engine (front-runner)

- **Repo:** https://github.com/maloquacious/zmachine · pkg.go.dev: https://pkg.go.dev/github.com/maloquacious/zmachine
- **License:** MIT.
- **What it is (from its README):** "a headless Z-machine Version 3 execution engine … not an interpreter you run — there is no terminal, no prompt, no main loop. It is a library a request handler calls." Core invariant: given an immutable validated `Story`, optional saved state, and one line of input, execute to the next input boundary and return captured output plus resumable state.
- **API shape (verified in `machine.go`, `execute.go`, `story.go`, `state.go`):**
  - `LoadStory([]byte) (*Story, error)` — validated, immutable, shareable across goroutines.
  - `New(story, opts...) (*Machine, error)` — cheap per-session instance.
  - `(*Machine).Start(ctx)` / `(*Machine).Run(ctx, input string) (Result, error)` — run to the next `sread` boundary.
  - `Result{Output string; UpperWindow string; StatusLine StatusLine; State []byte; Status Status}` — captured output text, parsed status line (object name, score/turns or time), and a complete Quetzal-format state blob.
  - `(*Machine).Restore(data []byte)` — rebuild from a `Result.State`.
  - Options: `WithRandomSeed`, `WithFrotzRandomSeed` (reproduces Frotz's PRNG for differential testing), `WithInstructionLimit`, `WithTracer`, `WithLogger`.
  - `Tracer` interface (`trace.go`): per-instruction events (PC, opcode name, operands, stores, branches, call depth) — useful for debugging/instrumentation.
- **z3 completeness:** full v3 opcode dispatch across `opcode_0op.go` (14 opcodes incl. `save`/`restore`/`show_status`/`verify`), `opcode_1op.go` (15), `opcode_2op.go`, `opcode_var.go` (15) — the whole v3 set of §14 of the Standard. Ships the shareware Zork 1/2/3 story files in `testdata/stories/` (with license files) and has: a full scripted Zork I playthrough test (`playthrough_test.go`), **differential tests against real Frotz output** with pinned seeds (`differential_test.go`, `testdata/frotz/`), and `local_stories_test.go` that runs second editions of Zork I, II, and III (fetched by script). Test files outnumber source ~2:1 (~18k total lines of Go, roughly half tests).
- **Requirement fit:**
  1. **Run Zork 1/2/3** — yes; tested against exactly these games, including differential comparison with Frotz.
  2. **I/O hooks** — yes by construction; `Run(ctx, cmd)` in, `Result.Output` out; nothing touches a terminal.
  3. **Read internals** — *not directly exported.* `Machine` internals (memory, object table, globals) are unexported by design; the engine treats state as opaque. However `Result.State` is a standard Quetzal image, and the sibling package `github.com/maloquacious/quetzal` (MIT, same author, v1.0 "feature complete", interop-tested against Frotz/Bocfel/jzip) exposes `File.Memory(story) (Memory, error)` and `DecodeCMem` which reconstruct the full dynamic memory byte array. From that, reading v3 globals (header word at 0x0C), the object table (header word at 0x0A; 9-byte entries, 32 attributes, ≤255 objects), and Flags 1/2 is a small fixed-layout reader (~150–250 LOC) omazork writes once. Sources: https://github.com/maloquacious/quetzal (`memory.go`), Z-machine Standard §12 (object table), §6.2/§11 (header/globals).
  4. **In-memory snapshot/restore** — yes, first-class: every `Result.State` is a complete in-memory snapshot; `Restore` + `Run` on a fresh `Machine` is tested to be turn-for-turn identical to keeping one machine alive ("plays Zork I both ways and compares every turn", README).
- **Maintenance signals:** last commit 2026-07-30 (Michael D Henderson); v0.x with an explicit semver/changelog policy; documented saved-state compatibility promise (state restorable as long as the story file is unchanged, independent of engine version). Requires **Go 1.26+**. Zero third-party deps beyond `maloquacious/quetzal`. 0 GitHub stars (new, single-author); repo contains `AGENTS.md`/`CLAUDE.md`, i.e. developed with heavy AI assistance — mitigated by the unusually strong differential/golden test suite.

### 2. `github.com/zombiezen/gonorth` — v3+ interpreter with UI interface

- **Repo:** https://github.com/zombiezen/gonorth (mirror of a 2012 Bitbucket project)
- **License:** BSD-style text embedded in `README.rst`; **no LICENSE file** (GitHub reports no detected license).
- **API shape (verified in `north/machine.go`):** `north` package with a `UI` interface (`Input`, `Output(window, text)`, `Save(m *Machine)`, `Restore(m *Machine)`) — I/O is abstracted (req 2 OK). Uses `encoding/gob` for machine serialization. Internals (memory, object table, globals) unexported.
- **Status:** last pushed 2020-04-26; pre-modules era idioms (`ioutil`); "supports Version 3 and later" but sparse tests (4 small test files), no evidence of full Zork playthroughs.
- **Verdict:** plausible base but stale, licensing is informal, and reqs 3/4 would need forking.

### 3. `github.com/Katharine/zmachine.go` and its ancestor `github.com/inhies/zmachine`

- **Repos:** https://github.com/Katharine/zmachine.go · https://github.com/inhies/zmachine (same codebase family: identical `dictionary.go`/`objects.go`/`stack.go` sizes)
- **License:** **none** in either repo — disqualifying for embedding.
- **API shape (verified in `zmachine.go`):** channel-based I/O (`New(file string, in, out chan string, err chan error)`) — req 2 OK. "Basic Z-Machine library in Go supporting up to version 3." Quetzal load/save to files. All machine internals unexported; no in-memory snapshot API; essentially no tests. Last pushed 2024-02 (Katharine) / 2018 (inhies).

### 4. `github.com/msinilo/zmachine`

- **Repo:** https://github.com/msinilo/zmachine · write-up: http://msinilo.pl/blog2/post/p1252/
- **License:** MIT. 20 stars (highest of the field). Last pushed 2021-12.
- **Shape:** a single 36 KB `zmachine.go` in `package main` — a terminal **binary, not a library** (pkg.go.dev lists it as a command). Plays Zork 1 (screenshot in README). Would require major surgery for reqs 2–4. Useful as a size datum (~1,500 LOC for a playable v3 interpreter), not as a dependency.

### 5. Remaining field (all unsuitable)

- `github.com/danieledapo/gork` — v3, library-ish layout with an I/O device interface, but README says "far from being complete … missing instructions"; no license. https://github.com/danieledapo/gork
- `github.com/DaveTCode/zmachine-golang`, `github.com/Drakmyth/golang-zmachine`, `github.com/benc-uk/gozm`, `github.com/awgh/zmachine`, `github.com/disposedtrolley/goz`, `github.com/jolson88/z-go`, `github.com/kelindar/zmachine` — self-described learning projects or WIPs, 0–1 stars, terminal-oriented, unverified completeness. (GitHub API search, 2026-08-29.)

## Scoping the write-it-ourselves option (v3-only VM)

Per the Z-Machine Standards Document 1.1 (https://inform-fiction.org/zmachine/standards/z1point1/, esp. §14 opcode tables):

- **Opcode surface in v3:** 2OP: 24 (`je` … `mod`), 1OP: 15 (`jz` … `not`), 0OP: 14 (`rtrue` … `verify`, incl. `save`/`restore`/`show_status`), VAR: 15 (`call`, `storew`, `storeb`, `put_prop`, `sread`, `print_char`, `print_num`, `random`, `push`, `pull`, `split_window`, `set_window`, `output_stream`, `input_stream`, `sound_effect`) — **~68 opcodes total**, no EXT opcodes (v5+). These counts match the per-file opcode dispatch in maloquacious/zmachine.
- **Subsystems:** memory map + header (§1, §11); instruction decoding (3 forms, §4); ZSCII and Z-string decoding with abbreviations (§3); dictionary + lexical analysis for `sread` (§13); object table — v3 is the simple case: 32 attributes, max 255 objects, 9-byte entries, 31-word property defaults (§12); routines/call stack/local variables (§5–6); output streams and status line (§7–8); save/restore state (Quetzal, or any private snapshot format).
- **v3-only savings vs. v4+:** no extended opcode set, no `call_1s/call_2s/call_vs2` family, small object table format, no unicode/undo/colour/mouse, status line rendered by the interpreter from globals 1–3 rather than by the game.
- **Effort estimate from real implementations:** msinilo's playable single-file v3 interpreter is ~1,500 LOC; Katharine's library ~1,300 LOC across 9 files; gonorth's `north` package ~1,900 LOC; maloquacious's engine is ~8–9k LOC of source *plus* an equal volume of tests — the delta is exactly the hard part: correctness at the edges (branch encodings, `div/mod` rounding, `random` semantics, `sread` tokenisation, status-line rules) and the test infrastructure (Frotz differential harness, golden playthroughs) needed to *know* Zork 2/3 run correctly, not just Zork 1's opening. Realistic in-repo scope: 2–3k LOC to "plays Zork," plus weeks of conformance debugging and a verification harness we would be rebuilding from scratch.

## Comparison against the hard requirements

| Requirement | maloquacious/zmachine | gonorth | Katharine/inhies | msinilo | write our own |
|---|---|---|---|---|---|
| 1. Runs Zork 1/2/3, full v3 | Yes — tested against all three, differential vs. Frotz | Claimed v3+, unverified | Claimed v3, untested | Zork 1 demonstrated | Eventually, after conformance work |
| 2. Programmatic I/O | Yes — core design (`Run`/`Result`) | Yes — `UI` interface | Yes — channels | No — terminal binary | Yes — by design |
| 3. Read objects/globals/flags | Indirect — decode `Result.State` with `maloquacious/quetzal` + small fixed-layout reader (~200 LOC) | No — fork needed | No — fork needed | No | Yes — trivially |
| 4. In-memory snapshot/restore | Yes — first-class, equivalence-tested | Partial (gob), fork needed | No (file Quetzal only) | No | Yes — by design |
| License | MIT | informal BSD, no LICENSE file | **none** | MIT | n/a |
| Maintained | Active (2026-07) | 2020 | 2024/2018 | 2021 | us |

## Recommendation

**Use `github.com/maloquacious/zmachine` (+ `github.com/maloquacious/quetzal`). Do not write a VM in-repo.**

Rationale:

1. It is the only candidate whose *stated purpose* is omazork's exact shape — a headless, embeddable v3 engine advancing a session one command per request — and it satisfies requirements 1, 2, and 4 out of the box, with the strongest verification story in the field (scripted Zork I playthrough, Frotz differential tests with pinned PRNG seeds, tests across two editions each of Zork I/II/III). MIT-licensed, actively maintained (July 2026), std-lib-only.
2. Requirement 3 costs a small, well-bounded adapter, not a fork: `Result.State` is standard Quetzal; `quetzal.File.Memory(story)` reconstructs full dynamic memory; the v3 header/global/object layouts are fixed and simple (Standard §11, §12). A ~200-LOC read-only "peek" package in omazork covers objects, attributes, properties, globals, and flags — and stays valid because the engine guarantees `State` remains a conforming Quetzal image.
3. Writing our own is not crazy (~68 opcodes, ~2–3k LOC) but the cost is dominated by conformance debugging and building the very Frotz-differential test harness this library already has. That effort buys nothing the adapter doesn't, and requirement 1 ("correctly") is precisely where hand-rolled interpreters silently fail on Zork 2/3.
4. Risks and mitigations: the library is v0.x, single-author, 0 stars, and requires Go 1.26. Mitigations: pin the version; its saved-state compatibility contract is explicitly version-independent; MIT means a vendored fork is the worst case, and even then we inherit its test suite. Fallback if it disappoints: fork it (we own reqs already met) — not gonorth (stale, licensing informal) and not the unlicensed Katharine/inhies line.

## Sources

- https://github.com/maloquacious/zmachine — README, `machine.go`, `execute.go`, `state.go`, `trace.go`, `opcode_*.go`, `playthrough_test.go`, `differential_test.go`, `local_stories_test.go`, `go.mod` (read 2026-08-29)
- https://github.com/maloquacious/quetzal — README, `memory.go` (`File.Memory`, `DecodeCMem`)
- https://pkg.go.dev/github.com/maloquacious/zmachine
- https://github.com/zombiezen/gonorth — `README.rst`, `north/machine.go`
- https://github.com/Katharine/zmachine.go — `zmachine.go`; https://github.com/inhies/zmachine
- https://github.com/msinilo/zmachine · http://msinilo.pl/blog2/post/p1252/
- https://github.com/danieledapo/gork · https://github.com/Drakmyth/golang-zmachine · https://github.com/DaveTCode/zmachine-golang · https://github.com/benc-uk/gozm · https://github.com/awgh/zmachine (GitHub API repo search, 2026-08-29)
- Z-Machine Standards Document 1.1: https://inform-fiction.org/zmachine/standards/z1point1/ (§3 text, §4 instructions, §5–6 routines/stack, §8 status line, §11 header, §12 object table, §13 dictionary, §14 opcode tables); mirror consulted: https://zspec.jaredreisinger.com/14-opcode-table
- Quetzal 1.4: https://ifarchive.org/if-archive/infocom/interpreters/specification/savefile_14.txt
