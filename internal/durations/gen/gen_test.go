package gen

import (
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/durations/llm"
	"github.com/lucasbertoni/omazork/internal/extract"
)

func testExtract() *extract.Extract {
	return &extract.Extract{
		SchemaVersion: extract.SchemaVersion,
		Game:          "zork1",
		Rooms: []*extract.Room{
			{ID: "KITCHEN", Name: "Kitchen", Obj: 10, Flags: []string{"RLANDBIT", "ONBIT"}},
			{ID: "CELLAR", Name: "Cellar", Obj: 11, Flags: []string{"RLANDBIT"}},
			{ID: "RIVER-1", Name: "Frigid River", Obj: 12, Flags: []string{"NONLANDBIT", "ONBIT"}},
			{ID: "RIVER-2", Name: "Frigid River", Obj: 13, Flags: []string{"NONLANDBIT", "ONBIT"}},
			{ID: "ATTIC", Name: "Attic", Obj: 14, Flags: []string{"RLANDBIT"}},
		},
		Objects: []*extract.Object{
			{ID: "LAMP", Name: "brass lantern", Synonyms: []string{"LAMP", "LANTERN"}, Adjectives: []string{"BRASS"}},
			{ID: "TROLL", Name: "troll", Synonyms: []string{"TROLL"}},
		},
		Syntax: []*extract.Syntax{
			{Verb: "TAKE", Pattern: []string{"OBJECT"}, Action: "V-TAKE"},
			{Verb: "DIG", Pattern: []string{"OBJECT"}, Action: "V-DIG"},
			{Verb: "FROBOZZ", Action: "V-FROBOZZ"},
			{Verb: "LOOK", Action: "V-LOOK"},
			{Verb: "PRAY", Action: "V-PRAY"},
			{Verb: "ATTACK", Pattern: []string{"OBJECT"}, Action: "V-ATTACK"},
		},
		Synonyms: []extract.Synonym{{Word: "TAKE", Synonyms: []string{"GET", "GRAB"}}},
	}
}

func testVerbs() *durations.Verbs {
	return &durations.Verbs{
		SchemaVersion: durations.SchemaVersion,
		FastVerbs:     []string{"look", "pray"},
		Verbs: []durations.VerbDefault{
			{Verb: "take", Synonyms: []string{"get", "grab"}, Class: durations.ClassManipulation, Minutes: 1},
			{Verb: "dig", Class: durations.ClassMechanism, Minutes: 10},
		},
	}
}

func generate(t *testing.T, x *extract.Extract) *durations.Table {
	t.Helper()
	return generateWith(t, x, llm.NewCache(x.Game)).Table
}

func generateWith(t *testing.T, x *extract.Extract, cache *llm.Cache) *Result {
	t.Helper()
	result, err := Generate(x, testVerbs(), cache)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return result
}

func edge(t *testing.T, table *durations.Table, from, to string) durations.EdgeRow {
	t.Helper()
	for _, e := range table.Edges {
		if e.From == from && e.To == to {
			return e
		}
	}
	t.Fatalf("no edge %s -> %s in %+v", from, to, table.Edges)
	return durations.EdgeRow{}
}

func TestEdgePricingFollowsTheRuleFormula(t *testing.T) {
	cases := []struct {
		name    string
		edge    *extract.Edge
		minutes int
		note    string
	}{
		{"plain lit", &extract.Edge{From: "CELLAR", Dir: "EAST", Kind: extract.KindPlain, To: "KITCHEN"}, 2, "base 2"},
		{"up into the dark", &extract.Edge{From: "KITCHEN", Dir: "UP", Kind: extract.KindPlain, To: "ATTIC"}, 5, "base 2 + up 2 + dark 1"},
		{"down into the dark", &extract.Edge{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}, 4, "base 2 + down 1 + dark 1"},
		{"through a door", &extract.Edge{From: "KITCHEN", Dir: "WEST", Kind: extract.KindCondDoor, To: "ATTIC", Door: "TRAP-DOOR"}, 4, "base 2 + door 1 + dark 1"},
		{"conditional passage", &extract.Edge{From: "CELLAR", Dir: "NORTH", Kind: extract.KindCondFlag, To: "ATTIC", If: "MAGIC-FLAG"}, 4, "base 2 + conditional 1 + dark 1"},
		{"water", &extract.Edge{From: "RIVER-1", Kind: extract.KindDrift, To: "RIVER-2"}, 5, "base 2 + water 3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := testExtract()
			x.Edges = []*extract.Edge{tc.edge}
			row := edge(t, generate(t, x), tc.edge.From, tc.edge.To)
			if row.Minutes != tc.minutes {
				t.Fatalf("minutes = %d, want %d (note %q)", row.Minutes, tc.minutes, row.Note)
			}
			if row.Note != tc.note {
				t.Fatalf("note = %q, want %q", row.Note, tc.note)
			}
			if row.Class != durations.ClassMovement || row.Source != durations.SourceRule {
				t.Fatalf("provenance = %s/%s", row.Class, row.Source)
			}
		})
	}
}

