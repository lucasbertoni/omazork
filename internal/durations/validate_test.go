package durations

import (
	"strings"
	"testing"
)

func fixture() (*Table, *Overlay, *Verbs) {
	table := &Table{
		SchemaVersion: SchemaVersion,
		Game:          "zork1",
		Rooms:         []Room{{ID: "KITCHEN", Obj: 10, Name: "Kitchen"}, {ID: "CELLAR", Obj: 11, Name: "Cellar"}},
		Edges: []EdgeRow{{From: "KITCHEN", To: "CELLAR", Dir: "DOWN", Kind: "plain",
			Seconds: 4 * Minute, Class: ClassMovement, Source: SourceRule}},
		Actions: []ActionRow{{Verb: "dig", Object: "SAND", Seconds: 12 * Minute, Class: ClassMechanism, Source: SourceLLM}},
		Vocab:   Vocab{Verbs: map[string]string{"dig": "dig", "take": "take"}, Nouns: map[string][]string{"sand": {"SAND"}}},
	}
	overlay := &Overlay{SchemaVersion: SchemaVersion, Game: "zork1"}
	verbs := &Verbs{SchemaVersion: SchemaVersion, FastVerbs: []string{"look"},
		Verbs: []VerbDefault{{Verb: "dig", Class: ClassMechanism, Seconds: 10 * Minute},
			{Verb: "take", Synonyms: []string{"get"}, Class: ClassManipulation, Seconds: 1 * Minute}}}
	return table, overlay, verbs
}

func validationError(t *testing.T, table *Table, overlay *Overlay, verbs *Verbs, want string) {
	t.Helper()
	errs := Validate(table, overlay, verbs)
	for _, err := range errs {
		if strings.Contains(err.Error(), want) {
			return
		}
	}
	t.Fatalf("no error containing %q; got %v", want, errs)
}

func TestValidateAcceptsCleanData(t *testing.T) {
	if errs := Validate(fixture()); len(errs) != 0 {
		t.Fatalf("clean data rejected: %v", errs)
	}
}

func TestValidateSchemaVersions(t *testing.T) {
	table, overlay, verbs := fixture()
	table.SchemaVersion = 1
	overlay.SchemaVersion = 0
	verbs.SchemaVersion = 7
	errs := Validate(table, overlay, verbs)
	if len(errs) < 3 {
		t.Fatalf("want a schemaVersion error per file, got %v", errs)
	}
}

func TestValidateOrphanedOverlayKeys(t *testing.T) {
	table, overlay, verbs := fixture()
	overlay.Edges = []OverlayEdge{{From: "KITCHEN", To: "ATTIC", Seconds: 3 * Minute}}
	overlay.Actions = []OverlayAction{{Verb: "dig", Object: "GRAVEL", Seconds: 3 * Minute}}
	overlay.VerbDefaults = map[string]int{"yodel": 2 * Minute}
	validationError(t, table, overlay, verbs, "ATTIC")
	validationError(t, table, overlay, verbs, "GRAVEL")
	validationError(t, table, overlay, verbs, "yodel")
}

func TestValidateOrphanedAcknowledgedKeys(t *testing.T) {
	table, overlay, verbs := fixture()
	overlay.Acknowledged = map[string]string{
		EdgeKey("KITCHEN", "CELLAR"): "abc",
		EdgeKey("KITCHEN", "ATTIC"):  "def",
		"nonsense":                   "ghi",
	}
	validationError(t, table, overlay, verbs, "KITCHEN>ATTIC")
	validationError(t, table, overlay, verbs, "nonsense")
	errs := Validate(table, overlay, verbs)
	for _, err := range errs {
		if strings.Contains(err.Error(), "KITCHEN>CELLAR") {
			t.Fatalf("live acknowledged key reported: %v", err)
		}
	}
}

func TestValidateBandsAndCaps(t *testing.T) {
	table, overlay, verbs := fixture()
	table.Edges[0].Seconds = 40 * Minute // above the movement cap and the uncurated cap
	validationError(t, table, overlay, verbs, "cap")

	table, overlay, verbs = fixture()
	table.Actions[0].Seconds = 1 * Minute // below the mechanism band
	validationError(t, table, overlay, verbs, "band")

	table, overlay, verbs = fixture()
	table.Actions[0].Class = "epic"
	validationError(t, table, overlay, verbs, "class")
}

func TestValidateUncuratedCapIsOverlayOnly(t *testing.T) {
	table, overlay, verbs := fixture()
	table.Actions[0].Seconds = 45 * Minute
	table.Actions[0].Class = ClassDramatic
	validationError(t, table, overlay, verbs, "uncurated")

	table, overlay, verbs = fixture()
	overlay.Actions = []OverlayAction{{Verb: "dig", Object: "SAND", Seconds: 45 * Minute, Class: ClassDramatic}}
	if errs := Validate(table, overlay, verbs); len(errs) != 0 {
		t.Fatalf("overlay row above the uncurated cap rejected: %v", errs)
	}
}

func TestValidateReasonRequiredAboveThreshold(t *testing.T) {
	table, overlay, verbs := fixture()
	overlay.Actions = []OverlayAction{{Verb: "dig", Object: "SAND", Seconds: 90 * Minute, Class: ClassDramatic}}
	validationError(t, table, overlay, verbs, "reason")

	overlay.Actions[0].Reason = "the exorcism is the centerpiece"
	if errs := Validate(table, overlay, verbs); len(errs) != 0 {
		t.Fatalf("reasoned overlay row rejected: %v", errs)
	}
}

func TestValidateRejectsDuplicateAndUnknownRows(t *testing.T) {
	table, overlay, verbs := fixture()
	table.Edges = append(table.Edges, table.Edges[0])
	validationError(t, table, overlay, verbs, "duplicate")

	table, overlay, verbs = fixture()
	table.Edges[0].To = "ATTIC"
	validationError(t, table, overlay, verbs, "ATTIC")
}
