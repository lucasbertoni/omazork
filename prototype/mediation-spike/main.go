// PROTOTYPE — throwaway spike for issue #7 (Mediation wrapper spike).
// Question: does maloquacious/zmachine + quetzal hold up as omazork's engine?
// Proves end-to-end in a plain terminal:
//   1. zork1.z3 playing through the embedded engine (no external deps)
//   2. reading internal state (score/moves/room) by Quetzal-decoding Result.State
//   3. one casual-mediation behavior: a scoring outcome is withheld, input is
//      blocked while it matures, then revealed with a "While you were away" recap
//
// Run:  go run .            (interactive; 15s delay on score changes)
//       go run . -delay 3s  (shorter delay)
//       go run . -demo      (scripted playthrough proving all three, no TTY)
package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maloquacious/quetzal"
	"github.com/maloquacious/zmachine"
)

//go:embed zork1.z3
var storyBytes []byte

// peek is the read-only internal-state view the research ticket said we could
// build (~200 LOC adapter in the real thing; this is the minimal core of it).
type peek struct {
	Room  uint16 // global 0: current room object number
	Score int16  // global 1
	Moves uint16 // global 2
}

// peekState Quetzal-decodes an engine Result.State and reads v3 globals out of
// the reconstructed dynamic memory. Header word at 0x0C is the global table
// address; global g is the big-endian word at table + 2g (Standard §6.2, §11).
func peekState(state []byte, story quetzal.Story) (peek, error) {
	f, err := quetzal.Decode(bytes.NewReader(state))
	if err != nil {
		return peek{}, fmt.Errorf("decode quetzal: %w", err)
	}
	mem, err := f.Memory(story)
	if err != nil {
		return peek{}, fmt.Errorf("reconstruct memory: %w", err)
	}
	globals := binary.BigEndian.Uint16(mem.Data[0x0C:])
	g := func(n int) uint16 {
		return binary.BigEndian.Uint16(mem.Data[int(globals)+2*n:])
	}
	return peek{Room: g(0), Score: int16(g(1)), Moves: g(2)}, nil
}

// pending is a withheld outcome: the turn already ran, its text is held back.
type pending struct {
	output   string
	delta    int16
	matures  time.Time
	statusAt zmachine.StatusLine
}

func main() {
	delay := flag.Duration("delay", 15*time.Second, "how long a scoring outcome is withheld")
	demo := flag.Bool("demo", false, "scripted non-interactive playthrough")
	flag.Parse()

	story, err := zmachine.LoadStory(storyBytes)
	if err != nil {
		fatal("load story: %v", err)
	}
	qstory, err := quetzal.ParseStory(storyBytes)
	if err != nil {
		fatal("parse story for quetzal: %v", err)
	}
	m, err := zmachine.New(story, zmachine.WithRandomSeed(1))
	if err != nil {
		fatal("new machine: %v", err)
	}
	res, err := m.Start(context.Background())
	if err != nil {
		fatal("start: %v", err)
	}
	fmt.Print(res.Output)

	last, err := peekState(res.State, qstory)
	if err != nil {
		fatal("initial peek: %v", err)
	}
	var hold *pending

	oneTurn := func(line string) bool {
		line = strings.TrimSpace(line)
		if line == "" {
			return true
		}
		// Meta commands (allowed even while an outcome is pending).
		switch line {
		case "/peek":
			fmt.Printf("[peek] room=%d score=%d moves=%d (from Quetzal-decoded engine state)\n",
				last.Room, last.Score, last.Moves)
			return true
		case "/pending":
			if hold == nil {
				fmt.Println("[mediator] nothing pending")
			} else {
				fmt.Printf("[mediator] outcome pending (%+d points), matures in %s\n",
					hold.delta, time.Until(hold.matures).Round(time.Second))
			}
			return true
		case "/quit":
			return false
		}

		// A matured outcome reveals before anything else happens.
		if hold != nil && time.Now().After(hold.matures) {
			fmt.Printf("\n=== While you were away... (%+d points) ===\n%s=== (score is now %d) ===\n\n",
				hold.delta, hold.output, hold.statusAt.Score)
			hold = nil
		}
		// Unmatured outcome blocks game input.
		if hold != nil {
			fmt.Printf("[mediator] The outcome of your last move is still settling (%s left). Try /pending or wait.\n",
				time.Until(hold.matures).Round(time.Second))
			return true
		}

		res, err := m.Run(context.Background(), line)
		if err != nil {
			fatal("run: %v", err)
		}
		now, err := peekState(res.State, qstory)
		if err != nil {
			fatal("peek: %v", err)
		}
		delta := now.Score - last.Score
		last = now

		if delta != 0 {
			// Casual pacing rule: delay iff score delta != 0.
			hold = &pending{output: res.Output, delta: delta,
				matures: time.Now().Add(*delay), statusAt: res.StatusLine}
			fmt.Printf("[mediator] Something happened (%+d points)... you'll find out in %s.\n", delta, *delay)
		} else {
			fmt.Print(res.Output)
		}
		// The status line is part of the outcome: while one is pending it
		// would spoil the new room/score, so it is withheld too.
		if res.StatusLine.Available && hold == nil {
			fmt.Printf("  -- %s | score %d / moves %d --\n",
				res.StatusLine.Name, res.StatusLine.Score, res.StatusLine.Turns)
		}
		return res.Status != zmachine.Halted
	}

	if *demo {
		script := []string{
			"open mailbox", "read leaflet", "/peek",
			"south", "east", "open window", "enter window", // Kitchen: +10 points -> withheld
			"/peek", "west", // blocked: outcome pending
			"/pending",
			"WAIT_FOR_MATURITY",
			"west", // reveal fires, then this turn plays (Living Room)
			"/peek",
		}
		for _, cmd := range script {
			if cmd == "WAIT_FOR_MATURITY" {
				fmt.Printf("[demo] sleeping %s for the outcome to mature...\n", *delay)
				time.Sleep(*delay + time.Second)
				continue
			}
			fmt.Printf("\n> %s\n", cmd)
			if !oneTurn(cmd) {
				break
			}
		}
		return
	}

	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !sc.Scan() {
			return
		}
		if !oneTurn(sc.Text()) {
			return
		}
	}
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "spike: "+format+"\n", a...)
	os.Exit(1)
}
