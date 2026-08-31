package extract

import (
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/zil"
)

func testExtractor() *extractor {
	return &extractor{
		game:       "zork1",
		gameNumber: "1",
		dirSet:     map[string]bool{},
		synonymIx:  map[string]int{},
		routines:   map[string]routineDef{},
		roomIx:     map[string]*Room{},
		globals:    map[string]*zil.Node{},
	}
}

func feed(t *testing.T, ex *extractor, src string) error {
	t.Helper()
	nodes, err := zil.Parse([]byte(src), "test.zil")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return ex.topLevel(nodes, src)
}

func mustFeed(t *testing.T, ex *extractor, src string) {
	t.Helper()
	if err := feed(t, ex, src); err != nil {
		t.Fatal(err)
	}
}

const testDirections = `<DIRECTIONS NORTH EAST WEST SOUTH NE NW SE SW UP DOWN IN OUT LAND>`

func TestExitClassification(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, testDirections+`
<ROOM WEST-OF-HOUSE
      (IN ROOMS)
      (DESC "West of House")
      (NORTH TO NORTH-OF-HOUSE)
      (EAST "The door is boarded and you can't remove the boards.")
      (SW TO STONE-BARROW IF WON-FLAG)
      (WEST TO STRANGE-PASSAGE IF MAGIC-FLAG ELSE "The door is nailed shut.")
      (UP TO LIVING-ROOM IF TRAP-DOOR IS OPEN)
      (DOWN PER TRAP-DOOR-EXIT)
      (ACTION WEST-HOUSE)
      (FLAGS RLANDBIT ONBIT SACREDBIT)>`)

	if len(ex.rooms) != 1 {
		t.Fatalf("rooms = %d", len(ex.rooms))
	}
	r := ex.rooms[0]
	if r.ID != "WEST-OF-HOUSE" || r.Name != "West of House" || r.Action != "WEST-HOUSE" {
		t.Errorf("room = %+v", r)
	}
	if len(r.Flags) != 3 || r.Flags[0] != "RLANDBIT" {
		t.Errorf("flags = %v", r.Flags)
	}
	want := []Edge{
		{From: "WEST-OF-HOUSE", Dir: "NORTH", Kind: "plain", To: "NORTH-OF-HOUSE"},
		{From: "WEST-OF-HOUSE", Dir: "EAST", Kind: "blocked", Text: "The door is boarded and you can't remove the boards."},
		{From: "WEST-OF-HOUSE", Dir: "SW", Kind: "cond_flag", To: "STONE-BARROW", If: "WON-FLAG"},
		{From: "WEST-OF-HOUSE", Dir: "WEST", Kind: "cond_flag", To: "STRANGE-PASSAGE", If: "MAGIC-FLAG", Else: "The door is nailed shut."},
		{From: "WEST-OF-HOUSE", Dir: "UP", Kind: "cond_door", To: "LIVING-ROOM", Door: "TRAP-DOOR"},
		{From: "WEST-OF-HOUSE", Dir: "DOWN", Kind: "routine", Per: "TRAP-DOOR-EXIT"},
	}
	if len(ex.edges) != len(want) {
		t.Fatalf("edges = %d, want %d", len(ex.edges), len(want))
	}
	for i, w := range want {
		if *ex.edges[i] != w {
			t.Errorf("edge %d = %+v, want %+v", i, *ex.edges[i], w)
		}
	}
}

func TestUnknownExitShapeIsAnError(t *testing.T) {
	ex := testExtractor()
	err := feed(t, ex, testDirections+`
<ROOM BAD-ROOM (DESC "Bad") (NORTH SORRY "no")>`)
	if err == nil || !strings.Contains(err.Error(), "no known shape") {
		t.Errorf("want closed-taxonomy error, got %v", err)
	}
}

func TestInRoomsIsNotAnExit(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, testDirections+`
<ROOM STONE-BARROW (IN ROOMS) (DESC "Stone Barrow") (IN TO INSIDE-BARROW)>`)
	if len(ex.edges) != 1 || ex.edges[0].Dir != "IN" || ex.edges[0].To != "INSIDE-BARROW" {
		t.Errorf("edges = %+v", ex.edges)
	}
}

func TestCondDescentByZorkNumber(t *testing.T) {
	src := `
<SYNTAX APPLY OBJECT TO OBJECT = V-PUT PRE-PUT>
<COND (<==? ,ZORK-NUMBER 2>
       <SYNTAX ATTACK OBJECT (FIND ACTORBIT) (ON-GROUND IN-ROOM) = V-ATTACK>)>
<COND (<N==? ,ZORK-NUMBER 3>
       <SYNONYM GIVE HAND>)>
<COND (<GASSIGNED? ZILCH> <SYNTAX NEVER = V-NEVER>)>`

	ex := testExtractor()
	mustFeed(t, ex, src)
	if len(ex.syntax) != 1 || ex.syntax[0].Verb != "APPLY" {
		t.Errorf("zork1 syntax = %+v", ex.syntax)
	}
	if len(ex.synonyms) != 1 || ex.synonyms[0].Word != "GIVE" {
		t.Errorf("zork1 synonyms = %+v", ex.synonyms)
	}

	ex2 := testExtractor()
	ex2.gameNumber = "2"
	mustFeed(t, ex2, src)
	if len(ex2.syntax) != 2 || ex2.syntax[1].Verb != "ATTACK" || ex2.syntax[1].Action != "V-ATTACK" {
		t.Errorf("zork2 syntax = %+v", ex2.syntax)
	}
}

