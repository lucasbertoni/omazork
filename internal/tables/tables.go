// Package tables loads the curated data tables bundled with the wrapper: the
// per-game score-event wait tables (data/waits/) and achievement tables
// (data/achievements/). Matching is (delta, room name), implemented once here
// and shared by the mediation layer and the achievement engine.
package tables

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	omazork "github.com/lucasbertoni/omazork"
)

// Event is one known score-awarding moment from a wait table.
type Event struct {
	ID       string
	Delta    int
	DeltaMax int    // when non-zero, matches any delta <= DeltaMax (Zork I case-removal)
	Room     string // display room name; empty = any room
	Wait     time.Duration
}

// Game is one game's loaded wait table.
type Game struct {
	Name        string
	MaxScore    int
	fallbackMin time.Duration
	fallbackMax time.Duration
	events      []Event
	rand        func() float64
}

type waitFile struct {
	Game            string `json:"game"`
	MaxScore        int    `json:"maxScore"`
	FallbackMinutes struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"fallbackMinutes"`
	Events []struct {
		ID          string  `json:"id"`
		Delta       *int    `json:"delta"`
		DeltaMax    *int    `json:"deltaMax"`
		RoomName    *string `json:"roomName"`
		WaitMinutes int     `json:"waitMinutes"`
	} `json:"events"`
}

// Load reads the embedded wait table for zork1/zork2/zork3.
func Load(game string) (*Game, error) {
	raw, err := omazork.Data.ReadFile("data/waits/" + game + ".json")
	if err != nil {
		return nil, fmt.Errorf("tables: no wait table for %q: %w", game, err)
	}
	var f waitFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("tables: %s wait table: %w", game, err)
	}
	g := &Game{
		Name:        f.Game,
		MaxScore:    f.MaxScore,
		fallbackMin: time.Duration(f.FallbackMinutes.Min) * time.Minute,
		fallbackMax: time.Duration(f.FallbackMinutes.Max) * time.Minute,
		rand:        rand.Float64,
	}
	for _, e := range f.Events {
		ev := Event{ID: e.ID, Wait: time.Duration(e.WaitMinutes) * time.Minute}
		if e.RoomName != nil {
			ev.Room = *e.RoomName
		}
		switch {
		case e.Delta != nil:
			ev.Delta = *e.Delta
		case e.DeltaMax != nil:
			ev.DeltaMax = *e.DeltaMax
		default:
			return nil, fmt.Errorf("tables: %s event %q has neither delta nor deltaMax", game, e.ID)
		}
		g.events = append(g.events, ev)
	}
	return g, nil
}

// Match finds the score event for a turn's (delta, room name). Exact-room
// entries win over any-room entries; deltaMax ranges are checked last.
func (g *Game) Match(delta int, room string) (Event, bool) {
	var anyRoom *Event
	var ranged *Event
	for i := range g.events {
		e := &g.events[i]
		if e.DeltaMax != 0 {
			if delta <= e.DeltaMax && (e.Room == "" || e.Room == room) && ranged == nil {
				ranged = e
			}
			continue
		}
		if e.Delta != delta {
			continue
		}
		if e.Room == room {
			return *e, true
		}
		if e.Room == "" && anyRoom == nil {
			anyRoom = e
		}
	}
	if anyRoom != nil {
		return *anyRoom, true
	}
	if ranged != nil {
		return *ranged, true
	}
	return Event{}, false
}

// HasEvent reports whether an event id exists in this table (achievement
// tables reference wait-table events by id).
func (g *Game) HasEvent(id string) bool {
	for _, e := range g.events {
		if e.ID == id {
			return true
		}
	}
	return false
}

// FallbackWait is the random wait for a score change no table row matches.
func (g *Game) FallbackWait() time.Duration {
	span := g.fallbackMax - g.fallbackMin
	return g.fallbackMin + time.Duration(g.rand()*float64(span))
}

// Achievement is one entry from an achievement table.
type Achievement struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Hidden      bool    `json:"hidden"`
	Trigger     Trigger `json:"trigger"`
}

// Trigger is one of three kinds: score-event (references a wait-table event
// id), score-reaches (first time score >= Score), milestone (wrapper-detected
// kind: first-death, grue-death, first-checkpoint, game-won, final-death).
type Trigger struct {
	Type    string `json:"type"`
	EventID string `json:"eventId"`
	Score   int    `json:"score"`
	Kind    string `json:"kind"`
}

// LoadAchievements reads the embedded achievement table for a game.
func LoadAchievements(game string) ([]Achievement, error) {
	raw, err := omazork.Data.ReadFile("data/achievements/" + game + ".json")
	if err != nil {
		return nil, fmt.Errorf("tables: no achievement table for %q: %w", game, err)
	}
	var f struct {
		Achievements []Achievement `json:"achievements"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("tables: %s achievement table: %w", game, err)
	}
	return f.Achievements, nil
}
