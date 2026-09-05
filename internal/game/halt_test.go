package game_test

import (
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/game"
	"github.com/lucasbertoni/omazork/internal/session"
)

func TestGrueDeathCountsAndUnlocksImmediately(t *testing.T) {
	cfg, clock := newCfg(t)
	// Casual mode: even here a death is never withheld (#5).
	s, _, err := game.New(cfg, "zork1", session.Casual)
	if err != nil {
		t.Fatal(err)
	}
	play(t, s, clock, "south", "east", "open window", "enter window", "west", "move rug", "open trap door")
	// The descent is a room-changing walk: withheld for its edge price.
	resp, _ := s.Command("down")
	if resp.Kind != game.KindWithheld {
		t.Fatalf("descent not withheld: %+v", resp)
	}
	clock.t = clock.t.Add(resp.Pending.Remaining)
	_, _ = s.Opened()
	play(t, s, clock, "north", "north")
	// Third dark move: eaten by a grue (seed 1). Death reveals immediately.
	death, err := s.Command("north")
	if err != nil {
		t.Fatal(err)
	}
	if death.Kind != game.KindOutput {
		t.Fatalf("death withheld: kind=%q", death.Kind)
	}
	if !strings.Contains(death.Output, "grue") {
		t.Fatalf("not a grue death:\n%s", death.Output)
	}
	if s.Stats().Deaths != 1 {
		t.Errorf("deaths stat = %d, want 1", s.Stats().Deaths)
	}
	got := map[string]bool{}
	for _, a := range death.Unlocked {
		got[a.ID] = true
	}
	if !got["occupational-hazard"] || !got["grue-food"] {
		t.Errorf("death unlocks = %v, want occupational-hazard + grue-food", got)
	}
}

func TestQuitHaltIsResumable(t *testing.T) {
	cfg, _ := newCfg(t)
	s, _, err := game.New(cfg, "zork1", session.Classic)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Command("open mailbox")
	_, _ = s.Command("quit")
	resp, err := s.Command("y")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindEnded || resp.Ended != "quit" {
		t.Fatalf("quit halt = %+v", resp)
	}
	// Quit is not a finish: resume returns to the pre-quit autosave.
	s2, r2, err := game.Resume(game.Config{Store: s.Store(), Now: cfg.Now, Seed: 1}, "zork1")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Status.Moves == 0 {
		t.Errorf("resume status = %+v", r2.Status)
	}
	if _, err := s2.Command("read leaflet"); err != nil {
		t.Fatalf("play after quit-resume: %v", err)
	}
}
