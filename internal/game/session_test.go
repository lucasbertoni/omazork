package game_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/game"
	"github.com/lucasbertoni/omazork/internal/session"
)

// fakeClock advances only when told to.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newCfg(t *testing.T) (game.Config, *fakeClock) {
	t.Helper()
	clock := &fakeClock{t: time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)}
	return game.Config{
		Store: session.NewStore(t.TempDir()),
		Now:   clock.now,
		Seed:  1,
	}, clock
}

func TestNewGameClassicPassthrough(t *testing.T) {
	cfg, _ := newCfg(t)
	s, resp, err := game.New(cfg, "zork1", session.Classic)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Output, "West of House") {
		t.Errorf("opening missing:\n%s", resp.Output)
	}
	for _, cmd := range []string{"south", "east", "open window"} {
		if _, err := s.Command(cmd); err != nil {
			t.Fatal(err)
		}
	}
	resp, err = s.Command("enter window")
	if err != nil {
		t.Fatal(err)
	}
	// Classic: scoring outcome passes straight through, status visible.
	if resp.Kind != game.KindOutput {
		t.Fatalf("kind = %q, want output", resp.Kind)
	}
	if !strings.Contains(resp.Output, "kitchen") && !strings.Contains(resp.Output, "Kitchen") {
		t.Errorf("kitchen output missing:\n%s", resp.Output)
	}
	if resp.Status == nil || resp.Status.Score != 10 || resp.Status.Room != "Kitchen" {
		t.Errorf("status = %+v, want Kitchen/10", resp.Status)
	}
}

func TestAutosaveAndResume(t *testing.T) {
	cfg, _ := newCfg(t)
	s, _, err := game.New(cfg, "zork1", session.Classic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Command("open mailbox"); err != nil {
		t.Fatal(err)
	}
	// A new session resumes exactly where play stopped (simulates SIGTERM + respawn).
	s2, resp, err := game.Resume(cfg, "zork1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status == nil || resp.Status.Moves != 1 {
		t.Errorf("resume status = %+v, want moves 1", resp.Status)
	}
	r2, err := s2.Command("read leaflet")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r2.Output, "ZORK") {
		t.Errorf("leaflet after resume:\n%s", r2.Output)
	}
}

func TestNewGameRefusesOverwriteWithoutReplace(t *testing.T) {
	cfg, _ := newCfg(t)
	if _, _, err := game.New(cfg, "zork1", session.Classic); err != nil {
		t.Fatal(err)
	}
	_, _, err := game.New(cfg, "zork1", session.Casual)
	if err != game.ErrPlaythroughExists {
		t.Fatalf("err = %v, want ErrPlaythroughExists", err)
	}
	// Replace deletes and starts fresh with the new mode.
	s, _, err := game.Replace(cfg, "zork1", session.Casual)
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode() != session.Casual {
		t.Errorf("mode = %v, want casual", s.Mode())
	}
}

// storeOf reopens the same store a session was built on (test helper).
func storeOf(t *testing.T, s *game.Session) *session.Store { return s.Store() }
