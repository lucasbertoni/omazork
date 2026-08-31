// Package llm is the offline LLM inference pass (docs/action-waits.md §5.3)
// and its committed cache. It renders one prompt per row from the pinned
// extract, hashes it with the pinned model id, and keeps the answer in
// data/actions/<game>.llm.json — so regeneration reuses the cache and
// re-inference is a reviewable diff, never a silent drift.
//
// Nothing here clamps a bad answer: an out-of-band duration gets one retry
// with the band restated and then hard-fails the run.
package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lucasbertoni/omazork/internal/durations"
)

// SchemaVersion is stamped on every cache file; loading asserts an exact
// match, exactly as the other duration files do (§4).
const SchemaVersion = 1

// ModelID is the pinned model. It is hashed into every row's inputHash, so
// changing this line invalidates the whole cache for every game — which is
// the point: a new model is a re-inference event, reviewed as a diff.
const ModelID = "claude-opus-5"

// Kind names the three prompt templates, and with them what the pass is
// allowed to return (§5.3). It is part of the cache row so a row that changes
// kind re-infers.
type Kind string

const (
	// KindEdge is a static edge: rule-priced, then nudged inside the Movement
	// band. The answer must be movement-class.
	KindEdge Kind = "edge"
	// KindRoutineEdge is a PER-routine edge that still resolves to a
	// destination: full classification, any class.
	KindRoutineEdge Kind = "routineEdge"
	// KindHandler is a (verb, object) handler pair: full classification.
	KindHandler Kind = "handler"
)

// Request is one row's rendered prompt and the hash that pins it. The rule
// baseline an edge prompt is anchored on lives in the prompt itself — that is
// what makes editing it an invalidation.
type Request struct {
	Key    string
	Kind   Kind
	Prompt string
	Hash   string
}

// NewRequest hashes a rendered prompt into a request.
func NewRequest(key string, kind Kind, prompt string) Request {
	return Request{Key: key, Kind: kind, Prompt: prompt, Hash: Hash(prompt)}
}

// Hash is the row's inputHash: sha256 over the pinned model id and the whole
// rendered prompt, template and payload together (§5.3).
func Hash(prompt string) string {
	sum := sha256.Sum256([]byte(ModelID + prompt))
	return hex.EncodeToString(sum[:])
}

// Row is one cached answer, kept raw — a Dramatic nomination is stored as
// nominated, and only the generated table applies the 30-minute cap.
type Row struct {
	Key       string          `json:"key"`
	Kind      Kind            `json:"kind"`
	InputHash string          `json:"inputHash"`
	Class     durations.Class `json:"class"`
	Minutes   int             `json:"minutes"`
	Rationale string          `json:"rationale"`
}

// Cache is one committed data/actions/<game>.llm.json.
type Cache struct {
	SchemaVersion int    `json:"schemaVersion"`
	Game          string `json:"game"`
	Model         string `json:"model"`
	Rows          []Row  `json:"rows"`

	// Primed records whether the cache was read from a committed file. An
	// absent file is an unprimed pass: rows nothing has inferred yet stay
	// rule-priced instead of failing the run. Once a cache exists it is
	// expected to cover every row, and -check enforces that.
	Primed bool `json:"-"`

	index map[string]Row
}

// NewCache returns an empty, unprimed cache for a game.
func NewCache(game string) *Cache {
	return &Cache{SchemaVersion: SchemaVersion, Game: game, Model: ModelID, Primed: false}
}

// LoadCache reads a game's committed cache. A missing file is not an error —
// it is an unprimed pass.
func LoadCache(path, game string) (*Cache, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewCache(game), nil
	}
	if err != nil {
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%s: schemaVersion %d, want exactly %d", path, c.SchemaVersion, SchemaVersion)
	}
	if c.Game != game {
		return nil, fmt.Errorf("%s: cache is for game %q, want %q", path, c.Game, game)
	}
	c.Primed = true
	return &c, nil
}

