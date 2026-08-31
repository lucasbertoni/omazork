#!/bin/bash
# Fixture verification helper (wayfinder ticket #30).
# usage: ./check.sh <game> <seed> <script.txt> [N]
# Runs the capture harness (internal/actions/capture) and prints a compact
# report: turn/score/move totals, suspicious turns, and the last N outputs.
set -euo pipefail
GAME=$1 SEED=$2 SCRIPT=$3 TAIL=${4:-3}
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
OUT=$(mktemp)
trap 'rm -f "$OUT"' EXIT
(cd "$ROOT" && go run ./internal/actions/capture "$GAME" "$SEED" < "$SCRIPT") > "$OUT"

SCRIPT_CMDS=$(grep -cvE '^\s*(#|$)' "$SCRIPT" || true)
jq -r --argjson tail "$TAIL" --argjson scriptCmds "$SCRIPT_CMDS" '
  . as $all | length as $n |
  "turns run: \($n - 1) of \($scriptCmds) script commands",
  "final: room=\(.[-1].room) score=\(.[-1].score) moves=\(.[-1].moves)",
  "",
  "--- suspicious turns ---",
  ( range(1; $n) | . as $i | $all[$i] as $cur | $all[$i-1] as $prev |
    select(
      ($cur.output | test("I don.t know the word|You can.t go that way|There is a wall|You don.t have|can.t see any|Huh\\?|What\\?|beg your pardon|It is too dark|would be a good idea|RESTART, RESTORE|You have died|is closed[.]"; "i"))
      or (($cur.input | test("^(n|s|e|w|ne|nw|se|sw|u|d|up|down|north|south|east|west|northeast|northwest|southeast|southwest|enter|exit|out|in|land|launch|cross)$"; "i")) and $cur.roomObj == $prev.roomObj)
    ) |
    "[\($i)] > \($cur.input) :: \($cur.output | gsub("\n"; " ") | .[0:140])"
  ),
  "",
  "--- last \($tail) turns ---",
  (.[-$tail:][] | "[\(.room) s=\(.score) m=\(.moves)] > \(.input)\n\(.output)")
' "$OUT"
