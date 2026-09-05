// Package tables loads the curated data tables bundled with the wrapper: the
// per-game score-event tables (data/events/) and achievement tables
// (data/achievements/). A score-event table is the game's vocabulary of
// point-awarding moments — it carries identity, never timing (action waits
// are priced elsewhere; see docs/action-waits.md §9). Matching is
// (delta, room name), implemented once here for the achievement engine.
package tables

import (
	"encoding/json"
	"fmt"

	omazork "github.com/lucasbertoni/omazork"
)

// SchemaVersion is stamped on every event table; loading asserts an exact
// match so a stale or future file fails loudly instead of matching wrong.
const SchemaVersion = 1

// Event is one known score-awarding moment from an event table.
type Event struct {
	ID       string
	Delta    int
	DeltaMax int    // when non-zero, matches any delta <= DeltaMax (Zork I case-removal)
	Room     string // display room name; empty = any room
}

// EventTable is one game's loaded score-event table.
type EventTable struct {
	Name     string
	MaxScore int
	events   []Event
}

type eventFile struct {
	SchemaVersion int    `json:"schemaVersion"`
	Game          string `json:"game"`
	MaxScore      int    `json:"maxScore"`
	Events        []struct {
		ID       string  `json:"id"`
		Delta    *int    `json:"delta"`
		DeltaMax *int    `json:"deltaMax"`
		RoomName *string `json:"roomName"`
	} `json:"events"`
}

// LoadEvents reads the embedded score-event table for zork1/zork2/zork3.
func LoadEvents(game string) (*EventTable, error) {
	raw, err := omazork.Data.ReadFile("data/events/" + game + ".json")
	if err != nil {
		return nil, fmt.Errorf("tables: no event table for %q: %w", game, err)
	}
	var f eventFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("tables: %s event table: %w", game, err)
	}
	if f.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("tables: %s event table: schemaVersion %d, want exactly %d", game, f.SchemaVersion, SchemaVersion)
	}
	g := &EventTable{Name: f.Game, MaxScore: f.MaxScore}
	for _, e := range f.Events {
		ev := Event{ID: e.ID}
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
func (g *EventTable) Match(delta int, room string) (Event, bool) {
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
// tables reference event-table rows by id).
func (g *EventTable) HasEvent(id string) bool {
	for _, e := range g.events {
		if e.ID == id {
			return true
		}
	}
	return false
}

// Achievement is one entry from an achievement table.
type Achievement struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Hidden      bool    `json:"hidden"`
	Trigger     Trigger `json:"trigger"`
}

// Trigger is one of three kinds: score-event (references an event-table row
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
