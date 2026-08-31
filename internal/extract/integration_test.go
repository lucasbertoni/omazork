package extract

import (
	"os"
	"path/filepath"
	"testing"
)

// Integration against the real pinned sources. Skips unless
// scripts/extract.sh has populated .cache/zil/<game>.
func extractPinned(t *testing.T, game string) *Extract {
	t.Helper()
	root := "../.."
	src := filepath.Join(root, ".cache", "zil", game)
	if _, err := os.Stat(src); err != nil {
		t.Skipf("pinned ZIL source not fetched; run scripts/extract.sh %s", game)
	}
	story, err := os.ReadFile(filepath.Join(root, "assets", "games", game+".z3"))
	if err != nil {
		t.Fatal(err)
	}
	x, err := Run(game, src, story)
	if err != nil {
		t.Fatal(err)
	}
	if x.SchemaVersion != 1 || x.Game != game {
		t.Errorf("header = %d %q", x.SchemaVersion, x.Game)
	}
	// Every room resolved to a distinct object number.
	seen := map[int]string{}
	for _, r := range x.Rooms {
		if r.Obj <= 0 {
			t.Errorf("room %s: no object number", r.ID)
		}
		if prev, dup := seen[r.Obj]; dup {
			t.Errorf("rooms %s and %s share obj %d", prev, r.ID, r.Obj)
		}
		seen[r.Obj] = r.ID
	}
	// Every routine-typed edge carries raw routine text.
	routineBodies := map[string]bool{}
	for _, r := range x.Routines {
		routineBodies[r.Name] = r.Source != ""
	}
	for _, e := range x.Edges {
		if e.Kind == KindRoutine && !routineBodies[e.Per] {
			t.Errorf("exit routine %s has no raw body", e.Per)
		}
	}
	return x
}

func edgeKinds(x *Extract) map[string]int {
	kinds := map[string]int{}
	for _, e := range x.Edges {
		kinds[e.Kind]++
	}
	return kinds
}

// staticCoverage is the share of walk edges (drift and candidate rows are
// synthesized, not walk exits) that the five static shapes cover.
func staticCoverage(kinds map[string]int) int {
	walk := kinds["plain"] + kinds["blocked"] + kinds["cond_flag"] + kinds["cond_door"] + kinds["routine"]
	return 100 * (walk - kinds["routine"]) / walk
}

func driftPairs(x *Extract) map[string]bool {
	pairs := map[string]bool{}
	for _, e := range x.Edges {
		if e.Kind == KindDrift {
			pairs[e.From+">"+e.To] = true
		}
	}
	return pairs
}

func TestExtractZork1(t *testing.T) {
	x := extractPinned(t, "zork1")
	if len(x.Rooms) != 110 {
		t.Errorf("rooms = %d, want 110", len(x.Rooms))
	}
	kinds := edgeKinds(x)
	// Counts confirmed against the research write-up (§5.1) and the source.
	// Drift: the four consecutive RIVER-NEXT pairs (§3.4).
	want := map[string]int{"plain": 277, "blocked": 37, "cond_flag": 25, "cond_door": 6, "routine": 7, "drift": 4}
	for k, n := range want {
		if kinds[k] != n {
			t.Errorf("%s edges = %d, want %d", k, kinds[k], n)
		}
	}
	// Static shapes must cover ~98% of edges (acceptance criterion).
	if c := staticCoverage(kinds); c < 97 {
		t.Errorf("static coverage %d%% below expectation", c)
	}

	pairs := driftPairs(x)
	for _, p := range []string{"RIVER-1>RIVER-2", "RIVER-2>RIVER-3", "RIVER-3>RIVER-4", "RIVER-4>RIVER-5"} {
		if !pairs[p] {
			t.Errorf("missing river drift edge %s", p)
		}
	}

	// Spot-checks from the sources.
	rooms := map[string]*Room{}
	for _, r := range x.Rooms {
		rooms[r.ID] = r
	}
	if rooms["WEST-OF-HOUSE"] == nil || rooms["WEST-OF-HOUSE"].Name != "West of House" {
		t.Error("WEST-OF-HOUSE missing or misnamed")
	}
	routineBodies := map[string]bool{}
	for _, r := range x.Routines {
		routineBodies[r.Name] = r.Source != ""
	}
	for _, per := range []string{"GRATING-EXIT", "TRAP-DOOR-EXIT", "UP-CHIMNEY-FUNCTION", "MAZE-DIODES"} {
		if !routineBodies[per] {
			t.Errorf("exit routine %s has no raw body", per)
		}
	}
}

