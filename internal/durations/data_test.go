package durations

import (
	"os"
	"testing"
)

// TestCommittedDataLoads runs the validator over exactly what ships: the
// generated tables, the hand-curated overlays, and the shared files.
func TestCommittedDataLoads(t *testing.T) {
	fsys := os.DirFS("../..")
	for _, game := range []string{"zork1", "zork2", "zork3"} {
		t.Run(game, func(t *testing.T) {
			l, err := LoadFS(fsys, game)
			if err != nil {
				t.Fatalf("LoadFS: %v", err)
			}
			if len(l.Rooms) == 0 || len(l.edges) == 0 {
				t.Fatalf("%s loaded empty: %d rooms, %d edges", game, len(l.Rooms), len(l.edges))
			}
			if _, ok := l.VerbDefault("get"); !ok {
				t.Fatal("shared verb defaults not layered in")
			}
			if !l.FastVerb("look") {
				t.Fatal("fast verbs not layered in")
			}
		})
	}
	if _, err := LoadCalibrationFS(fsys); err != nil {
		t.Fatalf("shared calibration thresholds: %v", err)
	}
}
