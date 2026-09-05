package actions_test

// Walkthrough fixture replay (spec §6.1, lifted by #32): the committed
// (script, seed) pairs replayed through the real seeded engine and the
// matcher, so every turn classifies exactly as the runtime will. Guards both
// the fixtures (exact final score/moves, zero parser rejections) and the
// classifier (per-kind tallies and combat transitions are pinned goldens
// cross-checked against the prototype's module; the FastVerb counts
// intentionally diverge from the prototype, which stubbed the spec §10
// fast-verb list with 8 of its 22 verbs — every divergent turn moved
// Action→FastVerb, both instant-adjacent buckets on the same turns).

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/actions"
)

func TestWalkthroughFixtures(t *testing.T) {
	cases := []struct {
		game        string
		finalRoom   string
		score       int
		moves       int
		kinds       map[actions.Kind]int
		transitions []string
	}{
		{
			game: "zork1", finalRoom: "Stone Barrow", score: 350, moves: 374,
			kinds: map[actions.Kind]int{
				actions.Aborted: 2, actions.Combat: 11, actions.Melee: 1, actions.FastVerb: 8,
				actions.Movement: 197, actions.Action: 142,
			},
			transitions: []string{
				"combat begins: troll melee line matched",
				"…and ends the same turn: villain died (black fog)",
				"combat begins: thief melee line matched",
				"combat ends: villain died (black fog)",
				"combat begins: thief melee line matched",
				"combat ends: the room changed (matches engine behavior)",
			},
		},
		{
			game: "zork2", finalRoom: "Landing", score: 400, moves: 345,
			kinds: map[actions.Kind]int{
				actions.Aborted: 2, actions.Combat: 6, actions.FastVerb: 10,
				actions.Movement: 134, actions.Action: 163,
			},
			transitions: []string{
				"combat begins: dragon melee line matched",
				"combat ends: glacier scene — dragon dead",
			},
		},
		{
			game: "zork3", finalRoom: "Treasury of Zork", score: 7, moves: 320,
			kinds: map[actions.Kind]int{
				actions.Aborted: 1, actions.Combat: 10, actions.FastVerb: 38, actions.FailedMove: 31,
				actions.Movement: 94, actions.Action: 90,
			},
			transitions: []string{
				"combat begins: figure melee line matched",
				"combat ends: hood removed — the intended resolution",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.game, func(t *testing.T) {
			t.Parallel()
			fx := actions.Fixtures[c.game]
			script, err := os.ReadFile(fx.Path("../.."))
			if err != nil {
				t.Fatal(err)
			}
			cmds := actions.ParseScript(script)
			results, last := replay(t, c.game, fx.Seed, cmds)

			kinds := map[actions.Kind]int{}
			var transitions []string
			for _, res := range results {
				kinds[res.Kind]++
				for _, ev := range res.CombatEvents {
					if strings.HasPrefix(ev, "combat begins") || strings.HasPrefix(ev, "combat ends") || strings.HasPrefix(ev, "…and ends") {
						transitions = append(transitions, ev)
					}
				}
			}
			state := results[len(results)-1].State

			if last.Room != c.finalRoom || last.Score != c.score || last.Moves != c.moves {
				t.Errorf("final room/score/moves = %s %d/%d, want %s %d/%d",
					last.Room, last.Score, last.Moves, c.finalRoom, c.score, c.moves)
			}
			if state.Combat != "" {
				t.Errorf("combat state %q left open at the end", state.Combat)
			}
			if !reflect.DeepEqual(kinds, c.kinds) {
				t.Errorf("kind tallies = %v, want %v", kinds, c.kinds)
			}
			if !reflect.DeepEqual(transitions, c.transitions) {
				t.Errorf("combat transitions = %q, want %q", transitions, c.transitions)
			}
		})
	}
}