// build lazily indexes the rows read off disk. Rows are the file's shape;
// the index is only ever this package's lookup.
func (c *Cache) build() {
	if c.index != nil {
		return
	}
	c.index = make(map[string]Row, len(c.Rows))
	for _, r := range c.Rows {
		c.index[r.Key] = r
	}
}

// Fresh returns the cached answer for a request when the row's inputHash
// still matches — a changed template, payload, or model id reads as absent.
func (c *Cache) Fresh(req Request) (Row, bool) {
	c.build()
	row, ok := c.index[req.Key]
	if !ok || row.InputHash != req.Hash || row.Kind != req.Kind {
		return Row{}, false
	}
	return row, true
}

// Put stores an answer, replacing any earlier one for the key. It also primes
// the cache: once something has been inferred there is a file worth writing.
func (c *Cache) Put(row Row) {
	c.build()
	if _, ok := c.index[row.Key]; ok {
		for i := range c.Rows {
			if c.Rows[i].Key == row.Key {
				c.Rows[i] = row
				c.index[row.Key] = row
				return
			}
		}
	}
	c.Rows = append(c.Rows, row)
	c.index[row.Key] = row
	c.Primed = true
}

// Prune drops cached answers for rows the generator no longer asks about, so
// a cache never outlives its extract. Dropped rows are returned for the run
// report.
func (c *Cache) Prune(live map[string]bool) []string {
	kept := c.Rows[:0]
	var dropped []string
	for _, r := range c.Rows {
		if live[r.Key] {
			kept = append(kept, r)
			continue
		}
		dropped = append(dropped, r.Key)
	}
	c.Rows = kept
	c.index = nil
	sort.Strings(dropped)
	return dropped
}

// JSON renders the cache deterministically: rows sorted by key, so a
// re-inference is a readable diff. It sorts and stamps the receiver in place —
// the committed file and the in-memory cache are meant to be the same thing.
func (c *Cache) JSON() ([]byte, error) {
	c.Model = ModelID
	c.SchemaVersion = SchemaVersion
	sort.Slice(c.Rows, func(i, j int) bool { return c.Rows[i].Key < c.Rows[j].Key })
	if c.Rows == nil {
		c.Rows = []Row{}
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// answer is the strict JSON contract every response must satisfy (§5.3).
type answer struct {
	Class     string `json:"class"`
	Minutes   int    `json:"minutes"`
	Rationale string `json:"rationale"`
}

// Decode parses one response against the contract and the request's kind. It
// is the same check the generator runs over a committed cache row, so a
// hand-edited cache fails exactly like a bad live answer.
func Decode(req Request, text string) (Row, error) {
	var a answer
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(text)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return Row{}, fmt.Errorf("malformed response %q: %w", clip(text), err)
	}
	if dec.More() {
		return Row{}, fmt.Errorf("malformed response %q: trailing content after the JSON object", clip(text))
	}
	row := Row{
		Key: req.Key, Kind: req.Kind, InputHash: req.Hash,
		Class: durations.Class(a.Class), Minutes: a.Minutes,
		Rationale: strings.Join(strings.Fields(a.Rationale), " "),
	}
	if err := Check(req.Kind, row.Class, row.Minutes); err != nil {
		return Row{}, err
	}
	if row.Rationale == "" {
		return Row{}, fmt.Errorf("empty rationale")
	}
	return row, nil
}

// Check enforces the §5.3 validator: class must be movement on a static edge,
// and minutes must sit inside the returned class's band. Nothing is clamped —
// a violation is the caller's to retry once and then fail on.
func Check(kind Kind, class durations.Class, minutes int) error {
	band, ok := durations.BandOf(class)
	if !ok {
		return fmt.Errorf("unknown class %q", class)
	}
	if kind == KindEdge && class != durations.ClassMovement {
		return fmt.Errorf("class %q on a static edge: static edges are movement, only the minutes are open", class)
	}
	if minutes < band.Low || minutes > band.High {
		return fmt.Errorf("%d min is outside the %s band %d-%d", minutes, class, band.Low, band.High)
	}
	return nil
}

// clip flattens a response to one short line for an error message: a model
// that ignored the contract can otherwise bury the reason in prose.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}
