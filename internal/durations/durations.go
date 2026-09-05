// Package durations holds the action-wait duration data (docs/action-waits.md
// §4): the generated per-game table, the hand-curated overlay, the shared
// verb-default table, and the shared calibration thresholds. It reads them,
// layers overlay over base in memory — layering is a load-time act, there is
// no merged artifact on disk — and validates the invariants the spec makes
// hard errors.
package durations

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion is stamped on every duration file; loading asserts an exact
// match and refuses the file otherwise (§4). Version 2 stores every duration
// in seconds (ADR 0004); a version-1 file, whose values were minutes, is
// refused rather than silently read sixty times too short.
const SchemaVersion = 2

// Minute is the number of seconds in a minute. Every duration in this package
// is an integer count of seconds; the spec's bands, caps, and thresholds are
// written in minutes, so they are expressed here as N * Minute.
const Minute = 60

// Class is a row's inference class (§2). Presentation keys on the duration,
// never on class; class exists to bound what a row may cost.
type Class string

const (
	ClassInstant      Class = "instant"
	ClassMovement     Class = "movement"
	ClassManipulation Class = "manipulation"
	ClassMechanism    Class = "mechanism"
	ClassDramatic     Class = "dramatic"
)

// Band is one row of the §2 class table: the band inference must stay inside,
// and the hard cap no row of the class may exceed. BandOf returns it in
// seconds, the storage unit; llm.MinuteBand returns the same row in minutes.
type Band struct {
	Low  int
	High int
	Cap  int
}

var bands = map[Class]Band{
	ClassInstant:      {0, 0, 0},
	ClassMovement:     {1 * Minute, 10 * Minute, 15 * Minute},
	ClassManipulation: {0, 3 * Minute, 5 * Minute},
	ClassMechanism:    {3 * Minute, 20 * Minute, 30 * Minute},
	ClassDramatic:     {30 * Minute, 180 * Minute, 240 * Minute},
}

// BandOf returns the class's band, or false for an unknown class.
func BandOf(c Class) (Band, bool) {
	b, ok := bands[c]
	return b, ok
}

// Row provenance sources (§4).
const (
	SourceRule    = "rule"
	SourceLLM     = "llm"
	SourceRuleLLM = "rule+llm"
	SourceOverlay = "overlay"
)

// UncuratedCap is the ceiling on any generated row (§2): drama requires a
// human signature, so only an overlay row may exceed it.
const UncuratedCap = 30 * Minute

// ReasonThreshold is the duration above which an overlay row must carry a
// reason (§4).
const ReasonThreshold = 60 * Minute

// KindDrift marks an edge row in the drift set (§3.4): a passage traversed by
// current or vehicle. The generator copies the extract's kind onto the row;
// the runtime reads this value as drift-set membership.
const KindDrift = "drift"

// Row is a resolved duration in seconds with its provenance — what layering
// yields and the runtime consumes.
type Row struct {
	Seconds int
	Class   Class
	Source  string
	Note    string
}

// Key strings name a row in the overlay's acknowledged map and in validation
// messages. They are the only stable, file-writable row identity.

// Key prefixes, one per row shape.
const (
	edgePrefix        = "edge:"
	actionPrefix      = "action:"
	verbDefaultPrefix = "verbDefault:"
)

// EdgeKey names a directed room edge.
func EdgeKey(from, to string) string { return edgePrefix + from + ">" + to }

// ActionKey names a (verb, object) row, or a verb-only row when object is
// empty.
func ActionKey(verb, object string) string {
	if object == "" {
		return actionPrefix + verb
	}
	return actionPrefix + verb + "/" + object
}

// VerbDefaultKey names a per-game verb-default override.
func VerbDefaultKey(verb string) string { return verbDefaultPrefix + verb }

// Table is one generated per-game file: data/actions/<game>.json. Regenerated
// wholesale, never hand-edited.
type Table struct {
	SchemaVersion int         `json:"schemaVersion"`
	Game          string      `json:"game"`
	Rooms         []Room      `json:"rooms"`
	Edges         []EdgeRow   `json:"edges"`
	Actions       []ActionRow `json:"actions"`
	Vocab         Vocab       `json:"vocab"`
}

// Room resolves a curator-readable ZIL id to the z-machine object number the
// runtime matches on (§3.1).
type Room struct {
	ID   string `json:"id"`
	Obj  int    `json:"obj"`
	Name string `json:"name"`
}

