package durations

import (
	"testing"
	"testing/fstest"
)

const testVerbs = `{"schemaVersion":2,"fastVerbs":["look","pray"],"verbs":[
 {"verb":"take","synonyms":["get"],"class":"manipulation","seconds":60},
 {"verb":"dig","class":"mechanism","seconds":600}]}`

const testTable = `{"schemaVersion":2,"game":"zork1",
 "rooms":[{"id":"KITCHEN","obj":10,"name":"Kitchen"},{"id":"CELLAR","obj":11,"name":"Cellar"}],
 "edges":[{"from":"KITCHEN","to":"CELLAR","dir":"DOWN","kind":"plain","seconds":240,"class":"movement","source":"rule","note":"base 2 + down 1 + dark 1"}],
 "actions":[{"verb":"dig","object":"SAND","seconds":720,"class":"mechanism","source":"llm"}],
 "vocab":{"verbs":{"get":"take","take":"take"},"nouns":{"sand":["SAND"]}}}`

func testFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func baseFS() fstest.MapFS {
	return testFS(map[string]string{
		"data/actions/verbs.json": testVerbs,
		"data/actions/zork1.json": testTable,
	})
}

func loadOK(t *testing.T, fsys fstest.MapFS) *Layered {
	t.Helper()
	l, err := LoadFS(fsys, "zork1")
	if err != nil {
		t.Fatalf("LoadFS: %v", err)
	}
	return l
}

func TestLoadWithoutOverlay(t *testing.T) {
	l := loadOK(t, baseFS())

	row, ok := l.Edge("KITCHEN", "CELLAR")
	if !ok || row.Seconds != 4*Minute || row.Source != SourceRule {
		t.Fatalf("edge = %+v, %v", row, ok)
	}
	if row, ok := l.Action("dig", "SAND"); !ok || row.Seconds != 12*Minute {
		t.Fatalf("action = %+v, %v", row, ok)
	}
	if row, ok := l.VerbDefault("dig"); !ok || row.Seconds != 10*Minute || row.Class != ClassMechanism {
		t.Fatalf("verb default = %+v, %v", row, ok)
	}
	if !l.FastVerb("look") || l.FastVerb("dig") {
		t.Fatal("fast verbs not loaded")
	}
	if obj, ok := l.RoomObj("CELLAR"); !ok || obj != 11 {
		t.Fatalf("room obj = %d, %v", obj, ok)
	}
}

func TestOverlayOverridesAddsAndForcesInstant(t *testing.T) {
	fsys := baseFS()
	fsys["data/actions/zork1.overlay.json"] = &fstest.MapFile{Data: []byte(`{"schemaVersion":2,"game":"zork1",
	 "edges":[{"from":"KITCHEN","to":"CELLAR","seconds":0,"reason":"the trapdoor drop is instant"}],
	 "actions":[{"verb":"take","object":"SAND","seconds":5400,"class":"dramatic","reason":"a curated set piece"}],
	 "verbDefaults":{"dig":1200}}`)}
	l := loadOK(t, fsys)

	row, ok := l.Edge("KITCHEN", "CELLAR")
	if !ok || row.Seconds != 0 || row.Source != SourceOverlay {
		t.Fatalf("overridden edge = %+v, %v", row, ok)
	}
	added, ok := l.Action("take", "SAND")
	if !ok || added.Seconds != 90*Minute {
		t.Fatalf("added action = %+v, %v", added, ok)
	}
	if row, ok := l.VerbDefault("dig"); !ok || row.Seconds != 20*Minute || row.Source != SourceOverlay {
		t.Fatalf("overridden verb default = %+v, %v", row, ok)
	}
	if row, ok := l.Action("dig", "SAND"); !ok || row.Seconds != 12*Minute {
		t.Fatalf("untouched base action = %+v, %v", row, ok)
	}
}

func TestLoadRejectsSchemaMismatch(t *testing.T) {
	for _, file := range []string{"data/actions/zork1.json", "data/actions/verbs.json"} {
		fsys := baseFS()
		// A version-1 file held minutes; reading it as seconds would be silently
		// sixty times too short, so it is refused outright (ADR 0004).
		fsys[file] = &fstest.MapFile{Data: []byte(`{"schemaVersion":1,"game":"zork1"}`)}
		if _, err := LoadFS(fsys, "zork1"); err == nil {
			t.Fatalf("%s: schemaVersion 1 accepted", file)
		}
	}
}

func TestLoadRejectsInvalidData(t *testing.T) {
	fsys := baseFS()
	fsys["data/actions/zork1.overlay.json"] = &fstest.MapFile{Data: []byte(`{"schemaVersion":2,"game":"zork1",
	 "edges":[{"from":"KITCHEN","to":"ATTIC","seconds":180}]}`)}
	if _, err := LoadFS(fsys, "zork1"); err == nil {
		t.Fatal("orphaned overlay edge accepted at load")
	}
}

func TestVocabNormalizes(t *testing.T) {
	l := loadOK(t, baseFS())
	if got := l.CanonicalVerb("get"); got != "take" {
		t.Fatalf("CanonicalVerb(get) = %q", got)
	}
	if got := l.CanonicalVerb("frobnicate"); got != "frobnicate" {
		t.Fatalf("unknown verb normalized to %q", got)
	}
	if got := l.Nouns("sand"); len(got) != 1 || got[0] != "SAND" {
		t.Fatalf("Nouns(sand) = %v", got)
	}
}
