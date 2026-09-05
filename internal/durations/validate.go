package durations

import (
	"fmt"
	"sort"
	"strings"
)

// Validate checks every invariant §4 and §2 make a hard error: exact
// schemaVersions, orphaned overlay and acknowledged keys, class bands and
// caps, the uncurated 30-minute ceiling, and the reason required above 60
// minutes — all compared in seconds, the storage unit. It returns every problem it finds, in a stable order, so one run
// reports the whole repair list.
func Validate(t *Table, o *Overlay, v *Verbs) []error {
	c := &checker{table: t, overlay: o, verbs: v}
	c.schemaVersions()
	c.rooms()
	c.verbTable()
	c.baseRows()
	c.overlayRows()
	c.acknowledged()
	return c.errs
}

type checker struct {
	table   *Table
	overlay *Overlay
	verbs   *Verbs
	errs    []error

	roomIDs   map[string]bool
	objects   map[string]bool
	verbWords map[string]bool
	baseKeys  map[string]bool
	overKeys  map[string]bool
}

func (c *checker) fail(format string, args ...any) {
	c.errs = append(c.errs, fmt.Errorf(format, args...))
}

func (c *checker) schemaVersions() {
	if c.table.SchemaVersion != SchemaVersion {
		c.errs = append(c.errs, schemaErr(c.table.Game+" table", c.table.SchemaVersion))
	}
	if c.overlay.SchemaVersion != SchemaVersion {
		c.errs = append(c.errs, schemaErr(c.table.Game+" overlay", c.overlay.SchemaVersion))
	}
	if c.verbs.SchemaVersion != SchemaVersion {
		c.errs = append(c.errs, schemaErr("verbs", c.verbs.SchemaVersion))
	}
	if c.overlay.Game != "" && c.overlay.Game != c.table.Game {
		c.fail("overlay is for game %q, table is %q", c.overlay.Game, c.table.Game)
	}
}

func (c *checker) rooms() {
	c.roomIDs = map[string]bool{}
	objs := map[int]string{}
	for _, r := range c.table.Rooms {
		if c.roomIDs[r.ID] {
			c.fail("duplicate room %s", r.ID)
		}
		c.roomIDs[r.ID] = true
		if prev, ok := objs[r.Obj]; ok {
			c.fail("rooms %s and %s share object number %d", prev, r.ID, r.Obj)
		}
		objs[r.Obj] = r.ID
	}
	c.objects = map[string]bool{}
	for _, ids := range c.table.Vocab.Nouns {
		for _, id := range ids {
			c.objects[id] = true
		}
	}
}

func (c *checker) verbTable() {
	c.verbWords = map[string]bool{}
	seen := map[string]string{}
	for _, d := range c.verbs.Verbs {
		for _, word := range append([]string{d.Verb}, d.Synonyms...) {
			if prev, ok := seen[word]; ok {
				c.fail("verb word %q is claimed by both %s and %s", word, prev, d.Verb)
			}
			seen[word] = d.Verb
		}
		c.verbWords[d.Verb] = true
		if _, ok := BandOf(d.Class); !ok {
			c.fail("verb default %s: unknown class %q", d.Verb, d.Class)
			continue
		}
		c.checkBand(VerbDefaultKey(d.Verb), d.Class, d.Seconds, false)
	}
	for _, word := range sortedKeys(c.table.Vocab.Verbs) {
		if c.table.Vocab.Verbs[word] == "" {
			c.fail("vocab verb %q maps to nothing", word)
		}
	}
}

// checkBand enforces the §2 class table: a row's seconds must sit inside its
// class band and never above the class cap. Generated rows carry the extra
// uncurated ceiling — drama needs a human signature.
func (c *checker) checkBand(key string, class Class, seconds int, overlay bool) {
	band, ok := BandOf(class)
	if !ok {
		c.fail("%s: unknown class %q", key, class)
		return
	}
	if seconds < 0 {
		c.fail("%s: negative seconds %d", key, seconds)
		return
	}
	if seconds > band.Cap {
		c.fail("%s: %ds is above the %s cap of %ds", key, seconds, class, band.Cap)
		return
	}
	if overlay {
		return // an overlay row may sit anywhere under the cap, including 0 (force instant)
	}
	if seconds < band.Low || seconds > band.High {
		c.fail("%s: %ds is outside the %s band %ds-%ds", key, seconds, class, band.Low, band.High)
	}
	if seconds > UncuratedCap {
		c.fail("%s: %ds exceeds the uncurated cap of %ds — only an overlay row may", key, seconds, UncuratedCap)
	}
}

