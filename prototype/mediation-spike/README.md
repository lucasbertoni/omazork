# PROTOTYPE: mediation wrapper spike (issue #7) — throwaway, do not productionize

Proves the Z-machine path from the [Go Z-machine strategy](https://github.com/lucasbertoni/omazork/issues/2) decision holds up in practice:

1. **Engine**: `maloquacious/zmachine` v0.2.0 plays `zork1.z3` (Release 119) headlessly, zero external deps, single static binary.
2. **Internal state**: score / moves / current-room read by Quetzal-decoding `Result.State` with `maloquacious/quetzal` (`/peek` command) — the "~200-LOC peek adapter" from the research, minimal core is ~25 lines here.
3. **Mediation**: casual pacing rule from [Casual pacing rules](https://github.com/lucasbertoni/omazork/issues/5) — a turn whose score delta ≠ 0 has its output *and status line* withheld; game input is blocked while pending (meta commands allowed); reveal is a "While you were away…" recap on the next interaction after maturity.

Run (needs Go 1.26+):

    go run .            # interactive, 15s outcome delay
    go run . -delay 3s  # shorter delay
    go run . -demo      # scripted playthrough, no TTY needed

Findings for the real implementation:
- The engine API (`Start`/`Run` → `Result{Output, StatusLine, State}`) maps 1:1 onto the NDJSON turn protocol decided in issue #4. No friction.
- The status line is part of the outcome: it must be withheld along with the output or it spoils the new room/score. (Caught live in this spike.)
- Quetzal decode of every turn's `State` is cheap (86KB story, ~10KB dynamic memory) — per-turn peeking is viable for score-event detection and autosave alike; `Result.State` doubles as the autosave payload with zero extra work.
- `/peek` here would spoil too in real life; in the product, internal-state reads feed the mediator, not the player.
