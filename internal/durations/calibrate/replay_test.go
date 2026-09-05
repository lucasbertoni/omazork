package calibrate

import (
	"os"
	"testing"

	"github.com/lucasbertoni/omazork/internal/actions"
	"github.com/lucasbertoni/omazork/internal/durations"
)

// TestReplayCommittedFixtures runs every committed walkthrough against the
// committed layered tables: the replay must run to the true ending, price
// every turn, and touch a real share of the table. Gate verdicts are not
// asserted here — that is actiongen's job, and thresholds are the curator's.
func TestReplayCommittedFixtures(t *testing.T) {
	fsys := os.DirFS("../../..")
	for game, fx := range actions.Fixtures {
		t.Run(game, func(t *testing.T) {
			l, err := durations.LoadFS(fsys, game)
			if err != nil {
				t.Fatal(err)
			}
			script, err := os.ReadFile(fx.Path("../../.."))
			if err != nil {
				t.Fatal(err)
			}
			rep, hit, err := replay(l, fx, script)
			if err != nil {
				t.Fatal(err)
			}
			if rep.Turns != len(actions.ParseScript(script)) {
				t.Fatalf("priced %d turns of %d commands", rep.Turns, len(actions.ParseScript(script)))
			}
			if rep.CumulativeSeconds == 0 || rep.MedianMovementSeconds == 0 {
				t.Fatalf("empty profile: %+v", rep)
			}
			if rep.ByKind["movement"].Turns == 0 || rep.BySlot["edge"].Turns == 0 {
				t.Fatalf("no movement priced through edges: kinds %v slots %v", rep.ByKind, rep.BySlot)
			}
			if rep.ByKind["combat"].Seconds != 0 {
				t.Fatalf("combat turns cost %ds, want 0", rep.ByKind["combat"].Seconds)
			}
			if len(hit) < 50 {
				t.Fatalf("walkthrough exercised only %d rows", len(hit))
			}
			if len(rep.LongestTurns) != longestTurns || len(rep.Waits) == 0 {
				t.Fatalf("report lists %d longest turns, %d bins", len(rep.LongestTurns), len(rep.Waits))
			}
		})
	}
}
