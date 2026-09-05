package game_test

// Action waits in the mediation layer (#37): Casual-mode turns price through
// the §2 lookup precedence — the same matcher and Resolve the calibration
// replay uses — instead of score deltas.

import (
	"os"
	"testing"
	"time"

	"github.com/lucasbertoni/omazork/internal/actions"
	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/engine"
	"github.com/lucasbertoni/omazork/internal/game"
	"github.com/lucasbertoni/omazork/internal/session"
)

// oracle prices a walkthrough the way the calibration replay does: real
// seeded engine, matcher, layered tables. It is the reference the live
// session must match turn for turn.
func oracle(t *testing.T, gameName string, seed uint64, cmds []string) []int {
	t.Helper()
	e, err := engine.New(gameName, engine.WithSeed(seed))
	if err != nil {
		t.Fatal(err)
	}
	m, err := actions.NewMatcher(gameName)
	if err != nil {
		t.Fatal(err)
	}
	l, err := durations.Load(gameName)
	if err != nil {
		t.Fatal(err)
	}
	banner, err := e.Start()
	if err != nil {
		t.Fatal(err)
	}
	state := actions.StartState(actions.Turn{Room: banner.Room, RoomObj: banner.RoomObj, Moves: banner.Moves})
	minutes := make([]int, 0, len(cmds))
	for _, cmd := range cmds {
		turn, err := e.Run(cmd)
		if err != nil {
			t.Fatal(err)
		}
		res := m.Classify(state, actions.Turn{Input: cmd, Output: turn.Output, Room: turn.Room, RoomObj: turn.RoomObj, Moves: turn.Moves})
		state = res.State
		r := l.Resolve(durations.QueryFor(res))
		if turn.Halted {
			r.Row.Minutes = 0 // halts always reveal immediately
		}
		minutes = append(minutes, r.Row.Minutes)
	}
	return minutes
}

// TestCasualReplayMatchesCalibration drives a live Casual session through
// each committed walkthrough fixture and checks that every turn waits exactly
// what the calibration replay priced it at (acceptance: verified turn-by-turn
// against the walkthrough fixtures).
func TestCasualReplayMatchesCalibration(t *testing.T) {
	for gameName, fx := range actions.Fixtures {
		t.Run(gameName, func(t *testing.T) {
			t.Parallel()
			script, err := os.ReadFile(fx.Path("../.."))
			if err != nil {
				t.Fatal(err)
			}
			cmds := actions.ParseScript(script)
			want := oracle(t, gameName, fx.Seed, cmds)

			cfg, clock := newCfg(t)
			cfg.Seed = fx.Seed
			s, _, err := game.New(cfg, gameName, session.Casual)
			if err != nil {
				t.Fatal(err)
			}
			withheld, instant := 0, 0
			for i, cmd := range cmds {
				resp, err := s.Command(cmd)
				if err != nil {
					t.Fatalf("turn %d %q: %v", i+1, cmd, err)
				}
				got := 0
				switch resp.Kind {
				case game.KindWithheld:
					got = int(resp.Pending.Remaining / time.Minute)
					withheld++
					// Mature it so the next command reveals and runs.
					clock.t = clock.t.Add(resp.Pending.Remaining)
				case game.KindOutput, game.KindEnded:
					instant++
				default:
					t.Fatalf("turn %d %q: kind %q", i+1, cmd, resp.Kind)
				}
				if got != want[i] {
					t.Errorf("turn %d %q: waited %d min, calibration replay priced %d", i+1, cmd, got, want[i])
				}
			}
			if withheld == 0 || instant == 0 {
				t.Errorf("withheld %d / instant %d turns — expected both", withheld, instant)
			}
		})
	}
}

// TestCasualHardClassesAndEdges is the demo path: LOOK is instant, a walk that
// changes the room waits a minutes-scale edge price, a failed walk is free.
func TestCasualHardClassesAndEdges(t *testing.T) {
	cfg, clock := newCfg(t)
	s, _, err := game.New(cfg, "zork1", session.Casual)
	if err != nil {
		t.Fatal(err)
	}
	look, _ := s.Command("look")
	if look.Kind != game.KindOutput {
		t.Errorf("look: kind %q, want instant output", look.Kind)
	}
	// "up" fails at West of House: the room doesn't change, so no wait.
	failed, _ := s.Command("up")
	if failed.Kind != game.KindOutput {
		t.Errorf("failed walk: kind %q, want instant output", failed.Kind)
	}
	walk, _ := s.Command("south")
	if walk.Kind != game.KindWithheld {
		t.Fatalf("walk: kind %q, want withheld", walk.Kind)
	}
	l, err := durations.Load("zork1")
	if err != nil {
		t.Fatal(err)
	}
	row, ok := l.Edge("WEST-OF-HOUSE", "SOUTH-OF-HOUSE")
	if !ok {
		t.Fatal("no edge row WEST-OF-HOUSE>SOUTH-OF-HOUSE")
	}
	if walk.Pending.Remaining != time.Duration(row.Minutes)*time.Minute {
		t.Errorf("walk wait = %v, want the edge row's %d min", walk.Pending.Remaining, row.Minutes)
	}
	if walk.Status.Room != "West of House" {
		t.Errorf("status leaked the destination: %+v", walk.Status)
	}
	clock.t = clock.t.Add(walk.Pending.Remaining)
	opened, _ := s.Opened()
	if opened.Reveal == nil || opened.Reveal.Status.Room != "South of House" || opened.Reveal.Delta != 0 {
		t.Errorf("reveal = %+v", opened.Reveal)
	}
}

