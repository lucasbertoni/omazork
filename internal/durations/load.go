package durations

import (
	"encoding/json"
	"fmt"
	"io/fs"

	omazork "github.com/lucasbertoni/omazork"
)

// Layered is base + overlay + shared verb defaults resolved in memory (§4).
// Nothing on disk holds this shape: regeneration only ever rewrites the base
// table, so curation never has to be re-merged.
type Layered struct {
	Game  string
	Rooms []Room
	Vocab Vocab

	edges        map[string]Row
	actions      map[string]Row
	verbDefaults map[string]Row
	verbAliases  map[string]string
	fastVerbs    map[string]bool
	roomObj      map[string]int
	roomID       map[int]string
	drift        map[string]bool
	acknowledged map[string]string
}

// Load reads a game's duration data from the embedded data/actions files.
func Load(game string) (*Layered, error) { return LoadFS(omazork.Data, game) }

// LoadFS reads a game's duration data from fsys, layers it, and validates the
// result — an invalid overlay is a refusal to start, never a silent skip.
func LoadFS(fsys fs.FS, game string) (*Layered, error) {
	verbs, err := LoadVerbsFS(fsys)
	if err != nil {
		return nil, err
	}
	table, err := LoadTableFS(fsys, game)
	if err != nil {
		return nil, err
	}
	overlay, err := LoadOverlayFS(fsys, game)
	if err != nil {
		return nil, err
	}
	if errs := Validate(table, overlay, verbs); len(errs) > 0 {
		return nil, fmt.Errorf("durations: %s: %w", game, errs[0])
	}
	return Layer(table, overlay, verbs), nil
}

// LoadTableFS reads one generated table.
func LoadTableFS(fsys fs.FS, game string) (*Table, error) {
	var t Table
	if err := readJSON(fsys, "data/actions/"+game+".json", &t); err != nil {
		return nil, err
	}
	if t.SchemaVersion != SchemaVersion {
		return nil, schemaErr("durations: "+game+" table", t.SchemaVersion)
	}
	return &t, nil
}

// LoadOverlayFS reads one game's overlay; a missing overlay is not an error.
func LoadOverlayFS(fsys fs.FS, game string) (*Overlay, error) {
	name := "data/actions/" + game + ".overlay.json"
	if _, err := fs.Stat(fsys, name); err != nil {
		return &Overlay{SchemaVersion: SchemaVersion, Game: game}, nil
	}
	var o Overlay
	if err := readJSON(fsys, name, &o); err != nil {
		return nil, err
	}
	if o.SchemaVersion != SchemaVersion {
		return nil, schemaErr("durations: "+game+" overlay", o.SchemaVersion)
	}
	return &o, nil
}

// LoadVerbsFS reads the shared verb-default table.
func LoadVerbsFS(fsys fs.FS) (*Verbs, error) {
	var v Verbs
	if err := readJSON(fsys, "data/actions/verbs.json", &v); err != nil {
		return nil, err
	}
	if v.SchemaVersion != SchemaVersion {
		return nil, schemaErr("durations: verbs", v.SchemaVersion)
	}
	return &v, nil
}

// LoadCalibrationFS reads the shared calibration thresholds.
func LoadCalibrationFS(fsys fs.FS) (*Calibration, error) {
	var c Calibration
	if err := readJSON(fsys, "data/actions/calibration.json", &c); err != nil {
		return nil, err
	}
	if c.SchemaVersion != SchemaVersion {
		return nil, schemaErr("durations: calibration", c.SchemaVersion)
	}
	return &c, nil
}

// LoadCalibration reads the shared thresholds from the embedded data.
func LoadCalibration() (*Calibration, error) { return LoadCalibrationFS(omazork.Data) }

func readJSON(fsys fs.FS, name string, into any) error {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("durations: %s: %w", name, err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("durations: %s: %w", name, err)
	}
	return nil
}

