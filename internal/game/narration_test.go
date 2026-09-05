package game_test

import (
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/game"
)

// TestWaitNarrationForVerb: a withheld action narrates what the player is
// doing in their own words (#43) — verb phrase plus the command tail as the
// parser read it (noise words dropped) — on the withheld line, the blocked
// line, and PendingInfo, and PendingInfo says when the wait began.
func TestWaitNarrationForVerb(t *testing.T) {
	s, clock := casualAtWindow(t)
	started := clock.t
	resp, err := s.Command("enter the window")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindWithheld || resp.Pending == nil {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Output != "You begin entering window..." {
		t.Errorf("withheld copy = %q", resp.Output)
	}
	if resp.Pending.Narration != "entering window" {
		t.Errorf("narration = %q", resp.Pending.Narration)
	}
	if resp.Pending.StartedAt == nil || !resp.Pending.StartedAt.Equal(started) {
		t.Errorf("startedAt = %v, want %v", resp.Pending.StartedAt, started)
	}
	if b, _ := s.Command("west"); b.Kind != game.KindBlocked || b.Output != "You are still entering window..." {
		t.Errorf("blocked = %+v", b)
	}
}

// TestLegacyPendingWithoutNarration: a pending outcome saved before wait
// narration carries neither narration nor start time; the blocked line falls
// back to the generic tier copy and PendingInfo reports no start.
func TestLegacyPendingWithoutNarration(t *testing.T) {
	s, clock := casualAtWindow(t)
	if _, err := s.Command("enter window"); err != nil {
		t.Fatal(err)
	}
	p, ok, err := s.Store().LoadPlaythrough("zork1")
	if err != nil || !ok {
		t.Fatalf("load: %v %v", ok, err)
	}
	p.Pending.Narration = ""
	p.Pending.StartedAt = time.Time{}
	if err := s.Store().SavePlaythrough(p); err != nil {
		t.Fatal(err)
	}
	r, _, err := game.Resume(game.Config{Store: s.Store(), Now: clock.now, Seed: 1}, "zork1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Command("west")
	if err != nil {
		t.Fatal(err)
	}
	// "enter window" is a quiet-tier wait, so the generic quiet line.
	if b.Kind != game.KindBlocked || b.Output != "Time is still passing..." {
		t.Errorf("blocked = %+v", b)
	}
	if b.Pending == nil || b.Pending.Narration != "" || b.Pending.StartedAt != nil {
		t.Errorf("pending = %+v", b.Pending)
	}
}

// TestWaitNarrationTwoWordVerb: "turn on X" is one verb to the parser, and the
// narration must not leak the second word back into the tail.
func TestWaitNarrationTwoWordVerb(t *testing.T) {
	s, clock := casualAtWindow(t)
	play(t, s, clock, "enter window", "west", "take lamp")
	resp, err := s.Command("turn on lamp")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindWithheld {
		t.Skipf("turn on lamp is instant here (%s); narration needs a priced turn", resp.Kind)
	}
	if resp.Pending.Narration != "lighting lamp" {
		t.Errorf("narration = %q, want lighting lamp", resp.Pending.Narration)
	}
}

// TestWaitNarrationClassFallback: a verb the shared table has no phrase for
// narrates by the priced row's class, standing alone — no tail, and nothing
// the player did not type.
func TestWaitNarrationClassFallback(t *testing.T) {
	s, clock := casualAtWindow(t)
	play(t, s, clock, "enter window", "west")
	resp, err := s.Command("kick rug")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindWithheld {
		t.Skipf("kick rug is instant here (%s); the fallback needs a priced turn", resp.Kind)
	}
	if resp.Pending.Narration != "handling" {
		t.Errorf("narration = %q, want the manipulation fallback", resp.Pending.Narration)
	}
}
