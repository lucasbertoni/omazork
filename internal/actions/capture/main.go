// Command capture replays a command script against the real seeded engine
// and emits one JSON array of per-turn records (input, output, room name,
// room object number, score, moves) — the transcript format the matcher and
// the walkthrough fixtures are built on. Lifted from the action-matcher
// prototype (#22) by #32.
//
//	go run ./internal/actions/capture <game> <seed> < script.txt
//
// The script is one command per line; blank lines and #-comments are
// skipped. See internal/actions/testdata/ for the committed fixtures and
// check.sh for a compact replay report.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/lucasbertoni/omazork/internal/actions"
	"github.com/lucasbertoni/omazork/internal/engine"
)

type turnRec struct {
	Input   string `json:"input"` // "" for the opening banner
	Output  string `json:"output"`
	Room    string `json:"room"`    // status-line name after the turn
	RoomObj uint16 `json:"roomObj"` // global 0 after the turn
	Score   int    `json:"score"`
	Moves   int    `json:"moves"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: capture <game> <seed> < script.txt")
		os.Exit(2)
	}
	game := os.Args[1]
	seed, err := strconv.ParseUint(os.Args[2], 10, 64)
	must(err)
	script, err := io.ReadAll(os.Stdin)
	must(err)

	e, err := engine.New(game, engine.WithSeed(seed))
	must(err)

	rec := func(input string, t engine.Turn) turnRec {
		return turnRec{Input: input, Output: t.Output, Room: t.Room, RoomObj: t.RoomObj, Score: t.Score, Moves: t.Moves}
	}
	t, err := e.Start()
	must(err)
	recs := []turnRec{rec("", t)}
	for _, cmd := range actions.ParseScript(script) {
		t, err = e.Run(cmd)
		must(err)
		recs = append(recs, rec(cmd, t))
		if t.Halted {
			break
		}
	}
	out, err := json.MarshalIndent(recs, "", " ")
	must(err)
	fmt.Println(string(out))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "capture:", err)
		os.Exit(1)
	}
}