func (c *checker) baseRows() {
	c.baseKeys = map[string]bool{}
	for _, e := range c.table.Edges {
		key := EdgeKey(e.From, e.To)
		c.markBase(key)
		for _, id := range []string{e.From, e.To} {
			if !c.roomIDs[id] {
				c.fail("%s: %s is not a room in this table", key, id)
			}
		}
		if e.Source == "" {
			c.fail("%s: no source", key)
		}
		c.checkBand(key, e.Class, e.Seconds, false)
	}
	for _, a := range c.table.Actions {
		key := ActionKey(a.Verb, a.Object)
		c.markBase(key)
		c.checkAction(key, a.Verb, a.Object)
		if a.Source == "" {
			c.fail("%s: no source", key)
		}
		c.checkBand(key, a.Class, a.Seconds, false)
	}
}

func (c *checker) markBase(key string) {
	if c.baseKeys[key] {
		c.fail("duplicate row %s", key)
	}
	c.baseKeys[key] = true
}

// checkAction enforces the orphan rule for a (verb, object) key: both halves
// must still be vocabulary the generator emits or a verb the shared table
// knows.
func (c *checker) checkAction(key, verb, object string) {
	if c.table.Vocab.Verbs[verb] != verb && !c.verbWords[verb] {
		c.fail("%s: %q is not a canonical verb", key, verb)
	}
	if object != "" && !c.objects[object] {
		c.fail("%s: %s is not an object in this table's vocab", key, object)
	}
}

func (c *checker) overlayRows() {
	c.overKeys = map[string]bool{}
	mark := func(key string) {
		if c.overKeys[key] {
			c.fail("duplicate overlay row %s", key)
		}
		c.overKeys[key] = true
	}
	for _, e := range c.overlay.Edges {
		key := EdgeKey(e.From, e.To)
		mark(key)
		for _, id := range []string{e.From, e.To} {
			if !c.roomIDs[id] {
				c.fail("overlay %s: %s is not a room in this table", key, id)
			}
		}
		c.checkOverlaySeconds(key, e.Class, e.Seconds, e.Reason)
	}
	for _, a := range c.overlay.Actions {
		key := ActionKey(a.Verb, a.Object)
		mark(key)
		c.checkAction("overlay "+key, a.Verb, a.Object)
		c.checkOverlaySeconds(key, a.Class, a.Seconds, a.Reason)
	}
	for _, verb := range sortedKeys(c.overlay.VerbDefaults) {
		seconds := c.overlay.VerbDefaults[verb]
		key := VerbDefaultKey(verb)
		mark(key)
		if !c.verbWords[verb] {
			c.fail("overlay %s: %q is not a verb in the shared table", key, verb)
		}
		switch {
		case seconds < 0:
			c.fail("overlay %s: negative seconds %d", key, seconds)
		case seconds > ReasonThreshold:
			// §4 requires a reason above this line, and the verbDefaults shape
			// is a bare map with nowhere to put one — so that is where a verb
			// default stops. A longer duration belongs on a row that can be
			// justified.
			c.fail("overlay %s: %ds is above %ds — price a verb default that long as a row with a reason",
				key, seconds, ReasonThreshold)
		}
	}
}

func (c *checker) checkOverlaySeconds(key string, class Class, seconds int, reason string) {
	if class != "" {
		c.checkBand("overlay "+key, class, seconds, true)
	} else if seconds < 0 || seconds > bands[ClassDramatic].Cap {
		c.fail("overlay %s: %ds is outside 0-%ds", key, seconds, bands[ClassDramatic].Cap)
	}
	if seconds > ReasonThreshold && strings.TrimSpace(reason) == "" {
		c.fail("overlay %s: %ds needs a reason (above %ds)", key, seconds, ReasonThreshold)
	}
}

// acknowledged enforces that every curation acknowledgement still points at a
// live row: an ack whose row is gone would silently suppress a candidate.
func (c *checker) acknowledged() {
	for _, key := range sortedKeys(c.overlay.Acknowledged) {
		if c.overlay.Acknowledged[key] == "" {
			c.fail("acknowledged %s: empty inputHash", key)
		}
		if c.baseKeys[key] || c.overKeys[key] {
			continue
		}
		if !strings.HasPrefix(key, edgePrefix) && !strings.HasPrefix(key, actionPrefix) && !strings.HasPrefix(key, verbDefaultPrefix) {
			c.fail("acknowledged %q is not a row key", key)
			continue
		}
		c.fail("acknowledged %s matches no row in the table or overlay", key)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