func TestSyntaxPatternAndHandlers(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, `
<SYNTAX TAKE OBJECT (FIND TAKEBIT) (ON-GROUND IN-ROOM MANY) = V-TAKE PRE-TAKE>
<SYNTAX JUMP OVER OBJECT = V-LEAP>
<SYNTAX VERBOSE = V-VERBOSE>`)
	want := []Syntax{
		{Verb: "TAKE", Pattern: []string{"OBJECT"}, Action: "V-TAKE", Preaction: "PRE-TAKE"},
		{Verb: "JUMP", Pattern: []string{"OVER", "OBJECT"}, Action: "V-LEAP"},
		{Verb: "VERBOSE", Action: "V-VERBOSE"},
	}
	if len(ex.syntax) != len(want) {
		t.Fatalf("syntax = %d rows", len(ex.syntax))
	}
	for i, w := range want {
		got := *ex.syntax[i]
		if got.Verb != w.Verb || got.Action != w.Action || got.Preaction != w.Preaction ||
			strings.Join(got.Pattern, " ") != strings.Join(w.Pattern, " ") {
			t.Errorf("syntax %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestSynonymMerging(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, `<SYNONYM GIVE DONATE OFFER FEED>
<SYNONYM GIVE HAND>`)
	if len(ex.synonyms) != 1 {
		t.Fatalf("synonyms = %+v", ex.synonyms)
	}
	if got := strings.Join(ex.synonyms[0].Synonyms, " "); got != "DONATE OFFER FEED HAND" {
		t.Errorf("merged = %q", got)
	}
}

func TestObjectExtraction(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, `
<OBJECT TRAP-DOOR
	(IN LIVING-ROOM)
	(SYNONYM DOOR TRAPDOOR TRAP-DOOR COVER)
	(ADJECTIVE TRAP DUSTY)
	(DESC "trap door")
	(FLAGS DOORBIT NDESCBIT INVISIBLE)
	(ACTION TRAP-DOOR-FCN)>`)
	if len(ex.objects) != 1 {
		t.Fatalf("objects = %d", len(ex.objects))
	}
	o := ex.objects[0]
	if o.ID != "TRAP-DOOR" || o.Name != "trap door" || o.Action != "TRAP-DOOR-FCN" ||
		len(o.Synonyms) != 4 || len(o.Adjectives) != 2 || len(o.Flags) != 3 {
		t.Errorf("object = %+v", o)
	}
}

func TestRoutineCaptureIsVerbatim(t *testing.T) {
	src := `<ROUTINE MAZE-DIODES ()
	 <TELL "You won't be able to get back up." CR CR>
	 <COND (<EQUAL? ,HERE ,MAZE-2> ,MAZE-4)>>`
	ex := testExtractor()
	mustFeed(t, ex, src)
	def, ok := ex.routines["MAZE-DIODES"]
	if !ok {
		t.Fatal("routine not indexed")
	}
	if def.src != src {
		t.Errorf("raw source = %q", def.src)
	}
	if def.line != 1 || def.file != "test.zil" {
		t.Errorf("location = %s:%d", def.file, def.line)
	}
}

func TestBuildResolvesReferencedRoutines(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, testDirections+`
<ROOM A-ROOM (DESC "A Room") (DOWN PER GO-DOWN) (ACTION A-ROOM-FCN)>
<ROUTINE GO-DOWN () <RTRUE>>
<ROUTINE A-ROOM-FCN (RARG) <RTRUE>>
<ROUTINE V-TAKE () <RTRUE>>
<SYNTAX TAKE OBJECT = V-TAKE>`)
	out, err := ex.build()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range out.Routines {
		names = append(names, r.Name)
	}
	if got := strings.Join(names, " "); got != "A-ROOM-FCN GO-DOWN V-TAKE" {
		t.Errorf("routines = %q", got)
	}
	if out.Handlers.Rooms["A-ROOM"] != "A-ROOM-FCN" {
		t.Errorf("handlers = %+v", out.Handlers)
	}
}

func TestBuildFailsOnMissingRoutine(t *testing.T) {
	ex := testExtractor()
	mustFeed(t, ex, `<SYNTAX TAKE OBJECT = V-TAKE>`)
	if _, err := ex.build(); err == nil || !strings.Contains(err.Error(), "V-TAKE") {
		t.Errorf("want missing-routine error, got %v", err)
	}
}
