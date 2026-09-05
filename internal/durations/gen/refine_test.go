package gen

import (
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/durations/llm"
	"github.com/lucasbertoni/omazork/internal/extract"
)

// handlerExtract adds an object handler the LLM pass has to classify: the lamp
// answers to TAKE and DIG, to LOOK and PRAY (both fast verbs), and to ATTACK
// (a melee verb the shared table refuses to price).
func handlerExtract() *extract.Extract {
	x := testExtract()
	x.Edges = nil
	x.Handlers.Objects = map[string]string{"LAMP": "LANTERN-F", "TROLL": "TROLL-F"}
	x.Routines = []extract.Routine{
		{Name: "LANTERN-F", Source: `<ROUTINE LANTERN-F () <COND (<VERB? TAKE DIG LOOK PRAY ATTACK> <RTRUE>)>>`},
		{Name: "TROLL-F", Source: `<ROUTINE TROLL-F () <COND (<EQUAL? ,PRSA ,V?TAKE> <RTRUE>)>>`},
	}
	return x
}

// request finds the request for a row key.
func request(t *testing.T, result *Result, key string) llm.Request {
	t.Helper()
	for _, req := range result.Requests {
		if req.Key == key {
			return req
		}
	}
	t.Fatalf("no request for %s", key)
	return llm.Request{}
}

func answer(req llm.Request, class durations.Class, minutes int, rationale string) llm.Row {
	return llm.Row{Key: req.Key, Kind: req.Kind, InputHash: req.Hash, Class: class, Minutes: minutes, Rationale: rationale}
}

// warm runs the generator twice: once to learn the requests, then again with
// the answers cached — exactly what actiongen does around an inference pass.
func warm(t *testing.T, x *extract.Extract, answers func(*Result, *llm.Cache)) *Result {
	t.Helper()
	cold := generateWith(t, x, llm.NewCache(x.Game))
	cache := llm.NewCache(x.Game)
	answers(cold, cache)
	return generateWith(t, x, cache)
}

func TestStaticEdgeIsNudgedInsideTheMovementBand(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	result := warm(t, x, func(cold *Result, cache *llm.Cache) {
		cache.Put(answer(request(t, cold, "edge:KITCHEN>CELLAR"), durations.ClassMovement, 7, "A steep drop into the dark."))
	})
	if len(result.Missing) != 0 {
		t.Fatalf("a warm cache still wants %d calls", len(result.Missing))
	}
	row := result.Table.Edges[0]
	if row.Seconds != 7*durations.Minute || row.Class != durations.ClassMovement {
		t.Fatalf("row = %+v", row)
	}
	if row.Source != durations.SourceRuleLLM {
		t.Fatalf("source = %q, want %q", row.Source, durations.SourceRuleLLM)
	}
	// The note keeps the rule arithmetic, names the nudge, and ends with the
	// model's own sentence — a curator reads all three.
	if want := "base 2 + down 1 + dark 1, llm 7 — A steep drop into the dark."; row.Note != want {
		t.Fatalf("note = %q, want %q", row.Note, want)
	}
}

func TestNudgeNoteKeepsTheCollisionNote(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{
		{From: "KITCHEN", Dir: "UP", Kind: extract.KindPlain, To: "ATTIC"},
		{From: "KITCHEN", Dir: "WEST", Kind: extract.KindPlain, To: "ATTIC"},
	}
	result := warm(t, x, func(cold *Result, cache *llm.Cache) {
		cache.Put(answer(request(t, cold, "edge:KITCHEN>ATTIC"), durations.ClassMovement, 2, "A short hop."))
	})
	note := result.Table.Edges[0].Note
	if !strings.Contains(note, "cheapest of 2 exits") || !strings.Contains(note, "llm 2") || !strings.HasSuffix(note, "A short hop.") {
		t.Fatalf("note = %q", note)
	}
}

