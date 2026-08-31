package extract

import (
	"os"
	"path/filepath"
	"testing"
)

// Integration against the real pinned sources. Skips unless
// scripts/extract.sh has populated .cache/zil/<game>.
func TestExtractZork1(t *testing.T) {
	root := "../.."
	src := filepath.Join(root, ".cache", "zil", "zork1")
	if _, err := os.Stat(src); err != nil {
		t.Skip("pinned ZIL source not fetched; run scripts/extract.sh zork1")
	}
	story, err := os.ReadFile(filepath.Join(root, "assets", "games", "zork1.z3"))
	if err != nil {
		t.Fatal(err)
	}
	x, err := Run("zork1", src, story)
	if err != nil {
		t.Fatal(err)
	}

	if x.SchemaVersion != 1 || x.Game != "zork1" {
		t.Errorf("header = %d %q", x.SchemaVersion, x.Game)
	}
	if len(x.Rooms) != 110 {
		t.Errorf("rooms = %d, want 110", len(x.Rooms))
	}
	kinds := map[string]int{}
	for _, e := range x.Edges {
		kinds[e.Kind]++
	}
	// Counts confirmed against the research write-up (§5.1) and the source.
	want := map[string]int{"plain": 277, "blocked": 37, "cond_flag": 25, "cond_door": 6, "routine": 7}
	for k, n := range want {
		if kinds[k] != n {
			t.Errorf("%s edges = %d, want %d", k, kinds[k], n)
		}
	}
	// Static shapes must cover ~98% of edges (acceptance criterion).
	static := len(x.Edges) - kinds["routine"]
	if 100*static/len(x.Edges) < 97 {
		t.Errorf("static coverage %d/%d below expectation", static, len(x.Edges))
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
