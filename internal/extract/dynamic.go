package extract

import (
	"fmt"
	"strings"

	"github.com/lucasbertoni/omazork/internal/zil"
)

// This file synthesizes the §3.4 edge rows that no <ROOM> direction property
// carries: drift edges (current/vehicle movement read off the games' ladder
// tables) and candidate plain edges for random-destination walk exits. Both
// are appended after correlation — they have no z-machine exit property.

// driftLadders names, per game, the table globals whose consecutive elements
// are drift edges: the Frigid River current (I-RIVER walks RIVER-NEXT
// downstream) and the Zork II balloon ladder (RISE-AND-SHINE / DECLINE-AND-
// FALL walk BALLOON-UPS / BALLOON-DOWNS). Slice order fixes output order.
// Zork III's mirror-box walk FEXITs are deliberately not enumerated (§3.4).
var driftLadders = map[string][]string{
	"zork1": {"RIVER-NEXT"},
	"zork2": {"BALLOON-UPS", "BALLOON-DOWNS"},
}

// randomExitRoutines names, per game, the PER exit routines that walk the
// player to a random destination. Each room the routine's body references
// becomes one plain candidate edge from every room using it; the matcher
// keys on the observed destination. (Zork II's Carousel Room randomizes the
// direction instead, so its candidates are already static plain edges.)
var randomExitRoutines = map[string][]string{
	"zork2": {"MAGNET-ROOM-EXIT"},
}

// driftEdges appends one drift edge per consecutive room pair in each of the
// game's ladder tables.
func (ex *extractor) driftEdges() error {
	for _, name := range driftLadders[ex.game] {
		g := ex.globals[name]
		if g == nil {
			return fmt.Errorf("drift ladder global %s not found", name)
		}
		val := g.Kids[2] // <GLOBAL name value>; arity guaranteed at capture
		if val.Kind != zil.KForm || (val.Head() != "LTABLE" && val.Head() != "TABLE") {
			return fmt.Errorf("%s:%d: drift ladder %s is not a table", g.File, g.Line, name)
		}
		var rooms []string
		for _, k := range val.Kids[1:] {
			if k.Kind == zil.KList {
				continue // table flags, e.g. (PURE)
			}
			a := k.Atom()
			if ex.roomIx[a] == nil {
				return fmt.Errorf("%s:%d: drift ladder %s: %s is not a room", g.File, g.Line, name, a)
			}
			rooms = append(rooms, a)
		}
		if len(rooms) < 2 {
			return fmt.Errorf("%s:%d: drift ladder %s has %d rooms", g.File, g.Line, name, len(rooms))
		}
		for i := 0; i+1 < len(rooms); i++ {
			ex.edges = append(ex.edges, &Edge{From: rooms[i], Kind: KindDrift, To: rooms[i+1]})
		}
	}
	return nil
}

// candidateEdges appends, for each random-exit routine, one plain edge per
// (room using it, room its body references) pair.
func (ex *extractor) candidateEdges() error {
	for _, name := range randomExitRoutines[ex.game] {
		def, ok := ex.routines[name]
		if !ok {
			return fmt.Errorf("random-exit routine %s not found", name)
		}
		nodes, err := zil.Parse([]byte(def.src), def.file)
		if err != nil {
			return fmt.Errorf("random-exit routine %s: %w", name, err)
		}
		var candidates []string
		seen := map[string]bool{}
		var walk func(ns []*zil.Node)
		walk = func(ns []*zil.Node) {
			for _, n := range ns {
				if id, found := strings.CutPrefix(n.Atom(), ","); found && ex.roomIx[id] != nil && !seen[id] {
					seen[id] = true
					candidates = append(candidates, id)
				}
				walk(n.Kids)
			}
		}
		walk(nodes)
		if len(candidates) == 0 {
			return fmt.Errorf("%s:%d: random-exit routine %s references no rooms", def.file, def.line, name)
		}
		var froms []string
		fromSeen := map[string]bool{}
		for _, e := range ex.edges {
			if e.Per == name && !fromSeen[e.From] {
				fromSeen[e.From] = true
				froms = append(froms, e.From)
			}
		}
		if len(froms) == 0 {
			return fmt.Errorf("random-exit routine %s is not used by any exit", name)
		}
		for _, from := range froms {
			for _, to := range candidates {
				ex.edges = append(ex.edges, &Edge{From: from, Kind: KindPlain, To: to})
			}
		}
	}
	return nil
}
