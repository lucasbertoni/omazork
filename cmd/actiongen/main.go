// actiongen is the offline duration generator (docs/action-waits.md §5.2,
// §5.3): it rule-prices data/extract/<game>.json into the committed duration
// table data/actions/<game>.json, refines it with the committed LLM cache
// data/actions/<game>.llm.json, then validates the table against the game's
// overlay and the shared verb defaults.
//
//	actiongen           regenerate from the extract and the warm cache; no API calls, ever
//	actiongen -llm      infer the rows the cache is missing or stale on, then regenerate
//	actiongen -check    regenerate in memory and fail on any drift; what CI runs
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lucasbertoni/omazork/internal/durations"
	"github.com/lucasbertoni/omazork/internal/durations/gen"
	"github.com/lucasbertoni/omazork/internal/durations/llm"
	"github.com/lucasbertoni/omazork/internal/extract"
)

var games = []string{"zork1", "zork2", "zork3"}

func main() {
	game := flag.String("game", "", "game to generate: zork1, zork2, or zork3 (default all three)")
	root := flag.String("root", ".", "repository root holding data/")
	check := flag.Bool("check", false, "regenerate in memory and fail on any difference from the committed files")
	infer := flag.Bool("llm", false, "run the LLM pass for rows the cache is missing or stale on (needs ANTHROPIC_API_KEY)")
	flag.Parse()

	if *check && *infer {
		fmt.Fprintln(os.Stderr, "actiongen: -check never calls the API; drop -llm")
		os.Exit(1)
	}
	targets := games
	if *game != "" {
		targets = []string{*game}
	}
	failed := false
	for _, g := range targets {
		if err := run(*root, g, *check, *infer); err != nil {
			fmt.Fprintln(os.Stderr, "actiongen:", err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

// artifact is one committed file the generator owns: where it lives and what
// this run says it should contain.
type artifact struct {
	path string
	data []byte
}

func run(root, game string, check, infer bool) error {
	fsys := os.DirFS(root)
	verbs, err := durations.LoadVerbsFS(fsys)
	if err != nil {
		return err
	}
	// Read the shared thresholds only to assert their schemaVersion here: a
	// stale calibration file should stop a regeneration, not surface later.
	if _, err := durations.LoadCalibrationFS(fsys); err != nil {
		return err
	}
	x, err := readExtract(filepath.Join(root, "data", "extract", game+".json"))
	if err != nil {
		return err
	}
	tablePath := filepath.Join(root, "data", "actions", game+".json")
	cachePath := filepath.Join(root, "data", "actions", game+".llm.json")
	cache, err := llm.LoadCache(cachePath, game)
	if err != nil {
		return err
	}
	result, err := gen.Generate(x, verbs, cache)
	if err != nil {
		return err
	}
	if infer && len(result.Missing) > 0 {
		if err := inferMissing(game, result, cache); err != nil {
			return err
		}
		// Regenerate against the now-warm cache: the table is always a pure
		// function of the extract plus the cache, never of the run's order.
		if result, err = gen.Generate(x, verbs, cache); err != nil {
			return err
		}
		if len(result.Missing) > 0 {
			return fmt.Errorf("%s: %d rows still uncached after the LLM pass", game, len(result.Missing))
		}
	}
	dropped := cache.Prune(liveKeys(result))

	overlay, err := durations.LoadOverlayFS(fsys, game)
	if err != nil {
		return err
	}
	if errs := durations.Validate(result.Table, overlay, verbs); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "actiongen:", game+":", e)
		}
		return fmt.Errorf("%s: %d validation errors", game, len(errs))
	}
	data, err := result.Table.JSON()
	if err != nil {
		return err
	}
	cacheData, err := cache.JSON()
	if err != nil {
		return err
	}
	table := artifact{tablePath, data}
	llmCache := artifact{cachePath, cacheData}
	if check {
		return verify(game, table, llmCache, cache.Primed, result)
	}
	if err := os.WriteFile(table.path, table.data, 0o644); err != nil {
		return err
	}
	// The cache file lands only once something has been inferred: an unprimed
	// pass should not commit an empty file that later reads as complete.
	if cache.Primed {
		if err := os.WriteFile(llmCache.path, llmCache.data, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("%s: %d rooms, %d edges, %d actions, %d verb words (%d routine exits unpriced)\n",
		game, len(result.Table.Rooms), len(result.Table.Edges), len(result.Table.Actions),
		len(result.Table.Vocab.Verbs), routineExits(x))
	fmt.Printf("%s: llm cache %d/%d rows warm%s\n", game,
		len(result.Requests)-len(result.Missing), len(result.Requests), droppedNote(dropped))
	return nil
}

// verify is the CI check (§7): the committed table must match, and — once a
// cache exists — it must cover every row, so regeneration never needs an API
// call. Until the first inference pass is committed there is no cache to be
// complete, so the API-call check reports its coverage instead of failing on
// every row at once.
func verify(game string, table, llmCache artifact, primed bool, result *gen.Result) error {
	if err := match(table); err != nil {
		return err
	}
	if !primed {
		fmt.Printf("%s: table matches the committed file (%d edges, %d rows await a first LLM pass)\n",
			game, len(result.Table.Edges), len(result.Requests))
		return nil
	}
	if len(result.Missing) > 0 {
		return fmt.Errorf("%s: %d rows would need an API call (first: %s) — rerun actiongen -llm and commit the cache",
			game, len(result.Missing), result.Missing[0].Key)
	}
	if err := match(llmCache); err != nil {
		return err
	}
	fmt.Printf("%s: table and llm cache match the committed files (%d edges, %d cached rows)\n",
		game, len(result.Table.Edges), len(result.Requests))
	return nil
}

// match fails when a committed file differs from what this run would write.
func match(a artifact) error {
	committed, err := os.ReadFile(a.path)
	if err != nil {
		return err
	}
	if !bytes.Equal(committed, a.data) {
		return fmt.Errorf("%s is stale — regenerate with actiongen", a.path)
	}
	return nil
}

// inferMissing runs the LLM pass over the rows the cache cannot answer. It is
// the only path in this tool that touches the network (§5.3).
func inferMissing(game string, result *gen.Result, cache *llm.Cache) error {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return fmt.Errorf("%s: -llm needs ANTHROPIC_API_KEY", game)
	}
	fmt.Printf("%s: inferring %d rows with %s\n", game, len(result.Missing), llm.ModelID)
	progress := func(done, total int, rowKey string) {
		fmt.Printf("  %s %d/%d %s\n", game, done, total, rowKey)
	}
	return llm.Run(context.Background(), llm.NewAPI(key), result.Missing, cache, progress)
}

// liveKeys is the set of rows this extract still asks about — what a cache is
// pruned against.
func liveKeys(result *gen.Result) map[string]bool {
	live := make(map[string]bool, len(result.Requests))
	for _, r := range result.Requests {
		live[r.Key] = true
	}
	return live
}

// droppedNote reports pruned cache rows in the run summary, so an answer
// leaving the repo is never silent.
func droppedNote(dropped []string) string {
	if len(dropped) == 0 {
		return ""
	}
	return fmt.Sprintf(", %d stale cache rows dropped (first: %s)", len(dropped), dropped[0])
}

// routineExits counts the PER-routine exits no rule can price: they have no
// static destination, so they never become rows — the LLM pass has no key to
// hang an answer on either (§5.3).
func routineExits(x *extract.Extract) int {
	n := 0
	for _, e := range x.Edges {
		if e.Kind == extract.KindRoutine && e.To == "" {
			n++
		}
	}
	return n
}

func readExtract(path string) (*extract.Extract, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var x extract.Extract
	if err := json.Unmarshal(raw, &x); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &x, nil
}
