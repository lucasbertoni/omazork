package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasbertoni/omazork/internal/durations"
)

func edgeReq(prompt string) Request { return NewRequest("edge:A>B", KindEdge, prompt) }

func TestHashPinsThePromptAndTheModel(t *testing.T) {
	first := edgeReq("price this edge")
	if first.Hash != Hash("price this edge") {
		t.Fatal("hash is not a function of the prompt alone")
	}
	if first.Hash == Hash("price this edge ") {
		t.Fatal("a changed prompt kept its hash — a template edit would not invalidate")
	}
	if first.Hash == Hash("price this edge"[1:]) {
		t.Fatal("unrelated prompts collide")
	}
	// The model id is inside the hash, so pinning a new model invalidates
	// every row of every game.
	if got := Hash(""); got == Hash(ModelID) {
		t.Fatal("model id is not part of the hash")
	}
}

func TestCacheReusesOnlyFreshRows(t *testing.T) {
	req := edgeReq("prompt one")
	cache := NewCache("zork1")
	if _, ok := cache.Fresh(req); ok {
		t.Fatal("empty cache answered")
	}
	cache.Put(Row{Key: req.Key, Kind: req.Kind, InputHash: req.Hash, Class: durations.ClassMovement, Minutes: 4, Rationale: "a long crawl"})
	if row, ok := cache.Fresh(req); !ok || row.Minutes != 4 {
		t.Fatalf("warm row not reused: %+v %v", row, ok)
	}
	// Editing the template rehashes exactly this kind's rows.
	if _, ok := cache.Fresh(edgeReq("prompt two")); ok {
		t.Fatal("a stale hash was reused")
	}
	// A row that changed kind re-infers rather than reusing an answer the new
	// contract might reject.
	if _, ok := cache.Fresh(NewRequest(req.Key, KindHandler, "prompt one")); ok {
		t.Fatal("a row of a different kind was reused")
	}
}

func TestPutReplacesInPlace(t *testing.T) {
	req := edgeReq("prompt")
	cache := NewCache("zork1")
	cache.Put(Row{Key: req.Key, Kind: req.Kind, InputHash: req.Hash, Minutes: 4})
	cache.Put(Row{Key: req.Key, Kind: req.Kind, InputHash: req.Hash, Minutes: 7})
	if len(cache.Rows) != 1 || cache.Rows[0].Minutes != 7 {
		t.Fatalf("rows = %+v", cache.Rows)
	}
}

func TestPruneDropsRowsTheGeneratorNoLongerAsksAbout(t *testing.T) {
	cache := NewCache("zork1")
	cache.Put(Row{Key: "edge:A>B", Minutes: 2})
	cache.Put(Row{Key: "edge:GONE>AWAY", Minutes: 2})
	dropped := cache.Prune(map[string]bool{"edge:A>B": true})
	if len(dropped) != 1 || dropped[0] != "edge:GONE>AWAY" {
		t.Fatalf("dropped = %v", dropped)
	}
	if len(cache.Rows) != 1 {
		t.Fatalf("rows = %+v", cache.Rows)
	}
	if _, ok := cache.Fresh(NewRequest("edge:GONE>AWAY", KindEdge, "x")); ok {
		t.Fatal("pruned row still answers")
	}
}

func TestCacheJSONIsSortedAndStamped(t *testing.T) {
	cache := NewCache("zork1")
	cache.Put(Row{Key: "edge:B>C"})
	cache.Put(Row{Key: "edge:A>B"})
	raw, err := cache.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var back Cache
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.SchemaVersion != SchemaVersion || back.Model != ModelID {
		t.Fatalf("header = %+v", back)
	}
	if back.Rows[0].Key != "edge:A>B" {
		t.Fatalf("rows not sorted: %+v", back.Rows)
	}
	again, err := cache.JSON()
	if err != nil || string(again) != string(raw) {
		t.Fatal("rendering is not stable")
	}
}

func TestLoadCacheTreatsAMissingFileAsUnprimed(t *testing.T) {
	cache, err := LoadCache(filepath.Join(t.TempDir(), "zork1.llm.json"), "zork1")
	if err != nil {
		t.Fatalf("missing cache is an error: %v", err)
	}
	if cache.Primed || len(cache.Rows) != 0 {
		t.Fatalf("cache = %+v", cache)
	}
}

