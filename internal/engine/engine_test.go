package engine_test

import (
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/engine"
)

func mustNew(t *testing.T, game string) *engine.Engine {
	t.Helper()
	e, err := engine.New(game, engine.WithSeed(1))
	if err != nil {
		t.Fatalf("New(%q): %v", game, err)
	}
	return e
}

func TestStartZork1(t *testing.T) {
	e := mustNew(t, "zork1")
	turn, err := e.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !strings.Contains(turn.Output, "West of House") {
		t.Errorf("opening output missing West of House:\n%s", turn.Output)
	}
	if turn.Score != 0 || turn.Moves != 0 {
		t.Errorf("initial score/moves = %d/%d, want 0/0", turn.Score, turn.Moves)
	}
	if turn.RoomObj != 64 {
		t.Errorf("initial room object = %d, want 64 (West of House)", turn.RoomObj)
	}
	if turn.Halted {
		t.Error("halted at start")
	}
	if len(turn.State) == 0 {
		t.Error("no snapshot state at input boundary")
	}
}

func TestRunToKitchenScores(t *testing.T) {
	e := mustNew(t, "zork1")
	if _, err := e.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	var turn engine.Turn
	for _, cmd := range []string{"south", "east", "open window", "enter window"} {
		var err error
		turn, err = e.Run(cmd)
		if err != nil {
			t.Fatalf("Run(%q): %v", cmd, err)
		}
	}
	if turn.Score != 10 {
		t.Errorf("score after entering kitchen = %d, want 10", turn.Score)
	}
	if turn.Room != "Kitchen" {
		t.Errorf("room = %q, want Kitchen", turn.Room)
	}
	if turn.Moves != 4 {
		t.Errorf("moves = %d, want 4", turn.Moves)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	e := mustNew(t, "zork1")
	if _, err := e.Start(); err != nil {
		t.Fatal(err)
	}
	turn, err := e.Run("open mailbox")
	if err != nil {
		t.Fatal(err)
	}
	// A fresh engine restored from the snapshot continues from the same point.
	e2 := mustNew(t, "zork1")
	if err := e2.Restore(turn.State); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	t2, err := e2.Run("read leaflet")
	if err != nil {
		t.Fatalf("Run after restore: %v", err)
	}
	if !strings.Contains(t2.Output, "ZORK") {
		t.Errorf("leaflet text missing after restore:\n%s", t2.Output)
	}
	if t2.Moves != turn.Moves+1 {
		t.Errorf("moves after restore = %d, want %d", t2.Moves, turn.Moves+1)
	}
}

func TestAllThreeGamesStart(t *testing.T) {
	for _, g := range []string{"zork1", "zork2", "zork3"} {
		e := mustNew(t, g)
		if _, err := e.Start(); err != nil {
			t.Errorf("%s Start: %v", g, err)
		}
	}
}

// TestZork23Smoke plays a few deterministic turns of Zork II and III: upstream
// zmachine deep-tests only Zork I, so this is the scripted smoke run #11
// recommends.
func TestZork23Smoke(t *testing.T) {
	plays := map[string][]string{
		"zork2": {"look", "south", "get all", "north", "inventory"},
		"zork3": {"look", "down", "get all", "inventory", "wait"},
	}
	for g, cmds := range plays {
		e := mustNew(t, g)
		if _, err := e.Start(); err != nil {
			t.Fatalf("%s Start: %v", g, err)
		}
		for _, c := range cmds {
			turn, err := e.Run(c)
			if err != nil {
				t.Fatalf("%s Run(%q): %v", g, c, err)
			}
			if turn.Halted {
				t.Fatalf("%s halted on %q", g, c)
			}
			if len(turn.State) == 0 {
				t.Fatalf("%s: no snapshot after %q", g, c)
			}
			// Snapshot round-trip: a restored engine accepts input.
			e2 := mustNew(t, g)
			if err := e2.Restore(turn.State); err != nil {
				t.Fatalf("%s restore after %q: %v", g, c, err)
			}
		}
	}
}

func TestPeekReadsGlobalsFromSnapshot(t *testing.T) {
	e := mustNew(t, "zork1")
	if _, err := e.Start(); err != nil {
		t.Fatal(err)
	}
	turn, err := e.Run("south")
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.Peek(turn.State)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if view.RoomObj != turn.RoomObj || view.Moves != turn.Moves || view.Score != turn.Score {
		t.Errorf("peek = %+v, want room %d moves %d score %d", view, turn.RoomObj, turn.Moves, turn.Score)
	}
	if view.RoomObj == 64 {
		t.Error("room object still West of House after walking south")
	}
}
