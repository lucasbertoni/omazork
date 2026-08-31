package extract

import (
	"strings"
	"testing"
)

// A miniature world exercising the correlation solver: two rooms named
// "Maze" that only exit structure can tell apart, plus uniquely-named rooms
// to seed direction-property discovery. Property numbers deliberately don't
// start at 31 — the solver must discover them, not assume them.
//
//	START (obj 5)  --NORTH(plain)--> MAZE-A (obj 9)
//	MAZE-A         --NORTH(plain)--> MAZE-B (obj 7)
//	MAZE-B         --EAST(plain)---> END (obj 3), --WEST(blocked)
//	END            --DOWN(cond_door TRAP via obj 12)--> START
const (
	propNorth = 30
	propEast  = 28
	propWest  = 27
	propDown  = 24
)

func mazeWorld(t *testing.T) *extractor {
	t.Helper()
	ex := testExtractor()
	mustFeed(t, ex, testDirections+`
<ROOM START (DESC "Start Room") (NORTH TO MAZE-A)>
<ROOM MAZE-A (DESC "Maze") (NORTH TO MAZE-B)>
<ROOM MAZE-B (DESC "Maze") (EAST TO END) (WEST "You can't go that way.")>
<ROOM END (DESC "End Room") (DOWN TO START IF TRAP-DOOR IS OPEN)>`)
	return ex
}

func mazeObjects() map[int]zObject {
	return map[int]zObject{
		3: {name: "End Room", props: map[int][]byte{propDown: {5, 12, 0, 0, 0}}},
		5: {name: "Start Room", props: map[int][]byte{propNorth: {9}}},
		7: {name: "Maze", props: map[int][]byte{propEast: {3}, propWest: {0, 0}}},
		9: {name: "Maze", props: map[int][]byte{propNorth: {7}}},
		// Distractors: a non-room object and an unrelated named thing.
		12: {name: "trap door", props: map[int][]byte{4: {1, 2}}},
		13: {name: "leaflet", props: map[int][]byte{propNorth: {1, 1}}},
	}
}

func TestCorrelateResolvesDuplicateNamesByExitStructure(t *testing.T) {
	ex := mazeWorld(t)
	if err := ex.correlateObjects(mazeObjects()); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"START": 5, "MAZE-A": 9, "MAZE-B": 7, "END": 3}
	for _, r := range ex.rooms {
		if r.Obj != want[r.ID] {
			t.Errorf("%s → obj %d, want %d", r.ID, r.Obj, want[r.ID])
		}
	}
}

func TestCorrelateFailsOnMissingName(t *testing.T) {
	ex := mazeWorld(t)
	objs := mazeObjects()
	delete(objs, 5)
	err := ex.correlateObjects(objs)
	if err == nil || !strings.Contains(err.Error(), "START") {
		t.Errorf("want missing-object error for START, got %v", err)
	}
}

func TestCorrelateFailsOnExitShapeMismatch(t *testing.T) {
	ex := mazeWorld(t)
	objs := mazeObjects()
	// END's DOWN exit becomes a plain exit in the story: shapes disagree.
	objs[3] = zObject{name: "End Room", props: map[int][]byte{propDown: {5}}}
	if err := ex.correlateObjects(objs); err == nil {
		t.Error("want error when story exit shape contradicts ZIL")
	}
}

func TestCorrelateFailsOnAmbiguity(t *testing.T) {
	ex := testExtractor()
	// Two identical rooms: nothing can tell them apart.
	mustFeed(t, ex, testDirections+`
<ROOM HUB (DESC "Hub") (NORTH TO TWIN-A) (SOUTH TO TWIN-B)>
<ROOM TWIN-A (DESC "Twin") (WEST "no")>
<ROOM TWIN-B (DESC "Twin") (WEST "no")>`)
	objs := map[int]zObject{
		1: {name: "Hub", props: map[int][]byte{propNorth: {2}, 25: {3}}},
		2: {name: "Twin", props: map[int][]byte{propWest: {0, 0}}},
		3: {name: "Twin", props: map[int][]byte{propWest: {0, 0}}},
	}
	err := ex.correlateObjects(objs)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("want ambiguity error, got %v", err)
	}
}
