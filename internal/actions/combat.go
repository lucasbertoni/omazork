package actions

import "regexp"

// Combat marker sets, straight from the combat inventory (#19) and proven in
// the matcher prototype (#22). Three games, three architectures (spec §3.3):
// Zork I a real melee state machine, Zork II hostile state windows, Zork III
// the single hooded-figure duel.
//
// Known gap against §3.3, carried over from the prototype: Zork II's second
// state window — the unleashed three-headed dog — is not modeled. Adding it
// needs per-villain room-change semantics (the dog's hostility is
// room-bound, the dragon pursues) and a validating transcript the prototype
// never captured. Until then a dog turn is priced as a normal action; the
// melee blanket rule still makes any attack on it instant.

var (
	// noSuch guards engagement: "You can't see any troll here!" quotes the
	// villain's name without any fight happening.
	noSuch      = regexp.MustCompile(`You can't see any`)
	deathBanner = regexp.MustCompile(`\*\*\*\*\s+You have died\s+\*\*\*\*`)
)

// exitRule ends a fight when its line appears, with a human-readable reason
// for the trace.
type exitRule struct {
	re     *regexp.Regexp
	reason string
}

type villain struct {
	// engage lines open combat (villain-initiated combat has no banner; the
	// first melee line is the marker).
	engage []*regexp.Regexp
	// active lines prove an ongoing fight without being engagement markers;
	// when empty, the engage set doubles as the active set.
	active []*regexp.Regexp
	exits  []exitRule
}

func (v *villain) exitMatch(output string) *exitRule {
	for i := range v.exits {
		if v.exits[i].re.MatchString(output) {
			return &v.exits[i]
		}
	}
	return nil
}

type combatRules struct {
	// survivesRoomChange: Zork I's engine clears combat when the room
	// changes; the Zork II dragon follows the player instead.
	survivesRoomChange bool
	villains           map[string]*villain
	// villainOrder fixes the engagement scan order (map iteration is random).
	villainOrder []string
}

func rx(exprs ...string) []*regexp.Regexp {
	res := make([]*regexp.Regexp, len(exprs))
	for i, e := range exprs {
		res[i] = regexp.MustCompile(e)
	}
	return res
}