// Layer resolves base + overlay + shared verb defaults in memory by the §4
// rules: overlay rows replace or join base rows, overlay verb defaults
// override the shared ones. It trusts its inputs — run Validate first, as
// LoadFS does.
func Layer(t *Table, o *Overlay, v *Verbs) *Layered {
	l := &Layered{
		Game:         t.Game,
		Rooms:        t.Rooms,
		Vocab:        t.Vocab,
		edges:        map[string]Row{},
		actions:      map[string]Row{},
		verbDefaults: map[string]Row{},
		verbAliases:  map[string]string{},
		fastVerbs:    map[string]bool{},
		roomObj:      map[string]int{},
		roomID:       map[int]string{},
		drift:        map[string]bool{},
		acknowledged: o.Acknowledged,
	}
	for _, r := range t.Rooms {
		l.roomObj[r.ID] = r.Obj
		l.roomID[r.Obj] = r.ID
	}
	for _, e := range t.Edges {
		l.edges[EdgeKey(e.From, e.To)] = Row{Seconds: e.Seconds, Class: e.Class, Source: e.Source, Note: e.Note}
		if e.Kind == KindDrift {
			l.drift[EdgeKey(e.From, e.To)] = true
		}
	}
	for _, a := range t.Actions {
		l.actions[ActionKey(a.Verb, a.Object)] = Row{Seconds: a.Seconds, Class: a.Class, Source: a.Source, Note: a.Note}
	}
	for _, d := range v.Verbs {
		l.verbDefaults[d.Verb] = Row{Seconds: d.Seconds, Class: d.Class, Source: SourceRule}
		l.verbAliases[d.Verb] = d.Verb
		for _, syn := range d.Synonyms {
			l.verbAliases[syn] = d.Verb
		}
	}
	for _, verb := range v.FastVerbs {
		l.fastVerbs[verb] = true
	}
	for _, e := range o.Edges {
		l.edges[EdgeKey(e.From, e.To)] = overlayRow(e.Seconds, e.Class, e.Note, e.Reason)
	}
	for _, a := range o.Actions {
		l.actions[ActionKey(a.Verb, a.Object)] = overlayRow(a.Seconds, a.Class, a.Note, a.Reason)
	}
	for verb, seconds := range o.VerbDefaults {
		row := l.verbDefaults[verb]
		row.Seconds = seconds
		row.Source = SourceOverlay
		l.verbDefaults[verb] = row
	}
	return l
}

func overlayRow(seconds int, class Class, note, reason string) Row {
	if note == "" {
		note = reason
	}
	return Row{Seconds: seconds, Class: class, Source: SourceOverlay, Note: note}
}

// Edge returns the layered row for a directed room edge.
func (l *Layered) Edge(from, to string) (Row, bool) {
	row, ok := l.edges[EdgeKey(from, to)]
	return row, ok
}

// Action returns the layered row for a (verb, object) pair. Choosing what to
// consult when a pair has no row is the runtime's precedence, not this
// package's.
func (l *Layered) Action(verb, object string) (Row, bool) {
	row, ok := l.actions[ActionKey(verb, object)]
	return row, ok
}

// VerbDefault returns the verb-only default, with any per-game override
// applied.
func (l *Layered) VerbDefault(verb string) (Row, bool) {
	if canon, ok := l.verbAliases[verb]; ok {
		verb = canon
	}
	row, ok := l.verbDefaults[verb]
	return row, ok
}

// FastVerb reports whether a verb is in the always-instant hard class (§10).
// The runtime and the calibration replay both enforce the class through the
// matcher in internal/actions, which carries the same list; this accessor
// serves the generator and validator.
func (l *Layered) FastVerb(verb string) bool { return l.fastVerbs[verb] }

// RoomObj resolves a ZIL room id to its z-machine object number (§3.1).
func (l *Layered) RoomObj(id string) (int, bool) {
	obj, ok := l.roomObj[id]
	return obj, ok
}

// RoomID resolves a z-machine object number back to its ZIL room id.
func (l *Layered) RoomID(obj int) (string, bool) {
	id, ok := l.roomID[obj]
	return id, ok
}

// CanonicalVerb normalizes a raw verb word through the generated vocab,
// returning the word unchanged when the vocab doesn't know it — a mismatch
// degrades to the verb-default row, never to an error (§4).
func (l *Layered) CanonicalVerb(word string) string {
	if canon, ok := l.Vocab.Verbs[word]; ok {
		return canon
	}
	return word
}

// Nouns returns the object ids a noun word can name, in table order.
func (l *Layered) Nouns(word string) []string { return l.Vocab.Nouns[word] }
