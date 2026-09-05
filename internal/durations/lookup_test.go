package durations

import (
	"testing"
	"testing/fstest"
)

// lookupFS is a table with a plain edge, a drift edge, a (verb, object) row, a
// verb-only row, and an overlay that overrides one action — enough to walk
// every §2 precedence slot.
func lookupFS() fstest.MapFS {
	fsys := testFS(map[string]string{
		"data/actions/verbs.json": testVerbs,
		"data/actions/zork1.json": `{"schemaVersion":2,"game":"zork1",
 "rooms":[{"id":"KITCHEN","obj":10,"name":"Kitchen"},{"id":"CELLAR","obj":11,"name":"Cellar"},
          {"id":"RIVER-1","obj":20,"name":"Frigid River"},{"id":"RIVER-2","obj":21,"name":"Frigid River"}],
 "edges":[{"from":"KITCHEN","to":"CELLAR","dir":"DOWN","kind":"plain","seconds":240,"class":"movement","source":"rule"},
          {"from":"RIVER-1","to":"RIVER-2","kind":"drift","seconds":360,"class":"movement","source":"rule"}],
 "actions":[{"verb":"dig","object":"SAND","seconds":720,"class":"mechanism","source":"llm"},
            {"verb":"wait","object":"BOAT","seconds":120,"class":"manipulation","source":"llm"},
            {"verb":"dig","seconds":420,"class":"mechanism","source":"llm"}],
 "vocab":{"verbs":{"get":"take","take":"take","dig":"dig","excavate":"dig","wait":"wait"},"nouns":{"sand":["SAND"],"boat":["BOAT"]}}}`,
		"data/actions/zork1.overlay.json": `{"schemaVersion":2,"game":"zork1",
 "actions":[{"verb":"take","object":"BOAT","seconds":540,"class":"mechanism","note":"curated"}]}`,
	})
	return fsys
}

func TestResolvePrecedence(t *testing.T) {
	l := loadOK(t, lookupFS())
	cases := []struct {
		name    string
		q       Query
		seconds int
		slot    Slot
		key     string
	}{
		{"hard class is instant whatever the table says", Query{Instant: true, Walk: true, Moved: true, From: 10, To: 11}, 0, SlotHard, ""},
		{"walk that changed the room prices the edge", Query{Walk: true, Moved: true, From: 10, To: 11}, 4 * Minute, SlotEdge, "edge:KITCHEN>CELLAR"},
		{"walk over an unlisted pair falls back", Query{Walk: true, Moved: true, From: 11, To: 10}, FallbackSeconds, SlotFallback, ""},
		{"(verb, object) row via the noun vocab", Query{Verb: "excavate", Object: "sand"}, 12 * Minute, SlotAction, "action:dig/SAND"},
		{"verb-only row when the object has none", Query{Verb: "dig", Object: "wall"}, 7 * Minute, SlotAction, "action:dig"},
		{"verb default through the shared synonyms", Query{Verb: "get", Object: "lamp"}, 1 * Minute, SlotVerbDefault, "verbDefault:take"},
		{"unknown verb falls back", Query{Verb: "frobnicate", Object: "lamp"}, FallbackSeconds, SlotFallback, ""},
		{"drift edge beats the (verb, object) row on a non-walk room change", Query{Moved: true, From: 20, To: 21, Verb: "wait", Object: "boat"}, 6 * Minute, SlotDrift, "edge:RIVER-1>RIVER-2"},
		{"teleport ignores a non-drift edge", Query{Moved: true, From: 10, To: 11, Verb: "wait", Object: "boat"}, 2 * Minute, SlotAction, "action:wait/BOAT"},
		{"overlay row beats the drift edge", Query{Moved: true, From: 20, To: 21, Verb: "take", Object: "boat"}, 9 * Minute, SlotOverlay, "action:take/BOAT"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := l.Resolve(c.q)
			if got.Row.Seconds != c.seconds || got.Slot != c.slot || got.Key != c.key {
				t.Fatalf("Resolve(%+v) = %ds, slot %s, key %q; want %d, %s, %q",
					c.q, got.Row.Seconds, got.Slot, got.Key, c.seconds, c.slot, c.key)
			}
		})
	}
}

func TestLayeredRows(t *testing.T) {
	l := loadOK(t, lookupFS())
	edges, acts := l.Rows()
	if len(edges) != 2 || len(acts) != 4 {
		t.Fatalf("Rows() = %d edges, %d actions; want 2, 4", len(edges), len(acts))
	}
	if edges[0].Key != "edge:KITCHEN>CELLAR" || edges[1].Key != "edge:RIVER-1>RIVER-2" {
		t.Fatalf("edges not in key order: %v %v", edges[0].Key, edges[1].Key)
	}
	var overlay int
	for _, a := range acts {
		if a.Source == SourceOverlay {
			overlay++
		}
	}
	if overlay != 1 {
		t.Fatalf("%d overlay rows in the layered actions, want 1", overlay)
	}
}