var combatRulesByGame = map[string]*combatRules{
	"zork1": {
		survivesRoomChange: false,
		villainOrder:       []string{"troll", "thief", "cyclops"},
		villains: map[string]*villain{
			"troll": {
				engage: rx(
					`The troll (swings|hits you|takes a fatal blow|is knocked out|hesitates)`,
					`the troll (parries|dodges|jumps nimbly aside|is on guard)`,
					`The (flat|haft) of (the troll's axe|your sword)`, `troll's axe`,
					`The axe (crashes|sweeps|gets you|knocks|barely misses)`,
					`curtains for the troll`, `knocks out the troll`, `unconscious troll`,
				),
				exits: []exitRule{
					{regexp.MustCompile(`cloud of sinister black fog`), "villain died (black fog)"},
					{regexp.MustCompile(`knocked out!|knocks out the troll|battered into unconsciousness`), "villain knocked unconscious"},
					{regexp.MustCompile(`cowers in terror`), "troll disarmed and cowering"},
				},
			},
			"thief": {
				engage: rx(
					`The thief (stabs|draws blood|knocks you out|amuses himself|rams|attacks|bows formally)`,
					`stiletto`, `the thief (dodges|jumps nimbly aside|parries|is on guard)`,
					`scream of anguish as you violate the robber's hideaway`,
				),
				exits: []exitRule{
					{regexp.MustCompile(`cloud of sinister black fog`), "villain died (black fog)"},
					{regexp.MustCompile(`steps backward into the gloom and disappears`), "thief fled the fight"},
					{regexp.MustCompile(`lying unconscious on the ground`), "thief knocked unconscious"},
					{regexp.MustCompile(`taken aback by your unexpected generosity`), "thief pacified by a gift"},
				},
			},
			"cyclops": {
				engage: rx(
					`The [Cc]yclops (misses|sends you crashing|breaks your neck|grabs|seems unable)`,
					`monster smashes his huge fist`, `A quick punch, but it was only a glancing blow`,
					`cyclops (seems somewhat agitated|appears to be getting more agitated|is moving)`,
				),
				exits: []exitRule{
					{regexp.MustCompile(`hearing the name of his father's deadly nemesis, flees`), "cyclops fled (Odysseus)"},
					{regexp.MustCompile(`falls fast asleep`), "cyclops asleep"},
				},
			},
		},
	},
	"zork2": {
		survivesRoomChange: true, // the dragon follows
		villainOrder:       []string{"dragon"},
		villains: map[string]*villain{
			"dragon": {
				engage: rx(
					`succeeded in annoying him`, `That captured his interest`, `surprised and interested`,
					`made him rather angry`, `turns his smoky yellow eyes`,
					`puts out a claw, grins`, `doubles back and charges`,
				),
				active: rx(`The dragon (follows you|continues to watch)`),
				exits: []exitRule{
					{regexp.MustCompile(`lost interest in you\. He wanders off`), "dragon lost interest"},
					{regexp.MustCompile(`must have become bored`), "dragon got bored"},
					{regexp.MustCompile(`sees his reflection on the icy surface`), "glacier scene — dragon dead"},
				},
			},
		},
	},
	"zork3": {
		survivesRoomChange: false, // observed: the figure lapses when you walk away
		villainOrder:       []string{"figure"},
		villains: map[string]*villain{
			"figure": {
				engage: rx(
					`Through the shadows, a cloaked and hooded figure appears`,
					`hooded figure (stabs|catches you|tries|attempts|swings|thrusts|jumps|is on guard|is hit|ignores)`,
					`sharp thrust and the hooded figure`, `quick stroke catches the hooded figure`,
					`Blood trickles down the figure's arm`,
				),
				exits: []exitRule{
					{regexp.MustCompile(`fatally wounded, slumps to the ground`), "figure slain (wrong path — cloak forfeited)"},
					{regexp.MustCompile(`You slowly remove the hood`), "hood removed — the intended resolution"},
				},
			},
		},
	},
}

// endReason names why an engaged fight ends on this output — a villain exit
// line or the player death banner — or returns false.
func endReason(v *villain, output string) (string, bool) {
	if ex := v.exitMatch(output); ex != nil {
		return ex.reason, true
	}
	if deathBanner.MatchString(output) {
		return "the player died", true
	}
	return "", false
}

// updateCombat advances one game's combat state machine by one turn.
// combat is the villain currently engaged ("" when out of combat); moved is
// whether the room object number changed this turn. It returns the new
// state, the trace events the turn produced, and whether the turn carried a
// combat transition (a fight beginning or ending).
func updateCombat(rules *combatRules, combat, output string, moved bool) (state string, events []string, transition bool) {
	state = combat
	end := func(reason string) {
		events = append(events, "combat ends: "+reason)
		state, transition = "", true
	}
	if state != "" {
		v := rules.villains[state]
		switch reason, ended := endReason(v, output); {
		case ended:
			end(reason)
		case moved && !rules.survivesRoomChange:
			end("the room changed (matches engine behavior)")
		case moved && rules.survivesRoomChange:
			events = append(events, "room changed but this foe pursues — combat continues")
		}
	}
	if state == "" && combat == "" && !noSuch.MatchString(output) {
		for _, name := range rules.villainOrder {
			v := rules.villains[name]
			if matchAny(v.engage, output) {
				events = append(events, "combat begins: "+name+" melee line matched")
				state, transition = name, true
				if reason, ended := endReason(v, output); ended {
					events = append(events, "…and ends the same turn: "+reason)
					state = ""
				}
				break
			}
		}
	}
	if state != "" && combat != "" {
		act := rules.villains[state].active
		if len(act) == 0 {
			act = rules.villains[state].engage
		}
		if matchAny(act, output) {
			events = append(events, "still fighting the "+state+" (melee line each turn)")
		}
	}
	return state, events, transition
}

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
