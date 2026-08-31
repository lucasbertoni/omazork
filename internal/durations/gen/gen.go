// Package gen turns an extract into the generated duration table
// (docs/action-waits.md §5.2): deterministic rule pricing for every room edge,
// the rooms map the runtime matches on, and the generator-owned vocab. Rows
// the rules cannot price — every (verb, object) handler pair — are left to the
// LLM pass and the shared verb-default table; the generator never invents them.
package gen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/extract"
)

// Edge pricing (§2): a 2-minute base plus mechanical modifiers. Every
// modifier that applies is named in the row's note, so a curator reading the
// table sees the arithmetic, not just the total.
const (
	baseMinutes        = 2
	upMinutes          = 2 // climbing costs more than descending
	downMinutes        = 1
	waterMinutes       = 3
	conditionalMinutes = 1
	darkMinutes        = 1
)

// Generate prices one game's extract into its duration table.
func Generate(x *extract.Extract, verbs *durations.Verbs) (*durations.Table, error) {
	if x.SchemaVersion != extract.SchemaVersion {
		return nil, fmt.Errorf("gen: %s extract: schemaVersion %d, want exactly %d",
			x.Game, x.SchemaVersion, extract.SchemaVersion)
	}
	rooms := map[string]*extract.Room{}
	table := &durations.Table{
		SchemaVersion: durations.SchemaVersion,
		Game:          x.Game,
		Actions:       []durations.ActionRow{},
	}
	for _, r := range x.Rooms {
		rooms[r.ID] = r
		table.Rooms = append(table.Rooms, durations.Room{ID: r.ID, Obj: r.Obj, Name: r.Name})
	}
	sort.Slice(table.Rooms, func(i, j int) bool { return table.Rooms[i].ID < table.Rooms[j].ID })

	edges, err := priceEdges(x, rooms)
	if err != nil {
		return nil, err
	}
	table.Edges = edges
	table.Vocab = buildVocab(x, verbs)
	return table, nil
}

// priceEdges prices every passage that can change the room, then collapses
// each directed (from, to) pair to its cheapest exit — the player will take
// the quickest way through, and one pair is one key (§4). A pair any of whose
// exits drifts stays kind "drift": the runtime reads that as drift-set
// membership (§3.4), so the cheapest exit may set the price but never revoke
// it.
//
// Routine-typed exits carry no static destination and so cannot become rows at
// all; they are the LLM pass's to classify (§5.3), and actiongen reports how
// many each game leaves behind.
func priceEdges(x *extract.Extract, rooms map[string]*extract.Room) ([]durations.EdgeRow, error) {
	type group struct {
		row   durations.EdgeRow
		dirs  []string
		n     int
		drift bool
	}
	var order []string
	groups := map[string]*group{}

	for _, e := range x.Edges {
		if e.To == "" || e.To == e.From {
			continue // a refusal or a self-loop never changes the room
		}
		from, to := rooms[e.From], rooms[e.To]
		if from == nil {
			return nil, fmt.Errorf("gen: %s: edge from unknown room %s", x.Game, e.From)
		}
		if to == nil {
			return nil, fmt.Errorf("gen: %s: edge %s -> unknown room %s", x.Game, e.From, e.To)
		}
		minutes, note := priceEdge(e, from, to)
		row := durations.EdgeRow{
			From: e.From, To: e.To, Dir: e.Dir, Kind: e.Kind,
			Minutes: minutes, Class: durations.ClassMovement, Source: durations.SourceRule, Note: note,
		}
		key := durations.EdgeKey(e.From, e.To)
		g, ok := groups[key]
		if !ok {
			g = &group{row: row, dirs: []string{exitLabel(e)}, n: 1}
			groups[key] = g
			order = append(order, key)
		} else {
			g.n++
			g.dirs = append(g.dirs, exitLabel(e))
			if minutes < g.row.Minutes {
				g.row = row
			}
		}
		g.drift = g.drift || e.Kind == extract.KindDrift
	}

	rows := make([]durations.EdgeRow, 0, len(order))
	for _, key := range order {
		g := groups[key]
		if g.drift {
			g.row.Kind = extract.KindDrift
		}
		if g.n > 1 {
			g.row.Note += fmt.Sprintf("; cheapest of %d exits (%s)", g.n, strings.Join(g.dirs, ", "))
		}
		rows = append(rows, g.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].From != rows[j].From {
			return rows[i].From < rows[j].From
		}
		return rows[i].To < rows[j].To
	})
	return rows, nil
}

