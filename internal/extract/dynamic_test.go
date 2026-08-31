package extract

import (
	"strings"
	"testing"
)

func TestDriftEdgesFromLadderTable(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, testDirections+`
<ROOM RIVER-1 (DESC "Frigid River")>
<ROOM RIVER-2 (DESC "Frigid River")>
<ROOM RIVER-3 (DESC "Frigid River")>
<GLOBAL RIVER-NEXT <LTABLE (PURE) RIVER-1 RIVER-2 RIVER-3>>`)
	if err := ex.driftEdges(); err != nil {
		t.Fatal(err)
	}
	want := []Edge{
		{From: "RIVER-1", Kind: "drift", To: "RIVER-2"},
		{From: "RIVER-2", Kind: "drift", To: "RIVER-3"},
	}
	if len(ex.edges) != len(want) {
		t.Fatalf("edges = %+v", ex.edges)
	}
	for i, w := range want {
		if *ex.edges[i] != w {
			t.Errorf("edge %d = %+v, want %+v", i, *ex.edges[i], w)
		}
	}
}

func TestDriftLadderMustNameRooms(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, testDirections+`
<ROOM RIVER-1 (DESC "Frigid River")>
<GLOBAL RIVER-NEXT <LTABLE (PURE) RIVER-1 NOT-A-ROOM>>`)
	err := ex.driftEdges()
	if err == nil || !strings.Contains(err.Error(), "NOT-A-ROOM") {
		t.Errorf("want non-room error, got %v", err)
	}
}

func TestDriftLadderGlobalMissing(t *testing.T) {
	ex := testExtractor()
	if err := ex.driftEdges(); err == nil || !strings.Contains(err.Error(), "RIVER-NEXT") {
		t.Errorf("want missing-global error, got %v", err)
	}
}

func TestCandidateEdgesFromRandomExitRoutine(t *testing.T) {
	ex := testExtractor()
	ex.game = "zork2"
	ex.gameNumber = "2"
	mustFeed(t, ex, testDirections+`
<ROOM MAGNET-ROOM (DESC "Low Room")
      (NORTH PER MAGNET-ROOM-EXIT)
      (SOUTH PER MAGNET-ROOM-EXIT)>
<ROOM MACHINE-ROOM (DESC "Machine Room")>
<ROOM TEA-ROOM (DESC "Tea Room")>
<ROUTINE MAGNET-ROOM-EXIT ()
	<COND (<PROB 50> ,MACHINE-ROOM)
	      (<EQUAL? ,PRSO ,P?EAST> ,MACHINE-ROOM)
	      (T ,TEA-ROOM)>>`)
	if err := ex.candidateEdges(); err != nil {
		t.Fatal(err)
	}
	// Two PER exits collapse to one candidate row per destination, in body
	// reference order, after the static edges.
	want := []Edge{
		{From: "MAGNET-ROOM", Dir: "NORTH", Kind: "routine", Per: "MAGNET-ROOM-EXIT"},
		{From: "MAGNET-ROOM", Dir: "SOUTH", Kind: "routine", Per: "MAGNET-ROOM-EXIT"},
		{From: "MAGNET-ROOM", Kind: "plain", To: "MACHINE-ROOM"},
		{From: "MAGNET-ROOM", Kind: "plain", To: "TEA-ROOM"},
	}
	if len(ex.edges) != len(want) {
		t.Fatalf("edges = %+v", ex.edges)
	}
	for i, w := range want {
		if *ex.edges[i] != w {
			t.Errorf("edge %d = %+v, want %+v", i, *ex.edges[i], w)
		}
	}
}

func TestGamesWithoutDynamicEdgesAddNothing(t *testing.T) {
	ex := testExtractor()
	ex.game = "zork3"
	ex.gameNumber = "3"
	mustFeed(t, ex, testDirections+`<ROOM MRG (DESC "Hallway")>`)
	if err := ex.driftEdges(); err != nil {
		t.Fatal(err)
	}
	if err := ex.candidateEdges(); err != nil {
		t.Fatal(err)
	}
	if len(ex.edges) != 0 {
		t.Errorf("edges = %+v", ex.edges)
	}
}
