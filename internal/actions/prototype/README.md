# THROWAWAY PROTOTYPE — runtime action matcher (wayfinder ticket #22)

Not production code. Answers: can the wrapper classify every turn (instant /
movement edge / (verb, object) action) from output text + room + moves alone,
and does the in-combat detector hold up on real transcripts?

- `matcher-demo.html` — double-click it. The pure matcher module (the liftable
  part) is the first `<script>` section; the page is a shell. Also published as
  a Claude artifact (link on the ticket).
- `main.go` — harness: `go run ./internal/actions/prototype capture <game> <seed> < script.txt`
  replays a command script against the real seeded engine and emits per-turn
  JSON (input, output, room name, room objnum via the Quetzal peek, moves);
  `... objects <game>` dumps the story file's object table and duplicate names.
- `z1-seed7.json` / `z3-seed11.json` — the captured transcripts embedded in the
  demo, with the scripts that produced them.
- `*-objects.json` — objnum → short-name dumps straight from the .z3 files.
