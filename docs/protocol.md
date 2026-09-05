# Wrapper protocol (NDJSON over stdio)

The QML overlay spawns `bin/omazork` as a Quickshell `Process` child and
exchanges one JSON object per line: requests on stdin, responses and events on
stdout ([#4](https://github.com/lucasbertoni/omazork/issues/4)). Every request
gets exactly one response line; an optional `"id"` field is echoed back for
correlation. Malformed input gets an `error` line, never a dropped stream.

The wrapper owns durability: every turn is autosaved to
`$XDG_STATE_HOME/omazork/<game>/`, so a SIGTERM (plugin rescan, shell restart)
plus respawn resumes invisibly. The binary takes `-state <dir>` and
`-maturation-tick <duration>` flags.

## Requests (stdin)

| type | fields | response |
|---|---|---|
| `picker` | — | `picker`: the three games with playthrough summaries; a pending outcome carries `pendingMaturesAt`, `pendingTier` and `pendingStartedAt` so the bar icon can rediscover a recap that matured while the wrapper was down, draw the in-flight dot meanwhile, and the picker can draw wait progress (no narration: spoiler-free) |
| `new` | `game`, `mode` (`classic`/`casual`), `replace` | opening `output`, or `confirm-replace` when a playthrough exists and `replace` is absent |
| `resume` | `game` | `output` with `status`, `transcript` (recent console lines), and `pending` when applicable; a matured outcome reveals on the follow-up `opened`, never here |
| `input` | `text` | `output` / `withheld` / `blocked` / `checkpoint` / `checkpoints` / `ended` (see below) |
| `restore-checkpoint` | `checkpoint` (id) | `output` rewound to the checkpoint |
| `opened` | — | `output`; carries `reveal` when a matured outcome is waiting |
| `closed` | — | `closed` (playtime accounting) |
| `stats` | — | `stats` for the active playthrough |
| `achievements` | `game` | `achievements`: full table with `unlocked`/`unlockedAt`; the UI renders hidden+locked entries as "???" |
| `check-maturation` | — | `matured` once per matured outcome, else `quiet` |

`input` is a raw player command. The wrapper intercepts `save` (creates a
checkpoint, responds `checkpoint`) and `restore` (responds `checkpoints`, a
list; the UI follows up with `restore-checkpoint`). Everything else goes to
the Z-machine.

## Turn responses

Common fields: `output` (story text), `status` (`{room, score, moves}`),
`unlocked` (achievements earned by this response), `reveal`, `pending`.

- **`output`** — a normal turn. In Classic mode every turn is this.
- **`withheld`** — Casual: the turn carries an action wait
  (docs/action-waits.md) and its outcome is held back.
  `status` stays at pre-turn values (the status line is part of the outcome
  and would spoil it); `pending` is `{maturesAt, remaining, tier, command,
  narration, startedAt}`: `remaining` in nanoseconds, `tier` the wait's
  presentation register (`"quiet"` below 5 minutes, `"full"` at or above —
  decided in the wrapper, never recomputed by the Console; docs/action-waits.md
  §8), `command` the player's own echoed command, `narration` the wait
  narration ("climbing the tree", built from the player's words; §8),
  `startedAt` when the wait began. `narration` and `startedAt` are absent on
  saves that predate them. The `output` copy is "You begin {narration}..."
  in both tiers.
- **`blocked`** — game input while an outcome is pending and unmatured.
  `save`/`restore` are game input and also answer `blocked`. Copy is "You are
  still {narration}..."; a pending outcome without narration falls back to the
  tiered "Time is still passing..." / "The outcome of your last action is still
  unfolding...".
- **`ended`** — the story halted; `ended` is `"won"`, `"quit"`, or `"died"`
  (Zork III's fourth death hard-quits). `quit` playthroughs resume from the
  last autosave; `won`/`died`-final playthroughs are `finished` in the picker.

`reveal` (on `opened` or the first `input` after maturation) is the matured
outcome: `{tier, command, output, delta, status, unlocked}` — achievements
earned on the withheld turn unlock here, not earlier. The Console frames a
`full` reveal as the "While you were away…" recap and shows a `quiet` one
plainly: command echo, output, score/achievement lines, no header.

## Events (stdout, unsolicited)

- **`matured`** `{game, tier}` — a pending outcome matured; emitted once per
  outcome (the wrapper checks every `-maturation-tick`). For a `full` wait the
  QML side fires the single non-spoiler desktop notification ("Something has
  happened in the Great Underground Empire"); `quiet` waits never notify, the
  bar dot alone carries the news.

Because unsolicited events interleave with responses, the QML reader should
dispatch on `type` (and `id`), not on request order.
