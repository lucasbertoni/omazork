// Package actions classifies player turns for action-wait pricing
// (docs/action-waits.md §3). It is the one implementation of turn
// classification shared by the calibration replay and the runtime mediation
// layer: room tracking via global 0, moves-counter no-op detection, output
// failure phrases, and the combat state machines for all three games.
//
// The package classifies; it does not price. A Result names the wrapper hard
// class the turn fell into, or — for Movement and Action turns — the keys
// (directed room edge, parsed verb and object) the caller resolves against
// the duration tables per the spec's lookup precedence.
//
// Proven on branch prototype/action-matcher (#22) and lifted here by #32.
package actions

import (
	"fmt"
	"regexp"
)

// failurePhrases are outputs the engine prints when a turn executed but
// accomplished nothing ("Too late for that."). Such turns consume a move, so
// the frozen-moves detector misses them; this text heuristic is the accepted
// fallback (spec §3.2 — mispricing residue falls back to normal action
// pricing, never to combat).
var failurePhrases = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^I don't know the word`),
	regexp.MustCompile(`(?m)^You can't`),
	regexp.MustCompile(`(?m)^You don't have`),
	regexp.MustCompile(`(?m)^There is a wall`),
	regexp.MustCompile(`(?m)^Too late for that`),
	regexp.MustCompile(`(?m)^It is already`),
	regexp.MustCompile(`(?m)^That would be`),
	regexp.MustCompile(`(?m)^What a concept`),
	regexp.MustCompile(`(?m)^You must tell me how`),
	regexp.MustCompile(`(?m)^It's already open`),
}

// Turn is the observable outcome of one player command, as the engine
// reports it.
type Turn struct {
	Input   string
	Output  string
	Room    string // status-line display name; trace only, never a key
	RoomObj uint16 // global 0 after the turn
	Moves   int    // global 2 after the turn
}

// State is what the classifier carries from one turn to the next. Seed it
// with StartState and thread each Result.State into the following Classify
// call.
type State struct {
	RoomObj uint16
	Room    string
	Moves   int
	Combat  string // villain currently engaged; "" out of combat
}

// StartState seeds classification from the opening banner turn.
func StartState(t Turn) State {
	return State{RoomObj: t.RoomObj, Room: t.Room, Moves: t.Moves}
}

// Kind is the classification bucket for one turn. The first six are the
// wrapper-enforced hard classes — always instant, overriding any table.
type Kind int

const (
	// Aborted: the parser stopped the turn before it ran (moves counter froze).
	Aborted Kind = iota
	// Combat: the turn began, spent, or ended in combat.
	Combat
	// Melee: a melee-verb input outside combat (blanket rule, spec §3.3).
	Melee
	// FastVerb: one of the fixed always-instant verbs (spec §10), on a turn
	// that stayed in the room — a fast verb that moved the player is a
	// handler teleport or drift traversal and classifies as Action instead.
	FastVerb
	// FailedMove: a walk command that did not change the room.
	FailedMove
	// FailedAction: the output matched a failure phrase (accepted heuristic).
	FailedAction
	// Movement: a walk command that changed the room; price the directed edge
	// From>To.
	Movement
	// Action: everything else; price by (verb, object), verb default, then
	// fallback. When Moved is also set the room changed without a walk
	// command: a teleport (priced as the action alone) or a drift edge if
	// From>To is in the drift set (spec §3.4).
	Action
)

// Instant reports whether the kind is a wrapper hard class that costs
// nothing regardless of any table.
func (k Kind) Instant() bool { return k != Movement && k != Action }

func (k Kind) String() string {
	switch k {
	case Aborted:
		return "aborted"
	case Combat:
		return "combat"
	case Melee:
		return "melee"
	case FastVerb:
		return "fastverb"
	case FailedMove:
		return "failedmove"
	case FailedAction:
		return "failedaction"
	case Movement:
		return "movement"
	case Action:
		return "action"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Result is one classified turn.
type Result struct {
	Kind         Kind
	Parsed       Parsed
	Moved        bool   // the room object number changed this turn
	From, To     uint16 // directed edge, set when Moved
	CombatEvents []string
	Trace        []string // human-readable derivation, for calibration reports
	State        State    // classifier state after this turn
}

// Matcher classifies turns for one game. It holds no mutable state; the
// caller threads State explicitly, so one Matcher serves both the
// calibration replay and live sessions.
type Matcher struct {
	rules *combatRules
}

// NewMatcher returns the matcher for one of zork1, zork2, zork3.
func NewMatcher(game string) (*Matcher, error) {
	rules, ok := combatRulesByGame[game]
	if !ok {
		return nil, fmt.Errorf("actions: unknown game %q", game)
	}
	return &Matcher{rules: rules}, nil
}

// Classify decides what one turn was, from output text, room object number,
// and moves counter alone.
func (m *Matcher) Classify(prev State, turn Turn) Result {
	var trace []string
	parsed := Parse(turn.Input)
	if parsed.Dir != "" {
		trace = append(trace, fmt.Sprintf("parsed as a walk command: %q", parsed.Dir))
	} else {
		trace = append(trace, fmt.Sprintf("parsed as (verb: %s, object: %s)", orDash(parsed.Verb), orDash(parsed.Object)))
	}

	moved := turn.RoomObj != prev.RoomObj
	aborted := turn.Moves == prev.Moves
	if moved {
		trace = append(trace, fmt.Sprintf("room changed: %s (#%d) → %s (#%d)", prev.Room, prev.RoomObj, turn.Room, turn.RoomObj))
	}
	if aborted {
		trace = append(trace, "move counter did not advance — the engine aborted this turn before it ran")
	}

	combat, events, transition := updateCombat(m.rules, prev.Combat, turn.Output, moved)
	res := Result{
		Parsed:       parsed,
		Moved:        moved,
		CombatEvents: events,
		State:        State{RoomObj: turn.RoomObj, Room: turn.Room, Moves: turn.Moves, Combat: combat},
	}
	if moved {
		res.From, res.To = prev.RoomObj, turn.RoomObj
	}
	done := func(k Kind, note string) Result {
		if note != "" {
			trace = append(trace, note)
		}
		res.Kind, res.Trace = k, trace
		return res
	}

	// Wrapper hard classes — always instant, overriding any table.
	switch {
	case aborted:
		return done(Aborted, "hard class: turn never ran (parser stopped it)")
	case prev.Combat != "" || combat != "" || transition:
		return done(Combat, "hard class: in combat — every combat turn is instant")
	case parsed.Verb == meleeVerb:
		return done(Melee, "hard class: melee verb — attacks are always instant, combat or not")
	case parsed.Verb != "" && fastVerbs[parsed.Verb] && !moved:
		return done(FastVerb, "hard class: fast verb (fixed list)")
	case parsed.Dir != "" && !moved:
		return done(FailedMove, "hard class: walk command but the room did not change — failed moves cost nothing")
	case parsed.Dir == "" && matchAny(failurePhrases, turn.Output):
		return done(FailedAction, "heuristic: output reads as a failure phrase — treated as a no-op")
	case parsed.Dir != "" && moved:
		return done(Movement, "movement: price the directed room edge")
	case moved:
		return done(Action, "room changed without a walk command (teleport, death, handler, drift) — priced as the action unless the edge is in the drift set")
	default:
		return done(Action, "action: price by (verb, object), verb default, then fallback")
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