func TestStaleCachedRowFallsBackToRulePricing(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	cache := llm.NewCache(x.Game)
	cache.Put(llm.Row{
		Key: "edge:KITCHEN>CELLAR", Kind: llm.KindEdge, InputHash: "hash of a template that no longer exists",
		Class: durations.ClassMovement, Minutes: 9, Rationale: "stale",
	})
	result := generateWith(t, x, cache)
	row := result.Table.Edges[0]
	if row.Seconds != 4*durations.Minute || row.Source != durations.SourceRule || strings.Contains(row.Note, "llm") {
		t.Fatalf("a stale answer was used: %+v", row)
	}
	if len(result.Missing) != 1 || result.Missing[0].Key != "edge:KITCHEN>CELLAR" {
		t.Fatalf("stale row not queued for re-inference: %+v", result.Missing)
	}
}

func TestCachedAnswerOutOfBandFailsGeneration(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	cold := generateWith(t, x, llm.NewCache(x.Game))
	for _, tc := range []struct {
		name    string
		class   durations.Class
		minutes int
	}{
		{"out of band", durations.ClassMovement, 40},
		{"not movement", durations.ClassMechanism, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := llm.NewCache(x.Game)
			cache.Put(answer(request(t, cold, "edge:KITCHEN>CELLAR"), tc.class, tc.minutes, "hand-edited"))
			if _, err := Generate(x, testVerbs(), cache); err == nil {
				t.Fatal("a hand-edited cache row was accepted")
			}
		})
	}
}

func TestHandlerPairsNeedTheCacheToBecomeRows(t *testing.T) {
	x := handlerExtract()
	cold := generateWith(t, x, llm.NewCache(x.Game))
	if len(cold.Table.Actions) != 0 {
		t.Fatalf("cold run invented rows: %+v", cold.Table.Actions)
	}
	keys := map[string]bool{}
	for _, req := range cold.Requests {
		keys[req.Key] = true
	}
	for _, want := range []string{"action:take/LAMP", "action:dig/LAMP", "action:take/TROLL"} {
		if !keys[want] {
			t.Fatalf("no request for %s (have %v)", want, keys)
		}
	}
	// LOOK is a fast verb and ATTACK is melee: both are hard classes the
	// wrapper prices before any row is consulted, so neither reaches the pass.
	if keys["action:look/LAMP"] {
		t.Fatal("a fast verb reached the LLM pass")
	}
	if keys["action:attack/LAMP"] {
		t.Fatal("a melee verb reached the LLM pass")
	}
	// PRAY is the documented exception (§10): instant everywhere except at an
	// altar, where it is a handler pair the pass prices.
	if !keys["action:pray/LAMP"] {
		t.Fatalf("prayer at a handler object was filtered out (have %v)", keys)
	}

	result := warm(t, x, func(cold *Result, cache *llm.Cache) {
		cache.Put(answer(request(t, cold, "action:take/LAMP"), durations.ClassManipulation, 2, "Lifting the lantern is quick."))
	})
	if len(result.Table.Actions) != 1 {
		t.Fatalf("actions = %+v", result.Table.Actions)
	}
	row := result.Table.Actions[0]
	if row.Verb != "take" || row.Object != "LAMP" || row.Seconds != 2*durations.Minute || row.Class != durations.ClassManipulation {
		t.Fatalf("row = %+v", row)
	}
	if row.Source != durations.SourceLLM || row.Note != "Lifting the lantern is quick." {
		t.Fatalf("provenance = %+v", row)
	}
	overlay := &durations.Overlay{SchemaVersion: durations.SchemaVersion, Game: "zork1"}
	if errs := durations.Validate(result.Table, overlay, testVerbs()); len(errs) != 0 {
		t.Fatalf("llm rows fail validation: %v", errs)
	}
}

func TestDramaticNominationIsCappedAndQueued(t *testing.T) {
	x := handlerExtract()
	result := warm(t, x, func(cold *Result, cache *llm.Cache) {
		cache.Put(answer(request(t, cold, "action:take/LAMP"), durations.ClassDramatic, 120, "Taking the lantern is a ceremony."))
	})
	row := result.Table.Actions[0]
	if row.Seconds != durations.UncuratedCap {
		t.Fatalf("nomination written at %ds — only an overlay row may exceed %ds", row.Seconds, durations.UncuratedCap)
	}
	if !strings.HasPrefix(row.Note, "llm nominated dramatic 120m — overlay candidate") {
		t.Fatalf("note = %q", row.Note)
	}
	if !strings.HasSuffix(row.Note, "Taking the lantern is a ceremony.") {
		t.Fatalf("the rationale is missing from the note: %q", row.Note)
	}
	overlay := &durations.Overlay{SchemaVersion: durations.SchemaVersion, Game: "zork1"}
	if errs := durations.Validate(result.Table, overlay, testVerbs()); len(errs) != 0 {
		t.Fatalf("capped nomination fails validation: %v", errs)
	}
}

