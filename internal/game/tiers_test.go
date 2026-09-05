package game_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/game"
)

// TestQuietTierWait: a sub-5-minute wait takes the quiet register
// (docs/action-waits.md §8) — brief withheld/blocked copy, a plain reveal with
// no recap header — and PendingInfo carries the tier and the echoed command.
func TestQuietTierWait(t *testing.T) {
	s, clock := casualAtWindow(t)
	resp, err := s.Command("enter window") // verb default: 2 min
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindWithheld || resp.Pending == nil {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Pending.Remaining != 2*time.Minute {
		t.Fatalf("remaining = %v, want 2m (the test relies on a quiet-tier price)", resp.Pending.Remaining)
	}
	if resp.Pending.Tier != game.TierQuiet {
		t.Errorf("tier = %q, want quiet", resp.Pending.Tier)
	}
	if resp.Pending.Command != "enter window" {
		t.Errorf("command = %q, want the echoed command", resp.Pending.Command)
	}
	// Withheld and blocked lines carry the wait narration in both tiers (#43).
	if resp.Output != "You begin entering window..." {
		t.Errorf("withheld copy = %q", resp.Output)
	}
	if b, _ := s.Command("west"); b.Kind != game.KindBlocked || b.Output != "You are still entering window..." {
		t.Errorf("blocked = %+v", b)
	}
	if b, _ := s.Command("save"); b.Kind != game.KindBlocked || b.Output != "You are still entering window..." {
		t.Errorf("blocked save = %+v", b)
	}
	if b, _ := s.Command("west"); b.Pending == nil || b.Pending.Tier != game.TierQuiet || b.Pending.Command != "enter window" {
		t.Errorf("blocked pending = %+v", b.Pending)
	}

	clock.t = clock.t.Add(2 * time.Minute)
	opened, err := s.Opened()
	if err != nil {
		t.Fatal(err)
	}
	if opened.Reveal == nil || opened.Reveal.Tier != game.TierQuiet {
		t.Fatalf("reveal = %+v", opened.Reveal)
	}
	if opened.Reveal.Command != "enter window" || opened.Reveal.Delta != 10 {
		t.Errorf("reveal = %+v", opened.Reveal)
	}

	tr := strings.Join(s.Transcript(), "\n")
	if strings.Contains(tr, "While you were away") || strings.Contains(tr, "Something is unfolding") {
		t.Errorf("quiet transcript carries full-register copy:\n%s", tr)
	}
	if !strings.Contains(tr, "[Time passes...]") {
		t.Errorf("quiet transcript lacks its marker:\n%s", tr)
	}
	// The transcript stays diegetic: no numbers about the wait, ever.
	for _, line := range s.Transcript() {
		if strings.Contains(line, "min") && strings.Contains(line, "~") {
			t.Errorf("transcript shows a duration: %q", line)
		}
	}
}

// TestFullTierWaitUnchanged: at or above 5 minutes the dramatic register
// (marker, recap header) is unchanged, and the tier says so; the withheld and
// blocked lines narrate the walk by direction (#43).
func TestFullTierWaitUnchanged(t *testing.T) {
	s, clock := casualAtWindow(t)
	play(t, s, clock, "enter window")
	resp, err := s.Command("up") // KITCHEN > ATTIC edge: 5 min, on the threshold
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindWithheld || resp.Pending == nil {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Pending.Remaining != 5*time.Minute {
		t.Fatalf("remaining = %v, want 5m (the test relies on a threshold price)", resp.Pending.Remaining)
	}
	if resp.Pending.Tier != game.TierFull || resp.Pending.Command != "up" {
		t.Errorf("pending = %+v, want full/up", resp.Pending)
	}
	if resp.Output != "You begin climbing up..." || resp.Pending.Narration != "climbing up" {
		t.Errorf("withheld = %q / %q", resp.Output, resp.Pending.Narration)
	}
	if b, _ := s.Command("down"); b.Output != "You are still climbing up..." {
		t.Errorf("blocked copy = %q", b.Output)
	}
	clock.t = clock.t.Add(5 * time.Minute)
	opened, _ := s.Opened()
	if opened.Reveal == nil || opened.Reveal.Tier != game.TierFull || opened.Reveal.Status.Room != "Attic" {
		t.Fatalf("reveal = %+v", opened.Reveal)
	}
	tr := strings.Join(s.Transcript(), "\n")
	if !strings.Contains(tr, "[Something is unfolding...]") || !strings.Contains(tr, "[While you were away...]") {
		t.Errorf("full transcript lost its register:\n%s", tr)
	}
}

// TestLegacyPendingIsFullTier: a pending outcome saved before tiers existed
// carries no tier and resumes in the full register (the only one it knew).
func TestLegacyPendingIsFullTier(t *testing.T) {
	s, clock := casualAtWindow(t)
	if _, err := s.Command("enter window"); err != nil {
		t.Fatal(err)
	}
	p, ok, err := s.Store().LoadPlaythrough("zork1")
	if err != nil || !ok {
		t.Fatalf("load: %v %v", ok, err)
	}
	p.Pending.Tier = ""
	if err := s.Store().SavePlaythrough(p); err != nil {
		t.Fatal(err)
	}
	s2, res, err := game.Resume(game.Config{Store: s.Store(), Now: clock.now, Seed: 1}, "zork1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pending == nil || res.Pending.Tier != game.TierFull {
		t.Errorf("resumed pending = %+v, want full tier", res.Pending)
	}
	clock.t = clock.t.Add(2 * time.Minute)
	opened, _ := s2.Opened()
	if opened.Reveal == nil || opened.Reveal.Tier != game.TierFull {
		t.Errorf("reveal = %+v, want full tier", opened.Reveal)
	}
}
