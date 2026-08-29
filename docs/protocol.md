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
| `picker` | — | `picker`: the three games with playthrough summaries |
| `new` | `game`, `mode` (`classic`/`casual`), `replace` | opening `output`, or `confirm-replace` when a playthrough exists and `replace` is absent |
| `resume` | `game` | `output` with `status`, `transcript` (recent console lines), and `pending`/`reveal` when applicable |
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
- **`withheld`** — Casual: the turn scored and its outcome is held back.
  `status` stays at pre-turn values (the status line is part of the outcome
  and would spoil it); `pending` is `{maturesAt, remaining}` (nanoseconds).
- **`blocked`** — game input while an outcome is pending and unmatured.
  `save`/`restore` are game input and also answer `blocked`.
- **`ended`** — the story halted; `ended` is `"won"`, `"quit"`, or `"died"`
  (Zork III's fourth death hard-quits). `quit` playthroughs resume from the
  last autosave; `won`/`died`-final playthroughs are `finished` in the picker.

`reveal` (on `opened`, `resume`, or the first `input` after maturation) is the
"While you were away…" recap: `{command, output, delta, status, unlocked}` —
achievements earned on the withheld turn unlock here, not earlier.

## Events (stdout, unsolicited)

- **`matured`** `{game}` — a pending outcome matured; emitted once per outcome
  (the wrapper checks every `-maturation-tick`). The QML side fires the single
  non-spoiler desktop notification ("Something has happened in the Great
  Underground Empire").

Because unsolicited events interleave with responses, the QML reader should
dispatch on `type` (and `id`), not on request order.
