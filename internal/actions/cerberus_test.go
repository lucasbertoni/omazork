package actions_test

// Zork II's second §3.3 state window (#41): the unleashed three-headed dog.
// The walkthrough fixture collars the dog on arrival and never triggers
// its hostility, so this replays a captured (script, seed) transcript that
// does — testdata/zork2-cerberus.txt, the walkthrough prefix plus a
// dog-window tail — and pins the tail's classifications turn by turn.

import (
	"os"
	"reflect"
	"testing"

	"github.com/lucasbertoni/omazork/internal/actions"
)

// tail is the dog-window sequence appended after the walkthrough prefix
// descends into the Cerberus Room, with the classification each turn must
// produce.
var tail = []struct {
	input  string
	kind   actions.Kind
	combat string
	events []string
}{
	{"east", actions.Combat, "dog", []string{"combat begins: dog melee line matched"}},
	{"up", actions.Combat, "", []string{"combat ends: the room changed (matches engine behavior)"}},
	{"down", actions.Movement, "", nil},
	{"examine dog", actions.Combat, "dog", []string{"combat begins: dog melee line matched"}},
	{"attack dog", actions.Combat, "dog", []string{"still fighting the dog (melee line each turn)"}},
	{"put collar on dog", actions.Combat, "", []string{"combat ends: dog pacified by the collar"}},
	{"east", actions.Movement, "", nil},
}

func TestCerberusWindowTranscript(t *testing.T) {
	const game, seed = "zork2", 1
	script, err := os.ReadFile("testdata/zork2-cerberus.txt")
	if err != nil {
		t.Fatal(err)
	}
	cmds := actions.ParseScript(script)

	// The prefix is the walkthrough fixture verbatim; drift there would
	// silently change the seed-tuned tail.
	walkthrough, err := os.ReadFile(actions.Fixtures[game].Path("../.."))
	if err != nil {
		t.Fatal(err)
	}
	prefix := actions.ParseScript(walkthrough)[:len(cmds)-len(tail)]
	if !reflect.DeepEqual(cmds[:len(prefix)], prefix) {
		t.Fatal("cerberus script prefix drifted from the zork2 walkthrough fixture")
	}

	results, last := replay(t, game, seed, cmds)
	if last.Room != "Crypt Anteroom" || last.Score != 398 || last.Moves != 345 {
		t.Errorf("final room/score/moves = %s %d/%d, want Crypt Anteroom 398/345", last.Room, last.Score, last.Moves)
	}

	// Every turn before the tail is out of combat with the dog (the prefix
	// only ever fights the dragon).
	for i, r := range results[:len(results)-len(tail)] {
		if r.State.Combat == "dog" {
			t.Errorf("command %d %q engaged the dog before the tail", i+1, cmds[i])
		}
	}
	for i, w := range tail {
		r := results[len(results)-len(tail)+i]
		if got := cmds[len(cmds)-len(tail)+i]; got != w.input {
			t.Fatalf("tail %d: script drifted, got %q want %q", i, got, w.input)
		}
		if r.Kind != w.kind || r.State.Combat != w.combat || !reflect.DeepEqual(r.CombatEvents, w.events) {
			t.Errorf("%q = %v combat=%q events=%q, want %v/%q/%q",
				w.input, r.Kind, r.State.Combat, r.CombatEvents, w.kind, w.combat, w.events)
		}
	}
}