// priceEdge applies the §2 formula to one passage and renders the arithmetic.
func priceEdge(e *extract.Edge, from, to *extract.Room) (int, string) {
	minutes := baseMinutes
	parts := []string{fmt.Sprintf("base %d", baseMinutes)}
	add := func(n int, label string) {
		minutes += n
		parts = append(parts, fmt.Sprintf("%s %d", label, n))
	}
	switch e.Dir {
	case "UP":
		add(upMinutes, "up")
	case "DOWN":
		add(downMinutes, "down")
	}
	if isWater(from) || isWater(to) {
		add(waterMinutes, "water")
	}
	switch e.Kind {
	case extract.KindCondDoor:
		add(conditionalMinutes, "door")
	case extract.KindCondFlag, extract.KindRoutine:
		add(conditionalMinutes, "conditional")
	}
	if !hasFlag(to, "ONBIT") {
		add(darkMinutes, "dark")
	}
	// No clamp here on purpose: the modifiers sum to at most 9, inside the
	// Movement band, and the validator gates the band for every row — a future
	// modifier that broke it should fail loudly, not be quietly trimmed.
	return minutes, strings.Join(parts, " + ")
}

// isWater reports whether a room is water the player must swim or float
// across. NONLANDBIT alone only means "not solid ground", which the games also
// hang on the volcano airspace (NWALLBIT) and on ledges a balloon can reach
// (RLANDBIT), so both are excluded. Zork III's lake rooms carry RLANDBIT too
// and so read as land here; the LLM nudge or an overlay row is where that gets
// its feel back (§5.3).
func isWater(r *extract.Room) bool {
	return hasFlag(r, "NONLANDBIT") && !hasFlag(r, "RLANDBIT") && !hasFlag(r, "NWALLBIT")
}

func hasFlag(r *extract.Room, flag string) bool {
	for _, f := range r.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// exitLabel names an exit in a collision note: its direction, or its kind for
// the synthesized rows that have none (drift, candidate).
func exitLabel(e *extract.Edge) string {
	if e.Dir != "" {
		return e.Dir
	}
	return e.Kind
}

// buildVocab maps every parser word the wrapper might see to the key it
// belongs to: verb words to the canonical verb of the shared table, object
// nouns to the ids that noun can name. Words the shared table doesn't know
// stay themselves and degrade to the verb-default row.
func buildVocab(x *extract.Extract, verbs *durations.Verbs) durations.Vocab {
	vocab := durations.Vocab{Verbs: map[string]string{}, Nouns: map[string][]string{}}

	synonyms := map[string][]string{}
	for _, s := range x.Synonyms {
		synonyms[s.Word] = s.Synonyms
	}
	for _, s := range x.Syntax {
		group := append([]string{s.Verb}, synonyms[s.Verb]...)
		canon := canonicalVerb(group, verbs)
		for _, word := range group {
			vocab.Verbs[strings.ToLower(word)] = canon
		}
	}

	nouns := map[string]map[string]bool{}
	for _, o := range x.Objects {
		for _, syn := range o.Synonyms {
			word := strings.ToLower(syn)
			if nouns[word] == nil {
				nouns[word] = map[string]bool{}
			}
			nouns[word][o.ID] = true
		}
	}
	for word, ids := range nouns {
		list := make([]string, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		sort.Strings(list)
		vocab.Nouns[word] = list
	}
	return vocab
}

// canonicalVerb resolves a ZIL verb and its synonyms to the shared table's
// spelling. ZIL vocabulary is truncated to six characters (INFLAT, EXTING), so
// a six-letter word that prefixes exactly one shared verb resolves to it.
func canonicalVerb(group []string, verbs *durations.Verbs) string {
	words := make([]string, 0, len(group))
	for _, w := range group {
		words = append(words, strings.ToLower(w))
	}
	for _, word := range words {
		for _, d := range verbs.Verbs {
			if word == d.Verb {
				return d.Verb
			}
			for _, syn := range d.Synonyms {
				if word == syn {
					return d.Verb
				}
			}
		}
	}
	for _, word := range words {
		if len(word) < 6 {
			continue
		}
		var match string
		for _, d := range verbs.Verbs {
			if strings.HasPrefix(d.Verb, word) {
				if match != "" && match != d.Verb {
					match = ""
					break
				}
				match = d.Verb
			}
		}
		if match != "" {
			return match
		}
	}
	return words[0]
}