func TestExtractZork2(t *testing.T) {
	x := extractPinned(t, "zork2")
	if len(x.Rooms) != 86 {
		t.Errorf("rooms = %d, want 86", len(x.Rooms))
	}
	kinds := edgeKinds(x)
	// Static shapes must cover ~95% of edges (acceptance criterion).
	if c := staticCoverage(kinds); c < 94 {
		t.Errorf("static coverage %d%% below expectation", c)
	}
	// Drift: the balloon ladder, up and down (§3.4).
	wantDrift := []string{
		"VAIR-1>VAIR-2", "VAIR-2>VAIR-3", "VAIR-3>VAIR-4",
		"VAIR-4>VAIR-3", "VAIR-3>VAIR-2", "VAIR-2>VAIR-1",
	}
	pairs := driftPairs(x)
	if kinds["drift"] != len(wantDrift) {
		t.Errorf("drift edges = %d, want %d", kinds["drift"], len(wantDrift))
	}
	for _, p := range wantDrift {
		if !pairs[p] {
			t.Errorf("missing balloon drift edge %s", p)
		}
	}
	// Random-destination candidates (§3.4): the Low Room magnet exit gets
	// one plain row per candidate; the Carousel Room's candidates are its
	// eight static plain exits.
	plain := map[string]int{}
	carousel := 0
	for _, e := range x.Edges {
		if e.Kind != KindPlain {
			continue
		}
		plain[e.From+">"+e.To]++
		if e.From == "CAROUSEL-ROOM" {
			carousel++
		}
	}
	for _, p := range []string{"MAGNET-ROOM>MACHINE-ROOM", "MAGNET-ROOM>TEA-ROOM"} {
		if plain[p] != 1 {
			t.Errorf("candidate edge %s: %d rows, want 1", p, plain[p])
		}
	}
	if carousel != 8 {
		t.Errorf("CAROUSEL-ROOM plain exits = %d, want 8", carousel)
	}
}

func TestExtractZork3(t *testing.T) {
	x := extractPinned(t, "zork3")
	if len(x.Rooms) != 89 {
		t.Errorf("rooms = %d, want 89", len(x.Rooms))
	}
	kinds := edgeKinds(x)
	// Static shapes must cover ~83% of edges (acceptance criterion); the
	// mirror box accounts for the routine-heavy remainder.
	if c := staticCoverage(kinds); c < 80 {
		t.Errorf("static coverage %d%% below expectation", c)
	}
	// Mirror-box FEXITs are deliberately not enumerated: no synthesized rows.
	if kinds["drift"] != 0 {
		t.Errorf("drift edges = %d, want 0", kinds["drift"])
	}
	// The identically-shaped mirror antechambers must still resolve.
	rooms := map[string]*Room{}
	for _, r := range x.Rooms {
		rooms[r.ID] = r
	}
	for _, pair := range [][2]string{{"MRDE", "MRDW"}, {"MRG", "MRC"}, {"MRG", "MRB"}} {
		a, b := rooms[pair[0]], rooms[pair[1]]
		if a == nil || b == nil {
			t.Fatalf("mirror rooms %v missing", pair)
		}
		if a.Obj == b.Obj {
			t.Errorf("%s and %s share obj %d", pair[0], pair[1], a.Obj)
		}
	}
}
