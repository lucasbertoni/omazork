package calibrate

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lucasbertoni/omazork/internal/durations"
)

// tableFS is a small layered table with known statistics: two edges (4 and 6
// min), four generated action rows across three classes including a Dramatic
// nomination sitting at the uncurated cap, and an overlay that curates one
// row, adds a mechanism row at its class cap, and acknowledges keys.
func tableFS(acknowledged string) fstest.MapFS {
	return fstest.MapFS{
		"data/actions/verbs.json": &fstest.MapFile{Data: []byte(`{"schemaVersion":2,"fastVerbs":["look"],"verbs":[
 {"verb":"take","synonyms":["get"],"class":"manipulation","seconds":60},
 {"verb":"wait","class":"manipulation","seconds":0},
 {"verb":"dig","class":"mechanism","seconds":600}]}`)},
		"data/actions/zork1.json": &fstest.MapFile{Data: []byte(`{"schemaVersion":2,"game":"zork1",
 "rooms":[{"id":"KITCHEN","obj":10,"name":"Kitchen"},{"id":"CELLAR","obj":11,"name":"Cellar"}],
 "edges":[{"from":"KITCHEN","to":"CELLAR","dir":"DOWN","kind":"plain","seconds":240,"class":"movement","source":"rule"},
          {"from":"CELLAR","to":"KITCHEN","dir":"UP","kind":"plain","seconds":360,"class":"movement","source":"rule"}],
 "actions":[{"verb":"dig","object":"SAND","seconds":720,"class":"mechanism","source":"llm"},
            {"verb":"wait","object":"BOAT","seconds":120,"class":"manipulation","source":"llm"},
            {"verb":"take","object":"SAND","seconds":60,"class":"manipulation","source":"llm"},
            {"verb":"take","object":"WALL","seconds":1800,"class":"dramatic","source":"llm","note":"llm nominated dramatic 120m — overlay candidate"}],
 "vocab":{"verbs":{"get":"take","take":"take","dig":"dig","wait":"wait"},"nouns":{"sand":["SAND"],"boat":["BOAT"],"wall":["WALL"]}}}`)},
		"data/actions/zork1.overlay.json": &fstest.MapFile{Data: []byte(`{"schemaVersion":2,"game":"zork1",
 "actions":[{"verb":"take","object":"BOAT","seconds":540,"class":"mechanism","note":"curated"},
            {"verb":"dig","object":"WALL","seconds":1800,"class":"mechanism","note":"curated to the cap"}],
 "acknowledged":{` + acknowledged + `}}`)},
	}
}