func TestNoGeneratedRowExceedsTheUncuratedCap(t *testing.T) {
	x := handlerExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	result := warm(t, x, func(cold *Result, cache *llm.Cache) {
		for _, req := range cold.Requests {
			class, minutes := durations.ClassDramatic, 180
			if req.Kind == llm.KindEdge {
				class, minutes = durations.ClassMovement, 10
			}
			cache.Put(answer(req, class, minutes, "as long as the model dared"))
		}
	})
	for _, row := range result.Table.Actions {
		if row.Seconds > durations.UncuratedCap {
			t.Fatalf("%s/%s written at %ds", row.Verb, row.Object, row.Seconds)
		}
	}
	for _, row := range result.Table.Edges {
		if row.Seconds > durations.UncuratedCap {
			t.Fatalf("edge %s>%s written at %ds", row.From, row.To, row.Seconds)
		}
	}
}

func TestRequestsAreStableAcrossRuns(t *testing.T) {
	x := handlerExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	first := generateWith(t, x, llm.NewCache(x.Game))
	for i := 0; i < 5; i++ {
		again := generateWith(t, x, llm.NewCache(x.Game))
		if len(again.Requests) != len(first.Requests) {
			t.Fatalf("request count moved: %d then %d", len(first.Requests), len(again.Requests))
		}
		for j := range first.Requests {
			if again.Requests[j].Key != first.Requests[j].Key || again.Requests[j].Hash != first.Requests[j].Hash {
				t.Fatalf("request %d moved: %+v vs %+v", j, first.Requests[j], again.Requests[j])
			}
		}
	}
}

func TestWarmCacheMakesNoCallsAndReproducesTheSameBytes(t *testing.T) {
	x := handlerExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	cache := llm.NewCache(x.Game)
	for _, req := range generateWith(t, x, llm.NewCache(x.Game)).Requests {
		class, minutes := durations.ClassManipulation, 2
		if req.Kind == llm.KindEdge {
			class, minutes = durations.ClassMovement, 5
		}
		cache.Put(answer(req, class, minutes, "because."))
	}
	first := generateWith(t, x, cache)
	if len(first.Missing) != 0 {
		t.Fatalf("a warm cache would still call the API for %d rows", len(first.Missing))
	}
	want, err := first.Table.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		got, err := generateWith(t, x, cache).Table.JSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatal("regeneration from a warm cache is not byte-identical")
		}
	}
}

// TestEachKindHashesItsOwnTemplate is the invalidation guarantee (§5.3):
// because the rendered prompt is what gets hashed, and each kind renders its
// own template, editing one template can only move that kind's hashes.
func TestEachKindHashesItsOwnTemplate(t *testing.T) {
	x := handlerExtract()
	x.Edges = []*extract.Edge{{From: "KITCHEN", Dir: "DOWN", Kind: extract.KindPlain, To: "CELLAR"}}
	result := generateWith(t, x, llm.NewCache(x.Game))
	x.Edges = append(x.Edges, routineEdge())
	x.Routines = append(x.Routines, extract.Routine{Name: "CELLAR-EXIT", Source: "<ROUTINE CELLAR-EXIT () <RTRUE>>"})
	result = generateWith(t, x, llm.NewCache(x.Game))
	marker := map[llm.Kind]string{
		llm.KindEdge:        "You are pricing how long one passage",
		llm.KindRoutineEdge: "This exit runs a game routine",
		llm.KindHandler:     "You are pricing one player action",
	}
	kinds := map[llm.Kind]int{}
	for _, req := range result.Requests {
		if req.Hash != llm.Hash(req.Prompt) {
			t.Fatalf("%s: hash is not the hash of its rendered prompt", req.Key)
		}
		if !strings.Contains(req.Prompt, marker[req.Kind]) {
			t.Fatalf("%s: kind %s did not render its own template:\n%s", req.Key, req.Kind, req.Prompt)
		}
		for kind, other := range marker {
			if kind != req.Kind && strings.Contains(req.Prompt, other) {
				t.Fatalf("%s: kind %s leaked the %s template", req.Key, req.Kind, kind)
			}
		}
		kinds[req.Kind]++
	}
	for kind := range marker {
		if kinds[kind] == 0 {
			t.Fatalf("fixture did not exercise kind %s: %v", kind, kinds)
		}
	}
}

