package actions

import "strings"

// Parsed is the wrapper's reading of one player input: either a walk command
// (Dir set, Verb/Object empty) or a (verb, object) action.
type Parsed struct {
	Dir    string // canonical direction; "" when not a walk command
	Verb   string // canonical verb; "" for walk commands and empty input
	Object string // head noun (last object token before "with"); "" when absent
}

var directions = map[string]string{
	"n": "north", "north": "north", "s": "south", "south": "south",
	"e": "east", "east": "east", "w": "west", "west": "west",
	"ne": "northeast", "northeast": "northeast", "nw": "northwest", "northwest": "northwest",
	"se": "southeast", "southeast": "southeast", "sw": "southwest", "southwest": "southwest",
	"u": "up", "up": "up", "d": "down", "down": "down",
}

var verbSynonyms = map[string]string{
	"l": "look", "x": "examine", "i": "inventory",
	"kill": "attack", "murder": "attack", "slay": "attack", "fight": "attack",
	"hit": "attack", "hurt": "attack", "injure": "attack", "dispatch": "attack",
	"stab": "attack", "strike": "attack", "swing": "attack", "thrust": "attack",
	"poke": "attack", "get": "take",
}

// meleeVerb is the canonical form every attack-family verb collapses to; a
// melee-verb input is always an instant turn (spec §3.3, blanket rule).
const meleeVerb = "attack"

// fastVerbs is the canonical fast-verb hard class (spec §10): always instant
// on turns that stay in the room. A fast-verb turn that changed the room is a
// handler teleport or a drift traversal and must stay priceable — the altar
// prayer (§10's pray footnote) and the balloon's wait-driven drift edges
// (§3.4) both move the player on otherwise-fast inputs.
var fastVerbs = map[string]bool{
	"look": true, "examine": true, "read": true, "inventory": true, "wait": true,
	"score": true, "diagnose": true, "verbose": true, "brief": true, "superbrief": true,
	"save": true, "restore": true, "restart": true, "quit": true, "script": true,
	"unscript": true, "version": true, "again": true, "oops": true, "pray": true,
	"hello": true, "count": true,
}

var noiseWords = map[string]bool{"the": true, "a": true, "an": true, "at": true, "to": true}

// Parse normalizes one raw player input into the matcher's keys.
func Parse(raw string) Parsed {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == ' ':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return ' '
		}
	}, raw)
	tokens := make([]string, 0, 4)
	for _, tok := range strings.Fields(clean) {
		if !noiseWords[tok] {
			tokens = append(tokens, tok)
		}
	}
	if len(tokens) > 0 && (tokens[0] == "go" || tokens[0] == "walk") {
		tokens = tokens[1:]
	}
	if len(tokens) == 1 && directions[tokens[0]] != "" {
		return Parsed{Dir: directions[tokens[0]]}
	}
	if len(tokens) >= 2 && tokens[0] == "pick" && tokens[1] == "up" {
		tokens = append([]string{"take"}, tokens[2:]...)
	}
	var verb string
	var rest []string
	if len(tokens) > 0 {
		verb = tokens[0]
		if syn := verbSynonyms[verb]; syn != "" {
			verb = syn
		}
		rest = tokens[1:]
	}
	if verb == "turn" && len(rest) > 0 && (rest[0] == "on" || rest[0] == "off") {
		verb, rest = "turn "+rest[0], rest[1:]
	}
	if verb == "knock" && len(rest) > 0 && rest[0] == "down" {
		verb, rest = meleeVerb, rest[1:]
	}
	objTokens := rest
	for i, tok := range rest {
		if tok == "with" {
			objTokens = rest[:i]
			break
		}
	}
	var object string
	if len(objTokens) > 0 {
		object = objTokens[len(objTokens)-1]
	}
	return Parsed{Verb: verb, Object: object}
}