func layered(t *testing.T, acknowledged string) *durations.Layered {
	t.Helper()
	l, err := durations.LoadFS(tableFS(acknowledged), "zork1")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func thresholds() *durations.Calibration {
	c := &durations.Calibration{SchemaVersion: 2}
	c.Replay.CumulativeHoursMin, c.Replay.CumulativeHoursMax = 15, 35
	c.Replay.MedianMovementSecondsMin, c.Replay.MedianMovementSecondsMax = 1*durations.Minute, 3*durations.Minute
	c.Replay.TurnsUnderQuietMin = 0.6
	c.Histogram.MeanEdgeSecondsMin, c.Histogram.MeanEdgeSecondsMax = 2*durations.Minute, 4*durations.Minute
	c.Histogram.DramaticRowsMax = 0.03
	c.Histogram.ClassMediansInner = true
	c.Rows.LongRowsMax = 5
	return c
}

func TestHistogram(t *testing.T) {
	edges, acts := layered(t, "").Rows()
	h := histogram(edges, acts)
	if h.Edges.Rows != 2 || h.Edges.Mean != 5*durations.Minute || h.Edges.Median != 5*durations.Minute {
		t.Fatalf("edges = %+v", h.Edges)
	}
	if h.Actions.Rows != 6 || h.Actions.Median != 10.5*durations.Minute {
		t.Fatalf("actions = %+v", h.Actions)
	}
	// 1 dramatic row of 6 layered action rows.
	if h.DramaticShare < 0.166 || h.DramaticShare > 0.167 {
		t.Fatalf("dramatic share = %v", h.DramaticShare)
	}
	mech := h.Classes[string(durations.ClassMechanism)]
	if mech.Rows != 3 || mech.Median != 12*durations.Minute || mech.InnerLow != 7.25*durations.Minute || mech.InnerHigh != 15.75*durations.Minute {
		t.Fatalf("mechanism = %+v", mech)
	}
	if len(h.LongRows) != 0 {
		t.Fatalf("long rows = %v, want none (30-min rows do not count)", h.LongRows)
	}
}

func TestGates(t *testing.T) {
	edges, acts := layered(t, "").Rows()
	h := histogram(edges, acts)
	rep := Replay{CumulativeHours: 20, MedianMovementSeconds: 2 * durations.Minute, TurnsUnderQuiet: 0.7}
	gates := evaluate(rep, h, thresholds())

	got := map[string]Gate{}
	for _, g := range gates {
		got[g.Name] = g
	}
	pass := []string{"replay.cumulativeHours", "replay.medianMovementSeconds", "replay.turnsUnderQuiet",
		"histogram.classMedian.movement", "histogram.classMedian.mechanism", "histogram.classMedian.manipulation", "rows.longRows"}
	for _, name := range pass {
		if g, ok := got[name]; !ok || !g.Pass {
			t.Errorf("gate %s = %+v, want pass", name, g)
		}
	}
	fail := []string{"histogram.meanEdgeSeconds", "histogram.dramaticShare"}
	for _, name := range fail {
		if g, ok := got[name]; !ok || g.Pass {
			t.Errorf("gate %s = %+v, want fail", name, g)
		}
	}
	if _, ok := got["histogram.classMedian.dramatic"]; ok {
		t.Error("dramatic class median gated: uncurated nominations sit at the cap by construction")
	}

	rep.CumulativeHours = 40
	var breach bool
	for _, g := range evaluate(rep, h, thresholds()) {
		if g.Name == "replay.cumulativeHours" && !g.Pass {
			breach = true
		}
	}
	if !breach {
		t.Error("40 h passed a 15–35 h gate")
	}
}

func TestCandidates(t *testing.T) {
	hashes := map[string]string{
		"edge:KITCHEN>CELLAR": "h-edge-1", "edge:CELLAR>KITCHEN": "h-edge-2",
		"action:dig/SAND": "h-dig-sand", "action:dig/WALL": "h-dig-wall",
		"action:wait/BOAT": "h-wait", "action:take/SAND": "h-take-sand", "action:take/WALL": "h-take-wall",
	}
	nominations := map[string]string{"action:take/WALL": "llm nominated dramatic 120m"}

	// Every row is a top-10 outlier in a table this small; what matters is the
	// reasons, the exclusion, and the re-surfacing.
	l := layered(t, `"action:dig/WALL":"h-dig-wall","action:take/WALL":"stale-hash"`)
	edges, acts := l.Rows()
	cands := candidates(edges, acts, l.Acknowledged(), hashes, nominations)
	byKey := map[string]Candidate{}
	for _, c := range cands {
		byKey[c.Key] = c
	}
	if _, ok := byKey["action:dig/WALL"]; ok {
		t.Error("acknowledged row with a matching hash still queued")
	}
	wall, ok := byKey["action:take/WALL"]
	if !ok {
		t.Fatal("acknowledged row whose input changed did not re-surface")
	}
	if !hasReason(wall, "dramatic nomination") || !hasReason(wall, "acknowledged") || wall.InputHash != "h-take-wall" {
		t.Errorf("re-surfaced nomination = %+v", wall)
	}
	if c := byKey["action:dig/SAND"]; !hasReason(c, "longest action rows") || hasReason(c, "class cap") {
		t.Errorf("dig/SAND = %+v", c)
	}
	if c := byKey["edge:CELLAR>KITCHEN"]; !hasReason(c, "longest edges") || c.Kind != "edge" {
		t.Errorf("edge = %+v", c)
	}
	if c := byKey["action:take/BOAT"]; c.InputHash == "" || c.Source != durations.SourceOverlay {
		t.Errorf("overlay-only row = %+v, want a content hash", c)
	}
	for i := 1; i < len(cands); i++ {
		if cands[i-1].Seconds < cands[i].Seconds {
			t.Fatalf("candidates not ordered longest first: %s (%d) before %s (%d)",
				cands[i-1].Key, cands[i-1].Seconds, cands[i].Key, cands[i].Seconds)
		}
	}

	// Once the curator pins the new hash, the nomination leaves the queue.
	l = layered(t, `"action:take/WALL":"h-take-wall"`)
	for _, c := range candidates(edges, acts, l.Acknowledged(), hashes, nominations) {
		if c.Key == "action:take/WALL" {
			t.Errorf("acknowledged nomination still queued: %+v", c)
		}
	}
}

func hasReason(c Candidate, want string) bool {
	for _, r := range c.Reasons {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}