// routineEdge is a PER-routine exit that still names a destination. None of the
// three games ships one — every committed routine exit resolves its room at run
// time and so has no (from, to) key — but the pass classifies one outright when
// it appears, and that path needs the same guards as the others.
func routineEdge() *extract.Edge {
	return &extract.Edge{From: "ATTIC", Dir: "DOWN", Kind: extract.KindRoutine, To: "CELLAR", Per: "CELLAR-EXIT"}
}

func TestRoutineEdgeIsClassifiedOutright(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{routineEdge()}
	x.Routines = []extract.Routine{{Name: "CELLAR-EXIT", Source: "<ROUTINE CELLAR-EXIT () <RTRUE>>"}}
	cold := generateWith(t, x, llm.NewCache(x.Game))
	req := request(t, cold, "edge:ATTIC>CELLAR")
	if req.Kind != llm.KindRoutineEdge {
		t.Fatalf("kind = %s, want %s", req.Kind, llm.KindRoutineEdge)
	}
	if !strings.Contains(req.Prompt, "<ROUTINE CELLAR-EXIT") {
		t.Fatalf("the routine text is not in the payload:\n%s", req.Prompt)
	}
	// Unlike a static edge, a routine exit may come back as any class.
	cache := llm.NewCache(x.Game)
	cache.Put(answer(req, durations.ClassMechanism, 12, "The trapdoor has to be worked open."))
	row := generateWith(t, x, cache).Table.Edges[0]
	if row.Seconds != 12*durations.Minute || row.Class != durations.ClassMechanism {
		t.Fatalf("row = %+v", row)
	}
}

func TestDramaticNominationOnARoutineEdgeIsCappedToo(t *testing.T) {
	x := testExtract()
	x.Edges = []*extract.Edge{routineEdge()}
	x.Routines = []extract.Routine{{Name: "CELLAR-EXIT", Source: "<ROUTINE CELLAR-EXIT () <RTRUE>>"}}
	result := warm(t, x, func(cold *Result, cache *llm.Cache) {
		cache.Put(answer(request(t, cold, "edge:ATTIC>CELLAR"), durations.ClassDramatic, 120, "A descent into the underworld."))
	})
	row := result.Table.Edges[0]
	if row.Seconds != durations.UncuratedCap {
		t.Fatalf("nomination on an edge written at %ds, want the %ds cap", row.Seconds, durations.UncuratedCap)
	}
	if !strings.Contains(row.Note, "llm nominated dramatic 120m — overlay candidate") {
		t.Fatalf("note = %q", row.Note)
	}
	overlay := &durations.Overlay{SchemaVersion: durations.SchemaVersion, Game: "zork1"}
	if errs := durations.Validate(result.Table, overlay, testVerbs()); len(errs) != 0 {
		t.Fatalf("capped edge nomination fails validation: %v", errs)
	}
}

// TestNominationAtTheCapStillReachesTheQueue guards the boundary: a Dramatic
// answer of exactly 30 minutes needs no capping, but it is still a nomination
// and still has to carry the note that queues it for curation (§2, §7).
func TestNominationAtTheCapStillReachesTheQueue(t *testing.T) {
	x := handlerExtract()
	result := warm(t, x, func(cold *Result, cache *llm.Cache) {
		cache.Put(answer(request(t, cold, "action:take/LAMP"), durations.ClassDramatic, durations.UncuratedCap/durations.Minute, "A solemn lifting."))
	})
	row := result.Table.Actions[0]
	if row.Seconds != durations.UncuratedCap {
		t.Fatalf("seconds = %d", row.Seconds)
	}
	if !strings.Contains(row.Note, "overlay candidate") {
		t.Fatalf("a nomination at the cap never reached the curation queue: %q", row.Note)
	}
}
