package tables_test

import (
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/tables"
)

func TestKnownScoreEventWait(t *testing.T) {
	g, err := tables.Load("zork1")
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := g.Match(10, "Kitchen")
	if !ok {
		t.Fatal("no match for +10 in Kitchen")
	}
	if ev.ID != "visit-kitchen" {
		t.Errorf("event = %q, want visit-kitchen", ev.ID)
	}
	if ev.Wait != 5*time.Minute {
		t.Errorf("wait = %v, want 5m", ev.Wait)
	}
}

func TestExactRoomBeatsAnyRoom(t *testing.T) {
	g, _ := tables.Load("zork1")
	// +4 in the Gallery is take-painting (exact room), not take-sceptre (room: null).
	ev, ok := g.Match(4, "Gallery")
	if !ok || ev.ID != "take-painting" {
		t.Errorf("got %+v, want take-painting", ev)
	}
	// +4 anywhere else is the sceptre's null-room entry.
	ev, ok = g.Match(4, "Frigid River")
	if !ok || ev.ID != "take-sceptre" {
		t.Errorf("got %+v, want take-sceptre", ev)
	}
}

func TestDeathNeverWaits(t *testing.T) {
	g, _ := tables.Load("zork1")
	ev, ok := g.Match(-10, "Cellar")
	if !ok || ev.ID != "death" || ev.Wait != 0 {
		t.Errorf("death match = %+v ok=%v, want death with 0 wait", ev, ok)
	}
}

func TestCaseRemovalMatchesAnyNegativeDelta(t *testing.T) {
	g, _ := tables.Load("zork1")
	ev, ok := g.Match(-5, "Living Room")
	if !ok || ev.ID != "case-removal" || ev.Wait != 0 {
		t.Errorf("got %+v ok=%v, want case-removal 0 wait", ev, ok)
	}
}

func TestUnmatchedDeltaFallsBack(t *testing.T) {
	g, _ := tables.Load("zork1")
	if _, ok := g.Match(3, "West of House"); ok {
		t.Fatal("unexpected match for +3")
	}
	for i := 0; i < 50; i++ {
		w := g.FallbackWait()
		if w < 15*time.Minute || w > 45*time.Minute {
			t.Fatalf("fallback wait %v outside 15–45m", w)
		}
	}
}

func TestZork3MatchesOnRoomName(t *testing.T) {
	g, err := tables.Load("zork3")
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := g.Match(1, "Land of Shadow")
	if !ok || ev.Wait != 0 {
		t.Errorf("Land of Shadow +1 = %+v ok=%v, want a 0-wait combat event", ev, ok)
	}
}

func TestAchievementTablesLoad(t *testing.T) {
	for _, game := range []string{"zork1", "zork2", "zork3"} {
		a, err := tables.LoadAchievements(game)
		if err != nil {
			t.Fatalf("%s: %v", game, err)
		}
		if len(a) < 10 {
			t.Errorf("%s: only %d achievements", game, len(a))
		}
		for _, ach := range a {
			switch ach.Trigger.Type {
			case "score-event":
				if _, err := tables.Load(game); err != nil {
					t.Fatal(err)
				}
				g, _ := tables.Load(game)
				if !g.HasEvent(ach.Trigger.EventID) {
					t.Errorf("%s/%s references unknown event %q", game, ach.ID, ach.Trigger.EventID)
				}
			case "score-reaches":
				if ach.Trigger.Score <= 0 {
					t.Errorf("%s/%s: score-reaches with score %d", game, ach.ID, ach.Trigger.Score)
				}
			case "milestone":
				if ach.Trigger.Kind == "" {
					t.Errorf("%s/%s: milestone without kind", game, ach.ID)
				}
			default:
				t.Errorf("%s/%s: unknown trigger type %q", game, ach.ID, ach.Trigger.Type)
			}
		}
	}
}
