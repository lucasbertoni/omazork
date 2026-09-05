package actions_test

import (
	"testing"

	"github.com/lucasbertoni/omazork/internal/actions"
)

func TestParse(t *testing.T) {
	cases := []struct {
		raw  string
		want actions.Parsed
	}{
		// Walk commands: abbreviations canonicalize, go/walk prefixes strip.
		{"north", actions.Parsed{Dir: "north"}},
		{"n", actions.Parsed{Dir: "north"}},
		{"NE", actions.Parsed{Dir: "northeast"}},
		{"go west", actions.Parsed{Dir: "west"}},
		{"walk up", actions.Parsed{Dir: "up"}},
		{"d", actions.Parsed{Dir: "down"}},

		// Verb synonyms and noise words.
		{"open mailbox", actions.Parsed{Verb: "open", Object: "mailbox", Rest: "mailbox"}},
		{"open the mailbox", actions.Parsed{Verb: "open", Object: "mailbox", Rest: "mailbox"}},
		{"get lamp", actions.Parsed{Verb: "take", Object: "lamp", Rest: "lamp"}},
		{"pick up the sword", actions.Parsed{Verb: "take", Object: "sword", Rest: "sword"}},
		{"x leaflet", actions.Parsed{Verb: "examine", Object: "leaflet", Rest: "leaflet"}},
		{"i", actions.Parsed{Verb: "inventory"}},

		// Head noun: the last object token before "with".
		{"open trap door", actions.Parsed{Verb: "open", Object: "door", Rest: "trap door"}},
		{"kill troll with sword", actions.Parsed{Verb: "attack", Object: "troll", Rest: "troll with sword"}},
		{"attack figure with sword", actions.Parsed{Verb: "attack", Object: "figure", Rest: "figure with sword"}},
		{"knock down thief", actions.Parsed{Verb: "attack", Object: "thief", Rest: "thief"}},
		{"swing sword", actions.Parsed{Verb: "attack", Object: "sword", Rest: "sword"}},

		// "turn on/off" fuse into the verb.
		{"turn on lamp", actions.Parsed{Verb: "turn on", Object: "lamp", Rest: "lamp"}},
		{"turn off the lamp", actions.Parsed{Verb: "turn off", Object: "lamp", Rest: "lamp"}},
		{"turn lamp", actions.Parsed{Verb: "turn", Object: "lamp", Rest: "lamp"}},

		// Punctuation is stripped; unknown verbs pass through.
		{"say \"hello\"", actions.Parsed{Verb: "say", Object: "hello", Rest: "hello"}},
		{"xyzzy", actions.Parsed{Verb: "xyzzy"}},
		{"", actions.Parsed{}},
	}
	for _, c := range cases {
		if got := actions.Parse(c.raw); got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.raw, got, c.want)
		}
	}
}
