package actions_test

// Turn-level classification cases, taken verbatim from the transcripts the
// prototype demo proved out (branch prototype/action-matcher): the troll
// fight including a disarm abort, the thief death-banner teleport, the Zork
// II dragon pursuit and the Wizard false-positive, and the Zork III
// hooded-figure duel.

import (
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/actions"
)

func mustMatcher(t *testing.T, game string) *actions.Matcher {
	t.Helper()
	m, err := actions.NewMatcher(game)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNewMatcherUnknownGame(t *testing.T) {
	if _, err := actions.NewMatcher("zork4"); err == nil {
		t.Error("NewMatcher(zork4) succeeded, want error")
	}
}

func TestClassifyHardClasses(t *testing.T) {
	m := mustMatcher(t, "zork1")
	prev := actions.State{RoomObj: 64, Room: "West of House", Moves: 2}

	// Parser rejection: the moves counter freezes — exact, free no-op.
	res := m.Classify(prev, actions.Turn{
		Input: "frotz the wug", Output: "I don't know the word \"frotz\".\n\n>",
		Room: "West of House", RoomObj: 64, Moves: 2,
	})
	if res.Kind != actions.Aborted || !res.Kind.Instant() {
		t.Errorf("frozen-moves turn = %v, want Aborted", res.Kind)
	}

	// Failed-but-executed action: consumes a move, caught by the phrase list.
	res = m.Classify(actions.State{RoomObj: 85, Room: "Behind House", Moves: 6}, actions.Turn{
		Input: "open window", Output: "Too late for that.\n\n>",
		Room: "Behind House", RoomObj: 85, Moves: 7,
	})
	if res.Kind != actions.FailedAction {
		t.Errorf("failure-phrase turn = %v, want FailedAction", res.Kind)
	}

	// Fast verbs are always instant.
	res = m.Classify(prev, actions.Turn{Input: "look", Output: "West of House\n\n>", Room: "West of House", RoomObj: 64, Moves: 3})
	if res.Kind != actions.FastVerb {
		t.Errorf("look = %v, want FastVerb", res.Kind)
	}

	// A walk that changed the room prices the directed edge, keyed on object
	// numbers.
	res = m.Classify(prev, actions.Turn{
		Input: "north", Output: "North of House\n...\n\n>", Room: "North of House", RoomObj: 137, Moves: 3,
	})
	if res.Kind != actions.Movement || res.From != 64 || res.To != 137 {
		t.Errorf("walk = %v edge %d>%d, want Movement 64>137", res.Kind, res.From, res.To)
	}

	// A walk that did not change the room costs nothing.
	res = m.Classify(prev, actions.Turn{
		Input: "south", Output: "The windows are all boarded.\n\n>", Room: "West of House", RoomObj: 64, Moves: 3,
	})
	if res.Kind != actions.FailedMove {
		t.Errorf("blocked walk = %v, want FailedMove", res.Kind)
	}

	// Everything else is an action for the tables to price.
	res = m.Classify(prev, actions.Turn{
		Input: "xyzzy", Output: "A hollow voice says \"Fool.\"\n\n>", Room: "West of House", RoomObj: 64, Moves: 3,
	})
	if res.Kind != actions.Action || res.Moved {
		t.Errorf("xyzzy = %v moved=%v, want Action, unmoved", res.Kind, res.Moved)
	}

	// A fast verb that changed the room is a handler teleport (the altar
	// prayer), not an instant turn — it must stay priceable (spec §10
	// footnote, §3.4 drift edges).
	res = m.Classify(actions.State{RoomObj: 500, Room: "Altar", Moves: 200}, actions.Turn{
		Input: "pray", Output: "Forest\nThis is a forest, with trees in all directions.\n\n>",
		Room: "Forest", RoomObj: 87, Moves: 201,
	})
	if res.Kind != actions.Action || !res.Moved {
		t.Errorf("altar prayer = %v moved=%v, want Action, moved", res.Kind, res.Moved)
	}
}

func TestClassifyTrollFight(t *testing.T) {
	m := mustMatcher(t, "zork1")
	// Villain-initiated combat has no banner; the first melee line is the
	// marker — here on a blocked walk out of the Troll Room.
	state := actions.State{RoomObj: 127, Room: "The Troll Room", Moves: 18}
	res := m.Classify(state, actions.Turn{
		Input:  "east",
		Output: "The troll fends you off with a menacing gesture.\nThe troll swings; the blade turns on your armor but crashes broadside into your head.\n\n>",
		Room:   "The Troll Room", RoomObj: 127, Moves: 19,
	})
	if res.Kind != actions.Combat || res.State.Combat != "troll" {
		t.Fatalf("engagement turn = %v combat=%q, want Combat/troll", res.Kind, res.State.Combat)
	}

	// The disarm froze the next attack's move counter: Aborted wins even in
	// combat, and combat stays engaged.
	res = m.Classify(res.State, actions.Turn{
		Input:  "kill troll with sword",
		Output: "You don't have the sword.\n\n>",
		Room:   "The Troll Room", RoomObj: 127, Moves: 19,
	})
	if res.Kind != actions.Aborted || res.State.Combat != "troll" {
		t.Fatalf("aborted combat turn = %v combat=%q, want Aborted/troll", res.Kind, res.State.Combat)
	}

	// The black-fog line ends the fight.
	res = m.Classify(res.State, actions.Turn{
		Input:  "kill troll with sword",
		Output: "The troll takes a fatal blow and slumps to the floor dead.\nAlmost as soon as the troll breathes his last breath, a cloud of sinister black fog envelops him, and when the fog lifts, the carcass has disappeared.\n\n>",
		Room:   "The Troll Room", RoomObj: 127, Moves: 20,
	})
	if res.Kind != actions.Combat || res.State.Combat != "" {
		t.Fatalf("killing blow = %v combat=%q, want Combat, fight over", res.Kind, res.State.Combat)
	}
	if len(res.CombatEvents) == 0 || !strings.Contains(res.CombatEvents[0], "black fog") {
		t.Errorf("killing blow events = %q, want black-fog exit", res.CombatEvents)
	}

	// "You can't see any troll here!" quotes the villain without a fight —
	// the noSuch guard blocks re-engagement; the melee verb still makes the
	// turn instant.
	res = m.Classify(res.State, actions.Turn{
		Input:  "kill troll with sword",
		Output: "You can't see any troll here!\n\n>",
		Room:   "The Troll Room", RoomObj: 127, Moves: 21,
	})
	if res.Kind != actions.Melee || res.State.Combat != "" {
		t.Errorf("post-fight attack = %v combat=%q, want Melee, no re-engagement", res.Kind, res.State.Combat)
	}
}

func TestClassifyThiefDeathTeleport(t *testing.T) {
	m := mustMatcher(t, "zork1")
	// The death banner ends combat, and the resurrection teleport is a room
	// change with no walk command — no edge may be priced.
	state := actions.State{RoomObj: 54, Room: "Maze", Moves: 35, Combat: "thief"}
	res := m.Classify(state, actions.Turn{
		Input:  "kill thief with sword",
		Output: "The thief bows formally, raises his stiletto, and with a wry grin, ends the battle and your life.\n \n    ****  You have died  **** \n\nForest\nThis is a forest, with trees in all directions.\n\n>",
		Room:   "Forest", RoomObj: 87, Moves: 36,
	})
	if res.Kind != actions.Combat || res.State.Combat != "" {
		t.Fatalf("death turn = %v combat=%q, want Combat, fight over", res.Kind, res.State.Combat)
	}
	if !res.Moved || res.Parsed.Dir != "" {
		t.Errorf("death teleport: moved=%v dir=%q, want a non-walk room change", res.Moved, res.Parsed.Dir)
	}
}

func TestClassifyDragonPursuit(t *testing.T) {
	m := mustMatcher(t, "zork2")
	state := actions.State{RoomObj: 900, Room: "Dragon Room", Moves: 102}

	// Engagement outranks the melee hard class: the turn is a combat turn.
	res := m.Classify(state, actions.Turn{
		Input:  "attack dragon with sword",
		Output: "Dragon hide is tough as steel, but you have succeeded in annoying him a bit.\n\n>",
		Room:   "Dragon Room", RoomObj: 900, Moves: 103,
	})
	if res.Kind != actions.Combat || res.State.Combat != "dragon" {
		t.Fatalf("dragon engagement = %v combat=%q, want Combat/dragon", res.Kind, res.State.Combat)
	}

	// Unlike Zork I, the dragon follows: combat survives the room change.
	res = m.Classify(res.State, actions.Turn{
		Input:  "south",
		Output: "Cave\nThis is a small cave with exits to the north and south.\nThe dragon follows you, out of mingled curiosity and anger.\n\n>",
		Room:   "Cave", RoomObj: 901, Moves: 104,
	})
	if res.Kind != actions.Combat || res.State.Combat != "dragon" {
		t.Fatalf("pursuit turn = %v combat=%q, want Combat/dragon", res.Kind, res.State.Combat)
	}

	// The dragon wandering off closes the window.
	res = m.Classify(res.State, actions.Turn{
		Input:  "wait",
		Output: "Time passes...\nThe dragon seems to have lost interest in you. He wanders off.\n\n>",
		Room:   "Cave", RoomObj: 901, Moves: 107,
	})
	if res.Kind != actions.Combat || res.State.Combat != "" {
		t.Errorf("dragon leaves = %v combat=%q, want Combat, fight over", res.Kind, res.State.Combat)
	}
}

func TestClassifyWizardIsNotCombat(t *testing.T) {
	m := mustMatcher(t, "zork2")
	// The Wizard's spell harassment is violent-sounding non-combat and must
	// not trip the detector (spec §3.3).
	res := m.Classify(actions.State{RoomObj: 901, Room: "Cave", Moves: 105}, actions.Turn{
		Input:  "wait",
		Output: "Time passes...\nThe Wizard draws forth his wand and waves it in your direction. It begins to glow with a faint blue glow. The Wizard, in a deep and resonant voice, speaks the word \"Fierce!\" He then vanishes, cackling gleefully.\n\n>",
		Room:   "Cave", RoomObj: 901, Moves: 108,
	})
	if res.State.Combat != "" || len(res.CombatEvents) != 0 {
		t.Errorf("wizard apparition: combat=%q events=%q, want none", res.State.Combat, res.CombatEvents)
	}
	if res.Kind != actions.FastVerb {
		t.Errorf("wizard apparition = %v, want FastVerb (wait)", res.Kind)
	}
}

func TestClassifyHoodedFigureDuel(t *testing.T) {
	m := mustMatcher(t, "zork3")
	// The figure's appearance opens combat even on a walk turn.
	state := actions.State{RoomObj: 64, Room: "Foggy Room", Moves: 5}
	res := m.Classify(state, actions.Turn{
		Input:  "west",
		Output: "Land of Shadow\nThrough the shadows, a cloaked and hooded figure appears before you, blocking the eastern exit from the room and carrying a brightly glowing sword.\n\n>",
		Room:   "Land of Shadow", RoomObj: 30, Moves: 6,
	})
	if res.Kind != actions.Combat || res.State.Combat != "figure" {
		t.Fatalf("figure appears = %v combat=%q, want Combat/figure", res.Kind, res.State.Combat)
	}

	// Walking away lapses the duel (the figure does not pursue).
	res = m.Classify(res.State, actions.Turn{
		Input:  "west",
		Output: "Land of Shadow\nYou are in a dark and shadowy land.\n\n>",
		Room:   "Land of Shadow", RoomObj: 134, Moves: 7,
	})
	if res.Kind != actions.Combat || res.State.Combat != "" {
		t.Fatalf("walk away = %v combat=%q, want Combat, duel lapsed", res.Kind, res.State.Combat)
	}

	// "take hood" both resolves the duel and ends combat in one turn.
	state = actions.State{RoomObj: 134, Room: "Land of Shadow", Moves: 16, Combat: "figure"}
	res = m.Classify(state, actions.Turn{
		Input:  "take hood",
		Output: "You slowly remove the hood from your badly wounded opponent and recoil in horror at the sight of your own face, weary and wounded.\n\n>",
		Room:   "Land of Shadow", RoomObj: 134, Moves: 17,
	})
	if res.Kind != actions.Combat || res.State.Combat != "" {
		t.Errorf("take hood = %v combat=%q, want Combat, duel resolved", res.Kind, res.State.Combat)
	}
	if len(res.CombatEvents) == 0 || !strings.Contains(res.CombatEvents[0], "hood removed") {
		t.Errorf("take hood events = %q, want the hood-removed exit", res.CombatEvents)
	}
}

func TestClassifyCerberusDeath(t *testing.T) {
	m := mustMatcher(t, "zork2")
	// The unleashed dog's attack response is a coin flip; the losing branch
	// (captured verbatim, seed 1) is the death banner plus the resurrection
	// teleport — the fight ends, and no edge may be priced.
	state := actions.State{RoomObj: 162, Room: "Cerberus Room", Moves: 344, Combat: "dog"}
	res := m.Classify(state, actions.Turn{
		Input:  "attack dog",
		Output: "The dog-thing snaps at you viciously, and succeeds. Your head, it seems, is only a small mouthful for the poor animal, who is just as hungry afterward.\n \n    ****  You have died  **** \n\nNow, let's take a look here... Well, you probably deserve another chance. I can't quite fix you up completely, but you can't have everything.\n\nRoom of Red Mist\nYou are inside a huge crystalline sphere filled with thin red mist. The mist becomes blue to the west.\nYou strain to look out through the mist... \nYou see only darkness.\n\n>",
		Room:   "Room of Red Mist", RoomObj: 224, Moves: 345,
	})
	if res.Kind != actions.Combat || res.State.Combat != "" {
		t.Fatalf("death turn = %v combat=%q, want Combat, fight over", res.Kind, res.State.Combat)
	}
	if !res.Moved || res.Parsed.Dir != "" {
		t.Errorf("death teleport: moved=%v dir=%q, want a non-walk room change", res.Moved, res.Parsed.Dir)
	}
	if len(res.CombatEvents) == 0 || res.CombatEvents[0] != "combat ends: the player died" {
		t.Errorf("events = %q, want the death exit", res.CombatEvents)
	}

	// Once collared, the dog is harmless: killing it is a one-shot, not a
	// window — no engagement on the post-pacification death line.
	res = m.Classify(actions.State{RoomObj: 162, Room: "Cerberus Room", Moves: 350}, actions.Turn{
		Input:  "attack dog",
		Output: "With a quiet bark of disappointment, the creature expires.\nIts six eyes look at you reproachfully. As it dies, it collapses\ninto a small pile of dust which blows away into nothing.\n\n>",
		Room:   "Cerberus Room", RoomObj: 162, Moves: 351,
	})
	if res.Kind != actions.Melee || res.State.Combat != "" || len(res.CombatEvents) != 0 {
		t.Errorf("collared dog killed = %v combat=%q events=%q, want Melee, no window", res.Kind, res.State.Combat, res.CombatEvents)
	}
}