func TestLoadCacheRejectsSkew(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, body string }{
		{"schema", `{"schemaVersion": 2, "game": "zork1", "rows": []}`},
		{"game", `{"schemaVersion": 1, "game": "zork2", "rows": []}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadCache(path, "zork1"); err == nil {
				t.Fatal("skew accepted")
			}
		})
	}
}

func TestDecodeEnforcesTheContract(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		text string
		want string // "" means the answer is valid
	}{
		{"valid edge", KindEdge, `{"class":"movement","minutes":4,"rationale":"A long dark crawl."}`, ""},
		{"valid handler", KindHandler, `{"class":"mechanism","minutes":8,"rationale":"Pumping takes a while."}`, ""},
		{"class off a static edge", KindEdge, `{"class":"mechanism","minutes":8,"rationale":"x"}`, "static edges are movement"},
		{"minutes out of band", KindHandler, `{"class":"manipulation","minutes":9,"rationale":"x"}`, "outside the manipulation band"},
		{"unknown class", KindHandler, `{"class":"epic","minutes":9,"rationale":"x"}`, "unknown class"},
		{"malformed", KindHandler, "sure! {\"class\":\"manipulation\"}", "malformed response"},
		{"extra keys", KindHandler, `{"class":"manipulation","minutes":1,"rationale":"x","confidence":0.9}`, "malformed response"},
		{"trailing content", KindHandler, `{"class":"manipulation","minutes":1,"rationale":"x"} and there you go`, "trailing content"},
		{"empty rationale", KindHandler, `{"class":"manipulation","minutes":1,"rationale":"  "}`, "empty rationale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := NewRequest("k", tc.kind, "prompt")
			row, err := Decode(req, tc.text)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("valid answer rejected: %v", err)
				}
				if row.InputHash != req.Hash || row.Kind != tc.kind {
					t.Fatalf("row not pinned to its request: %+v", row)
				}
				return
			}
			if err == nil {
				t.Fatalf("bad answer accepted as %+v", row)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestDecodeKeepsADramaticNominationRaw(t *testing.T) {
	row, err := Decode(NewRequest("action:exorcise/BELL", KindHandler, "p"),
		`{"class":"dramatic","minutes":120,"rationale":"The exorcism is the game's centerpiece."}`)
	if err != nil {
		t.Fatalf("nomination rejected: %v", err)
	}
	if row.Class != durations.ClassDramatic || row.Minutes != 120 {
		t.Fatalf("nomination was not kept raw: %+v", row)
	}
}

// fake replays scripted responses, one per call.
type fake struct {
	replies []string
	err     error
	prompts []string
}

func (f *fake) Infer(_ context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return "", f.err
	}
	if len(f.replies) == 0 {
		return "", nil
	}
	reply := f.replies[0]
	f.replies = f.replies[1:]
	return reply, nil
}

func TestRunRetriesOnceAndThenSucceeds(t *testing.T) {
	inf := &fake{replies: []string{
		`{"class":"movement","minutes":40,"rationale":"way out of band"}`,
		`{"class":"movement","minutes":6,"rationale":"A long crawl."}`,
	}}
	cache := NewCache("zork1")
	req := edgeReq("prompt")
	if err := Run(context.Background(), inf, []Request{req}, cache, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	row, ok := cache.Fresh(req)
	if !ok || row.Minutes != 6 {
		t.Fatalf("row = %+v %v", row, ok)
	}
	if len(inf.prompts) != 2 {
		t.Fatalf("want exactly one retry, got %d calls", len(inf.prompts))
	}
	if !strings.Contains(inf.prompts[1], "outside the movement band 1-10") {
		t.Fatalf("the retry did not restate the band:\n%s", inf.prompts[1])
	}
}

func TestRunHardFailsAfterTheRetry(t *testing.T) {
	inf := &fake{replies: []string{
		`{"class":"movement","minutes":40,"rationale":"x"}`,
		`{"class":"movement","minutes":41,"rationale":"x"}`,
	}}
	cache := NewCache("zork1")
	err := Run(context.Background(), inf, []Request{edgeReq("prompt")}, cache, nil)
	if err == nil {
		t.Fatal("an out-of-band answer was accepted — the run must hard-fail, never clamp")
	}
	if !strings.Contains(err.Error(), "edge:A>B") || !strings.Contains(err.Error(), "after one retry") {
		t.Fatalf("error = %v", err)
	}
	if len(cache.Rows) != 0 {
		t.Fatalf("a rejected answer was cached: %+v", cache.Rows)
	}
	if len(inf.prompts) != 2 {
		t.Fatalf("want two calls, got %d", len(inf.prompts))
	}
}

func TestRunDoesNotRetryATransportFailure(t *testing.T) {
	inf := &fake{err: context.DeadlineExceeded}
	err := Run(context.Background(), inf, []Request{edgeReq("prompt")}, NewCache("zork1"), nil)
	if err == nil {
		t.Fatal("transport failure ignored")
	}
	if len(inf.prompts) != 1 {
		t.Fatalf("want one call, got %d", len(inf.prompts))
	}
}
