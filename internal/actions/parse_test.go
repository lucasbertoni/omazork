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
		{"open mailbox", actions.Parsed{Verb: "open", Object: "mailbox"}},
		{"open the mailbox", actions.Parsed{Verb: "open", Object: "mailbox"}},
		{"get lamp", actions.Parsed{Verb: "take", Object: "lamp"}},
		{"pick up the sword", actions.Parsed{Verb: "take", Object: "sword"}},
		{"x leaflet", actions.Parsed{Verb: "examine", Object: "leaflet"}},
		{"i", actions.Parsed{Verb: "inventory"}},

		// Head noun: the last object token before "with".
		{"open trap door", actions.Parsed{Verb: "open", Object: "door"}},
		{"kill troll with sword", actions.Parsed{Verb: "attack", Object: "troll"}},
		{"attack figure with sword", actions.Parsed{Verb: "attack", Object: "figure"}},
		{"knock down thief", actions.Parsed{Verb: "attack", Object: "thief"}},
		{"swing sword", actions.Parsed{Verb: "attack", Object: "sword"}},

		// "turn on/off" fuse into the verb.
		{"turn on lamp", actions.Parsed{Verb: "turn on", Object: "lamp"}},
		{"turn off the lamp", actions.Parsed{Verb: "turn off", Object: "lamp"}},
		{"turn lamp", actions.Parsed{Verb: "turn", Object: "lamp"}},

		// Punctuation is stripped; unknown verbs pass through.
		{"say \"hello\"", actions.Parsed{Verb: "say", Object: "hello"}},
		{"xyzzy", actions.Parsed{Verb: "xyzzy"}},
		{"", actions.Parsed{}},
	}
	for _, c := range cases {
		if got := actions.Parse(c.raw); got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.raw, got, c.want)
		}
	}
}
