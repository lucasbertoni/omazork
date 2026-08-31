// Package extract turns historicalsource ZIL into the per-game
// data/extract/<game>.json files: rooms, typed edges, syntax, handlers, and
// raw routine text — the pinned input for the duration generator and its LLM
// pass (docs/action-waits.md §4, §5.1).
//
// The exit taxonomy is closed (five shapes, confirmed by V-WALK); anything
// else is an extraction error, never a silent skip.
package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lucasbertoni/omazork/internal/zil"
)

// SchemaVersion is stamped on every extract file; consumers assert an exact
// match.
const SchemaVersion = 1

// Room is one <ROOM> form.
type Room struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`            // DESC short name, the z-machine object name
	Obj    int      `json:"obj"`             // z-machine object number (correlated)
	LDesc  string   `json:"ldesc,omitempty"` // LDESC long description
	Flags  []string `json:"flags,omitempty"`
	Action string   `json:"action,omitempty"`
}

// Edge kinds: the closed five-shape exit taxonomy (§5.1) plus "drift" —
// a room-to-room passage traversed by current or vehicle rather than a walk
// command (§3.4). Values are the JSON vocabulary shared with the generator.
const (
	KindPlain    = "plain"
	KindBlocked  = "blocked"
	KindCondFlag = "cond_flag"
	KindCondDoor = "cond_door"
	KindRoutine  = "routine"
	KindDrift    = "drift"
)

// Edge is one direction property on a room, classified into the closed
// five-shape taxonomy — or a synthesized row (§3.4): a drift edge, or a
// candidate edge for a random-destination walk exit. Synthesized rows carry
// no Dir.
type Edge struct {
	From string `json:"from"`
	Dir  string `json:"dir,omitempty"`
	Kind string `json:"kind"`           // one of the Kind* constants
	To   string `json:"to,omitempty"`   // absent on blocked
	If   string `json:"if,omitempty"`   // cond_flag: global flag
	Door string `json:"door,omitempty"` // cond_door: door object
	Else string `json:"else,omitempty"` // cond refusal text
	Text string `json:"text,omitempty"` // blocked refusal text
	Per  string `json:"per,omitempty"`  // routine: exit routine name
}

// Object is one <OBJECT> form (vocab nouns and LLM handler-pair context).
type Object struct {
	ID         string   `json:"id"`
	Name       string   `json:"name,omitempty"` // DESC
	Synonyms   []string `json:"synonyms,omitempty"`
	Adjectives []string `json:"adjectives,omitempty"`
	LDesc      string   `json:"ldesc,omitempty"`
	FDesc      string   `json:"fdesc,omitempty"`
	Text       string   `json:"text,omitempty"`
	Flags      []string `json:"flags,omitempty"`
	Action     string   `json:"action,omitempty"`
}

// Syntax is one <SYNTAX> grammar line. Pattern holds prepositions and the
// literal token "OBJECT" for object slots; FIND/scope constraints are
// dropped (wait keys don't need them).
type Syntax struct {
	Verb      string   `json:"verb"`
	Pattern   []string `json:"pattern,omitempty"`
	Action    string   `json:"action"`
	Preaction string   `json:"preaction,omitempty"`
}

// Synonym is one <SYNONYM> line: Word's aliases.
type Synonym struct {
	Word     string   `json:"word"`
	Synonyms []string `json:"synonyms"`
}

// Routine is a captured <ROUTINE> body, verbatim.
type Routine struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Source string `json:"source"`
}

// Extract is the whole per-game file.
type Extract struct {
	SchemaVersion int       `json:"schemaVersion"`
	Game          string    `json:"game"`
	Directions    []string  `json:"directions"`
	Rooms         []*Room   `json:"rooms"`
	Edges         []*Edge   `json:"edges"`
	Objects       []*Object `json:"objects"`
	Syntax        []*Syntax `json:"syntax"`
	Synonyms      []Synonym `json:"synonyms"`
	Handlers      Handlers  `json:"handlers"`
	Routines      []Routine `json:"routines"`
}

// Handlers maps room/object ids to their ACTION routine names.
type Handlers struct {
	Rooms   map[string]string `json:"rooms"`
	Objects map[string]string `json:"objects"`
}

type routineDef struct {
	file string
	line int
	src  string
}

type extractor struct {
	game       string // "zork1" | "zork2" | "zork3"
	gameNumber string // "1" | "2" | "3"

	directions []string
	dirSet     map[string]bool
	rooms      []*Room
	edges      []*Edge
	objects    []*Object
	syntax     []*Syntax
	synonyms   []Synonym
	synonymIx  map[string]int
	routines   map[string]routineDef
	roomIx     map[string]*Room
	globals    map[string]*zil.Node
}

// Run extracts one game from the ZIL sources in srcDir, correlating room
// object numbers against the story file bytes.
func Run(game, srcDir string, story []byte) (*Extract, error) {
	num := map[string]string{"zork1": "1", "zork2": "2", "zork3": "3"}[game]
	if num == "" {
		return nil, fmt.Errorf("unknown game %q", game)
	}
	ex := &extractor{
		game:       game,
		gameNumber: num,
		dirSet:     map[string]bool{},
		synonymIx:  map[string]int{},
		routines:   map[string]routineDef{},
		roomIx:     map[string]*Room{},
		globals:    map[string]*zil.Node{},
	}

	files, err := manifest(srcDir, game)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if err := ex.walkFile(f); err != nil {
			return nil, err
		}
	}

	if err := ex.validate(); err != nil {
		return nil, err
	}
	// Correlation matches the walk exits against the story file's exit
	// properties, so synthesized rows (drift, random-exit candidates —
	// they have no exit property) are appended only afterwards.
	if err := ex.correlate(story); err != nil {
		return nil, err
	}
	if err := ex.driftEdges(); err != nil {
		return nil, err
	}
	if err := ex.candidateEdges(); err != nil {
		return nil, err
	}
	return ex.build()
}

// manifest reads the root zorkN.zil and returns the <INSERT-FILE> list as
// absolute paths, resolved case-insensitively. Globbing *.zil is wrong —
// the repos carry uncompiled legacy files (§5.1).
func manifest(srcDir, game string) ([]string, error) {
	root, err := resolveFile(srcDir, game+".zil")
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(root)
	if err != nil {
		return nil, err
	}
	nodes, err := zil.Parse(src, filepath.Base(root))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, n := range nodes {
		if n.Kind != zil.KForm || n.Head() != "INSERT-FILE" || len(n.Kids) < 2 || n.Kids[1].Kind != zil.KString {
			continue
		}
		path, err := resolveFile(srcDir, n.Kids[1].Text+".zil")
		if err != nil {
			return nil, err
		}
		files = append(files, path)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: no <INSERT-FILE> forms found", root)
	}
	return files, nil
}

func resolveFile(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("%s: no file matching %q", dir, name)
}

func (ex *extractor) walkFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	nodes, err := zil.Parse(src, filepath.Base(path))
	if err != nil {
		return err
	}
	return ex.topLevel(nodes, string(src))
}

// topLevel walks top-level forms, descending into <COND> branches guarded on
// ,ZORK-NUMBER (the trilogy's shared files select per-game content that way).
func (ex *extractor) topLevel(nodes []*zil.Node, src string) error {
	for _, n := range nodes {
		if n.Kind != zil.KForm {
			continue // bare section-header strings
		}
		var err error
		switch n.Head() {
		case "DIRECTIONS":
			err = ex.directionsForm(n)
		case "ROOM":
			err = ex.room(n)
		case "OBJECT":
			err = ex.object(n)
		case "SYNTAX":
			err = ex.syntaxForm(n)
		case "SYNONYM", "VERB-SYNONYM", "PREP-SYNONYM", "ADJ-SYNONYM", "DIR-SYNONYM":
			err = ex.synonym(n)
		case "ROUTINE":
			err = ex.routine(n, src)
		case "GLOBAL":
			// Indexed opportunistically — a missing global errors at its
			// point of use (driftEdges). The arity guard makes Kids[2]
			// (the value) safe for consumers.
			if len(n.Kids) >= 3 && n.Kids[1].Kind == zil.KAtom {
				ex.globals[n.Kids[1].Text] = n
			}
		case "COND":
			err = ex.cond(n, src)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// cond evaluates a top-level <COND> clause guard against this game's
// ZORK-NUMBER and walks the first matching clause's body.
func (ex *extractor) cond(n *zil.Node, src string) error {
	for _, clause := range n.Kids[1:] {
		if clause.Kind != zil.KList || len(clause.Kids) == 0 {
			continue
		}
		ok, known := ex.guard(clause.Kids[0])
		if !known {
			return nil // compile-config COND (GASSIGNED? …), not game content
		}
		if ok {
			return ex.topLevel(clause.Kids[1:], src)
		}
	}
	return nil
}

// guard evaluates a COND guard. known is false for guards that aren't
// ZORK-NUMBER dispatch (or T/ELSE).
func (ex *extractor) guard(g *zil.Node) (value, known bool) {
	if g.IsAtom("T") || g.IsAtom("ELSE") {
		return true, true
	}
	if g.Kind != zil.KForm || len(g.Kids) != 3 || !g.Kids[1].IsAtom(",ZORK-NUMBER") {
		return false, false
	}
	eq := g.Kids[2].Atom() == ex.gameNumber
	switch g.Head() {
	case "==?", "=?":
		return eq, true
	case "N==?", "N=?":
		return !eq, true
	}
	return false, false
}

func (ex *extractor) directionsForm(n *zil.Node) error {
	for _, k := range n.Kids[1:] {
		d := k.Atom()
		if d == "" {
			return fmt.Errorf("%s:%d: non-atom in <DIRECTIONS>", n.File, n.Line)
		}
		ex.directions = append(ex.directions, d)
		ex.dirSet[d] = true
	}
	return nil
}

func (ex *extractor) room(n *zil.Node) error {
	if len(n.Kids) < 2 || n.Kids[1].Kind != zil.KAtom {
		return fmt.Errorf("%s:%d: <ROOM> without id", n.File, n.Line)
	}
	if len(ex.dirSet) == 0 {
		return fmt.Errorf("%s:%d: <ROOM> before <DIRECTIONS>", n.File, n.Line)
	}
	r := &Room{ID: n.Kids[1].Text}
	if ex.roomIx[r.ID] != nil {
		return fmt.Errorf("%s:%d: duplicate room %s", n.File, n.Line, r.ID)
	}
	for _, prop := range n.Kids[2:] {
		if prop.Kind != zil.KList || len(prop.Kids) == 0 {
			return fmt.Errorf("%s:%d: room %s: non-list property", n.File, n.Line, r.ID)
		}
		name := prop.Kids[0].Atom()
		vals := prop.Kids[1:]
		// (IN ROOMS) is the object-tree location, not the IN direction: a
		// direction property always continues TO / PER / "refusal".
		if name == "IN" && len(vals) == 1 && vals[0].Kind == zil.KAtom {
			continue
		}
		if ex.dirSet[name] {
			edge, err := classifyExit(r.ID, name, vals)
			if err != nil {
				return fmt.Errorf("%s:%d: %w", prop.File, prop.Line, err)
			}
			ex.edges = append(ex.edges, edge)
			continue
		}
		switch name {
		case "DESC":
			r.Name = stringVal(vals)
		case "LDESC":
			r.LDesc = stringVal(vals)
		case "FLAGS":
			r.Flags = atomList(vals)
		case "ACTION":
			r.Action = vals[0].Atom()
		}
	}
	if r.Name == "" {
		return fmt.Errorf("%s:%d: room %s has no DESC", n.File, n.Line, r.ID)
	}
	ex.rooms = append(ex.rooms, r)
	ex.roomIx[r.ID] = r
	return nil
}

// classifyExit maps a direction property to one of the five exit shapes.
// The taxonomy is closed; an unrecognized shape is an error by charter.
func classifyExit(room, dir string, vals []*zil.Node) (*Edge, error) {
	e := &Edge{From: room, Dir: dir}
	switch {
	case len(vals) == 1 && vals[0].Kind == zil.KString:
		e.Kind = KindBlocked
		e.Text = vals[0].Text
		return e, nil
	case len(vals) == 2 && vals[0].IsAtom("PER"):
		e.Kind = KindRoutine
		e.Per = vals[1].Atom()
		return e, nil
	case len(vals) >= 2 && vals[0].IsAtom("TO") && vals[1].Kind == zil.KAtom:
		e.To = vals[1].Text
		rest := vals[2:]
		if len(rest) == 0 {
			e.Kind = KindPlain
			return e, nil
		}
		if !rest[0].IsAtom("IF") || len(rest) < 2 {
			break
		}
		cond := rest[1].Atom()
		rest = rest[2:]
		if len(rest) >= 2 && rest[0].IsAtom("IS") && rest[1].IsAtom("OPEN") {
			e.Kind = KindCondDoor
			e.Door = cond
			rest = rest[2:]
		} else {
			e.Kind = KindCondFlag
			e.If = cond
		}
		if len(rest) == 0 {
			return e, nil
		}
		if len(rest) == 2 && rest[0].IsAtom("ELSE") && rest[1].Kind == zil.KString {
			e.Else = rest[1].Text
			return e, nil
		}
	}
	return nil, fmt.Errorf("room %s: exit (%s …) matches no known shape — the five-shape taxonomy is closed, refusing to guess", room, dir)
}

func (ex *extractor) object(n *zil.Node) error {
	if len(n.Kids) < 2 || n.Kids[1].Kind != zil.KAtom {
		return fmt.Errorf("%s:%d: <OBJECT> without id", n.File, n.Line)
	}
	o := &Object{ID: n.Kids[1].Text}
	for _, prop := range n.Kids[2:] {
		if prop.Kind != zil.KList || len(prop.Kids) == 0 {
			continue
		}
		vals := prop.Kids[1:]
		switch prop.Kids[0].Atom() {
		case "DESC":
			o.Name = stringVal(vals)
		case "SYNONYM":
			o.Synonyms = atomList(vals)
		case "ADJECTIVE":
			o.Adjectives = atomList(vals)
		case "LDESC":
			o.LDesc = stringVal(vals)
		case "FDESC":
			o.FDesc = stringVal(vals)
		case "TEXT":
			o.Text = stringVal(vals)
		case "FLAGS":
			o.Flags = atomList(vals)
		case "ACTION":
			// gglobals has one placeholder (ACTION 0): no handler.
			if a := vals[0].Atom(); a != "0" {
				o.Action = a
			}
		}
	}
	ex.objects = append(ex.objects, o)
	return nil
}

func (ex *extractor) syntaxForm(n *zil.Node) error {
	kids := n.Kids[1:]
	eq := -1
	for i, k := range kids {
		if k.IsAtom("=") {
			eq = i
			break
		}
	}
	if eq < 1 || eq+1 >= len(kids) {
		return fmt.Errorf("%s:%d: <SYNTAX> without = handler", n.File, n.Line)
	}
	s := &Syntax{Verb: kids[0].Atom()}
	if s.Verb == "" {
		return fmt.Errorf("%s:%d: <SYNTAX> verb is not an atom", n.File, n.Line)
	}
	for _, k := range kids[1:eq] {
		if k.Kind == zil.KAtom {
			s.Pattern = append(s.Pattern, k.Text)
		}
		// (FIND bit) / scope lists constrain parsing, not wait keys: dropped.
	}
	s.Action = kids[eq+1].Atom()
	if eq+2 < len(kids) {
		s.Preaction = kids[eq+2].Atom()
	}
	ex.syntax = append(ex.syntax, s)
	return nil
}

func (ex *extractor) synonym(n *zil.Node) error {
	if len(n.Kids) < 3 {
		return nil
	}
	word := n.Kids[1].Atom()
	if i, ok := ex.synonymIx[word]; ok {
		ex.synonyms[i].Synonyms = append(ex.synonyms[i].Synonyms, atomList(n.Kids[2:])...)
		return nil
	}
	ex.synonymIx[word] = len(ex.synonyms)
	ex.synonyms = append(ex.synonyms, Synonym{Word: word, Synonyms: atomList(n.Kids[2:])})
	return nil
}

func (ex *extractor) routine(n *zil.Node, src string) error {
	if len(n.Kids) < 2 {
		return fmt.Errorf("%s:%d: <ROUTINE> without name", n.File, n.Line)
	}
	name := n.Kids[1].Atom()
	ex.routines[name] = routineDef{file: n.File, line: n.Line, src: src[n.Start:n.End]}
	return nil
}

func stringVal(vals []*zil.Node) string {
	if len(vals) > 0 && vals[0].Kind == zil.KString {
		return vals[0].Text
	}
	return ""
}

func atomList(vals []*zil.Node) []string {
	var out []string
	for _, v := range vals {
		if a := v.Atom(); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// roomCounts are the games' known totals — a built-in sanity check on the
// manifest walk (§5.1).
var roomCounts = map[string]int{"zork1": 110, "zork2": 86, "zork3": 89}

func (ex *extractor) validate() error {
	if want := roomCounts[ex.game]; len(ex.rooms) != want {
		return fmt.Errorf("%s: extracted %d rooms, want %d", ex.game, len(ex.rooms), want)
	}
	for _, e := range ex.edges {
		if e.To != "" && ex.roomIx[e.To] == nil {
			return fmt.Errorf("edge %s %s: target %s is not a room", e.From, e.Dir, e.To)
		}
		if e.Kind == "routine" {
			if _, ok := ex.routines[e.Per]; !ok {
				return fmt.Errorf("edge %s %s: PER routine %s not found", e.From, e.Dir, e.Per)
			}
		}
	}
	return nil
}

// build assembles the final Extract, resolving the referenced-routine set:
// exit routines, room/object ACTION handlers, and syntax handlers.
func (ex *extractor) build() (*Extract, error) {
	out := &Extract{
		SchemaVersion: SchemaVersion,
		Game:          ex.game,
		Directions:    ex.directions,
		Rooms:         ex.rooms,
		Edges:         ex.edges,
		Objects:       ex.objects,
		Syntax:        ex.syntax,
		Synonyms:      ex.synonyms,
		Handlers:      Handlers{Rooms: map[string]string{}, Objects: map[string]string{}},
	}
	referenced := map[string]bool{}
	for _, r := range ex.rooms {
		if r.Action != "" {
			out.Handlers.Rooms[r.ID] = r.Action
			referenced[r.Action] = true
		}
	}
	for _, o := range ex.objects {
		if o.Action != "" {
			out.Handlers.Objects[o.ID] = o.Action
			referenced[o.Action] = true
		}
	}
	for _, e := range ex.edges {
		if e.Per != "" {
			referenced[e.Per] = true
		}
	}
	for _, s := range ex.syntax {
		referenced[s.Action] = true
		if s.Preaction != "" {
			referenced[s.Preaction] = true
		}
	}
	var missing []string
	for name := range referenced {
		def, ok := ex.routines[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		out.Routines = append(out.Routines, Routine{Name: name, File: def.file, Line: def.line, Source: def.src})
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("referenced routines not found: %s", strings.Join(missing, " "))
	}
	sortRoutines(out.Routines)
	return out, nil
}