// EdgeRow is one directed (from, to) passage. Dir is informational — one row
// per pair, cheapest exit wins (§4).
type EdgeRow struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Dir     string `json:"dir,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Seconds int    `json:"seconds"`
	Class   Class  `json:"class"`
	Source  string `json:"source"`
	Note    string `json:"note,omitempty"`
}

// ActionRow is one (verb, object) pair; Object empty means a verb-only row.
type ActionRow struct {
	Verb    string `json:"verb"`
	Object  string `json:"object,omitempty"`
	Seconds int    `json:"seconds"`
	Class   Class  `json:"class"`
	Source  string `json:"source"`
	Note    string `json:"note,omitempty"`
}

// Vocab is the generator-owned normalization vocabulary (§4): verb word to
// canonical verb, and object noun to the ids that noun can name. The overlay
// cannot touch it.
type Vocab struct {
	Verbs map[string]string   `json:"verbs"`
	Nouns map[string][]string `json:"nouns"`
}

// Overlay is one hand-curated file: data/actions/<game>.overlay.json. Humans
// only.
type Overlay struct {
	SchemaVersion int               `json:"schemaVersion"`
	Game          string            `json:"game"`
	Edges         []OverlayEdge     `json:"edges,omitempty"`
	Actions       []OverlayAction   `json:"actions,omitempty"`
	VerbDefaults  map[string]int    `json:"verbDefaults,omitempty"`
	Acknowledged  map[string]string `json:"acknowledged,omitempty"`
}

// OverlayEdge overrides — or adds — one edge row. Seconds 0 forces instant.
type OverlayEdge struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Seconds int    `json:"seconds"`
	Class   Class  `json:"class,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Note    string `json:"note,omitempty"`
}

// OverlayAction overrides — or adds — one (verb, object) row.
type OverlayAction struct {
	Verb    string `json:"verb"`
	Object  string `json:"object,omitempty"`
	Seconds int    `json:"seconds"`
	Class   Class  `json:"class,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Verbs is the shared hand-authored verb-default table, data/actions/verbs.json
// (§10). Fast verbs are listed for completeness; the wrapper enforces them as
// a hard class, not by table lookup.
type Verbs struct {
	SchemaVersion int           `json:"schemaVersion"`
	FastVerbs     []string      `json:"fastVerbs"`
	Verbs         []VerbDefault `json:"verbs"`
}

// VerbDefault is one verb-only fallback row.
type VerbDefault struct {
	Verb     string   `json:"verb"`
	Synonyms []string `json:"synonyms,omitempty"`
	Class    Class    `json:"class"`
	Seconds  int      `json:"seconds"`
	// Narration is the verb's progressive phrase for wait narration
	// (docs/action-waits.md §8): "climbing", "inflating". Player-facing.
	Narration string `json:"narration,omitempty"`
}

// Calibration is the shared threshold file, data/actions/calibration.json
// (§6). Every threshold is a generation-failing validation error; they live in
// one committed file so all three games gate identically.
type Calibration struct {
	SchemaVersion int `json:"schemaVersion"`
	Replay        struct {
		CumulativeHoursMin       float64 `json:"cumulativeHoursMin"`
		CumulativeHoursMax       float64 `json:"cumulativeHoursMax"`
		MedianMovementSecondsMin int     `json:"medianMovementSecondsMin"`
		MedianMovementSecondsMax int     `json:"medianMovementSecondsMax"`
		TurnsUnderQuietMin       float64 `json:"turnsUnderQuietMin"`
	} `json:"replay"`
	Histogram struct {
		MeanEdgeSecondsMin float64 `json:"meanEdgeSecondsMin"`
		MeanEdgeSecondsMax float64 `json:"meanEdgeSecondsMax"`
		DramaticRowsMax    float64 `json:"dramaticRowsMax"`
		ClassMediansInner  bool    `json:"classMediansInsideInnerHalf"`
	} `json:"histogram"`
	Rows struct {
		LongRowsMax int `json:"longRowsMax"`
	} `json:"rows"`
}

func schemaErr(what string, got int) error {
	return fmt.Errorf("%s: schemaVersion %d, want exactly %d", what, got, SchemaVersion)
}

// JSON renders a generated table deterministically: rows already sit in a
// stable order and encoding/json sorts map keys, so regeneration from the same
// extract is byte-identical.
func (t *Table) JSON() ([]byte, error) {
	out, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
