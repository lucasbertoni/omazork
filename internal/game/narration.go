package game

import (
	"github.com/lucasbertoni/omazork/internal/actions"
	"github.com/lucasbertoni/omazork/internal/durations"
)

// Wait narration (docs/action-waits.md §8, #43): the player-facing phrase for
// what an in-flight wait is about, built only from the player's own command —
// never the destination room, never the pricing class.

// classNarration is the fallback when the verb has no phrase of its own.
var classNarration = map[durations.Class]string{
	durations.ClassManipulation: "handling",
	durations.ClassMechanism:    "working on",
	durations.ClassMovement:     "making your way",
	durations.ClassDramatic:     "seeing it through",
}

const genericNarration = "acting"

// narrate composes the narration for one priced turn: the verb's progressive
// phrase followed by the rest of what the player typed, as the parser read it
// ("climbing" + "tree"); a walk command narrates by direction. Class fallbacks
// stand alone.
func narrate(res actions.Result, class durations.Class, durs *durations.Layered) string {
	if dir := res.Parsed.Dir; dir != "" {
		switch dir {
		case "up", "down":
			return "climbing " + dir
		}
		return "heading " + dir
	}
	if phrase, ok := durs.Narration(res.Parsed.Verb); ok {
		if res.Parsed.Rest == "" {
			return phrase
		}
		return phrase + " " + res.Parsed.Rest
	}
	if n, ok := classNarration[class]; ok {
		return n
	}
	return genericNarration
}

// Wait copy around a narration: the withheld line and the blocked line.
func withheldNarration(n string) string { return "You begin " + n + "..." }
func blockedNarration(n string) string  { return "You are still " + n + "..." }
