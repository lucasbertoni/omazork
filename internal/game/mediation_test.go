package game_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/game"
	"github.com/lucasbertoni/omazork/internal/session"
)

// walkToWindow plays up to the scoring move in a casual game.
func casualAtWindow(t *testing.T) (*game.Session, *fakeClock) {
	t.Helper()
	cfg, clock := newCfg(t)
	s, _, err := game.New(cfg, "zork1", session.Casual)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"south", "east", "open window"} {
		if _, err := s.Command(cmd); err != nil {
			t.Fatal(err)
		}
	}
	return s, clock
}

func TestCasualWithholdsScoringOutcome(t *testing.T) {
	s, clock := casualAtWindow(t)
	resp, err := s.Command("enter window")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindWithheld {
		t.Fatalf("kind = %q, want withheld", resp.Kind)
	}
	if strings.Contains(resp.Output, "Kitchen") || strings.Contains(resp.Output, "kitchen") {
		t.Errorf("withheld response leaks the outcome:\n%s", resp.Output)
	}
	// The status line is withheld with the output (#7): still pre-turn values.
	if resp.Status == nil || resp.Status.Score != 0 || resp.Status.Room == "Kitchen" {
		t.Errorf("status leaks: %+v", resp.Status)
	}
	// visit-kitchen waits 5 minutes per the zork1 table.
	if resp.Pending == nil || resp.Pending.Remaining != 5*time.Minute {
		t.Errorf("pending = %+v, want 5m remaining", resp.Pending)
	}

	// Game input is blocked while pending; meta stays available (#5).
	blocked, _ := s.Command("west")
	if blocked.Kind != game.KindBlocked {
		t.Errorf("kind = %q, want blocked", blocked.Kind)
	}
	// SAVE and RESTORE are game input: blocked too (#6).
	saveResp, _ := s.Command("save")
	if saveResp.Kind != game.KindBlocked {
		t.Errorf("save while pending: kind = %q, want blocked", saveResp.Kind)
	}

	// Maturation: notification flag flips once.
	if s.PendingMatured() {
		t.Error("matured too early")
	}
	clock.t = clock.t.Add(5*time.Minute + time.Second)
	if !s.PendingMatured() {
		t.Error("not matured after wait")
	}
	if err := s.MarkNotified(); err != nil {
		t.Fatal(err)
	}
	if s.PendingMatured() {
		t.Error("PendingMatured still true after MarkNotified")
	}

	// Reveal on next console open: recap carries the withheld outcome.
	opened, err := s.Opened()
	if err != nil {
		t.Fatal(err)
	}
	if opened.Reveal == nil {
		t.Fatal("no reveal on open after maturation")
	}
	r := opened.Reveal
	if r.Command != "enter window" || r.Delta != 10 || r.Status.Room != "Kitchen" || r.Status.Score != 10 {
		t.Errorf("recap = %+v", r)
	}
	// The kitchen achievement unlocks at the reveal (#10).
	found := false
	for _, a := range r.Unlocked {
		if a.ID == "breaking-and-entering" {
			found = true
		}
	}
	if !found {
		t.Errorf("breaking-and-entering not in recap unlocks: %+v", r.Unlocked)
	}
	// Status now shows the revealed state.
	if opened.Status.Room != "Kitchen" || opened.Status.Score != 10 {
		t.Errorf("post-reveal status = %+v", opened.Status)
	}
	// Play continues normally.
	next, err := s.Command("west")
	if err != nil {
		t.Fatal(err)
	}
	if next.Kind != game.KindOutput || next.Status.Room != "Living Room" {
		t.Errorf("after reveal: %+v", next)
	}
}

func TestPendingSurvivesRestart(t *testing.T) {
	s, clock := casualAtWindow(t)
	if _, err := s.Command("enter window"); err != nil {
		t.Fatal(err)
	}
	// Wrapper dies (shell rescan) and comes back: pending persists (#6).
	cfg := game.Config{Store: storeOf(t, s), Now: clock.now, Seed: 1}
	_ = cfg
	s2, resp, err := game.Resume(game.Config{Store: storeOf(t, s), Now: clock.now, Seed: 1}, "zork1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Pending == nil {
		t.Fatal("pending lost across restart")
	}
	clock.t = clock.t.Add(6 * time.Minute)
	opened, _ := s2.Opened()
	if opened.Reveal == nil || opened.Reveal.Delta != 10 {
		t.Errorf("reveal after restart = %+v", opened.Reveal)
	}
}

func TestClassicUnlocksImmediately(t *testing.T) {
	cfg, _ := newCfg(t)
	s, _, err := game.New(cfg, "zork1", session.Classic)
	if err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"south", "east", "open window"} {
		_, _ = s.Command(cmd)
	}
	resp, _ := s.Command("enter window")
	found := false
	for _, a := range resp.Unlocked {
		if a.ID == "breaking-and-entering" {
			found = true
		}
	}
	if !found {
		t.Errorf("classic: kitchen achievement not unlocked on the turn: %+v", resp.Unlocked)
	}
}

func TestCheckpointSaveAndRestore(t *testing.T) {
	cfg, clock := newCfg(t)
	s, _, err := game.New(cfg, "zork1", session.Classic)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Command("open mailbox")
	cpResp, err := s.Command("save")
	if err != nil {
		t.Fatal(err)
	}
	if cpResp.Kind != game.KindCheckpoint {
		t.Fatalf("kind = %q, want checkpoint", cpResp.Kind)
	}
	// first-checkpoint achievement fires.
	if len(cpResp.Unlocked) == 0 {
		t.Error("no unlock on first checkpoint")
	}
	// Move on, then RESTORE lists checkpoints.
	_, _ = s.Command("south")
	list, _ := s.Command("restore")
	if list.Kind != game.KindCheckpoints || len(list.Checkpoints) != 1 {
		t.Fatalf("restore list = %+v", list)
	}
	clock.t = clock.t.Add(time.Minute)
	back, err := s.RestoreCheckpoint(list.Checkpoints[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.Status.Room != "West of House" {
		t.Errorf("restored room = %q, want West of House", back.Status.Room)
	}
	// The engine really rewound: the mailbox is already open again.
	again, _ := s.Command("open mailbox")
	if !strings.Contains(strings.ToLower(again.Output), "already open") {
		t.Errorf("engine state not rewound:\n%s", again.Output)
	}
}
func TestNoStatusLeakWhileWithheld(t *testing.T) {
	s, _ := casualAtWindow(t)
	if _, err := s.Command("enter window"); err != nil {
		t.Fatal(err)
	}
	// Every surface must keep showing pre-turn status while pending (#7):
	blocked, _ := s.Command("west")
	if blocked.Status.Room == "Kitchen" || blocked.Status.Score != 0 {
		t.Errorf("blocked leaks status: %+v", blocked.Status)
	}
	opened, _ := s.Opened()
	if opened.Status.Room == "Kitchen" || opened.Status.Score != 0 {
		t.Errorf("opened leaks status: %+v", opened.Status)
	}
	saveResp, _ := s.Command("save")
	if saveResp.Status != nil && (saveResp.Status.Room == "Kitchen" || saveResp.Status.Score != 0) {
		t.Errorf("save leaks status: %+v", saveResp.Status)
	}
}