// TestCombatStateSurvivesRestart: the matcher's combat window is part of the
// playthrough, so a wrapper respawn mid-fight keeps pricing turns instant.
func TestCombatStateSurvivesRestart(t *testing.T) {
	cfg, clock := newCfg(t)
	cfg.Seed = 4 // the troll parries the first blow, so the window stays open
	s, _, err := game.New(cfg, "zork1", session.Casual)
	if err != nil {
		t.Fatal(err)
	}
	play(t, s, clock, "n", "n", "u", "get egg", "d", "s", "e", "open window", "w", "w",
		"take sword", "take lamp", "move rug", "open trapdoor", "d", "turn on lamp", "n")
	first, _ := s.Command("kill troll with sword")
	if first.Kind != game.KindOutput {
		t.Fatalf("melee turn: kind %q, want instant", first.Kind)
	}
	if s.Combat() != "troll" {
		t.Fatalf("combat state after the first blow = %q, want troll", s.Combat())
	}
	s2, _, err := game.Resume(game.Config{Store: storeOf(t, s), Now: clock.now, Seed: cfg.Seed}, "zork1")
	if err != nil {
		t.Fatal(err)
	}
	if s2.Combat() != "troll" {
		t.Fatalf("combat state after resume = %q, want troll", s2.Combat())
	}
	// A non-melee, non-fast action mid-fight is still instant: in combat.
	resp, _ := s2.Command("take sword")
	if resp.Kind != game.KindOutput {
		t.Errorf("in-combat turn after resume: kind %q, want instant", resp.Kind)
	}
}

// TestPreflightRejectsSchemaSkew: the wrapper refuses to start when a
// duration file's schemaVersion doesn't match.
func TestPreflightRejectsSchemaSkew(t *testing.T) {
	if err := game.Preflight(); err != nil {
		t.Fatalf("Preflight on the committed data: %v", err)
	}
	for _, file := range []string{"zork1.json", "zork2.overlay.json", "verbs.json"} {
		if err := game.PreflightFS(skewedData(t, file)); err == nil {
			t.Errorf("PreflightFS accepted a schemaVersion mismatch in %s", file)
		}
	}
}

// TestScoreEraPendingStillMatures: a save written before action waits — a
// pending outcome keyed on a score delta, no combat field — resumes, matures,
// and reveals without migration; session.Pending is self-contained.
func TestScoreEraPendingStillMatures(t *testing.T) {
	s, clock := casualAtWindow(t)
	resp, err := s.Command("enter window")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Kind != game.KindWithheld {
		t.Fatalf("kind %q, want withheld", resp.Kind)
	}
	// Rewrite the stored playthrough the way the score-era wrapper would have
	// left it: a 15–45 min random wait, and none of the newer fields.
	p, ok, err := s.Store().LoadPlaythrough("zork1")
	if err != nil || !ok {
		t.Fatalf("load playthrough: %v %v", ok, err)
	}
	p.Pending.MaturesAt = clock.t.Add(27 * time.Minute)
	p.Combat = ""
	if err := s.Store().SavePlaythrough(p); err != nil {
		t.Fatal(err)
	}

	s2, res, err := game.Resume(game.Config{Store: s.Store(), Now: clock.now, Seed: 1}, "zork1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pending == nil || res.Pending.Remaining != 27*time.Minute {
		t.Fatalf("resumed pending = %+v, want 27m", res.Pending)
	}
	if b, _ := s2.Command("west"); b.Kind != game.KindBlocked {
		t.Errorf("before maturity: kind %q, want blocked", b.Kind)
	}
	clock.t = clock.t.Add(27 * time.Minute)
	opened, err := s2.Opened()
	if err != nil {
		t.Fatal(err)
	}
	if opened.Reveal == nil || opened.Reveal.Delta != 10 || opened.Reveal.Status.Room != "Kitchen" {
		t.Errorf("reveal = %+v", opened.Reveal)
	}
}

// TestClassicNeverClassifies: Classic playthroughs are byte-for-byte what they
// were — no combat field ever lands in the file, whatever the player does.
func TestClassicNeverClassifies(t *testing.T) {
	cfg, clock := newCfg(t)
	cfg.Seed = 4
	s, _, err := game.New(cfg, "zork1", session.Classic)
	if err != nil {
		t.Fatal(err)
	}
	play(t, s, clock, "n", "n", "u", "get egg", "d", "s", "e", "open window", "w", "w",
		"take sword", "take lamp", "move rug", "open trapdoor", "d", "turn on lamp", "n", "kill troll with sword")
	p, ok, err := cfg.Store.LoadPlaythrough("zork1")
	if err != nil || !ok {
		t.Fatalf("load playthrough: %v %v", ok, err)
	}
	if p.Combat != "" {
		t.Errorf("classic playthrough file carries combat %q", p.Combat)
	}
	if s.Combat() != "" {
		t.Errorf("classic session tracked combat %q", s.Combat())
	}
}
