package durations

import "sort"

// QuietThreshold is the wait-tier line (§8): a wait under this many minutes is
// quiet, at or above it is full. The calibration replay gates on the share of
// turns that land under it; the wrapper keys presentation on it.
const QuietThreshold = 5

// FallbackMinutes is the §2 global fallback: what a turn costs when no table
// has a row for it.
const FallbackMinutes = 1

// Query is one classified turn reduced to what the tables key on. The
// classifier (internal/actions) decides Instant, Walk, and Moved; the room
// object numbers come straight from global 0; verb and object are the parsed
// words, still to be normalized through the vocab here.
type Query struct {
	Instant bool // a wrapper hard class: combat, fast verb, failed or no-op turn
	Walk    bool // a directional walk command
	Moved   bool // the room object number changed
	From    int  // room object numbers, set when Moved
	To      int
	Verb    string // parsed verb word, "" on walk commands
	Object  string // parsed head noun, "" when absent
}

// Slot names the §2 precedence step that priced a turn.
type Slot string

const (
	SlotHard        Slot = "hard"        // 1. wrapper hard class
	SlotOverlay     Slot = "overlay"     // 2. hand-curated overlay row
	SlotDrift       Slot = "drift"       // 3. drift edge on a non-walk room change
	SlotAction      Slot = "action"      // 4. (verb, object) or verb-only row
	SlotEdge        Slot = "edge"        // 5. edge row on a walk command
	SlotVerbDefault Slot = "verbDefault" // 6. shared verb default, with any per-game override
	SlotFallback    Slot = "fallback"    // 7. global fallback
)

// Resolution is a priced turn: the row, which precedence slot supplied it, and
// the row key consulted ("" for the hard class and the fallback).
type Resolution struct {
	Row  Row
	Slot Slot
	Key  string
}

// Resolve prices one turn by the §2 lookup precedence, first match wins. It is
// the one implementation shared by the calibration replay and the runtime, so
// a replay turn costs exactly what the same turn costs a player.
func (l *Layered) Resolve(q Query) Resolution {
	if q.Instant {
		return Resolution{Row: Row{Class: ClassInstant, Source: SourceRule}, Slot: SlotHard}
	}
	edgeKey := ""
	if q.Moved {
		from, okFrom := l.roomID[q.From]
		to, okTo := l.roomID[q.To]
		if okFrom && okTo {
			edgeKey = EdgeKey(from, to)
		}
	}
	if q.Walk {
		// A walk command only ever consults its edge (§10's footnote: a room
		// change on a walk command resolves through the edge row first, and a
		// walk has no verb to fall through to).
		if row, ok := l.edges[edgeKey]; ok && q.Moved {
			return Resolution{Row: row, Slot: slotFor(row, SlotEdge), Key: edgeKey}
		}
		return fallback()
	}

	verb := l.CanonicalVerb(q.Verb)
	actionKey, actionRow, hasAction := l.actionRow(verb, q.Object)
	driftRow, hasDrift := Row{}, false
	if q.Moved && l.drift[edgeKey] {
		driftRow, hasDrift = l.edges[edgeKey]
	}
	// 2. An overlay row outranks everything but the hard classes — whichever
	// shape it has. A teleport still never prices as an edge: only a drift
	// pair's edge is in play on a non-walk turn.
	if hasAction && actionRow.Source == SourceOverlay {
		return Resolution{Row: actionRow, Slot: SlotOverlay, Key: actionKey}
	}
	if hasDrift && driftRow.Source == SourceOverlay {
		return Resolution{Row: driftRow, Slot: SlotOverlay, Key: edgeKey}
	}
	if hasDrift {
		return Resolution{Row: driftRow, Slot: SlotDrift, Key: edgeKey}
	}
	if hasAction {
		return Resolution{Row: actionRow, Slot: SlotAction, Key: actionKey}
	}
	if canon, ok := l.verbAliases[verb]; ok {
		return Resolution{Row: l.verbDefaults[canon], Slot: SlotVerbDefault, Key: VerbDefaultKey(canon)}
	}
	return fallback()
}

// actionRow finds the (verb, object) row for a parsed noun — the noun may name
// several objects, first with a row wins — else the verb-only row.
func (l *Layered) actionRow(verb, object string) (string, Row, bool) {
	if object != "" {
		for _, id := range l.Nouns(object) {
			key := ActionKey(verb, id)
			if row, ok := l.actions[key]; ok {
				return key, row, true
			}
		}
	}
	key := ActionKey(verb, "")
	row, ok := l.actions[key]
	return key, row, ok
}

// slotFor reports the precedence slot a matched row belongs to: an overlay
// row is slot 2 whatever shape it has.
func slotFor(row Row, base Slot) Slot {
	if row.Source == SourceOverlay {
		return SlotOverlay
	}
	return base
}

// fallback is the §2 global fallback resolution.
func fallback() Resolution {
	return Resolution{Row: Row{Minutes: FallbackMinutes, Class: ClassManipulation, Source: SourceRule}, Slot: SlotFallback}
}

// KeyedRow is one layered row with its key, for whole-table passes.
type KeyedRow struct {
	Key string
	Row
}

// Rows returns every layered edge row and every layered action row, each in
// key order — the population the §6 histograms and the §7 outlier search run
// over. Overlay rows have already replaced or joined the base rows.
func (l *Layered) Rows() (edges, actions []KeyedRow) {
	return keyed(l.edges), keyed(l.actions)
}

// keyed flattens a row map into key order.
func keyed(m map[string]Row) []KeyedRow {
	rows := make([]KeyedRow, 0, len(m))
	for k, r := range m {
		rows = append(rows, KeyedRow{Key: k, Row: r})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	return rows
}

// Acknowledged is the overlay's curation acknowledgements, row key to the
// inputHash the curator signed off on (§7).
func (l *Layered) Acknowledged() map[string]string { return l.acknowledged }
