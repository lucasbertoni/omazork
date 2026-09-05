package actions_test

import (
	"regexp"
	"testing"

	"github.com/lucasbertoni/omazork/internal/actions"
	"github.com/lucasbertoni/omazork/internal/engine"
)

// parserRejection would mean a script drifted from its seed; the committed
// scripts are tuned so none occur.
var parserRejection = regexp.MustCompile(`I don't know the word|You can't go that way|Huh\?|What\?|beg your pardon|You used the word`)

// replay runs a script through the real seeded engine and the matcher,
// returning one classified Result per command and the engine's final turn.
// It fails the test on a parser rejection or on the story halting before
// the last command.
func replay(t *testing.T, game string, seed uint64, cmds []string) ([]actions.Result, engine.Turn) {
	t.Helper()
	e, err := engine.New(game, engine.WithSeed(seed))
	if err != nil {
		t.Fatal(err)
	}
	m := mustMatcher(t, game)
	banner, err := e.Start()
	if err != nil {
		t.Fatal(err)
	}
	state := actions.StartState(actions.Turn{Room: banner.Room, RoomObj: banner.RoomObj, Moves: banner.Moves})

	results := make([]actions.Result, 0, len(cmds))
	var last engine.Turn
	for i, cmd := range cmds {
		last, err = e.Run(cmd)
		if err != nil {
			t.Fatalf("command %d %q: %v", i+1, cmd, err)
		}
		if parserRejection.MatchString(last.Output) {
			t.Errorf("parser rejection at command %d %q:\n%.160s", i+1, cmd, last.Output)
		}
		res := m.Classify(state, actions.Turn{
			Input: cmd, Output: last.Output, Room: last.Room, RoomObj: last.RoomObj, Moves: last.Moves,
		})
		state = res.State
		results = append(results, res)
		if last.Halted && i != len(cmds)-1 {
			t.Fatalf("story halted early at command %d %q", i+1, cmd)
		}
	}
	return results, last
}
