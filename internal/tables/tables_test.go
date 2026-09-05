package tables_test

import (
	"encoding/json"
	"testing"

	omazork "github.com/lucasbertoni/omazork"
	"github.com/lucasbertoni/omazork/internal/tables"
)

func TestKnownScoreEvent(t *testing.T) {
	g, err := tables.LoadEvents("zork1")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "zork1" || g.MaxScore != 350 {
		t.Errorf("table identity = %q/%d, want zork1/350", g.Name, g.MaxScore)
	}
	ev, ok := g.Match(10, "Kitchen")
	if !ok {
		t.Fatal("no match for +10 in Kitchen")
	}
	if ev.ID != "visit-kitchen" {
		t.Errorf("event = %q, want visit-kitchen", ev.ID)
	}
}

func TestExactRoomBeatsAnyRoom(t *testing.T) {
	g, _ := tables.LoadEvents("zork1")
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

func TestDeathIsAnEvent(t *testing.T) {
	g, _ := tables.LoadEvents("zork1")
	ev, ok := g.Match(-10, "Cellar")
	if !ok || ev.ID != "death" {
		t.Errorf("death match = %+v ok=%v, want death", ev, ok)
	}
}

func TestCaseRemovalMatchesAnyNegativeDelta(t *testing.T) {
	g, _ := tables.LoadEvents("zork1")
	ev, ok := g.Match(-5, "Living Room")
	if !ok || ev.ID != "case-removal" {
		t.Errorf("got %+v ok=%v, want case-removal", ev, ok)
	}
}

func TestUnmatchedDeltaIsNoEvent(t *testing.T) {
	g, _ := tables.LoadEvents("zork1")
	if ev, ok := g.Match(3, "West of House"); ok {
		t.Fatalf("unexpected match %+v for +3", ev)
	}
}

func TestZork3MatchesOnRoomName(t *testing.T) {
	g, err := tables.LoadEvents("zork3")
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := g.Match(1, "Land of Shadow")
	if !ok || ev.Room != "Land of Shadow" {
		t.Errorf("Land of Shadow +1 = %+v ok=%v, want a shadow event", ev, ok)
	}
}

// TestEventTablesArePureVocabularies pins the §9 file shape: schemaVersion
// exact-match, the identity fields only, no timing fields, and the row counts
// the old wait tables carried.
func TestEventTablesArePureVocabularies(t *testing.T) {
	wantRows := map[string]int{"zork1": 34, "zork2": 27, "zork3": 7}
	for game, n := range wantRows {
		raw, err := omazork.Data.ReadFile("data/events/" + game + ".json")
		if err != nil {
			t.Fatalf("%s: %v", game, err)
		}
		var f struct {
			SchemaVersion int                          `json:"schemaVersion"`
			Events        []map[string]json.RawMessage `json:"events"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("%s: %v", game, err)
		}
		if f.SchemaVersion != tables.SchemaVersion {
			t.Errorf("%s: schemaVersion %d, want %d", game, f.SchemaVersion, tables.SchemaVersion)
		}
		if len(f.Events) != n {
			t.Errorf("%s: %d rows, want %d", game, len(f.Events), n)
		}
		var top map[string]json.RawMessage
		_ = json.Unmarshal(raw, &top)
		if _, ok := top["fallbackMinutes"]; ok {
			t.Errorf("%s: fallbackMinutes survives", game)
		}
		for _, row := range f.Events {
			if _, ok := row["waitMinutes"]; ok {
				t.Errorf("%s: row %s carries waitMinutes", game, row["id"])
			}
			for k := range row {
				switch k {
				case "id", "delta", "deltaMax", "room", "roomName", "note":
				default:
					t.Errorf("%s: row %s has unexpected field %q", game, row["id"], k)
				}
			}
		}
		g, err := tables.LoadEvents(game)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range f.Events {
			var id string
			_ = json.Unmarshal(row["id"], &id)
			if !g.HasEvent(id) {
				t.Errorf("%s: loader dropped row %q", game, id)
			}
		}
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
		g, err := tables.LoadEvents(game)
		if err != nil {
			t.Fatal(err)
		}
		for _, ach := range a {
			switch ach.Trigger.Type {
			case "score-event":
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
