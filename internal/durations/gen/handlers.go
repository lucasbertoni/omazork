package gen

import (
	"regexp"
	"sort"
	"strings"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/extract"
)

// Handler pairs (§5.3): every (verb, object) an object's ACTION routine
// answers to. ZIL dispatches on PRSA two ways — the VERB? macro and a bare
// V?<verb> constant — so both spellings are read out of the raw routine text.
var (
	verbQuery = regexp.MustCompile(`(?s)<VERB\?([^<>]*)>`)
	verbConst = regexp.MustCompile(`V\?([A-Z0-9?/.$%&*+=\-]+)`)
)

// prayer is the one fast verb that still gets a handler row: §10 lists PRAY as
// always-instant except at the altar, where it is a handler pair the LLM or an
// overlay prices. Every other melee or fast verb is filtered by the shared
// table itself — it deliberately carries no melee entry (§3.3, §10).
const prayer = "pray"

// meleeVerbs are the §3.3 attack families. A melee-verb input is priced
// instant by the wrapper's blanket hard rule, so a row for one could never be
// consulted — unless the shared table itself prices the word, as it does for
// KNOCK (on a door) and SLICE, which are ordinary actions that only happen to
// sit in a melee family. The shared table is the tiebreaker, not this list.
var meleeVerbs = map[string]bool{
	"attack": true, "fight": true, "kill": true, "murder": true,
	"stab": true, "strike": true, "swing": true, "knock": true, "slice": true,
}

// pair is one (verb, object) row the LLM pass has to classify.
type pair struct {
	verb    string
	object  *extract.Object
	handler string
	source  string
	syntax  []string
}

// handlerPairs enumerates the pairs in a stable order. It reads only object
// handlers: a room handler's key would be a room id, and the (verb, object)
// key shape has nowhere to put one — those turns fall to the verb default.
//
// A pair is dropped when either half is unaddressable: a verb the shared table
// vocab cannot address, a melee verb the shared table does not price (the
// wrapper's hard class outranks any row), a fast verb (same reason), or an
// object with no noun in the vocab, since nothing the player types could name
// it.
func handlerPairs(x *extract.Extract, vocab durations.Vocab, verbs *durations.Verbs) []pair {
	objects := map[string]*extract.Object{}
	for _, o := range x.Objects {
		objects[o.ID] = o
	}
	routines := map[string]string{}
	for _, r := range x.Routines {
		routines[r.Name] = r.Source
	}
	nouns := map[string]bool{}
	for _, ids := range vocab.Nouns {
		for _, id := range ids {
			nouns[id] = true
		}
	}
	fast := map[string]bool{}
	for _, v := range verbs.FastVerbs {
		fast[v] = true
	}
	// A verb is addressable when the runtime could resolve a player's word to
	// it: the shared table knows it, or the generated vocab canonicalizes to
	// it. That is exactly the validator's orphan rule for an action key.
	priced := map[string]bool{}
	for _, d := range verbs.Verbs {
		priced[d.Verb] = true
	}
	addressable := func(verb string) bool {
		return priced[verb] || vocab.Verbs[verb] == verb
	}
	syntax := syntaxByVerb(x, vocab)

	var pairs []pair
	for _, id := range sortedKeys(x.Handlers.Objects) {
		object, ok := objects[id]
		if !ok || !nouns[id] {
			continue
		}
		handler := x.Handlers.Objects[id]
		source := routines[handler]
		for _, verb := range handlerVerbs(source, vocab) {
			if !addressable(verb) || (fast[verb] && verb != prayer) {
				continue
			}
			if meleeVerbs[verb] && !priced[verb] {
				continue
			}
			pairs = append(pairs, pair{
				verb: verb, object: object, handler: handler,
				source: source, syntax: syntax[verb],
			})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		a := durations.ActionKey(pairs[i].verb, pairs[i].object.ID)
		b := durations.ActionKey(pairs[j].verb, pairs[j].object.ID)
		return a < b
	})
	return pairs
}

// handlerVerbs reads the canonical verbs one handler dispatches on, deduped
// and sorted.
func handlerVerbs(source string, vocab durations.Vocab) []string {
	seen := map[string]bool{}
	add := func(word string) {
		word = strings.ToLower(strings.Trim(word, ",()"))
		if canon, ok := vocab.Verbs[word]; ok && canon != "" {
			seen[canon] = true
		}
	}
	for _, m := range verbQuery.FindAllStringSubmatch(source, -1) {
		for _, word := range strings.Fields(m[1]) {
			add(word)
		}
	}
	for _, m := range verbConst.FindAllStringSubmatch(source, -1) {
		add(m[1])
	}
	verbs := make([]string, 0, len(seen))
	for v := range seen {
		verbs = append(verbs, v)
	}
	sort.Strings(verbs)
	return verbs
}

// syntaxByVerb collects the grammar lines a canonical verb was parsed from —
// context for the prompt, so the model sees the shapes the parser accepts.
func syntaxByVerb(x *extract.Extract, vocab durations.Vocab) map[string][]string {
	lines := map[string][]string{}
	seen := map[string]bool{}
	for _, s := range x.Syntax {
		canon := vocab.Verbs[strings.ToLower(s.Verb)]
		if canon == "" {
			continue
		}
		line := strings.TrimSpace(s.Verb + " " + strings.Join(s.Pattern, " "))
		if seen[canon+"|"+line] {
			continue
		}
		seen[canon+"|"+line] = true
		lines[canon] = append(lines[canon], line)
	}
	for verb := range lines {
		sort.Strings(lines[verb])
	}
	return lines
}

// sortedKeys iterates a map in a stable order: every list this package builds
// has to render identically on every run.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
