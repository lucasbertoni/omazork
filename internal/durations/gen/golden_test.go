package gen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/durations/llm"
	"github.com/lucasbertoni/omazork/internal/extract"
)

var repoGames = []string{"zork1", "zork2", "zork3"}

const repoRoot = "../../.."

func readExtract(t *testing.T, game string) *extract.Extract {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "data", "extract", game+".json"))
	if err != nil {
		t.Fatalf("read extract: %v", err)
	}
	var x extract.Extract
	if err := json.Unmarshal(raw, &x); err != nil {
		t.Fatalf("parse extract: %v", err)
	}
	return &x
}

func repoVerbs(t *testing.T) *durations.Verbs {
	t.Helper()
	verbs, err := durations.LoadVerbsFS(os.DirFS(repoRoot))
	if err != nil {
		t.Fatalf("load verbs: %v", err)
	}
	return verbs
}

// TestCommittedTablesAreReproducible is the drift check: regenerating from the
// committed extract must reproduce the committed table byte for byte.
func TestCommittedTablesAreReproducible(t *testing.T) {
	verbs := repoVerbs(t)
	for _, game := range repoGames {
		t.Run(game, func(t *testing.T) {
			result, err := Generate(readExtract(t, game), verbs, repoCache(t, game))
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			got, err := result.Table.JSON()
			if err != nil {
				t.Fatalf("JSON: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(repoRoot, "data", "actions", game+".json"))
			if err != nil {
				t.Fatalf("read committed table: %v", err)
			}
			if string(got) != string(want) {
				t.Fatalf("data/actions/%s.json is stale — run `go run ./cmd/actiongen`", game)
			}
		})
	}
}

// TestGenerationIsDeterministic guards the map iteration the vocab is built
// from: two runs of the same extract must render identically.
func TestGenerationIsDeterministic(t *testing.T) {
	verbs := repoVerbs(t)
	x := readExtract(t, "zork1")
	first, err := mustJSON(Generate(x, verbs, repoCache(t, "zork1")))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		again, err := mustJSON(Generate(x, verbs, repoCache(t, "zork1")))
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatal("regeneration differs between runs")
		}
	}
}

func mustJSON(r *Result, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	return r.Table.JSON()
}

// repoCache reads a game's committed LLM cache, or an unprimed one when the
// game has not been through an inference pass yet.
func repoCache(t *testing.T, game string) *llm.Cache {
	t.Helper()
	cache, err := llm.LoadCache(filepath.Join(repoRoot, "data", "actions", game+".llm.json"), game)
	if err != nil {
		t.Fatalf("load llm cache: %v", err)
	}
	return cache
}