func TestBlockedAndSelfEdgesAreNotRows(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{
		{From: "KITCHEN", Dir: "EAST", Kind: extract.KindBlocked, Text: "The door is boarded."},
		{From: "KITCHEN", Dir: "NORTH", Kind: extract.KindPlain, To: "KITCHEN"},
	}
	if rows := generate(t, x).Edges; len(rows) != 0 {
		t.Fatalf("want no rows, got %+v", rows)
	}
}

func TestDuplicatePairsCollapseToTheCheapest(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{
		{From: "KITCHEN", Dir: "UP", Kind: extract.KindPlain, To: "ATTIC"},       // 5
		{From: "KITCHEN", Dir: "WEST", Kind: extract.KindPlain, To: "ATTIC"},     // 3
		{From: "KITCHEN", Dir: "NORTH", Kind: extract.KindCondFlag, To: "ATTIC"}, // 4
	}
	table := generate(t, x)
	if len(table.Edges) != 1 {
		t.Fatalf("want one collapsed row, got %+v", table.Edges)
	}
	row := table.Edges[0]
	if row.Minutes != 3 || row.Dir != "WEST" {
		t.Fatalf("collapsed to %+v", row)
	}
	if !strings.Contains(row.Note, "cheapest of 3 exits") {
		t.Fatalf("no collision note: %q", row.Note)
	}
}

func TestRowsAreSortedAndRoomsMapped(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{
		{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"},
		{From: "ATTIC", Dir: "DOWN", Kind: extract.KindPlain, To: "KITCHEN"},
	}
	table := generate(t, x)
	if table.Edges[0].From != "ATTIC" {
		t.Fatalf("edges not sorted: %+v", table.Edges)
	}
	if len(table.Rooms) != 5 || table.Rooms[0].ID != "ATTIC" || table.Rooms[0].Obj != 14 {
		t.Fatalf("rooms = %+v", table.Rooms)
	}
	if table.Game != "zork1" || table.SchemaVersion != durations.SchemaVersion {
		t.Fatalf("header = %+v", table)
	}
}

func TestVocabMapsVerbsAndNouns(t *testing.T) {
	table := generate(t, testExtract())
	if got := table.Vocab.Verbs["get"]; got != "take" {
		t.Fatalf("get -> %q, want take (shared table canonical form)", got)
	}
	if got := table.Vocab.Verbs["frobozz"]; got != "frobozz" {
		t.Fatalf("frobozz -> %q, want itself", got)
	}
	if got := table.Vocab.Nouns["lantern"]; len(got) != 1 || got[0] != "LAMP" {
		t.Fatalf("lantern -> %v", got)
	}
}

func TestGeneratedTableValidates(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	table := generate(t, x)
	overlay := &durations.Overlay{SchemaVersion: durations.SchemaVersion, Game: "zork1"}
	if errs := durations.Validate(table, overlay, testVerbs()); len(errs) != 0 {
		t.Fatalf("generated table fails validation: %v", errs)
	}
}

func TestGenerateRejectsUnknownRooms(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "NOWHERE"}}
	if _, err := Generate(x, testVerbs(), nil); err == nil {
		t.Fatal("edge to an unknown room accepted")
	}
}

func TestWaterIsOnlyWater(t *testing.T) {
	x := testExtract()
	x.Rooms = append(x.Rooms,
		&extract.Room{ID: "VAIR-1", Name: "Volcano air", Obj: 20, Flags: []string{"NONLANDBIT", "NWALLBIT"}},
		&extract.Room{ID: "LEDGE", Name: "Ledge", Obj: 21, Flags: []string{"RLANDBIT", "NONLANDBIT", "ONBIT"}},
	)
	x.Edges = []*extract.Edge{
		{From: "VAIR-1", Kind: extract.KindDrift, To: "LEDGE"},
		{From: "LEDGE", Dir: "EAST", Kind: extract.KindPlain, To: "KITCHEN"},
	}
	table := generate(t, x)
	for _, row := range table.Edges {
		if strings.Contains(row.Note, "water") {
			t.Fatalf("balloon airspace priced as water: %+v", row)
		}
	}
}

func TestDriftMembershipSurvivesCollapse(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{
		{From: "RIVER-1", Dir: "DOWN", Kind: extract.KindPlain, To: "RIVER-2"},
		{From: "RIVER-1", Kind: extract.KindDrift, To: "RIVER-2"},
	}
	row := edge(t, generate(t, x), "RIVER-1", "RIVER-2")
	if row.Kind != extract.KindDrift {
		t.Fatalf("collapse dropped drift membership: %+v", row)
	}
	if row.Minutes != 5 {
		t.Fatalf("collapse did not take the cheapest exit: %+v", row)
	}
}
